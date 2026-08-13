package mailmeta

import (
	"testing"
	"time"
)

type mapLookup struct {
	byMID map[string]string
	bySub map[string]string
}

func (m mapLookup) ByMessageID(id string) (string, bool) {
	v, ok := m.byMID[NormalizeMessageID(id)]
	return v, ok
}

func (m mapLookup) BySubjectParticipants(subj string, _ []string, _ string) (string, bool) {
	v, ok := m.bySub[subj]
	return v, ok
}

func TestNormalizeMessageID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"<A@X>", "a@x"},
		{"  <foo@bar>  ", "foo@bar"},
		{"bare@id", "bare@id"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeMessageID(c.in); got != c.want {
			t.Fatalf("%q: got %q want %q", c.in, got, c.want)
		}
	}
}

func TestCollectMessageIDs(t *testing.T) {
	got := CollectMessageIDs("<a@x>", "<b@x> <c@x>, <a@x>")
	want := []string{"a@x", "b@x", "c@x"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestNewThreadIDStable(t *testing.T) {
	a := NewThreadID("<msg@x>")
	b := NewThreadID("<msg@x>")
	if a == "" || a != b {
		t.Fatalf("expected stable non-empty id, got %q and %q", a, b)
	}
}

func TestNewThreadIDEmpty(t *testing.T) {
	if got := NewThreadID(""); got == "" {
		t.Fatal("expected non-empty id for empty messageID")
	}
}

func TestResolveThreadViaInReplyTo(t *testing.T) {
	lu := mapLookup{byMID: map[string]string{"a@x": "thr1"}}
	got := ResolveThreadID("<b@x>", "<a@x>", "", "Re: Hi", "a@b.com", "c@d.com", lu, time.Now())
	if got != "thr1" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveThreadFallbackSubject(t *testing.T) {
	lu := mapLookup{bySub: map[string]string{"hi": "thr2"}}
	got := ResolveThreadID("<c@x>", "", "", "Re: Hi", "a@b.com", "c@d.com", lu, time.Now())
	if got != "thr2" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveThreadNewRoot(t *testing.T) {
	lu := mapLookup{}
	got := ResolveThreadID("<new@x>", "", "", "Unique", "a@b.com", "c@d.com", lu, time.Now())
	if got == "" {
		t.Fatal("expected new id")
	}
}
