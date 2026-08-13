package mailmeta

import (
	"net/mail"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

func UpsertContactFromMessage(app core.App, msg *core.Record) error {
	email := NormalizeEmail(msg.GetString("from_addr"))
	if email == "" {
		return nil
	}
	if accountID := msg.GetString("account"); accountID != "" {
		if account, err := app.FindRecordById("accounts", accountID); err == nil &&
			NormalizeEmail(account.GetString("email")) == email {
			return nil
		}
	}

	collection, err := app.FindCollectionByNameOrId("contacts")
	if err != nil {
		return err
	}
	contact, err := app.FindFirstRecordByFilter(collection.Id, "email = {:email}", dbx.Params{"email": email})
	if err != nil {
		contact = core.NewRecord(collection)
		contact.Set("email", email)
		contact.Set("message_count", 1)
	} else {
		contact.Set("message_count", contact.GetFloat("message_count")+1)
	}
	if displayName := displayNameFromAddress(msg.GetString("from_addr")); displayName != "" {
		contact.Set("display_name", displayName)
	}
	contact.Set("last_message_at", msg.GetString("date"))
	contact.Set("updated_at", time.Now().UTC().Format(time.RFC3339))
	return app.Save(contact)
}

func displayNameFromAddress(value string) string {
	address, err := mail.ParseAddress(value)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(address.Name)
}
