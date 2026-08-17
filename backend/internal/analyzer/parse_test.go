package analyzer

import (
	"strings"
	"testing"
)

func TestParseResultValidJSON(t *testing.T) {
	raw := `{"priority":"high","suggested_action":"move_to_folder","action_target":"Receipts","suggested_reply":"Thanks!"}`
	got, err := ParseResult(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Priority != PriorityHigh {
		t.Fatalf("priority: got %q want %q", got.Priority, PriorityHigh)
	}
	if got.SuggestedAction != ActionMoveToFolder {
		t.Fatalf("suggested_action: got %q want %q", got.SuggestedAction, ActionMoveToFolder)
	}
	if got.ActionTarget != "Receipts" {
		t.Fatalf("action_target: got %q want %q", got.ActionTarget, "Receipts")
	}
	if got.SuggestedReply != "Thanks!" {
		t.Fatalf("suggested_reply: got %q want %q", got.SuggestedReply, "Thanks!")
	}
	if got.CreateFolder {
		t.Fatal("create_folder should default false")
	}
}

func TestParseResultCreateFolder(t *testing.T) {
	raw := `{"priority":"low","suggested_action":"move_to_folder","action_target":"Receipts 2026","create_folder":true}`
	got, err := ParseResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreateFolder {
		t.Fatal("expected create_folder")
	}
	raw = `{"priority":"low","suggested_action":"add_todo","action_target":"Buy milk","create_folder":true}`
	got, err = ParseResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.CreateFolder {
		t.Fatal("create_folder only applies to move_to_folder")
	}
}

func TestParseResultFencedJSON(t *testing.T) {
	raw := "```json\n" + `{"priority":"medium","suggested_action":"add_todo","action_target":"","suggested_reply":null}` + "\n```"
	got, err := ParseResult(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Priority != PriorityMedium {
		t.Fatalf("priority: got %q want %q", got.Priority, PriorityMedium)
	}
	if got.SuggestedAction != ActionAddTodo {
		t.Fatalf("suggested_action: got %q want %q", got.SuggestedAction, ActionAddTodo)
	}
	if got.SuggestedReply != "" {
		t.Fatalf("suggested_reply: got %q want empty", got.SuggestedReply)
	}
}

func TestParseResultInvalidPriority(t *testing.T) {
	raw := `{"priority":"urgent","suggested_action":"move_to_spam","action_target":"","suggested_reply":null}`
	_, err := ParseResult(raw)
	if err == nil {
		t.Fatal("expected error for invalid priority")
	}
	if !strings.Contains(err.Error(), "priority") {
		t.Fatalf("error should mention priority: %v", err)
	}
}

func TestParseResultInvalidAction(t *testing.T) {
	raw := `{"priority":"low","suggested_action":"delete","action_target":"","suggested_reply":null}`
	_, err := ParseResult(raw)
	if err == nil {
		t.Fatal("expected error for invalid suggested_action")
	}
	if !strings.Contains(err.Error(), "suggested_action") {
		t.Fatalf("error should mention suggested_action: %v", err)
	}
}

func TestParseResultAddEventFields(t *testing.T) {
	raw := `{"priority":"medium","suggested_action":"add_event","action_target":"Sync","event_starts_at":"2026-08-20T15:00:00Z","event_ends_at":"2026-08-20T16:00:00Z","attendees":["a@b.com"," A@B.com "]}`
	got, err := ParseResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.EventStartsAt != "2026-08-20T15:00:00Z" || got.EventEndsAt != "2026-08-20T16:00:00Z" {
		t.Fatalf("times: %#v %#v", got.EventStartsAt, got.EventEndsAt)
	}
	if len(got.Attendees) != 1 || got.Attendees[0] != "a@b.com" {
		t.Fatalf("attendees: %#v", got.Attendees)
	}
	raw = `{"priority":"low","suggested_action":"add_todo","action_target":"X","event_starts_at":"2026-08-20T15:00:00Z","attendees":["a@b.com"]}`
	got, err = ParseResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.EventStartsAt != "" || len(got.Attendees) != 0 {
		t.Fatal("event fields should clear for non-add_event")
	}
}

func TestParseResultAddEventStartOnlyDefaultsEnd(t *testing.T) {
	raw := `{"priority":"high","suggested_action":"add_event","action_target":"WestJet YYC-YVR","event_starts_at":"2026-09-01T14:30:00Z"}`
	got, err := ParseResult(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.EventStartsAt != "2026-09-01T14:30:00Z" {
		t.Fatalf("start: %q", got.EventStartsAt)
	}
	if got.EventEndsAt != "2026-09-01T15:30:00Z" {
		t.Fatalf("expected default end start+1h, got %q", got.EventEndsAt)
	}
}

func TestParseResultMissingFields(t *testing.T) {
	raw := `{"action_target":"Inbox","suggested_reply":null}`
	_, err := ParseResult(raw)
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
}
