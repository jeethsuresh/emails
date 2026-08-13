package syncer

import (
	"testing"

	"email.local/backend/internal/mailstore"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func TestFindOrCreateMessageAdoptsLocalSentPlaceholder(t *testing.T) {
	app := newSyncTestApp(t)
	account := saveSyncRecord(t, app, "accounts", map[string]any{
		"email":     "me@example.com",
		"username":  "me@example.com",
		"password":  "secret",
		"imap_host": "imap.example.com",
		"imap_port": 993,
		"smtp_host": "smtp.example.com",
		"smtp_port": 465,
	})
	sent := saveSyncRecord(t, app, "folders", map[string]any{
		"account": account.Id,
		"name":    "Sent",
		"role":    "sent",
	})
	// Locally persisted SMTP copy: synthetic negative uid, real Message-ID.
	placeholder := saveSyncRecord(t, app, "messages", map[string]any{
		"account":    account.Id,
		"folder":     sent.Id,
		"uid":        -1712,
		"message_id": "<sent@example.com>",
		"subject":    "Hello",
		"thread_id":  "threadsent00001",
	})

	messages, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		t.Fatal(err)
	}
	rec, created := findOrCreateMessage(app, messages, account.Id, sent.Id, 42, "<sent@example.com>")
	if created {
		t.Fatal("server copy of a locally sent message must not create a second row")
	}
	if rec.Id != placeholder.Id {
		t.Fatalf("adopted record = %q, want placeholder %q", rec.Id, placeholder.Id)
	}
	if got := rec.GetFloat("uid"); got != 42 {
		t.Fatalf("adopted uid = %v, want 42", got)
	}
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	all, err := app.FindAllRecords("messages")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("messages = %d rows, want 1", len(all))
	}
}

func TestFindOrCreateMessageMatchesUnbracketedMessageID(t *testing.T) {
	app := newSyncTestApp(t)
	account := saveSyncRecord(t, app, "accounts", map[string]any{
		"email":     "me@example.com",
		"username":  "me@example.com",
		"password":  "secret",
		"imap_host": "imap.example.com",
		"imap_port": 993,
		"smtp_host": "smtp.example.com",
		"smtp_port": 465,
	})
	sent := saveSyncRecord(t, app, "folders", map[string]any{
		"account": account.Id,
		"name":    "Sent",
		"role":    "sent",
	})
	placeholder := saveSyncRecord(t, app, "messages", map[string]any{
		"account":    account.Id,
		"folder":     sent.Id,
		"uid":        -99,
		"message_id": "sent@example.com",
	})

	messages, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		t.Fatal(err)
	}
	rec, created := findOrCreateMessage(app, messages, account.Id, sent.Id, 7, "<sent@example.com>")
	if created || rec.Id != placeholder.Id {
		t.Fatalf("bracket-only difference must still match (created=%v id=%q)", created, rec.Id)
	}
}

func TestFindOrCreateMessageKeepsCopyInOtherFolder(t *testing.T) {
	app := newSyncTestApp(t)
	account := saveSyncRecord(t, app, "accounts", map[string]any{
		"email":     "me@example.com",
		"username":  "me@example.com",
		"password":  "secret",
		"imap_host": "imap.example.com",
		"imap_port": 993,
		"smtp_host": "smtp.example.com",
		"smtp_port": 465,
	})
	inbox := saveSyncRecord(t, app, "folders", map[string]any{
		"account": account.Id,
		"name":    "Inbox",
		"role":    "inbox",
	})
	archive := saveSyncRecord(t, app, "folders", map[string]any{
		"account": account.Id,
		"name":    "Archive",
		"role":    "other",
	})
	inboxCopy := saveSyncRecord(t, app, "messages", map[string]any{
		"account":    account.Id,
		"folder":     inbox.Id,
		"uid":        11,
		"message_id": "<shared@example.com>",
	})

	messages, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		t.Fatal(err)
	}
	rec, created := findOrCreateMessage(app, messages, account.Id, archive.Id, 11, "<shared@example.com>")
	if !created {
		t.Fatal("a server copy in another folder is a distinct row")
	}
	if rec.Id == inboxCopy.Id {
		t.Fatal("must not steal the row of the copy in the other folder")
	}
	if got := rec.GetString("folder"); got != archive.Id {
		t.Fatalf("new record folder = %q, want %q", got, archive.Id)
	}
}

func newSyncTestApp(t *testing.T) *pocketbase.PocketBase {
	t.Helper()
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: t.TempDir()})
	mailstore.Register(app)
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.ResetBootstrapState(); err != nil {
			t.Error(err)
		}
	})
	return app
}

func saveSyncRecord(t *testing.T, app core.App, collection string, fields map[string]any) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatal(err)
	}
	record := core.NewRecord(col)
	for key, value := range fields {
		record.Set(key, value)
	}
	if err := app.Save(record); err != nil {
		t.Fatal(err)
	}
	return record
}
