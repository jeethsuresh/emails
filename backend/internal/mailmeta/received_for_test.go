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

func TestReceivedForPrefersXOriginalToWhenDeliveredToAbsent(t *testing.T) {
	h := map[string]string{
		"X-Original-To": "uber@jeeth.dev",
		"To":            "me@jeeth.dev",
	}
	got := ReceivedFor(h, "me@jeeth.dev", "", "me@jeeth.dev")
	if got != "uber@jeeth.dev" {
		t.Fatalf("got %q", got)
	}
}

func TestReceivedForPrefersEnvelopeToWhenEarlierAliasHeadersAbsent(t *testing.T) {
	h := map[string]string{
		"Envelope-To": "uber@jeeth.dev",
		"To":          "me@jeeth.dev",
	}
	got := ReceivedFor(h, "me@jeeth.dev", "", "me@jeeth.dev")
	if got != "uber@jeeth.dev" {
		t.Fatalf("got %q", got)
	}
}

func TestReceivedForFallsBackToCcDomainMatch(t *testing.T) {
	h := map[string]string{}
	got := ReceivedFor(h, "friend@gmail.com", "uber@jeeth.dev", "me@jeeth.dev")
	if got != "uber@jeeth.dev" {
		t.Fatalf("got %q", got)
	}
}

func TestReceivedForExactAccountEmailWhenDomainDiffers(t *testing.T) {
	h := map[string]string{}
	got := ReceivedFor(h, "friend@gmail.com, me@other.com", "", "me@other.com")
	if got != "me@other.com" {
		t.Fatalf("got %q", got)
	}
}

func TestReceivedForAliasHeaderUsesFirstAddress(t *testing.T) {
	h := map[string]string{
		"Delivered-To": "uber@jeeth.dev, other@jeeth.dev",
	}
	got := ReceivedFor(h, "me@jeeth.dev", "", "me@jeeth.dev")
	if got != "uber@jeeth.dev" {
		t.Fatalf("got %q", got)
	}
}
