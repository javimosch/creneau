package main

// The commands. Reads and plain writes go straight to bkn's HTTP API; the two
// operations that must not race — book and reschedule — are bkn scripts,
// because `lock` has no HTTP route (docs/ledger.md rows 1-2).

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

//go:embed scripts/*.js
var scripts embed.FS

// install declares the collections and loads the scripts.
//
// Script management is CLI-only in bkn — there is no create route — so this
// shells out to the `bkn` binary rather than pretending it can provision a
// remote instance over HTTP. That is ledger row 4.
func install(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	bin := fs.String("bkn", "bkn", "path to the bkn binary")
	dry := fs.Bool("dry-run", false, "print what would be done")
	_ = fs.Parse(args)

	c := newBkn()
	colls := []string{"labs", "calendars", "availability", "events", "bookings"}
	for _, coll := range colls {
		if *dry {
			fmt.Fprintf(os.Stderr, "  declare %s/%s\n", ns, coll)
			continue
		}
		var norm map[string]string
		if coll == "bookings" {
			norm = map[string]string{"who": "trim_lower"}
		}
		if err := c.declare(ns, coll, norm); err != nil {
			fail(exitUnavailable, "declare_failed", err.Error(), "is bkn running? BKN_URL="+c.base)
		}
	}

	names, _ := scripts.ReadDir("scripts")
	installed := []string{}
	for _, f := range names {
		body, _ := scripts.ReadFile("scripts/" + f.Name())
		name := strings.TrimSuffix(f.Name(), ".js")
		tmp, err := os.CreateTemp("", name+"-*.js")
		if err != nil {
			fail(exitUnavailable, "temp_failed", err.Error())
		}
		_, _ = tmp.Write(body)
		_ = tmp.Close()
		if *dry {
			fmt.Fprintf(os.Stderr, "  %s script create %s --file %s\n", *bin, name, tmp.Name())
			_ = os.Remove(tmp.Name())
			continue
		}
		cmd := exec.Command(*bin, "script", "create", name, "--file", tmp.Name())
		outBytes, err := cmd.CombinedOutput()
		if err != nil && !strings.Contains(string(outBytes), "already exists") {
			// update rather than create, so install is idempotent
			cmd = exec.Command(*bin, "script", "update", name, "--file", tmp.Name())
			if out2, err2 := cmd.CombinedOutput(); err2 != nil {
				_ = os.Remove(tmp.Name())
				fail(exitUnavailable, "script_install_failed",
					strings.TrimSpace(string(outBytes)+" / "+string(out2)),
					"creneau install --bkn /path/to/bkn")
			}
		}
		_ = os.Remove(tmp.Name())
		installed = append(installed, name)
	}
	out(map[string]any{"ok": true, "collections": len(colls), "scripts": installed, "bkn": c.base})
}

// --- availability ---------------------------------------------------------

func availabilitySet(args []string) {
	fs := flag.NewFlagSet("availability set", flag.ExitOnError)
	cal := fs.String("calendar", "default", "calendar id")
	tz := fs.String("tz", "UTC", "IANA timezone, e.g. Europe/Paris")
	day := map[string]*string{}
	for _, d := range weekdays {
		day[d] = fs.String(d, "", "windows for "+d+", e.g. 09:00-12:00,13:00-17:00")
	}
	closed := fs.String("closed", "", "comma-separated dates to close, e.g. 2026-12-25")
	_ = fs.Parse(args)

	if _, err := time.LoadLocation(*tz); err != nil {
		fail(exitUsage, "invalid_timezone", fmt.Sprintf("unknown timezone %q", *tz), "--tz Europe/Paris")
	}
	rules := map[string][]string{}
	for d, v := range day {
		if *v == "" {
			continue
		}
		for _, w := range strings.Split(*v, ",") {
			if _, _, err := parseWindow(w); err != nil {
				fail(exitUsage, "invalid_window", err.Error(), "--mon 09:00-17:00")
			}
			rules[d] = append(rules[d], strings.TrimSpace(w))
		}
	}
	overrides := map[string][]string{}
	for _, d := range strings.Split(*closed, ",") {
		if d = strings.TrimSpace(d); d != "" {
			overrides[d] = []string{}
		}
	}

	c := newBkn()
	rec, err := c.put(ns, "availability", *cal, doc{
		"calendar": *cal, "tz": *tz, "rules": rules, "overrides": overrides,
	})
	if err != nil {
		failBkn(err)
	}
	out(map[string]any{"ok": true, "availability": rec})
}

// --- event types ----------------------------------------------------------

func eventCreate(args []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fail(exitUsage, "missing_argument", "event create needs a slug",
			"creneau event create intro-30 --minutes 30")
	}
	slug := args[0]
	fs := flag.NewFlagSet("event create", flag.ExitOnError)
	cal := fs.String("calendar", "default", "calendar id")
	minutes := fs.Int("minutes", 30, "duration in minutes")
	before := fs.Int("buffer-before", 0, "minutes to keep free before")
	after := fs.Int("buffer-after", 0, "minutes to keep free after")
	notice := fs.String("min-notice", "0m", "minimum notice, e.g. 4h")
	cap_ := fs.Int("daily-cap", 0, "maximum bookings per day, 0 for unlimited")
	_ = fs.Parse(args[1:])

	if *minutes <= 0 {
		fail(exitUsage, "invalid_value", "--minutes must be positive")
	}
	mins, err := parseNotice(*notice)
	if err != nil {
		fail(exitUsage, "invalid_value", err.Error(), "--min-notice 4h")
	}

	c := newBkn()
	rec, err2 := c.put(ns, "events", slug, doc{
		"slug": slug, "calendar": *cal, "minutes": *minutes,
		"buffer_before": *before, "buffer_after": *after,
		"min_notice_minutes": mins, "daily_cap": *cap_,
	})
	if err2 != nil {
		failBkn(err2)
	}
	out(map[string]any{"ok": true, "event": rec})
}

// --- slots ----------------------------------------------------------------

// loadSlots is shared by the CLI and the public HTTP surface.
func loadSlots(c *bkn, eventSlug, fromS, toS string) ([]slot, eventType, error) {
	evDoc, err := c.get(ns, "events", eventSlug)
	if err != nil {
		return nil, eventType{}, err
	}
	ev := eventFrom(evDoc)

	avDoc, err := c.get(ns, "availability", ev.Calendar)
	if err != nil {
		return nil, ev, fmt.Errorf("no availability for calendar %q: %w", ev.Calendar, err)
	}
	av := availabilityFrom(avDoc)

	from, err := time.Parse(time.RFC3339, fromS)
	if err != nil {
		if from, err = time.Parse("2006-01-02", fromS); err != nil {
			return nil, ev, fmt.Errorf("--from must be a date or RFC3339 time")
		}
	}
	to, err := time.Parse(time.RFC3339, toS)
	if err != nil {
		if to, err = time.Parse("2006-01-02", toS); err != nil {
			return nil, ev, fmt.Errorf("--to must be a date or RFC3339 time")
		}
		to = to.Add(24 * time.Hour)
	}

	// The range read is a script because two bounds on one field do not
	// survive the HTTP query string (ledger row 3).
	res, err := scriptResult(c.run("creneau-range", map[string]any{
		"calendar": ev.Calendar,
		"from":     from.UTC().Format(stamp),
		"to":       to.UTC().Format(stamp),
	}))
	if err != nil {
		return nil, ev, err
	}
	booked := []interval{}
	if rows, ok := res["bookings"].([]any); ok {
		for _, r := range rows {
			m, ok := r.(map[string]any)
			if !ok {
				continue
			}
			s, err1 := time.Parse(time.RFC3339, asStr(m["start"]))
			e, err2 := time.Parse(time.RFC3339, asStr(m["end"]))
			if err1 == nil && err2 == nil {
				booked = append(booked, interval{s, e})
			}
		}
	}

	slots, err := computeSlots(ev, av, booked, from, to, time.Now().UTC())
	return slots, ev, err
}

func slotsCmd(args []string) {
	fs := flag.NewFlagSet("slots", flag.ExitOnError)
	event := fs.String("event", "", "event type slug")
	from := fs.String("from", time.Now().UTC().Format("2006-01-02"), "start date")
	to := fs.String("to", "", "end date (inclusive)")
	_ = fs.Parse(args)
	if *event == "" {
		fail(exitUsage, "missing_argument", "--event is required", "creneau slots --event intro-30 --from 2026-09-10 --to 2026-09-12")
	}
	if *to == "" {
		*to = *from
	}
	c := newBkn()
	slots, ev, err := loadSlots(c, *event, *from, *to)
	if err != nil {
		failBkn(err)
	}
	out(map[string]any{"ok": true, "event": ev.Slug, "calendar": ev.Calendar,
		"count": len(slots), "slots": slots})
}

// --- book / cancel / reschedule -------------------------------------------

func bookCmd(args []string) {
	fs := flag.NewFlagSet("book", flag.ExitOnError)
	event := fs.String("event", "", "event type slug")
	at := fs.String("at", "", "slot start, RFC3339")
	who := fs.String("who", "", "who is booking (email)")
	name := fs.String("name", "", "display name")
	_ = fs.Parse(args)
	if *event == "" || *at == "" || *who == "" {
		fail(exitUsage, "missing_argument", "--event, --at and --who are required",
			"creneau book --event intro-30 --at 2026-09-10T14:00:00Z --who ada@example.io")
	}

	c := newBkn()
	rec, err := book(c, *event, *at, *who, *name)
	if err != nil {
		failBkn(err)
	}
	out(map[string]any{"ok": true, "booking": rec})
}

func book(c *bkn, eventSlug, at, who, name string) (doc, error) {
	evDoc, err := c.get(ns, "events", eventSlug)
	if err != nil {
		return nil, err
	}
	ev := eventFrom(evDoc)
	start, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return nil, fmt.Errorf("--at must be RFC3339, e.g. 2026-09-10T14:00:00Z")
	}
	end := start.Add(time.Duration(ev.Minutes) * time.Minute)

	// The member has no account, so the booking carries its own capability: a
	// 128-bit token minted here. It is what lets whoever booked cancel it later
	// without logging in, and it is why the id alone is not enough — ids are
	// ULIDs and therefore time-ordered and partly guessable.
	tok, terr := manageToken()
	if terr != nil {
		return nil, terr
	}
	res, err := scriptResult(c.run("creneau-book", map[string]any{
		"calendar": ev.Calendar, "event": ev.Slug,
		"start": start.UTC().Format(stamp), "end": end.UTC().Format(stamp),
		"who": who, "name": name, "manage_token": tok,
	}))
	if err != nil {
		return nil, err
	}
	b, _ := res["booking"].(map[string]any)
	return b, nil
}

// manageToken mints the per-booking capability handed back to whoever booked.
func manageToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// cancelBooking is the compare-and-set behind both `creneau cancel` and the
// public POST /v1/cancel. Cancelling twice is a conflict, not a silent success.
func cancelBooking(c *bkn, id string) (doc, error) {
	return c.patchIf(ns, "bookings", id,
		doc{"status": "cancelled", "cancelled_at": time.Now().UTC().Format(stamp)},
		map[string]string{"status": "confirmed"})
}

func cancelCmd(args []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fail(exitUsage, "missing_argument", "cancel needs a booking id", "creneau cancel <id>")
	}
	c := newBkn()
	// A plain compare-and-set: cancelling twice is a conflict, not a silent
	// success, and this one needs no script.
	rec, err := cancelBooking(c, args[0])
	if err != nil {
		failBkn(err)
	}
	out(map[string]any{"ok": true, "booking": rec})
}

func rescheduleCmd(args []string) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		fail(exitUsage, "missing_argument", "reschedule needs a booking id",
			"creneau reschedule <id> --at 2026-09-11T15:00:00Z")
	}
	id := args[0]
	fs := flag.NewFlagSet("reschedule", flag.ExitOnError)
	at := fs.String("at", "", "new start, RFC3339")
	_ = fs.Parse(args[1:])
	if *at == "" {
		fail(exitUsage, "missing_argument", "--at is required")
	}

	c := newBkn()
	oldDoc, err := c.get(ns, "bookings", id)
	if err != nil {
		failBkn(err)
	}
	evDoc, err := c.get(ns, "events", asStr(oldDoc["event"]))
	if err != nil {
		failBkn(err)
	}
	ev := eventFrom(evDoc)
	start, err2 := time.Parse(time.RFC3339, *at)
	if err2 != nil {
		fail(exitUsage, "invalid_value", "--at must be RFC3339")
	}
	end := start.Add(time.Duration(ev.Minutes) * time.Minute)

	res, err3 := scriptResult(c.run("creneau-reschedule", map[string]any{
		"id": id, "start": start.UTC().Format(stamp), "end": end.UTC().Format(stamp),
	}))
	if err3 != nil {
		failBkn(err3)
	}
	out(map[string]any{"ok": true, "booking": res["booking"], "cancelled": res["cancelled"]})
}

func bookingsList(args []string) {
	fs := flag.NewFlagSet("bookings list", flag.ExitOnError)
	cal := fs.String("calendar", "", "filter by calendar")
	who := fs.String("who", "", "filter by booker")
	upcoming := fs.Bool("upcoming", false, "only bookings that have not started")
	limit := fs.Int("limit", 50, "maximum rows")
	_ = fs.Parse(args)

	q := url.Values{}
	q.Set("status", "confirmed")
	q.Set("order_by", "start")
	q.Set("order", "asc")
	q.Set("limit", fmt.Sprint(*limit))
	if *cal != "" {
		q.Set("calendar", *cal)
	}
	if *who != "" {
		q.Set("who", strings.ToLower(strings.TrimSpace(*who)))
	}
	if *upcoming {
		q.Set("start", "gte:"+time.Now().UTC().Format(stamp))
	}

	c := newBkn()
	rows, err := c.list(ns, "bookings", q)
	if err != nil {
		failBkn(err)
	}
	out(map[string]any{"ok": true, "count": len(rows), "bookings": rows})
}

// reconcile closes the window reschedule cannot: a crash between writing the
// new booking and retiring the old one leaves both confirmed. Anything whose
// rescheduled_from still points at a confirmed booking is that case.
func reconcile(args []string) {
	fs := flag.NewFlagSet("reconcile", flag.ExitOnError)
	dry := fs.Bool("dry-run", false, "report without writing")
	_ = fs.Parse(args)

	c := newBkn()
	q := url.Values{}
	q.Set("status", "confirmed")
	q.Set("limit", "500")
	rows, err := c.list(ns, "bookings", q)
	if err != nil {
		failBkn(err)
	}
	fixed := []string{}
	for _, r := range rows {
		from := asStr(r["rescheduled_from"])
		if from == "" {
			continue
		}
		old, err := c.get(ns, "bookings", from)
		if err != nil || asStr(old["status"]) != "confirmed" {
			continue
		}
		if *dry {
			fixed = append(fixed, from)
			continue
		}
		if _, err := c.patchIf(ns, "bookings", from,
			doc{"status": "cancelled", "cancelled_at": time.Now().UTC().Format(stamp),
				"cancelled_by": "reconcile"},
			map[string]string{"status": "confirmed"}); err == nil {
			fixed = append(fixed, from)
		}
	}
	out(map[string]any{"ok": true, "orphans": len(fixed), "cancelled": fixed, "dry_run": *dry})
}

// failBkn maps a bkn or script refusal onto creneau's exit codes.
func failBkn(err error) {
	be, ok := err.(*bknError)
	if !ok {
		fail(exitUnavailable, "bkn_error", err.Error())
	}
	switch {
	case be.Status == 409:
		fail(exitConflict, "conflict", be.Msg, "creneau slots --event <slug> --from <date>")
	case be.Status == 404:
		fail(exitNotFound, "not_found", be.Msg)
	case be.Status == 0:
		fail(exitUnavailable, "bkn_unreachable", be.Msg, "is bkn running? set BKN_URL")
	default:
		fail(exitUnavailable, "bkn_error", be.Error())
	}
}
