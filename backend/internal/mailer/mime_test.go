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
