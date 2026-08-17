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
	// Mode: "reply" (default), "reply_all", or "forward".
	Mode string `json:"mode"`
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
	mode := strings.ToLower(strings.TrimSpace(request.Mode))
	if mode == "" {
		mode = "reply"
	}
	switch mode {
	case "reply", "reply_all", "forward":
	default:
		return re.BadRequestError("mode must be reply, reply_all, or forward", nil)
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

	quoted := quoteOriginal(
		message.GetString("date"),
		message.GetString("from_addr"),
		message.GetString("body_text"),
	)

	if mode == "forward" {
		return re.JSON(200, map[string]any{
			"from":       from,
			"to":         []string{},
			"cc":         []string{},
			"subject":    forwardSubject(message.GetString("subject")),
			"bodyText":   quoted,
			"inReplyTo":  "",
			"references": "",
			"threadId":   message.GetString("thread_id"),
		})
	}

	suggested := ""
	if request.UseSuggestedReply && mode == "reply" {
		analysis, analysisErr := re.App.FindFirstRecordByFilter(
			"message_analysis",
			"message = {:message}",
			dbx.Params{"message": message.Id},
		)
		if analysisErr == nil {
			suggested = analysis.GetString("suggested_reply")
		}
	}
	bodyText := mergeSuggestedReply(suggested, quoted)

	to := []string{replyCounterpart(message.GetString("from_addr"), message.GetString("to_addrs"), ownAddress)}
	cc := []string{}
	if mode == "reply_all" {
		to, cc = replyAllRecipients(
			message.GetString("from_addr"),
			message.GetString("to_addrs"),
			message.GetString("cc_addrs"),
			ownAddress,
		)
	}

	return re.JSON(200, map[string]any{
		"from":       from,
		"to":         to,
		"cc":         cc,
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

func forwardSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if len(subject) >= 4 && strings.EqualFold(subject[:4], "fwd:") {
		return subject
	}
	if len(subject) >= 3 && strings.EqualFold(subject[:3], "fw:") {
		return subject
	}
	if subject == "" {
		return "Fwd:"
	}
	return "Fwd: " + subject
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

// replyAllRecipients puts the original sender in To and every other non-self
// address from To/Cc into Cc.
func replyAllRecipients(fromAddress, toAddresses, ccAddresses, ownAddress string) (to, cc []string) {
	own := mailmeta.NormalizeEmail(ownAddress)
	seen := map[string]bool{own: true}
	primary := replyCounterpart(fromAddress, toAddresses, ownAddress)
	if primary != "" {
		to = []string{primary}
		seen[primary] = true
	}
	for _, list := range []string{toAddresses, ccAddresses, fromAddress} {
		for _, recipient := range mailmeta.ParseAddressList(list) {
			email := mailmeta.NormalizeEmail(recipient)
			if email == "" || seen[email] {
				continue
			}
			seen[email] = true
			if len(to) == 0 {
				to = []string{email}
				continue
			}
			cc = append(cc, email)
		}
	}
	if to == nil {
		to = []string{}
	}
	if cc == nil {
		cc = []string{}
	}
	return to, cc
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

func mergeSuggestedReply(suggested, quoted string) string {
	suggested = strings.TrimSpace(suggested)
	if suggested == "" {
		return quoted
	}
	if quoted == "" {
		return suggested
	}
	return suggested + "\n" + quoted
}

func quoteOriginal(date, from, body string) string {
	body = strings.TrimSpace(body)
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	header := fmt.Sprintf("\n\nOn %s, %s wrote:\n", strings.TrimSpace(date), strings.TrimSpace(from))
	return header + strings.Join(lines, "\n")
}
