package analyzer

import (
	"strings"
	"testing"
)

func TestClipPromptBodyKeepsFullWhenUnderCap(t *testing.T) {
	body := strings.Repeat("a", 10_000)
	if got := clipPromptBody(body); got != body {
		t.Fatalf("expected full body, got len %d", len(got))
	}
}

func TestClipPromptBodyKeepsHeadAndTailWhenHuge(t *testing.T) {
	body := strings.Repeat("H", promptBodyHeadKeep) + strings.Repeat("M", 50_000) + strings.Repeat("T", promptBodyTailKeep)
	got := clipPromptBody(body)
	if !strings.HasPrefix(got, strings.Repeat("H", 100)) {
		t.Fatal("expected head preserved")
	}
	if !strings.HasSuffix(got, strings.Repeat("T", 100)) {
		t.Fatal("expected tail preserved")
	}
	if !strings.Contains(got, "[middle truncated]") {
		t.Fatal("expected truncation marker")
	}
}
