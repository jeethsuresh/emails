package analyzer

import (
	"fmt"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

const (
	itemStatusDraft    = "draft"
	itemStatusApproved = "approved"
)

// upsertDraftFromAnalysis creates or refreshes a draft todo/event when
// analysis completes with add_todo / add_event. Idempotent per source_message:
// approved rows are left alone; existing drafts have title/fields updated.
func upsertDraftFromAnalysis(app core.App, messageID string, result Result) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil
	}
	switch result.SuggestedAction {
	case ActionAddTodo:
		return upsertDraftItem(app, "todos", messageID, result.ActionTarget, map[string]any{
			"deadline": "",
		})
	case ActionAddEvent:
		return upsertDraftItem(app, "events", messageID, result.ActionTarget, map[string]any{
			"starts_at": "",
			"ends_at":   "",
		})
	default:
		return nil
	}
}

func upsertDraftItem(app core.App, collection, messageID, actionTarget string, extras map[string]any) error {
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		return err
	}

	recs, err := app.FindRecordsByFilter(col.Id, "source_message = {:m}", "", 0, 0, dbx.Params{"m": messageID})
	if err != nil {
		return err
	}
	for _, rec := range recs {
		if rec.GetString("status") == itemStatusApproved {
			return nil
		}
	}

	title := strings.TrimSpace(actionTarget)
	if title == "" {
		title = messageSubjectFallback(app, messageID)
	}

	var draft *core.Record
	for _, rec := range recs {
		if rec.GetString("status") == itemStatusDraft || rec.GetString("status") == "" {
			draft = rec
			break
		}
	}
	if draft == nil {
		draft = core.NewRecord(col)
		draft.Set("source_message", messageID)
		draft.Set("notes", "")
		draft.Set("created_at", time.Now().UTC().Format(time.RFC3339))
		for k, v := range extras {
			draft.Set(k, v)
		}
	}
	draft.Set("title", title)
	draft.Set("status", itemStatusDraft)
	if err := app.Save(draft); err != nil {
		return fmt.Errorf("save %s draft: %w", collection, err)
	}
	return nil
}

func messageSubjectFallback(app core.App, messageID string) string {
	msgCol, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		return "(no subject)"
	}
	msg, err := app.FindRecordById(msgCol, messageID)
	if err != nil {
		return "(no subject)"
	}
	subject := strings.TrimSpace(msg.GetString("subject"))
	if subject == "" {
		return "(no subject)"
	}
	return subject
}
