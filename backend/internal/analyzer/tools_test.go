package analyzer

import (
	"strings"
	"testing"

	"email.local/backend/internal/mailstore"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func TestSearchEmailsPartialMatch(t *testing.T) {
	app := newAnalyzerTestApp(t)
	account := newAnalyzerAccount(t, app)
	inbox := newAnalyzerFolder(t, app, account.Id, "INBOX", "inbox")
	current := newAnalyzerMessage(t, app, account.Id, inbox.Id, 1, map[string]any{
		"subject": "Current",
		"date":    "2026-08-15T12:00:00Z",
	})
	hit := newAnalyzerMessage(t, app, account.Id, inbox.Id, 2, map[string]any{
		"subject":      "Q3 invoice attached",
		"from_addr":    "billing@acme.test",
		"to_addrs":     "me@example.com",
		"received_for": "me@example.com",
		"body_text":    "Please pay the remainder",
		"date":         "2026-08-14T12:00:00Z",
	})
	session := newSession(app, current)

	out, err := toolSearchEmails(session, "invo", "subject")
	if err != nil {
		t.Fatal(err)
	}
	items := out.(map[string]any)["items"].([]searchHit)
	if len(items) != 1 || items[0].ID != hit.Id {
		t.Fatalf("subject search = %#v", items)
	}

	out, err = toolSearchEmails(session, "acme", "sender")
	if err != nil {
		t.Fatal(err)
	}
	items = out.(map[string]any)["items"].([]searchHit)
	if len(items) != 1 || items[0].ID != hit.Id {
		t.Fatalf("sender search = %#v", items)
	}
}

func TestGetSentEmailBodyRejectsInbox(t *testing.T) {
	app := newAnalyzerTestApp(t)
	account := newAnalyzerAccount(t, app)
	inbox := newAnalyzerFolder(t, app, account.Id, "INBOX", "inbox")
	msg := newAnalyzerMessage(t, app, account.Id, inbox.Id, 1, map[string]any{
		"subject": "Hi",
		"date":    "2026-08-15T12:00:00Z",
	})
	session := newSession(app, msg)
	if _, err := toolGetSentBody(session, msg.Id); err == nil {
		t.Fatal("expected not a sent message")
	}
}

func TestBuildUserPromptIncludesFoldersAndSent(t *testing.T) {
	app := newAnalyzerTestApp(t)
	account := newAnalyzerAccount(t, app)
	inbox := newAnalyzerFolder(t, app, account.Id, "INBOX", "inbox")
	sent := newAnalyzerFolder(t, app, account.Id, "Sent", "sent")
	newAnalyzerFolder(t, app, account.Id, "Receipts", "other")
	current := newAnalyzerMessage(t, app, account.Id, inbox.Id, 1, map[string]any{
		"subject":      "Hello",
		"from_addr":    "alice@example.net",
		"received_for": "me@example.com",
		"body_text":    "body",
		"date":         "2026-08-15T12:00:00Z",
	})
	newAnalyzerMessage(t, app, account.Id, sent.Id, 2, map[string]any{
		"subject":   "Re: Hello",
		"from_addr": "me@example.com",
		"body_text": "thanks",
		"date":      "2026-08-14T12:00:00Z",
	})
	prompt, _, err := buildUserPrompt(app, current.Id)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Receipts", "INBOX", "Sent", "Re: Hello", "Existing folders"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func newAnalyzerTestApp(t *testing.T) *pocketbase.PocketBase {
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

func newAnalyzerAccount(t *testing.T, app *pocketbase.PocketBase) *core.Record {
	t.Helper()
	accounts, err := app.FindCollectionByNameOrId("accounts")
	if err != nil {
		t.Fatal(err)
	}
	account := core.NewRecord(accounts)
	account.Set("email", "me@example.com")
	account.Set("username", "me@example.com")
	account.Set("password", "secret")
	account.Set("imap_host", "imap.example.com")
	account.Set("imap_port", 993)
	account.Set("smtp_host", "smtp.example.com")
	account.Set("smtp_port", 465)
	if err := app.Save(account); err != nil {
		t.Fatal(err)
	}
	return account
}

func newAnalyzerFolder(t *testing.T, app *pocketbase.PocketBase, accountID, name, role string) *core.Record {
	t.Helper()
	folders, err := app.FindCollectionByNameOrId("folders")
	if err != nil {
		t.Fatal(err)
	}
	folder := core.NewRecord(folders)
	folder.Set("account", accountID)
	folder.Set("name", name)
	folder.Set("role", role)
	if err := app.Save(folder); err != nil {
		t.Fatal(err)
	}
	return folder
}

func newAnalyzerMessage(t *testing.T, app *pocketbase.PocketBase, accountID, folderID string, uid int, fields map[string]any) *core.Record {
	t.Helper()
	messages, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		t.Fatal(err)
	}
	message := core.NewRecord(messages)
	message.Set("account", accountID)
	message.Set("folder", folderID)
	message.Set("uid", uid)
	for key, value := range fields {
		message.Set(key, value)
	}
	if err := app.Save(message); err != nil {
		t.Fatal(err)
	}
	return message
}
