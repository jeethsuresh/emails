package mailer

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"time"

	"email.local/backend/internal/mailmeta"
)

type SendInput struct {
	From       string
	To         []string
	Cc         []string
	Subject    string
	BodyText   string
	InReplyTo  string
	References string
	MessageID  string
}

func BuildRFC822(in SendInput) ([]byte, string, error) {
	from, err := parseAddress(in.From)
	if err != nil {
		return nil, "", fmt.Errorf("from: %w", err)
	}
	to, err := parseAddressList(in.To)
	if err != nil {
		return nil, "", fmt.Errorf("to: %w", err)
	}
	cc, err := parseAddressList(in.Cc)
	if err != nil {
		return nil, "", fmt.Errorf("cc: %w", err)
	}
	if len(to)+len(cc) == 0 {
		return nil, "", fmt.Errorf("at least one recipient is required")
	}

	messageID := strings.TrimSpace(in.MessageID)
	if messageID == "" {
		domain := mailmeta.DomainOf(from.Address)
		if domain == "" {
			return nil, "", fmt.Errorf("from address has no domain")
		}
		id, err := newUUID()
		if err != nil {
			return nil, "", fmt.Errorf("generate message-id: %w", err)
		}
		messageID = "<" + id + "@" + domain + ">"
	}

	headers := [][2]string{
		{"Date", time.Now().Format(time.RFC1123Z)},
		{"Message-ID", messageID},
		{"From", formatAddress(from)},
		{"To", joinAddresses(to)},
	}
	if len(cc) > 0 {
		headers = append(headers, [2]string{"Cc", joinAddresses(cc)})
	}
	headers = append(headers, [2]string{"Subject", mime.QEncoding.Encode("UTF-8", in.Subject)})
	if strings.TrimSpace(in.InReplyTo) != "" {
		headers = append(headers, [2]string{"In-Reply-To", strings.TrimSpace(in.InReplyTo)})
	}
	if strings.TrimSpace(in.References) != "" {
		headers = append(headers, [2]string{"References", strings.TrimSpace(in.References)})
	}
	headers = append(headers,
		[2]string{"MIME-Version", "1.0"},
		[2]string{"Content-Type", `text/plain; charset="UTF-8"`},
		[2]string{"Content-Transfer-Encoding", "quoted-printable"},
	)

	var raw bytes.Buffer
	for _, header := range headers {
		if err := writeHeader(&raw, header[0], header[1]); err != nil {
			return nil, "", err
		}
	}
	raw.WriteString("\r\n")
	body := quotedprintable.NewWriter(&raw)
	if _, err := body.Write([]byte(in.BodyText)); err != nil {
		return nil, "", fmt.Errorf("encode body: %w", err)
	}
	if err := body.Close(); err != nil {
		return nil, "", fmt.Errorf("encode body: %w", err)
	}

	return raw.Bytes(), messageID, nil
}

func parseAddress(value string) (*mail.Address, error) {
	if containsNewline(value) {
		return nil, fmt.Errorf("address contains a newline")
	}
	address, err := mail.ParseAddress(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	return address, nil
}

func parseAddressList(values []string) ([]*mail.Address, error) {
	addresses := make([]*mail.Address, 0, len(values))
	for _, value := range values {
		address, err := parseAddress(value)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, address)
	}
	return addresses, nil
}

func joinAddresses(addresses []*mail.Address) string {
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		values = append(values, formatAddress(address))
	}
	return strings.Join(values, ", ")
}

func formatAddress(address *mail.Address) string {
	if address.Name == "" {
		return address.Address
	}
	return address.String()
}

func writeHeader(raw *bytes.Buffer, name, value string) error {
	if containsNewline(value) {
		return fmt.Errorf("%s header contains a newline", strings.ToLower(name))
	}
	raw.WriteString(foldHeader(name, value))
	raw.WriteString("\r\n")
	return nil
}

// foldHeader wraps a header at 78 octets with RFC 5322 folding whitespace so
// Q-encoded subjects (and other long values) stay within the 998-octet limit.
func foldHeader(name, value string) string {
	const limit = 78
	prefix := name + ": "
	if len(prefix)+len(value) <= limit {
		return prefix + value
	}
	var b strings.Builder
	b.WriteString(prefix)
	col := len(prefix)
	remaining := value
	for remaining != "" {
		space := limit - col
		if space < 1 {
			b.WriteString("\r\n ")
			col = 1
			space = limit - 1
		}
		if len(remaining) <= space {
			b.WriteString(remaining)
			break
		}
		chunk := remaining[:space]
		if i := strings.LastIndex(chunk, "?="); i >= 0 && i+2 <= len(remaining) {
			chunk = remaining[:i+2]
		}
		if chunk == "" {
			chunk = remaining[:1]
		}
		b.WriteString(chunk)
		remaining = remaining[len(chunk):]
		if remaining != "" {
			b.WriteString("\r\n ")
			col = 1
		}
	}
	return b.String()
}

func containsNewline(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

func newUUID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		id[0:4], id[4:6], id[6:8], id[8:10], id[10:16]), nil
}
