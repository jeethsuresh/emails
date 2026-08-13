package mailstore

import (
	"fmt"

	"email.local/backend/internal/mailmeta"

	"github.com/pocketbase/pocketbase/core"
)

const mailMetaBackfillBatchSize = 100

func BackfillMailMeta(app core.App) error {
	accounts, err := app.FindAllRecords("accounts")
	if err != nil {
		return err
	}
	accountEmails := make(map[string]string, len(accounts))
	for _, account := range accounts {
		accountEmails[account.Id] = account.GetString("email")
	}

	messages, err := app.FindRecordsByFilter(
		"messages",
		"thread_id = '' || received_for = ''",
		"date",
		0,
		0,
		nil,
	)
	if err != nil {
		return err
	}
	for start := 0; start < len(messages); start += mailMetaBackfillBatchSize {
		end := min(start+mailMetaBackfillBatchSize, len(messages))
		for _, message := range messages[start:end] {
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
			if err := mailmeta.UpsertContactFromMessage(app, message); err != nil {
				return fmt.Errorf("backfill contact for message %s: %w", message.Id, err)
			}
		}
	}
	return nil
}
