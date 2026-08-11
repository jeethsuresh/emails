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

func TestParseResultMissingFields(t *testing.T) {
	raw := `{"action_target":"Inbox","suggested_reply":null}`
	_, err := ParseResult(raw)
	if err == nil {
		t.Fatal("expected error for missing required fields")
	}
}
