package calendar

import (
	"testing"
	"time"
)

func TestExpandDailyRRule(t *testing.T) {
	start := time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	from := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	got := expandRRuleInstances("FREQ=DAILY;COUNT=5", false, start, end, from, to)
	if len(got) != 3 { // 11,12,13
		t.Fatalf("got %d occurrences, want 3: %#v", len(got), got)
	}
}

func TestExpandWeeklyByDay(t *testing.T) {
	// Monday Aug 10 2026
	start := time.Date(2026, 8, 10, 14, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	from := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	got := expandRRuleInstances("FREQ=WEEKLY;BYDAY=MO,WE", false, start, end, from, to)
	if len(got) < 2 {
		t.Fatalf("expected multiple weekly hits, got %d", len(got))
	}
}
