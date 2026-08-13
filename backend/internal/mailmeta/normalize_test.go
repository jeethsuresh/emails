package mailmeta

import "testing"

func TestNormalizeSubject(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Re: Hello", "hello"},
		{"RE: re: Fwd: Hello", "hello"},
		{"  Hello  ", "hello"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeSubject(c.in); got != c.want {
			t.Fatalf("%q: got %q want %q", c.in, got, c.want)
		}
	}
}
