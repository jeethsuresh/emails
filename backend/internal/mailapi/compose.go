package mailapi

import (
	"fmt"
	"strings"

	"email.local/backend/internal/mailmeta"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type composeReplyRequest struct {
	MessageID         string `json:"messageId"`
	UseSuggestedReply bool   `json:"useSuggestedReply"`
}

func handleComposeReply(re *core.RequestEvent) error {
	var request composeReplyRequest
	if err := re.BindBody(&request); err != nil {
		return re.BadRequestError("invalid json", err)
	}
	request.MessageID = strings.TrimSpace(request.MessageID)
	if request.MessageID == "" {
		return re.BadRequestError("messageId required", nil)
	}
	message, err := re.App.FindRecordById("messages", request.MessageID)
	if err != nil {
		return re.NotFoundError("message not found", err)
	}
	account, err := re.App.FindRecordById("accounts", message.GetString("account"))
	if err != nil {
		return re.BadRequestError("message account not found", err)
	}

	accountEmail := mailmeta.NormalizeEmail(account.GetString("email"))
	from := mailmeta.NormalizeEmail(message.GetString("received_for"))
	if from == "" {
		from = accountEmail
	}
	ownAddress := accountEmail
	if mailmeta.NormalizeEmail(message.GetString("from_addr")) == from {
		ownAddress = from
	}

	bodyText := quoteOriginal(
		message.GetString("date"),
		message.GetString("from_addr"),
		message.GetString("body_text"),
	)
	if request.UseSuggestedReply {
		analysis, analysisErr := re.App.FindFirstRecordByFilter(
			"message_analysis",
			"message = {:message}",
			dbx.Params{"message": message.Id},
		)
		if analysisErr == nil && strings.TrimSpace(analysis.GetString("suggested_reply")) != "" {
			bodyText = analysis.GetString("suggested_reply")
		}
	}

	return re.JSON(200, map[string]any{
		"from":       from,
		"to":         []string{replyCounterpart(message.GetString("from_addr"), message.GetString("to_addrs"), ownAddress)},
		"cc":         []string{},
		"subject":    replySubject(message.GetString("subject")),
		"bodyText":   bodyText,
		"inReplyTo":  strings.TrimSpace(message.GetString("message_id")),
		"references": replyReferences(message.GetString("references"), message.GetString("message_id")),
		"threadId":   message.GetString("thread_id"),
	})
}

func replySubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if len(subject) >= 3 && strings.EqualFold(subject[:3], "re:") {
		return subject
	}
	if subject == "" {
		return "Re:"
	}
	return "Re: " + subject
}

func replyCounterpart(fromAddress, toAddresses, ownAddress string) string {
	from := mailmeta.NormalizeEmail(fromAddress)
	if from != "" && from != mailmeta.NormalizeEmail(ownAddress) {
		return from
	}
	for _, recipient := range mailmeta.ParseAddressList(toAddresses) {
		if recipient != mailmeta.NormalizeEmail(ownAddress) {
			return recipient
		}
	}
	return ""
}

func replyReferences(references, messageID string) string {
	references = strings.TrimSpace(references)
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return references
	}
	normalizedMessageID := mailmeta.NormalizeMessageID(messageID)
	for _, reference := range mailmeta.CollectMessageIDs("", references) {
		if reference == normalizedMessageID {
			return references
		}
	}
	if references == "" {
		return messageID
	}
	return references + " " + messageID
}

func quoteOriginal(date, from, body string) string {
	lines := strings.Split(strings.TrimRight(body, "\r\n"), "\n")
	for i := range lines {
		lines[i] = "> " + strings.TrimSuffix(lines[i], "\r")
	}
	attribution := strings.TrimSpace(from)
	if attribution == "" {
		attribution = "the sender"
	}
	if strings.TrimSpace(date) == "" {
		return fmt.Sprintf("\n\n%s wrote:\n%s", attribution, strings.Join(lines, "\n"))
	}
	return fmt.Sprintf("\n\nOn %s, %s wrote:\n%s", date, attribution, strings.Join(lines, "\n"))
}
