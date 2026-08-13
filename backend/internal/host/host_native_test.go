//go:build !js

package host

import "testing"

func TestMimeHeaderGetUnfoldsContinuationLines(t *testing.T) {
	raw := "References: <first@example.com>\r\n\t<second@example.com>\r\n <third@example.com>\r\nSubject: next\r\n\r\nbody"

	got := MimeHeaderGet(raw, "References")
	want := "<first@example.com> <second@example.com> <third@example.com>"
	if got != want {
		t.Fatalf("MimeHeaderGet() = %q, want %q", got, want)
	}
}
