package calendar

import (
	"testing"
	"time"
)

func TestWallToUTCUsesEventTimezone(t *testing.T) {
	got, err := wallToUTC("2026-08-12T15:30", "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	// EDT (UTC-4) → 19:30Z
	want := time.Date(2026, 8, 12, 19, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestPackTimedLanes(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	events := []WindowEvent{
		{ID: "a", DisplayDay: "2026-08-12", DisplayStart: "2026-08-12T10:00:00", DisplayEnd: "2026-08-12T11:00:00"},
		{ID: "b", DisplayDay: "2026-08-12", DisplayStart: "2026-08-12T10:30:00", DisplayEnd: "2026-08-12T11:30:00"},
		{ID: "c", DisplayDay: "2026-08-12", DisplayStart: "2026-08-12T12:00:00", DisplayEnd: "2026-08-12T13:00:00"},
	}
	packTimedLanes(events, loc)
	if events[0].LaneCount != 2 || events[1].LaneCount != 2 {
		t.Fatalf("overlap laneCount=%d/%d want 2", events[0].LaneCount, events[1].LaneCount)
	}
	if events[0].Lane == events[1].Lane {
		t.Fatalf("overlapping events share lane %d", events[0].Lane)
	}
	if events[2].LaneCount != 2 {
		// same day group keeps max lane count
		t.Fatalf("laneCount for non-overlap=%d", events[2].LaneCount)
	}
}
