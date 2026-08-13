package mailmeta

import (
	"sort"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func UpsertThreadFromMessage(app core.App, msg *core.Record) error {
	threadID := strings.TrimSpace(msg.GetString("thread_id"))
	if threadID == "" {
		return nil
	}
	collection, err := app.FindCollectionByNameOrId("threads")
	if err != nil {
		return err
	}
	thread, err := app.FindRecordById(collection.Id, threadID)
	if err != nil {
		thread = core.NewRecord(collection)
		thread.Id = threadID
	}

	messages, err := app.FindRecordsByFilter(
		"messages",
		"thread_id = {:thread}",
		"-date",
		0,
		0,
		dbx.Params{"thread": threadID},
	)
	if err != nil {
		return err
	}
	if len(messages) == 0 {
		messages = []*core.Record{msg}
	}
	latest := messages[0]
	participants := map[string]struct{}{}
	unreadCount := 0
	for _, message := range messages {
		for _, email := range append(ParseAddressList(message.GetString("from_addr")), ParseAddressList(message.GetString("to_addrs"))...) {
			if email != "" {
				participants[email] = struct{}{}
			}
		}
		if !message.GetBool("seen") {
			unreadCount++
		}
	}
	participantList := make([]string, 0, len(participants))
	for email := range participants {
		participantList = append(participantList, email)
	}
	sort.Strings(participantList)

	thread.Set("subject", latest.GetString("subject"))
	thread.Set("normalized_subject", latest.GetString("normalized_subject"))
	thread.Set("snippet", latest.GetString("snippet"))
	thread.Set("last_date", latest.GetString("date"))
	thread.Set("message_count", len(messages))
	thread.Set("participants", strings.Join(participantList, ", "))
	thread.Set("received_for", latest.GetString("received_for"))
	thread.Set("folder", latest.GetString("folder"))
	thread.Set("unread_count", unreadCount)
	thread.Set("updated_at", time.Now().UTC().Format(time.RFC3339))
	return app.Save(thread)
}
