package analyzer

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Priority string

const (
	PriorityHigh   Priority = "high"
	PriorityMedium Priority = "medium"
	PriorityLow    Priority = "low"
)

type SuggestedAction string

const (
	ActionMoveToFolder SuggestedAction = "move_to_folder"
	ActionMoveToSpam   SuggestedAction = "move_to_spam"
	ActionAddEvent     SuggestedAction = "add_event"
	ActionAddTodo      SuggestedAction = "add_todo"
)

type Result struct {
	Priority        Priority
	SuggestedAction SuggestedAction
	ActionTarget    string
	SuggestedReply  string
}

type rawResult struct {
	Priority        Priority        `json:"priority"`
	SuggestedAction SuggestedAction `json:"suggested_action"`
	ActionTarget    string          `json:"action_target"`
	SuggestedReply  *string         `json:"suggested_reply"`
}

func ParseResult(raw string) (Result, error) {
	payload := stripJSONFence(strings.TrimSpace(raw))

	var parsed rawResult
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return Result{}, fmt.Errorf("parse JSON: %w", err)
	}

	if err := validatePriority(parsed.Priority); err != nil {
		return Result{}, err
	}
	if err := validateSuggestedAction(parsed.SuggestedAction); err != nil {
		return Result{}, err
	}

	result := Result{
		Priority:        parsed.Priority,
		SuggestedAction: parsed.SuggestedAction,
		ActionTarget:    parsed.ActionTarget,
	}
	if parsed.SuggestedReply != nil {
		result.SuggestedReply = *parsed.SuggestedReply
	}
	return result, nil
}

func stripJSONFence(raw string) string {
	if !strings.HasPrefix(raw, "```") {
		return raw
	}

	lines := strings.Split(raw, "\n")
	if len(lines) < 2 {
		return raw
	}

	start := 1
	end := len(lines)
	for i := len(lines) - 1; i >= start; i-- {
		if strings.TrimSpace(lines[i]) == "```" {
			end = i
			break
		}
	}

	if end <= start {
		return raw
	}

	return strings.Join(lines[start:end], "\n")
}

func validatePriority(p Priority) error {
	switch p {
	case PriorityHigh, PriorityMedium, PriorityLow:
		return nil
	default:
		return fmt.Errorf("invalid priority: %q", p)
	}
}

func validateSuggestedAction(a SuggestedAction) error {
	switch a {
	case ActionMoveToFolder, ActionMoveToSpam, ActionAddEvent, ActionAddTodo:
		return nil
	default:
		return fmt.Errorf("invalid suggested_action: %q", a)
	}
}
