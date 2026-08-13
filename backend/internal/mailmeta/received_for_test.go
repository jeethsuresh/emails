package mailmeta

import "testing"

func TestReceivedForPrefersDeliveredTo(t *testing.T) {
	h := map[string]string{
		"Delivered-To": "uber@jeeth.dev",
		"To":           "me@jeeth.dev",
	}
	got := ReceivedFor(h, "me@jeeth.dev", "", "me@jeeth.dev")
	if got != "uber@jeeth.dev" {
		t.Fatalf("got %q", got)
	}
}

func TestReceivedForFallsBackToDomainMatch(t *testing.T) {
	h := map[string]string{}
	got := ReceivedFor(h, "friend@gmail.com, uber@jeeth.dev", "", "me@jeeth.dev")
	if got != "uber@jeeth.dev" {
		t.Fatalf("got %q", got)
	}
}
