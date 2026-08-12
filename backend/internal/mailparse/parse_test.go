package mailparse

import (
	"strings"
	"testing"
)

func TestParsePlain(t *testing.T) {
	raw := []byte("From: a@b.com\r\nTo: c@d.com\r\nSubject: Hi\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nHello body world\r\n")
	p := ParseRFC822(raw)
	if p.Text != "Hello body world" {
		t.Fatalf("text=%q", p.Text)
	}
	if !strings.Contains(p.Snippet, "Hello") {
		t.Fatalf("snippet=%q", p.Snippet)
	}
}

func TestParseMultipartPrefersPlain(t *testing.T) {
	raw := []byte("Subject: Multi\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/alternative; boundary=b1\r\n\r\n" +
		"--b1\r\nContent-Type: text/plain\r\n\r\nPlain part\r\n" +
		"--b1\r\nContent-Type: text/html\r\n\r\n<html><body>HTML part</body></html>\r\n" +
		"--b1--\r\n")
	p := ParseRFC822(raw)
	if p.Text != "Plain part" {
		t.Fatalf("text=%q", p.Text)
	}
	if !strings.Contains(p.HTML, "HTML part") {
		t.Fatalf("html=%q", p.HTML)
	}
}

func TestParseHTMLOnly(t *testing.T) {
	raw := []byte("Subject: H\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Only html</p>\r\n")
	p := ParseRFC822(raw)
	if p.Text != "Only html" {
		t.Fatalf("text=%q", p.Text)
	}
	if p.Snippet != "Only html" {
		t.Fatalf("snippet=%q", p.Snippet)
	}
}
