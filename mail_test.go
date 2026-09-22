package main

import (
	"strings"
	"testing"
)

func TestDeliverable(t *testing.T) {
	for _, a := range []string{
		"javi@intrane.fr", "someone@gmail.com", "a.b+tag@sub.domain.co.uk",
	} {
		if !deliverable(a) {
			t.Errorf("%q should be deliverable", a)
		}
	}
	for _, a := range []string{
		"me@fake.com", "me@example.com", "me@anything.test", "me@foo.invalid",
		"me@localhost", "me@nodots", "nobody", "", "two words@x.com",
	} {
		if deliverable(a) {
			t.Errorf("%q should NOT be deliverable", a)
		}
	}
}

func TestWhenLabelUsesTheLabZone(t *testing.T) {
	rec := doc{"start": "2026-09-22T12:00:00Z"}
	paris := whenLabel(lab{TZ: "Europe/Paris"}, rec)
	if !strings.Contains(paris, "14:00") {
		t.Errorf("Europe/Paris: 12:00Z should read 14:00 in summer, got %q", paris)
	}
	utc := whenLabel(lab{TZ: "UTC"}, rec)
	if !strings.Contains(utc, "12:00") {
		t.Errorf("UTC: got %q", utc)
	}
	// An unset or bogus zone must still render something, not blow up.
	if whenLabel(lab{}, rec) == "" || whenLabel(lab{TZ: "Not/AZone"}, rec) == "" {
		t.Error("a missing or unknown zone should fall back, not render empty")
	}
}
