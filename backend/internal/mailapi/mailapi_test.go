package mailapi

import (
	"reflect"
	"testing"

	"email.local/backend/internal/mailstore"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

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

func TestPersistSentMessageCreatesLocalSentThread(t *testing.T) {
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
