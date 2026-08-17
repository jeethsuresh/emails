//go:build !js

package host

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
)

type Status struct {
	State          string  `json:"state"`
	Message        string  `json:"message"`
	LastSyncAt     *string `json:"lastSyncAt"`
	FoldersSynced  int     `json:"foldersSynced"`
	MessagesSynced int     `json:"messagesSynced"`
}

func DataDir() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".emails", "pb_data")
	}
	return "./pb_data"
}
func AttachmentsDir() string { return "./attachments" }
func ReportStatus(Status)    {}

func MimeHeaderGet(raw, name string) string {
	return mimeHeaderGet(raw, name)
}

func Hash(input string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(input))
	return fmt.Sprintf("%08x", h.Sum32())
}

func Tokenize(text string) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '@' || r == '.' || r == '_' || r == '+' || r == '-')
	})
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, f := range fields {
		if len(f) < 2 {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
		if len(out) >= 400 {
			break
		}
	}
	return out
}

func NormalizeContact(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func LoadDBSnapshot(string) []byte  { return nil }
func SaveDBSnapshot(string, []byte) {}
