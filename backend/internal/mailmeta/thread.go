package mailmeta

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"
)

type ThreadLookup interface {
	ByMessageID(id string) (threadID string, ok bool)
	BySubjectParticipants(normSubject string, participants []string, sinceRFC3339 string) (threadID string, ok bool)
}

func NormalizeMessageID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	id = strings.TrimPrefix(id, "<")
	id = strings.TrimSuffix(id, ">")
	return id
}

func CollectMessageIDs(inReplyTo, references string) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(raw string) {
		for _, tok := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == ' ' || r == ',' || r == '\t' || r == '\n' || r == '\r'
		}) {
			n := NormalizeMessageID(tok)
			if n == "" {
				continue
			}
			if _, ok := seen[n]; ok {
				continue
			}
			seen[n] = struct{}{}
			out = append(out, n)
		}
	}
	add(inReplyTo)
	add(references)
	return out
}

func NewThreadID(messageID string) string {
	n := NormalizeMessageID(messageID)
	if n == "" {
		return fmt.Sprintf("t%014x", uint64(time.Now().UnixNano())&0x3fffffffffffff)
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte("thread|" + n))
	return fmt.Sprintf("%015x", h.Sum64()&0x0fffffffffffffff)
}

func ResolveThreadID(messageID, inReplyTo, references, subject, from string, toAddrs string, lookup ThreadLookup, now time.Time) string {
	for _, mid := range CollectMessageIDs(inReplyTo, references) {
		if tid, ok := lookup.ByMessageID(mid); ok && tid != "" {
			return tid
		}
	}
	subj := NormalizeSubject(subject)
	parts := append([]string{NormalizeEmail(from)}, ParseAddressList(toAddrs)...)
	since := now.UTC().Add(-90 * 24 * time.Hour).Format(time.RFC3339)
	if tid, ok := lookup.BySubjectParticipants(subj, parts, since); ok && tid != "" {
		return tid
	}
	return NewThreadID(messageID)
}
