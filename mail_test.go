package main

import "testing"

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
