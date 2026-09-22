package main

// One gate in front of every outbound mail.
//
// Test signups use addresses that cannot receive anything (@fake.com and
// friends). Handing those to Resend earns hard bounces, and a sending domain's
// bounce rate is reputation that is slow to rebuild — so a throwaway signup
// while testing quietly damages delivery for real labs.
//
// This is deliberately a small, explicit list of addresses that are reserved or
// conventionally fake, not a cleverness contest. Guessing which real domains
// "look fake" would refuse mail to actual customers, which is far worse than
// sending one bounce.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// Reserved by RFC 2606 / RFC 6761 for documentation and testing — these can
// never receive mail — plus the conventions used when testing this product.
var undeliverableSuffixes = []string{
	".test", ".example", ".invalid", ".localhost", ".local", ".fake",
	"@example.com", "@example.org", "@example.net",
	"@fake.com", "@test.com", "@invalid.com", "@localhost",
}

// deliverable reports whether it is worth handing this address to the mailer.
// A false answer is not an error: the caller carries on and says it skipped.
func deliverable(addr string) bool {
	a := strings.ToLower(strings.TrimSpace(addr))
	at := strings.LastIndex(a, "@")
	if at <= 0 || at == len(a)-1 {
		return false // no local part, or no domain
	}
	if strings.Contains(a, " ") {
		return false
	}
	domain := a[at+1:]
	if !strings.Contains(domain, ".") && domain != "localhost" {
		return false // a bare hostname cannot receive internet mail
	}
	for _, s := range undeliverableSuffixes {
		if strings.HasSuffix(a, s) {
			return false
		}
	}
	return true
}

// mailFrom is the sender for everything creneau sends.
func mailFrom() string {
	if f := os.Getenv("CRENEAU_MAIL_FROM"); f != "" {
		return f
	}
	return "creneau <javi@intrane.fr>"
}

// sendMail posts one message to Resend. Callers treat failure as non-fatal:
// a booking must not fail because a mail server is slow, and a cancellation
// must still happen if the notice bounces.
func sendMail(to, subject, text string) error {
	key := os.Getenv("RESEND_API_KEY")
	if key == "" {
		return fmt.Errorf("RESEND_API_KEY not set")
	}
	if !deliverable(to) {
		return fmt.Errorf("address is not deliverable: %s", to)
	}
	body, _ := json.Marshal(map[string]any{
		"from": mailFrom(), "to": []string{to}, "subject": subject, "text": text,
	})
	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<12))
		return fmt.Errorf("resend %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

// sendBookingMail confirms a booking and hands over the manage link. Without
// this the manage token only ever existed in the browser that booked, so a
// guest on another device could not cancel at all.
func sendBookingMail(to, labID, labName, machine, when, bookingID, token string) error {
	manage := selfURL() + "/" + labID + "/b/" + bookingID + "?t=" + token
	return sendMail(to, labName+" — booked: "+machine+", "+when,
		"Your booking is confirmed.\n\n"+
			"  "+machine+"\n  "+when+"\n  "+labName+"\n\n"+
			"Need to cancel? Open this — it works on any device:\n\n  "+manage+"\n\n"+
			"Anyone with that link can cancel this booking, so keep it to yourself.\n"+
			"The board is at "+selfURL()+"/"+labID+"\n")
}

// sendCancelMail tells a guest the organizer cancelled their slot. The optional
// note is the organizer's own words — a cancellation with no reason reads as a
// mistake, and the guest has no way to ask.
func sendCancelMail(to, labID, labName, machine, when, note string) error {
	body := "Your booking was cancelled by the organizer.\n\n" +
		"  " + machine + "\n  " + when + "\n  " + labName + "\n\n"
	if strings.TrimSpace(note) != "" {
		body += "Message from the organizer:\n\n  " + strings.TrimSpace(note) + "\n\n"
	}
	body += "The slot is free again — you can book another at:\n\n  " +
		selfURL() + "/" + labID + "\n"
	return sendMail(to, labName+" — cancelled: "+machine+", "+when, body)
}

// machineLabel turns what a booking record stores into something a person
// recognises. The record's "event" is the calendar key ("lab:machine"), so a
// bare lookup fails and the guest gets "mail-test-jla:laser" in their email.
func machineLabel(l lab, rec doc) string {
	id := asStr(rec["event"])
	if id == "" {
		id = asStr(rec["calendar"])
	}
	if i := strings.LastIndex(id, ":"); i >= 0 {
		id = id[i+1:]
	}
	if m, ok := l.machineByID(id); ok && m.Name != "" {
		return m.Name
	}
	return id
}

// whenLabel renders a stored UTC timestamp in the lab's own wall clock. A
// fablab in Chambery does not think in UTC, and a confirmation email that is
// two hours off the door sign is worse than no email.
func whenLabel(l lab, rec doc) string {
	w := asStr(rec["start"])
	t, err := time.Parse(stamp, w)
	if err != nil {
		if len(w) >= 16 {
			return strings.Replace(w[:16], "T", " at ", 1) + " UTC"
		}
		return w
	}
	zone := l.TZ
	if zone == "" {
		zone = "UTC"
	}
	loc, lerr := time.LoadLocation(zone)
	if lerr != nil {
		loc = time.UTC
	}
	return t.In(loc).Format("Mon 2 Jan at 15:04 (MST)")
}
