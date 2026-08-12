// Package mailparse extracts text/HTML bodies and snippets from RFC822 messages.
package mailparse

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"unicode/utf8"
)

const (
	maxBodyStore = 512 * 1024 // cap stored body size
	maxSnippet   = 180
)

// Parsed is the useful content pulled from a raw RFC822 message.
type Parsed struct {
	Text    string
	HTML    string
	Snippet string
}

// ParseRFC822 walks a full message and returns plain text, HTML, and a snippet.
func ParseRFC822(raw []byte) Parsed {
	if len(raw) == 0 {
		return Parsed{}
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		// Not a well-formed message — treat whole buffer as text-ish.
		text := sanitize(string(raw), maxBodyStore)
		return Parsed{Text: text, Snippet: makeSnippet(text)}
	}

	text, html := walkPart(msg.Header.Get("Content-Type"), msg.Header.Get("Content-Transfer-Encoding"), msg.Body)
	text = sanitize(text, maxBodyStore)
	html = sanitize(html, maxBodyStore)
	if text == "" && html != "" {
		text = sanitize(stripTags(html), maxBodyStore)
	}
	snippet := makeSnippet(text)
	if snippet == "" {
		snippet = makeSnippet(stripTags(html))
	}
	return Parsed{Text: text, HTML: html, Snippet: snippet}
}

func walkPart(contentType, transferEncoding string, body io.Reader) (text, html string) {
	if contentType == "" {
		contentType = "text/plain"
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain"
		params = map[string]string{}
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		boundary := params["boundary"]
		if boundary == "" {
			return "", ""
		}
		mr := multipart.NewReader(body, boundary)
		var texts, htmls []string
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			t, h := walkPart(part.Header.Get("Content-Type"), part.Header.Get("Content-Transfer-Encoding"), part)
			if t != "" {
				texts = append(texts, t)
			}
			if h != "" {
				htmls = append(htmls, h)
			}
		}
		return strings.Join(texts, "\n\n"), strings.Join(htmls, "\n")
	}

	data, err := io.ReadAll(io.LimitReader(body, maxBodyStore*2))
	if err != nil && len(data) == 0 {
		return "", ""
	}
	decoded := decodeTransfer(transferEncoding, data)
	decoded = decodeCharset(params["charset"], decoded)
	content := string(decoded)

	switch {
	case strings.EqualFold(mediaType, "text/plain"):
		return content, ""
	case strings.EqualFold(mediaType, "text/html"):
		return "", content
	default:
		if strings.HasPrefix(strings.ToLower(mediaType), "text/") {
			return content, ""
		}
		return "", ""
	}
}

func decodeTransfer(encoding string, data []byte) (out []byte) {
	defer func() {
		if recover() != nil {
			out = data
		}
	}()
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "base64":
		cleaned := make([]byte, 0, len(data))
		for _, b := range data {
			if b != '\r' && b != '\n' && b != ' ' && b != '\t' {
				cleaned = append(cleaned, b)
			}
		}
		decoded, err := base64.StdEncoding.DecodeString(string(cleaned))
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(string(cleaned))
			if err != nil {
				return data
			}
		}
		return decoded
	case "quoted-printable":
		decoded, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(data)))
		if err != nil {
			return data
		}
		return decoded
	default:
		return data
	}
}

func decodeCharset(charset string, data []byte) []byte {
	cs := strings.ToLower(strings.TrimSpace(charset))
	switch cs {
	case "", "utf-8", "utf8", "us-ascii", "ascii", "iso-8859-1", "latin1":
		if utf8.Valid(data) {
			return data
		}
		// Best-effort: interpret as Latin-1.
		runes := make([]rune, len(data))
		for i, b := range data {
			runes[i] = rune(b)
		}
		return []byte(string(runes))
	default:
		if utf8.Valid(data) {
			return data
		}
		return data
	}
}

func sanitize(s string, max int) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.TrimSpace(s)
	if max > 0 && len(s) > max {
		s = s[:max]
	}
	return s
}

func makeSnippet(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxSnippet {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxSnippet]) + "…"
}

func stripTags(html string) string {
	lower := strings.ToLower(html)
	var b strings.Builder
	b.Grow(len(html))
	i := 0
	for i < len(html) {
		if lower[i] == '<' {
			// Drop script/style blocks entirely.
			if strings.HasPrefix(lower[i:], "<script") || strings.HasPrefix(lower[i:], "<style") {
				endTag := "</script>"
				if strings.HasPrefix(lower[i:], "<style") {
					endTag = "</style>"
				}
				if j := strings.Index(lower[i:], endTag); j >= 0 {
					i += j + len(endTag)
					continue
				}
			}
			if j := strings.IndexByte(html[i:], '>'); j >= 0 {
				i += j + 1
				continue
			}
			break
		}
		b.WriteByte(html[i])
		i++
	}
	out := b.String()
	out = strings.ReplaceAll(out, "&nbsp;", " ")
	out = strings.ReplaceAll(out, "&amp;", "&")
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&gt;", ">")
	out = strings.ReplaceAll(out, "&quot;", "\"")
	return out
}
