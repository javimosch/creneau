package main

// The booking model, faithful to Cal.com's shape: an organizer publishes
// weekly availability in a timezone, an event type fixes a duration and its
// guard rails, and a slot is what survives availability minus bookings minus
// buffers minus notice.
//
// Everything here is pure computation over documents fetched from bkn. The one
// thing it cannot do in this process is the atomic write — see booking.go.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const ns = "creneau"

// RFC3339 in UTC is the only time format stored, because bkn compares filter
// values as text: a range query is only correct if every stored timestamp
// sorts lexicographically, which fixed-width UTC does and a local offset
// does not.
const stamp = "2006-01-02T15:04:05Z"

var weekdays = [...]string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

type availability struct {
	Calendar  string              `json:"calendar"`
	TZ        string              `json:"tz"`
	Rules     map[string][]string `json:"rules"`     // "mon": ["09:00-12:00","13:00-17:00"]
	Overrides map[string][]string `json:"overrides"` // "2026-12-25": [] closes the day
}

type eventType struct {
	Slug         string `json:"slug"`
	Calendar     string `json:"calendar"`
	Minutes      int    `json:"minutes"`
	BufferBefore int    `json:"buffer_before"`
	BufferAfter  int    `json:"buffer_after"`
	MinNotice    int    `json:"min_notice_minutes"`
	DailyCap     int    `json:"daily_cap"`
}

type slot struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// --- parsing --------------------------------------------------------------

func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		i, _ := strconv.Atoi(n)
		return i
	}
	return 0
}

func asStr(v any) string {
	s, _ := v.(string)
	return s
}

func asStrList(v any) []string {
	switch l := v.(type) {
	case []string:
		return l
	case []any:
		out := make([]string, 0, len(l))
		for _, e := range l {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func availabilityFrom(d doc) availability {
	a := availability{
		Calendar:  asStr(d["calendar"]),
		TZ:        asStr(d["tz"]),
		Rules:     map[string][]string{},
		Overrides: map[string][]string{},
	}
	if m, ok := d["rules"].(map[string]any); ok {
		for k, v := range m {
			a.Rules[strings.ToLower(k)] = asStrList(v)
		}
	}
	if m, ok := d["overrides"].(map[string]any); ok {
		for k, v := range m {
			a.Overrides[k] = asStrList(v)
		}
	}
	if a.TZ == "" {
		a.TZ = "UTC"
	}
	return a
}

func eventFrom(d doc) eventType {
	return eventType{
		Slug:         asStr(d["slug"]),
		Calendar:     asStr(d["calendar"]),
		Minutes:      asInt(d["minutes"]),
		BufferBefore: asInt(d["buffer_before"]),
		BufferAfter:  asInt(d["buffer_after"]),
		MinNotice:    asInt(d["min_notice_minutes"]),
		DailyCap:     asInt(d["daily_cap"]),
	}
}

// parseWindow reads "09:00-17:00" into minutes-since-midnight.
func parseWindow(w string) (int, int, error) {
	a, b, ok := strings.Cut(strings.TrimSpace(w), "-")
	if !ok {
		return 0, 0, fmt.Errorf("window %q must be HH:MM-HH:MM", w)
	}
	hm := func(s string) (int, error) {
		h, m, ok := strings.Cut(strings.TrimSpace(s), ":")
		if !ok {
			return 0, fmt.Errorf("time %q must be HH:MM", s)
		}
		hh, err1 := strconv.Atoi(h)
		mm, err2 := strconv.Atoi(m)
		if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
			return 0, fmt.Errorf("time %q must be HH:MM", s)
		}
		return hh*60 + mm, nil
	}
	from, err := hm(a)
	if err != nil {
		return 0, 0, err
	}
	to, err := hm(b)
	if err != nil {
		return 0, 0, err
	}
	if to <= from {
		return 0, 0, fmt.Errorf("window %q ends before it starts", w)
	}
	return from, to, nil
}

// ParseDuration accepts the notice shorthand the CLI takes: 90m, 4h, 2d.
func parseNotice(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	unit := s[len(s)-1]
	n, err := strconv.Atoi(s[:len(s)-1])
	if err != nil || n < 0 {
		return 0, fmt.Errorf("notice %q must look like 30m, 4h or 2d", s)
	}
	switch unit {
	case 'm':
		return n, nil
	case 'h':
		return n * 60, nil
	case 'd':
		return n * 1440, nil
	}
	return 0, fmt.Errorf("notice %q must end in m, h or d", s)
}

// --- slot computation -----------------------------------------------------

type interval struct{ start, end time.Time }

func (i interval) overlaps(j interval) bool {
	// Touching ends do not overlap; that is what makes back-to-back bookable.
	return i.start.Before(j.end) && j.start.Before(i.end)
}

// computeSlots is availability minus bookings minus buffers minus notice, in
// the calendar's own timezone so a DST change moves the working day with it
// rather than sliding every slot by an hour.
func computeSlots(ev eventType, av availability, booked []interval, from, to, now time.Time) ([]slot, error) {
	loc, err := time.LoadLocation(av.TZ)
	if err != nil {
		return nil, fmt.Errorf("unknown timezone %q: %w", av.TZ, err)
	}
	if ev.Minutes <= 0 {
		return nil, fmt.Errorf("event %q has no duration", ev.Slug)
	}

	notAfter := now.Add(time.Duration(ev.MinNotice) * time.Minute)
	out := []slot{}

	day := from.In(loc)
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	for !day.After(to.In(loc)) {
		date := day.Format("2006-01-02")
		windows, overridden := av.Overrides[date]
		if !overridden {
			windows = av.Rules[weekdays[int(day.Weekday())]]
		}

		perDay := 0
		if ev.DailyCap > 0 {
			for _, b := range booked {
				if b.start.In(loc).Format("2006-01-02") == date {
					perDay++
				}
			}
		}

		for _, w := range windows {
			openMin, closeMin, err := parseWindow(w)
			if err != nil {
				return nil, err
			}
			for m := openMin; m+ev.Minutes <= closeMin; m += ev.Minutes {
				if ev.DailyCap > 0 && perDay >= ev.DailyCap {
					break
				}
				start := day.Add(time.Duration(m) * time.Minute)
				cand := interval{start, start.Add(time.Duration(ev.Minutes) * time.Minute)}

				if cand.start.Before(notAfter) || cand.start.Before(from) || !cand.end.After(from) {
					continue
				}
				if !cand.start.Before(to) {
					continue
				}
				// A booking blocks a candidate if either one's buffer reaches
				// the other: the gap has to satisfy both sides.
				guarded := interval{
					cand.start.Add(-time.Duration(ev.BufferBefore) * time.Minute),
					cand.end.Add(time.Duration(ev.BufferAfter) * time.Minute),
				}
				clash := false
				for _, b := range booked {
					blocked := interval{
						b.start.Add(-time.Duration(ev.BufferBefore) * time.Minute),
						b.end.Add(time.Duration(ev.BufferAfter) * time.Minute),
					}
					if cand.overlaps(blocked) || guarded.overlaps(b) {
						clash = true
						break
					}
				}
				if clash {
					continue
				}
				out = append(out, slot{
					Start: cand.start.UTC().Format(stamp),
					End:   cand.end.UTC().Format(stamp),
				})
			}
		}
		day = day.AddDate(0, 0, 1)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out, nil
}
