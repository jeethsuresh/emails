package calendar

import "testing"

func TestNormalizeAttendeesEmails(t *testing.T) {
	got := NormalizeAttendeesEmails([]string{"  A@B.com ", "a@b.com", "nope", "c@d.org"})
	if len(got) != 2 || got[0] != "a@b.com" || got[1] != "c@d.org" {
		t.Fatalf("got %#v", got)
	}
}

func TestEncodeDecodeAttendeesJSON(t *testing.T) {
	enc := EncodeAttendeesJSON([]string{"a@b.com", "c@d.org"})
	got := DecodeAttendeesJSON(enc)
	if len(got) != 2 || got[0] != "a@b.com" {
		t.Fatalf("roundtrip %#v from %q", got, enc)
	}
	if DecodeAttendeesJSON("") != nil && len(DecodeAttendeesJSON("")) != 0 {
		t.Fatal("empty should decode empty")
	}
}
