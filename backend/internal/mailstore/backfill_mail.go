package mailstore

import (
	"fmt"
	"log"

	"email.local/backend/internal/mailmeta"

	"github.com/pocketbase/pocketbase/core"
)

const mailMetaBackfillBatchSize = 100

// StartMailMetaBackfill repairs thread/contact metadata for already-stored
// messages in the background. Bootstrap must not block on it: the work scales
// with mailbox size, and every request (including the first) is served by the
// same process that runs it.
func StartMailMetaBackfill(app core.App) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("mail meta backfill panicked: %v", r)
			}
		}()
		if err := BackfillMailMeta(app); err != nil {
			log.Printf("mail meta backfill failed: %v", err)
		}
	}()
}

func BackfillMailMeta(app core.App) error {
	accounts, err := app.FindAllRecords("accounts")
	if err != nil {
		return err
	}
	accountEmails := make(map[string]string, len(accounts))
	for _, account := range accounts {
		accountEmails[account.Id] = account.GetString("email")
	}

	lastID := ""
	for {
		messages, err := app.FindRecordsByFilter(
			"messages",
			"(thread_id = '' || received_for = '') && id > {:lastID}",
			"id",
			mailMetaBackfillBatchSize,
			0,
			map[string]any{"lastID": lastID},
		)
		if err != nil {
			return err
		}
		if len(messages) == 0 {
			break
		}
		for _, message := range messages {
			headers := map[string]string{
				"In-Reply-To": message.GetString("in_reply_to"),
				"References":  message.GetString("references"),
				"To":          message.GetString("to_addrs"),
			}
			mailmeta.ApplyMessageMeta(app, message, headers, accountEmails[message.GetString("account")])
			if err := app.Save(message); err != nil {
				return fmt.Errorf("backfill message %s: %w", message.Id, err)
			}
			if err := mailmeta.UpsertThreadFromMessage(app, message); err != nil {
				return fmt.Errorf("backfill thread for message %s: %w", message.Id, err)
			}
			if err := mailmeta.UpsertContactFromMessage(app, message, false); err != nil {
				return fmt.Errorf("backfill contact for message %s: %w", message.Id, err)
			}
			lastID = message.Id
		}
	}
	return nil
}
