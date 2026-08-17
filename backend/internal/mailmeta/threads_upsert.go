package mailmeta

import (
	"sort"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// threadMemberRow projects only the columns thread aggregates need, so
// recomputing a thread never pulls message bodies into memory.
type threadMemberRow struct {
	Subject           string `db:"subject"`
	NormalizedSubject string `db:"normalized_subject"`
	Snippet           string `db:"snippet"`
	Date              string `db:"date"`
	FromAddr          string `db:"from_addr"`
	ToAddrs           string `db:"to_addrs"`
	ReceivedFor       string `db:"received_for"`
	Folder            string `db:"folder"`
	Role              string `db:"role"`
	Seen              bool   `db:"seen"`
}

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

	members, err := threadMembers(app, threadID)
	if err != nil {
		return err
	}
	if len(members) == 0 {
		members = []threadMemberRow{memberFromRecord(msg)}
	}
	applyThreadAggregates(thread, members)
	return app.Save(thread)
}

// RecountThread refreshes a thread from its remaining messages and deletes it
// when the last message moved to another thread, so no empty thread lingers.
func RecountThread(app core.App, threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil
	}
	thread, err := app.FindRecordById("threads", threadID)
	if err != nil {
		return nil
	}
	members, err := threadMembers(app, threadID)
	if err != nil {
		return err
	}
	if len(members) == 0 {
		return app.Delete(thread)
	}
	applyThreadAggregates(thread, members)
	return app.Save(thread)
}

func threadMembers(app core.App, threadID string) ([]threadMemberRow, error) {
	rows := make([]threadMemberRow, 0, 8)
	err := app.DB().NewQuery(`
		SELECT m.subject, m.normalized_subject, m.snippet, m.date,
		       m.from_addr, m.to_addrs, m.received_for, m.folder, m.seen,
		       COALESCE(f.role, '') AS role
		FROM messages m
		LEFT JOIN folders f ON f.id = m.folder
		WHERE m.thread_id = {:thread}
		ORDER BY m.date DESC
	`).Bind(dbx.Params{"thread": threadID}).All(&rows)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func applyThreadAggregates(thread *core.Record, members []threadMemberRow) {
	latest := members[0]
	participants := map[string]struct{}{}
	unreadCount := 0
	for _, member := range members {
		for _, email := range append(ParseAddressList(member.FromAddr), ParseAddressList(member.ToAddrs)...) {
			if email != "" {
				participants[email] = struct{}{}
			}
		}
		if !member.Seen {
			unreadCount++
		}
	}
	participantList := make([]string, 0, len(participants))
	for email := range participants {
		participantList = append(participantList, email)
	}
	sort.Strings(participantList)

	thread.Set("subject", latest.Subject)
	thread.Set("normalized_subject", latest.NormalizedSubject)
	thread.Set("snippet", latest.Snippet)
	thread.Set("last_date", latest.Date)
	thread.Set("message_count", len(members))
	thread.Set("participants", strings.Join(participantList, ", "))
	thread.Set("received_for", latest.ReceivedFor)
	thread.Set("folder", listingFolder(members))
	thread.Set("unread_count", unreadCount)
	thread.Set("updated_at", time.Now().UTC().Format(time.RFC3339))
}

// listingFolder prefers the newest message that still lives in a mailbox the
// user reads (not Sent/Drafts/Trash/Junk), so a reply does not rehome the
// denormalized folder to Sent.
func listingFolder(members []threadMemberRow) string {
	for _, member := range members {
		switch strings.ToLower(strings.TrimSpace(member.Role)) {
		case "sent", "drafts", "trash", "junk", "spam":
			continue
		}
		if member.Folder != "" {
			return member.Folder
		}
	}
	if len(members) > 0 {
		return members[0].Folder
	}
	return ""
}

func memberFromRecord(msg *core.Record) threadMemberRow {
	return threadMemberRow{
		Subject:           msg.GetString("subject"),
		NormalizedSubject: msg.GetString("normalized_subject"),
		Snippet:           msg.GetString("snippet"),
		Date:              msg.GetString("date"),
		FromAddr:          msg.GetString("from_addr"),
		ToAddrs:           msg.GetString("to_addrs"),
		ReceivedFor:       msg.GetString("received_for"),
		Folder:            msg.GetString("folder"),
		Seen:              msg.GetBool("seen"),
	}
}
