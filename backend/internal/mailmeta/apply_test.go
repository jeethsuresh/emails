package mailmeta

import (
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func TestApplyAndUpsertMessageMetadata(t *testing.T) {
	app := newMailmetaTestApp(t)
	account := saveTestRecord(t, app, "accounts", map[string]any{"email": "me@example.com"})
	folder := saveTestRecord(t, app, "folders", map[string]any{"name": "Inbox"})
	msg := saveTestRecord(t, app, "messages", map[string]any{
		"account":    account.Id,
		"folder":     folder.Id,
		"message_id": "<root@example.net>",
		"subject":    "Re: Project",
		"from_addr":  "Alice Example <alice@example.net>",
		"to_addrs":   "friend@elsewhere.net",
		"date":       "2026-08-13T12:00:00Z",
		"snippet":    "Latest update",
		"seen":       false,
	})

	ApplyMessageMeta(app, msg, map[string]string{
		"Cc":            "alias@example.com",
		"In-Reply-To":   "<parent@example.net>",
		"References":    "<older@example.net> <parent@example.net>",
		"X-Original-To": "alias@example.com",
	}, account.GetString("email"))

	if got := msg.GetString("received_for"); got != "alias@example.com" {
		t.Fatalf("received_for = %q", got)
	}
	if got := msg.GetString("normalized_subject"); got != "project" {
		t.Fatalf("normalized_subject = %q", got)
	}
	if got := msg.GetString("thread_id"); got == "" {
		t.Fatal("thread_id is empty")
	}
	if err := app.Save(msg); err != nil {
		t.Fatal(err)
	}
	if err := UpsertThreadFromMessage(app, msg); err != nil {
		t.Fatal(err)
	}
	if err := UpsertContactFromMessage(app, msg, true); err != nil {
		t.Fatal(err)
	}

	thread, err := app.FindRecordById("threads", msg.GetString("thread_id"))
	if err != nil {
		t.Fatal(err)
	}
	if thread.Id != msg.GetString("thread_id") {
		t.Fatalf("thread id = %q", thread.Id)
	}
	if got := thread.GetFloat("message_count"); got != 1 {
		t.Fatalf("message_count = %v", got)
	}
	if got := thread.GetFloat("unread_count"); got != 1 {
		t.Fatalf("unread_count = %v", got)
	}

	contact, err := app.FindFirstRecordByFilter("contacts", "email = 'alice@example.net'")
	if err != nil {
		t.Fatal(err)
	}
	if got := contact.GetString("display_name"); got != "Alice Example" {
		t.Fatalf("display_name = %q", got)
	}
}

func TestUpsertContactSkipsAccountAddress(t *testing.T) {
	app := newMailmetaTestApp(t)
	account := saveTestRecord(t, app, "accounts", map[string]any{"email": "me@example.com"})
	msg := saveTestRecord(t, app, "messages", map[string]any{
		"account":   account.Id,
		"folder":    "folder01",
		"from_addr": "Me <ME@example.com>",
	})

	if err := UpsertContactFromMessage(app, msg, true); err != nil {
		t.Fatal(err)
	}
	contacts, err := app.FindAllRecords("contacts")
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 0 {
		t.Fatalf("got %d contacts", len(contacts))
	}
}

func TestUpsertContactCountsOnlyNewMessages(t *testing.T) {
	app := newMailmetaTestApp(t)
	account := saveTestRecord(t, app, "accounts", map[string]any{"email": "me@example.com"})
	first := saveTestRecord(t, app, "messages", map[string]any{
		"account":   account.Id,
		"folder":    "folder01",
		"from_addr": "Alice <alice@example.net>",
		"date":      "2026-08-13T12:00:00Z",
	})

	if err := UpsertContactFromMessage(app, first, true); err != nil {
		t.Fatal(err)
	}
	if err := UpsertContactFromMessage(app, first, false); err != nil {
		t.Fatal(err)
	}

	contact, err := app.FindFirstRecordByFilter("contacts", "email = 'alice@example.net'")
	if err != nil {
		t.Fatal(err)
	}
	if got := contact.GetFloat("message_count"); got != 1 {
		t.Fatalf("message_count after reprocessing = %v", got)
	}

	second := saveTestRecord(t, app, "messages", map[string]any{
		"account":   account.Id,
		"folder":    "folder01",
		"from_addr": "Alice <alice@example.net>",
		"date":      "2026-08-13T13:00:00Z",
	})
	if err := UpsertContactFromMessage(app, second, true); err != nil {
		t.Fatal(err)
	}
	contact, err = app.FindFirstRecordByFilter("contacts", "email = 'alice@example.net'")
	if err != nil {
		t.Fatal(err)
	}
	if got := contact.GetFloat("message_count"); got != 2 {
		t.Fatalf("message_count after new message = %v", got)
	}
}

func TestUpsertContactKeepsNewestMessageDate(t *testing.T) {
	app := newMailmetaTestApp(t)
	account := saveTestRecord(t, app, "accounts", map[string]any{"email": "me@example.com"})
	newer := saveTestRecord(t, app, "messages", map[string]any{
		"account":   account.Id,
		"folder":    "folder01",
		"from_addr": "Alice <alice@example.net>",
		"date":      "2026-08-13T13:00:00Z",
	})
	older := saveTestRecord(t, app, "messages", map[string]any{
		"account":   account.Id,
		"folder":    "folder01",
		"from_addr": "Alice <alice@example.net>",
		"date":      "2026-08-13T12:00:00Z",
	})

	if err := UpsertContactFromMessage(app, newer, true); err != nil {
		t.Fatal(err)
	}
	if err := UpsertContactFromMessage(app, older, true); err != nil {
		t.Fatal(err)
	}

	contact, err := app.FindFirstRecordByFilter("contacts", "email = 'alice@example.net'")
	if err != nil {
		t.Fatal(err)
	}
	if got := contact.GetString("last_message_at"); got != "2026-08-13T13:00:00Z" {
		t.Fatalf("last_message_at = %q", got)
	}
}

func TestShouldUpdateLastMessageAtRejectsUnparseableDates(t *testing.T) {
	tests := []struct {
		name      string
		current   string
		candidate string
		want      bool
	}{
		{name: "invalid candidate", current: "2026-08-13T12:00:00Z", candidate: "not-a-date", want: false},
		{name: "invalid current", current: "not-a-date", candidate: "2026-08-13T12:00:00Z", want: false},
		{name: "empty current", current: "", candidate: "not-a-date", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldUpdateLastMessageAt(tt.current, tt.candidate); got != tt.want {
				t.Fatalf("shouldUpdateLastMessageAt(%q, %q) = %v, want %v", tt.current, tt.candidate, got, tt.want)
			}
		})
	}
}

func newMailmetaTestApp(t *testing.T) core.App {
	t.Helper()
	pb := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	if err := pb.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pb.ResetBootstrapState() })

	collections := []*core.Collection{
		core.NewBaseCollection("accounts"),
		core.NewBaseCollection("folders"),
		core.NewBaseCollection("messages"),
		core.NewBaseCollection("threads"),
		core.NewBaseCollection("contacts"),
	}
	collections[0].Fields.Add(&core.TextField{Name: "email"})
	collections[1].Fields.Add(&core.TextField{Name: "name"})
	collections[1].Fields.Add(&core.TextField{Name: "role"})
	for _, field := range []core.Field{
		&core.TextField{Name: "account"}, &core.TextField{Name: "folder"},
		&core.TextField{Name: "message_id"}, &core.TextField{Name: "subject"},
		&core.TextField{Name: "from_addr"}, &core.TextField{Name: "to_addrs"},
		&core.TextField{Name: "date"}, &core.TextField{Name: "snippet"},
		&core.BoolField{Name: "seen"}, &core.TextField{Name: "in_reply_to"},
		&core.TextField{Name: "references"}, &core.TextField{Name: "thread_id"},
		&core.TextField{Name: "received_for"}, &core.TextField{Name: "normalized_subject"},
	} {
		collections[2].Fields.Add(field)
	}
	for _, field := range []core.Field{
		&core.TextField{Name: "subject"}, &core.TextField{Name: "normalized_subject"},
		&core.TextField{Name: "snippet"}, &core.TextField{Name: "last_date"},
		&core.NumberField{Name: "message_count"}, &core.TextField{Name: "participants"},
		&core.TextField{Name: "received_for"}, &core.TextField{Name: "folder"},
		&core.NumberField{Name: "unread_count"}, &core.TextField{Name: "updated_at"},
	} {
		collections[3].Fields.Add(field)
	}
	for _, field := range []core.Field{
		&core.TextField{Name: "email"}, &core.TextField{Name: "display_name"},
		&core.TextField{Name: "last_message_at"}, &core.NumberField{Name: "message_count"},
		&core.TextField{Name: "updated_at"},
	} {
		collections[4].Fields.Add(field)
	}
	for _, collection := range collections {
		if err := pb.Save(collection); err != nil {
			t.Fatal(err)
		}
	}
	return pb
}

func saveTestRecord(t *testing.T, app core.App, collection string, values map[string]any) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	for key, value := range values {
		rec.Set(key, value)
	}
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}
