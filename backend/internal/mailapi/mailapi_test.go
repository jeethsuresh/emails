package mailapi

import (
	"reflect"
	"strings"
	"testing"

	"email.local/backend/internal/mailstore"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

func TestValidateSMTPAccountRequiresPassword(t *testing.T) {
	app := newTestMailApp(t)
	account := newTestAccount(t, app)
	if err := validateSMTPAccount(account); err != nil {
		t.Fatalf("complete account: %v", err)
	}
	account.Set("password", "")
	if err := validateSMTPAccount(account); err == nil {
		t.Fatal("expected incomplete SMTP settings without password")
	}
}

func TestForwardSubject(t *testing.T) {
	if got := forwardSubject("Hello"); got != "Fwd: Hello" {
		t.Fatalf("forwardSubject = %q", got)
	}
	if got := forwardSubject("Fwd: Hello"); got != "Fwd: Hello" {
		t.Fatalf("already forwarded = %q", got)
	}
}

func TestReplyAllRecipients(t *testing.T) {
	to, cc := replyAllRecipients(
		"Sender <sender@example.com>",
		"me@example.com, other@example.com",
		"cc@example.com",
		"me@example.com",
	)
	if len(to) != 1 || to[0] != "sender@example.com" {
		t.Fatalf("to = %#v", to)
	}
	got := map[string]bool{}
	for _, addr := range cc {
		got[addr] = true
	}
	if !got["other@example.com"] || !got["cc@example.com"] {
		t.Fatalf("cc = %#v", cc)
	}
	if got["me@example.com"] || got["sender@example.com"] {
		t.Fatalf("cc should exclude self and primary: %#v", cc)
	}
}

func TestReplySubject(t *testing.T) {
	tests := map[string]string{
		"Hello":     "Re: Hello",
		"Re: Hello": "Re: Hello",
		"RE: Hello": "RE: Hello",
		"":          "Re:",
	}
	for input, want := range tests {
		if got := replySubject(input); got != want {
			t.Errorf("replySubject(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestReplyCounterpart(t *testing.T) {
	if got := replyCounterpart("Sender <sender@example.com>", "me@example.com", "me@example.com"); got != "sender@example.com" {
		t.Fatalf("inbound counterpart = %q", got)
	}
	if got := replyCounterpart("Me <me@example.com>", "First <first@example.com>, second@example.com", "me@example.com"); got != "first@example.com" {
		t.Fatalf("outbound counterpart = %q", got)
	}
}

func TestReplyReferencesIncludesOriginalMessage(t *testing.T) {
	if got, want := replyReferences("<root@example.com>", "<original@example.com>"), "<root@example.com> <original@example.com>"; got != want {
		t.Fatalf("replyReferences() = %q, want %q", got, want)
	}
	if got := replyReferences("<original@example.com>", "<original@example.com>"); got != "<original@example.com>" {
		t.Fatalf("replyReferences() duplicated message id: %q", got)
	}
}

func TestEnvelopeRecipientsMergeToAndCc(t *testing.T) {
	got := envelopeRecipients([]string{"to@example.com"}, []string{"cc@example.com", "to@example.com"})
	want := []string{"to@example.com", "cc@example.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("envelopeRecipients() = %#v, want %#v", got, want)
	}
}

func TestMergeSuggestedReplyKeepsQuotedOriginal(t *testing.T) {
	quoted := quoteOriginal("2026-08-13T12:00:00Z", "Sender <sender@example.com>", "first")
	got := mergeSuggestedReply("Thanks!", quoted)
	if !strings.HasPrefix(got, "Thanks!") {
		t.Fatalf("missing suggestion: %q", got)
	}
	if !strings.Contains(got, "> first") {
		t.Fatalf("dropped quote: %q", got)
	}
	if got := mergeSuggestedReply("  ", quoted); got != quoted {
		t.Fatalf("empty suggestion should leave quote only, got %q", got)
	}
}

func TestQuoteOriginal(t *testing.T) {
	got := quoteOriginal("2026-08-13T12:00:00Z", "Sender <sender@example.com>", "first\nsecond")
	want := "\n\nOn 2026-08-13T12:00:00Z, Sender <sender@example.com> wrote:\n> first\n> second"
	if got != want {
		t.Fatalf("quoteOriginal() = %q, want %q", got, want)
	}
}

func TestPaginationBounds(t *testing.T) {
	if page, perPage := pagination("", ""); page != 1 || perPage != 50 {
		t.Fatalf("default pagination = %d/%d", page, perPage)
	}
	if page, perPage := pagination("-2", "500"); page != 1 || perPage != 200 {
		t.Fatalf("bounded pagination = %d/%d", page, perPage)
	}
}

func TestFindContactMessagesNormalizesStoredFromAddress(t *testing.T) {
	app := newTestMailApp(t)
	account := newTestAccount(t, app)
	folder := newTestFolder(t, app, account.Id)
	messages, err := app.FindCollectionByNameOrId("messages")
	if err != nil {
		t.Fatal(err)
	}
	for i, from := range []string{
		"contact@example.com",
		"Contact Name <contact@example.com>",
		"CONTACT NAME <CONTACT@EXAMPLE.COM>",
		"notcontact@example.com",
	} {
		message := core.NewRecord(messages)
		message.Set("account", account.Id)
		message.Set("folder", folder.Id)
		message.Set("uid", i+1)
		message.Set("subject", from)
		message.Set("from_addr", from)
		message.Set("date", "2026-08-13T12:00:00Z")
		if err := app.Save(message); err != nil {
			t.Fatal(err)
		}
	}

	firstPage, total, err := findContactMessages(app, "CONTACT@example.com", 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(firstPage) != 2 {
		t.Fatalf("first page = %d records, total %d; want 2 records, total 3", len(firstPage), total)
	}
	secondPage, total, err := findContactMessages(app, "contact@example.com", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 || len(secondPage) != 1 {
		t.Fatalf("second page = %d records, total %d; want 1 record, total 3", len(secondPage), total)
	}
	for _, message := range append(firstPage, secondPage...) {
		if message.GetString("from_addr") == "notcontact@example.com" {
			t.Fatal("substring address must not match the contact")
		}
	}
}

func TestFindContactMessagesIgnoresLikeWildcardsInAddress(t *testing.T) {
	app := newTestMailApp(t)
	account := newTestAccount(t, app)
	folder := newTestFolder(t, app, account.Id)
	for i, from := range []string{"a_b@example.com", "axb@example.com"} {
		newTestMessage(t, app, account.Id, folder.Id, i+1, map[string]any{
			"from_addr": from,
			"date":      "2026-08-13T12:00:00Z",
		})
	}

	messages, total, err := findContactMessages(app, "a_b@example.com", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(messages) != 1 {
		t.Fatalf("underscore address matched %d records (total %d), want 1", len(messages), total)
	}
	if got := messages[0].GetString("from_addr"); got != "a_b@example.com" {
		t.Fatalf("from_addr = %q", got)
	}
}

func TestFindThreadIDsKeepsThreadInFolderOfOlderMessage(t *testing.T) {
	app := newTestMailApp(t)
	account := newTestAccount(t, app)
	inbox := newTestFolder(t, app, account.Id)
	sent := newTestSentFolder(t, app, account.Id)
	thread := newTestThread(t, app, "threadreplied01", map[string]any{
		// The reply is newest, so the denormalized thread folder is Sent.
		"folder":    sent.Id,
		"last_date": "2026-08-13T13:00:00Z",
	})
	newTestMessage(t, app, account.Id, inbox.Id, 1, map[string]any{
		"thread_id":    thread.Id,
		"date":         "2026-08-13T12:00:00Z",
		"received_for": "alias@example.com",
	})
	newTestMessage(t, app, account.Id, sent.Id, 2, map[string]any{
		"thread_id": thread.Id,
		"date":      "2026-08-13T13:00:00Z",
	})

	ids, total, err := findThreadIDs(app, inbox.Id, "", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(ids) != 1 || ids[0] != thread.Id {
		t.Fatalf("inbox listing = %v (total %d), want the replied thread", ids, total)
	}

	ids, total, err = findThreadIDs(app, sent.Id, "", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(ids) != 1 {
		t.Fatalf("sent listing = %v (total %d), want the replied thread", ids, total)
	}

	ids, total, err = findThreadIDs(app, inbox.Id, "alias@example.com", 1, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(ids) != 1 {
		t.Fatalf("alias listing = %v (total %d), want the replied thread", ids, total)
	}
}

func TestFindThreadIDsSkipsThreadsWithoutMessages(t *testing.T) {
	app := newTestMailApp(t)
	account := newTestAccount(t, app)
	inbox := newTestFolder(t, app, account.Id)
	ghost := newTestThread(t, app, "threadghost0001", map[string]any{
		"folder":        inbox.Id,
		"last_date":     "2026-08-13T14:00:00Z",
		"message_count": 3,
	})
	live := newTestThread(t, app, "threadlive00001", map[string]any{
		"folder":    inbox.Id,
		"last_date": "2026-08-13T12:00:00Z",
	})
	newTestMessage(t, app, account.Id, inbox.Id, 1, map[string]any{
		"thread_id": live.Id,
		"date":      "2026-08-13T12:00:00Z",
	})

	for name, folder := range map[string]string{"folder filter": inbox.Id, "no filter": ""} {
		ids, total, err := findThreadIDs(app, folder, "", 1, 50)
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || len(ids) != 1 || ids[0] != live.Id {
			t.Fatalf("%s listing = %v (total %d), want only %s", name, ids, total, live.Id)
		}
		if ids[0] == ghost.Id {
			t.Fatalf("%s listed the ghost thread", name)
		}
	}
}

func TestMarkDraftSentRecordsLocalPersistenceWarning(t *testing.T) {
	app := newTestMailApp(t)
	account := newTestAccount(t, app)
	drafts, err := app.FindCollectionByNameOrId("drafts")
	if err != nil {
		t.Fatal(err)
	}
	draft := core.NewRecord(drafts)
	draft.Set("account", account.Id)
	draft.Set("status", "sending")
	if err := app.Save(draft); err != nil {
		t.Fatal(err)
	}

	const warning = "local persist failed: disk full"
	if err := markDraftSent(app, draft, "<sent@example.com>", "thread-1", warning); err != nil {
		t.Fatal(err)
	}
	saved, err := app.FindRecordById("drafts", draft.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := saved.GetString("status"); got != "sent" {
		t.Fatalf("draft status = %q, want sent", got)
	}
	if got := saved.GetString("last_error"); got != warning {
		t.Fatalf("draft last_error = %q, want %q", got, warning)
	}
	if saved.GetString("sent_at") == "" {
		t.Fatal("draft sent_at must be populated")
	}
}

func TestPersistSentMessageCreatesLocalSentThread(t *testing.T) {
	app := newTestMailApp(t)
	account := newTestAccount(t, app)

	message, err := persistSentMessage(app, account, sendRequest{
		From:     "me@example.com",
		To:       []string{"friend@example.net"},
		Subject:  "Hello",
		BodyText: "Sent body",
	}, "<sent@example.com>")
	if err != nil {
		t.Fatal(err)
	}
	if message.GetFloat("uid") == 0 {
		t.Fatal("local sent message must have a nonzero uid")
	}
	if message.GetString("thread_id") == "" {
		t.Fatal("local sent message must have a thread id")
	}
	folder, err := app.FindRecordById("folders", message.GetString("folder"))
	if err != nil {
		t.Fatal(err)
	}
	if folder.GetString("role") != "sent" {
		t.Fatalf("folder role = %q", folder.GetString("role"))
	}
	if _, err := app.FindRecordById("threads", message.GetString("thread_id")); err != nil {
		t.Fatalf("sent thread was not materialized: %v", err)
	}
}

func newTestMailApp(t *testing.T) *pocketbase.PocketBase {
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

func newTestAccount(t *testing.T, app *pocketbase.PocketBase) *core.Record {
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

func newTestFolder(t *testing.T, app *pocketbase.PocketBase, accountID string) *core.Record {
	t.Helper()
	return newTestFolderWithRole(t, app, accountID, "Inbox", "inbox")
}

func newTestSentFolder(t *testing.T, app *pocketbase.PocketBase, accountID string) *core.Record {
	t.Helper()
	return newTestFolderWithRole(t, app, accountID, "Sent", "sent")
}

func newTestFolderWithRole(t *testing.T, app *pocketbase.PocketBase, accountID, name, role string) *core.Record {
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

func newTestMessage(
	t *testing.T,
	app *pocketbase.PocketBase,
	accountID, folderID string,
	uid int,
	fields map[string]any,
) *core.Record {
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

func newTestThread(t *testing.T, app *pocketbase.PocketBase, id string, fields map[string]any) *core.Record {
	t.Helper()
	threads, err := app.FindCollectionByNameOrId("threads")
	if err != nil {
		t.Fatal(err)
	}
	thread := core.NewRecord(threads)
	thread.Id = id
	for key, value := range fields {
		thread.Set(key, value)
	}
	if err := app.Save(thread); err != nil {
		t.Fatal(err)
	}
	return thread
}
