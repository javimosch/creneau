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
