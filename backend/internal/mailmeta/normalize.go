package mailmeta

import (
	"net/mail"
	"regexp"
	"strings"
)

var rePrefix = regexp.MustCompile(`(?i)^(re|fwd|fw)\s*:\s*`)

func NormalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(ExtractEmail(s)))
}

func ExtractEmail(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if a, err := mail.ParseAddress(addr); err == nil {
		return strings.ToLower(strings.TrimSpace(a.Address))
	}
	// bare email or angle-bracket scrapes
	if i := strings.LastIndex(addr, "<"); i >= 0 {
		j := strings.LastIndex(addr, ">")
		if j > i {
			return strings.ToLower(strings.TrimSpace(addr[i+1 : j]))
		}
	}
	return strings.ToLower(addr)
}

func ParseAddressList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	list, err := mail.ParseAddressList(s)
	if err != nil {
		// split on commas best-effort
		parts := strings.Split(s, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			if e := NormalizeEmail(p); e != "" {
				out = append(out, e)
			}
		}
		return out
	}
	out := make([]string, 0, len(list))
	for _, a := range list {
		out = append(out, strings.ToLower(strings.TrimSpace(a.Address)))
	}
	return out
}

func DomainOf(email string) string {
	email = NormalizeEmail(email)
	i := strings.LastIndex(email, "@")
	if i < 0 {
		return ""
	}
	return email[i+1:]
}

func NormalizeSubject(subject string) string {
	s := strings.TrimSpace(strings.ToLower(subject))
	for {
		ns := strings.TrimSpace(rePrefix.ReplaceAllString(s, ""))
		if ns == s {
			return ns
		}
		s = ns
	}
}
