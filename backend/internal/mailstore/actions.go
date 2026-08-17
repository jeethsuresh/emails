package mailstore

import (
	"strings"
	"time"

	"email.local/backend/internal/mailmeta"

	"github.com/pocketbase/pocketbase/core"
)

// RecordMailAction logs a user-applied (or IMAP-applied) action so later
// analysis can see what this sender / receiving address has already done.
func RecordMailAction(app core.App, messageID, action, target string) {
	messageID = strings.TrimSpace(messageID)
	action = strings.TrimSpace(action)
	if messageID == "" || action == "" {
		return
	}
	col, err := app.FindCollectionByNameOrId("mail_actions")
	if err != nil {
		return
	}
	rec := core.NewRecord(col)
	rec.Set("message", messageID)
	rec.Set("action", action)
	rec.Set("target", strings.TrimSpace(target))
	rec.Set("created_at", time.Now().UTC().Format(time.RFC3339))

	if msg, err := app.FindRecordById("messages", messageID); err == nil {
		rec.Set("from_addr", mailmeta.NormalizeEmail(msg.GetString("from_addr")))
		rec.Set("received_for", mailmeta.NormalizeEmail(msg.GetString("received_for")))
	}
	_ = app.Save(rec)
}
