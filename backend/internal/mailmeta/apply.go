package mailmeta

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func ApplyMessageMeta(app core.App, rec *core.Record, headers map[string]string, accountEmail string) {
	inReplyTo := headerGet(headers, "In-Reply-To")
	references := headerGet(headers, "References")
	rec.Set("in_reply_to", inReplyTo)
	rec.Set("references", references)
	rec.Set("normalized_subject", NormalizeSubject(rec.GetString("subject")))
	rec.Set("received_for", ReceivedFor(
		headers,
		rec.GetString("to_addrs"),
		headerGet(headers, "Cc"),
		accountEmail,
	))

	messageDate := time.Now()
	if date := rec.GetString("date"); date != "" {
		if parsed, err := time.Parse(time.RFC3339, date); err == nil {
			messageDate = parsed
		}
	}
	threadID := ResolveThreadID(
		rec.GetString("message_id"),
		inReplyTo,
		references,
		rec.GetString("subject"),
		rec.GetString("from_addr"),
		rec.GetString("to_addrs"),
		PBLookup{App: app},
		messageDate,
	)
	rec.Set("thread_id", threadID)
}
