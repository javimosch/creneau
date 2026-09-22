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

import "strings"

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
