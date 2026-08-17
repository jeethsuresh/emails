package mailer

import (
	"strings"
	"testing"
)

func TestBuildRFC822IncludesReplyHeaders(t *testing.T) {
	raw, mid, err := BuildRFC822(SendInput{
		From:       "uber@jeeth.dev",
		To:         []string{"a@x.com"},
		Subject:    "Re: Hi",
		BodyText:   "ok",
		InReplyTo:  "<a@x>",
		References: "<a@x>",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if !strings.Contains(s, "In-Reply-To: <a@x>") {
		t.Fatal(s)
	}
	if !strings.Contains(s, "References: <a@x>") {
		t.Fatal(s)
	}
	if !strings.Contains(s, "From: uber@jeeth.dev") {
		t.Fatal(s)
	}
	if mid == "" {
		t.Fatal("message-id")
	}
}

func TestBuildRFC822FoldsLongSubject(t *testing.T) {
	raw, _, err := BuildRFC822(SendInput{
		From:     "me@example.com",
		To:       []string{"a@x.com"},
		Subject:  strings.Repeat("Subject line with unicode café ", 12),
		BodyText: "ok",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		if line == "" {
			break
		}
		if len(line) > 78 {
			t.Fatalf("header line exceeds 78 octets (%d): %q", len(line), line)
		}
	}
	if !strings.Contains(string(raw), "\r\n ") {
		t.Fatal("expected folded subject continuation")
	}
}
