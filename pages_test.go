package main

import (
	"regexp"
	"strings"
	"testing"
)

// The served pages are string templates with two nearly identical placeholders:
// __LAB__ is the human-readable NAME and __LABID__ is the url slug. A name in a
// heading is correct and a name in an href is a broken link — and it only breaks
// for labs whose name differs from their id, which is every real one. The
// organizer link on the board shipped with "/__LAB__/admin" and produced
// /Fablab%20JLA/admin.
//
// This walks every href and src in the templates and fails if any of them
// carries the name placeholder.
func TestTemplateURLsUseTheLabID(t *testing.T) {
	attr := regexp.MustCompile(`(?:href|src|action)\s*=\s*"([^"]*)"`)
	for name, tpl := range map[string]string{
		"boardHTML":   boardHTML,
		"adminHTML":   adminHTML,
		"recoverHTML": recoverHTML,
	} {
		for _, m := range attr.FindAllStringSubmatch(tpl, -1) {
			if strings.Contains(m[1], "__LAB__") {
				t.Errorf("%s: URL %q uses __LAB__ (the display name); URLs must use __LABID__", name, m[1])
			}
		}
	}
}

// The same trap in the other direction: a heading that renders the slug instead
// of the name is not a broken link, just ugly, so this only checks the titles we
// deliberately render as names.
func TestTemplatesStillCarryBothPlaceholders(t *testing.T) {
	if !strings.Contains(boardHTML, "__LABID__") {
		t.Error("boardHTML no longer substitutes __LABID__ — the board JS needs it")
	}
	if !strings.Contains(adminHTML, "__LABID__") {
		t.Error("adminHTML no longer substitutes __LABID__")
	}
}

// Times are stored in UTC and must be RENDERED in the lab's zone. The board
// once told a Paris workshop its 09:00-18:00 day ran 07:00-16:00, because the
// page sliced the UTC string directly. Guard the placeholder and the slicing.
func TestPagesRenderInTheLabTimezone(t *testing.T) {
	for name, tpl := range map[string]string{"boardHTML": boardHTML, "adminHTML": adminHTML} {
		if !strings.Contains(tpl, "__TZ__") {
			t.Errorf("%s: no __TZ__ placeholder — times would render in UTC", name)
		}
	}
	// The bug was reading the UTC hour straight off a SLOT string. The one
	// remaining slice(11,16) is the deliberate fallback inside tzParts for an
	// unknown zone, so match the slot expressions specifically.
	for _, bad := range []string{"s.slice(11,16)", "set[s.slice(", "return s.slice(11,16)==="} {
		if strings.Contains(boardHTML, bad) {
			t.Errorf("boardHTML still reads the UTC hour off a slot (%q); use hmOf()", bad)
		}
	}
	if !strings.Contains(boardHTML, "hmOf(") {
		t.Error("boardHTML does not convert slot times with hmOf()")
	}
	if strings.Contains(boardHTML, "as returned by the API") {
		t.Error("boardHTML still claims times are shown in UTC")
	}
}
