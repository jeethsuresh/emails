package mailapi

import (
	"fmt"
	"strings"
	"time"

	"email.local/backend/internal/mailer"
	"email.local/backend/internal/mailmeta"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type sendRequest struct {
	DraftID    string   `json:"draftId"`
	From       string   `json:"from"`
	To         []string `json:"to"`
	Cc         []string `json:"cc"`
	Subject    string   `json:"subject"`
	BodyText   string   `json:"bodyText"`
	InReplyTo  string   `json:"inReplyTo"`
	References string   `json:"references"`
	ThreadID   string   `json:"threadId"`
}

func handleSend(re *core.RequestEvent) error {
	var request sendRequest
	if err := re.BindBody(&request); err != nil {
		return re.BadRequestError("invalid json", err)
	}
	request.From = mailmeta.NormalizeEmail(request.From)
	if request.From == "" {
		return re.BadRequestError("from required", nil)
	}
	account, err := findSendAccount(re.App, request.From)
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	if err := validateSMTPAccount(account); err != nil {
		return re.BadRequestError(err.Error(), err)
	}

	var draft *core.Record
	if strings.TrimSpace(request.DraftID) != "" {
		draft, err = re.App.FindRecordById("drafts", strings.TrimSpace(request.DraftID))
		if err != nil {
			return re.BadRequestError("draft not found", err)
		}
		if draft.GetString("account") != account.Id {
			return re.BadRequestError("draft belongs to a different account", nil)
		}
	}

	raw, messageID, err := mailer.BuildRFC822(mailer.SendInput{
		From:       request.From,
		To:         request.To,
		Cc:         request.Cc,
		Subject:    request.Subject,
		BodyText:   request.BodyText,
		InReplyTo:  request.InReplyTo,
		References: request.References,
	})
	if err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	if draft != nil {
		draft.Set("status", "sending")
		draft.Set("last_error", "")
		if err := re.App.Save(draft); err != nil {
			return re.InternalServerError("mark draft sending", err)
		}
	}

	recipients := envelopeRecipients(request.To, request.Cc)
	if err := mailer.SendSMTP(account, raw, request.From, recipients); err != nil {
		markDraftFailed(re.App, draft, err)
		return re.InternalServerError("send email", err)
	}

	threadID := strings.TrimSpace(request.ThreadID)
	warning := ""
	sentMessage, persistErr := persistSentMessage(re.App, account, request, messageID)
	if persistErr != nil {
		warning = "local persist failed: " + persistErr.Error()
	} else {
		threadID = sentMessage.GetString("thread_id")
	}
	if draft != nil {
		if err := markDraftSent(re.App, draft, messageID, threadID, warning); err != nil {
			if warning != "" {
				warning += "; "
			}
			warning += "local draft update failed: " + err.Error()
		}
	}

	// IMAP APPEND is intentionally deferred in v1. The local Sent row keeps the
	// offline UI and thread materialization consistent after successful SMTP.
	response := map[string]any{
		"ok":        true,
		"messageId": messageID,
		"threadId":  threadID,
	}
	if warning != "" {
		response["warning"] = warning
	}
	return re.JSON(200, response)
}

func findSendAccount(app core.App, from string) (*core.Record, error) {
	accounts, err := app.FindAllRecords("accounts")
	if err != nil {
		return nil, err
	}
	from = mailmeta.NormalizeEmail(from)
	for _, account := range accounts {
		if mailmeta.NormalizeEmail(account.GetString("email")) == from {
			return account, nil
		}
	}
	aliasMessage, aliasErr := app.FindFirstRecordByFilter(
		"messages",
		"received_for = {:from}",
		dbx.Params{"from": from},
	)
	if aliasErr == nil {
		for _, account := range accounts {
			if account.Id == aliasMessage.GetString("account") {
				return account, nil
			}
		}
	}
	return nil, fmt.Errorf("no account configured for from address")
}

func validateSMTPAccount(account *core.Record) error {
	if account == nil ||
		strings.TrimSpace(account.GetString("smtp_host")) == "" ||
		account.GetInt("smtp_port") <= 0 ||
		strings.TrimSpace(account.GetString("username")) == "" {
		return fmt.Errorf("account SMTP settings are incomplete")
	}
	return nil
}

func envelopeRecipients(to, cc []string) []string {
	all := append(append([]string{}, to...), cc...)
	recipients := make([]string, 0, len(all))
	seen := make(map[string]struct{}, len(all))
	for _, value := range all {
		email := mailmeta.NormalizeEmail(value)
		if email == "" {
			continue
		}
		if _, exists := seen[email]; exists {
			continue
		}
		seen[email] = struct{}{}
		recipients = append(recipients, email)
	}
	return recipients
}

func markDraftFailed(app core.App, draft *core.Record, failure error) {
	if draft == nil {
		return
	}
	draft.Set("status", "failed")
	draft.Set("last_error", failure.Error())
	_ = app.Save(draft)
}

func markDraftSent(app core.App, draft *core.Record, messageID, threadID, warning string) error {
	if draft == nil {
		return nil
	}
	draft.Set("status", "sent")
	draft.Set("last_error", warning)
	draft.Set("sent_at", time.Now().UTC().Format(time.RFC3339))
	draft.Set("message_id", messageID)
	draft.Set("thread_id", threadID)
	return app.Save(draft)
}

func persistSentMessage(app core.App, account *core.Record, request sendRequest, messageID string) (*core.Record, error) {
	folder, err := findOrCreateSentFolder(app, account.Id)
	if err != nil {
		return nil, err
	}
	message, err := app.FindFirstRecordByFilter(
		"messages",
		"account = {:account} && message_id = {:message_id}",
		dbx.Params{"account": account.Id, "message_id": messageID},
	)
	if err != nil {
		collection, collectionErr := app.FindCollectionByNameOrId("messages")
		if collectionErr != nil {
			return nil, collectionErr
		}
		message = core.NewRecord(collection)
		message.Set("account", account.Id)
		// Locally persisted SMTP messages have no server UID until a later IMAP
		// sync. Use a nonzero negative value because the schema requires uid.
		message.Set("uid", -float64(time.Now().UnixMicro()))
	}

	now := time.Now().UTC().Format(time.RFC3339)
	recipients := envelopeRecipients(request.To, request.Cc)
	message.Set("folder", folder.Id)
	message.Set("message_id", messageID)
	message.Set("subject", request.Subject)
	message.Set("from_addr", request.From)
	message.Set("to_addrs", strings.Join(recipients, ", "))
	message.Set("date", now)
	message.Set("snippet", sentSnippet(request.BodyText))
	message.Set("body_text", request.BodyText)
	message.Set("body_html", "")
	message.Set("seen", true)
	message.Set("flagged", false)
	message.Set("search_tokens", strings.ToLower(strings.Join([]string{
		request.Subject,
		request.From,
		strings.Join(recipients, " "),
		request.BodyText,
	}, " ")))
	message.Set("content_hash", messageID)
	message.Set("in_reply_to", strings.TrimSpace(request.InReplyTo))
	message.Set("references", strings.TrimSpace(request.References))
	message.Set("normalized_subject", mailmeta.NormalizeSubject(request.Subject))
	message.Set("received_for", request.From)

	threadID := strings.TrimSpace(request.ThreadID)
	if threadID == "" {
		threadID = mailmeta.ResolveThreadID(
			messageID,
			request.InReplyTo,
			request.References,
			request.Subject,
			request.From,
			strings.Join(recipients, ", "),
			mailmeta.PBLookup{App: app},
			time.Now(),
		)
	}
	message.Set("thread_id", threadID)
	if err := app.Save(message); err != nil {
		return nil, err
	}
	if err := mailmeta.UpsertThreadFromMessage(app, message); err != nil {
		return nil, err
	}
	return message, nil
}

func findOrCreateSentFolder(app core.App, accountID string) (*core.Record, error) {
	folder, err := app.FindFirstRecordByFilter(
		"folders",
		"account = {:account} && role = 'sent'",
		dbx.Params{"account": accountID},
	)
	if err == nil {
		return folder, nil
	}
	collection, err := app.FindCollectionByNameOrId("folders")
	if err != nil {
		return nil, err
	}
	folder = core.NewRecord(collection)
	folder.Set("account", accountID)
	folder.Set("name", "Sent")
	folder.Set("role", "sent")
	if err := app.Save(folder); err != nil {
		return nil, err
	}
	return folder, nil
}

func sentSnippet(body string) string {
	body = strings.Join(strings.Fields(body), " ")
	runes := []rune(body)
	if len(runes) > 240 {
		runes = runes[:240]
	}
	return string(runes)
}
