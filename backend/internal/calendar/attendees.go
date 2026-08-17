package calendar

import (
	"encoding/json"
	"strings"
)

// NormalizeAttendeesEmails cleans optional attendee emails (lowercase, trim, unique).
func NormalizeAttendeesEmails(raw []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		email := strings.ToLower(strings.TrimSpace(item))
		if email == "" || !strings.Contains(email, "@") {
			continue
		}
		if _, ok := seen[email]; ok {
			continue
		}
		seen[email] = struct{}{}
		out = append(out, email)
	}
	return out
}

// EncodeAttendeesJSON stores attendees as a JSON array string (empty → "").
func EncodeAttendeesJSON(emails []string) string {
	emails = NormalizeAttendeesEmails(emails)
	if len(emails) == 0 {
		return ""
	}
	raw, err := json.Marshal(emails)
	if err != nil {
		return ""
	}
	return string(raw)
}

// DecodeAttendeesJSON parses a stored attendees JSON array (or empty).
func DecodeAttendeesJSON(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var emails []string
	if err := json.Unmarshal([]byte(raw), &emails); err != nil {
		// Fallback: comma / newline separated.
		parts := strings.FieldsFunc(raw, func(r rune) bool {
			return r == ',' || r == ';' || r == '\n' || r == '\r'
		})
		return NormalizeAttendeesEmails(parts)
	}
	return NormalizeAttendeesEmails(emails)
}
