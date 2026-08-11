package analyzer

import "strings"

// FolderIsExcludedFromAnalysis reports whether a folder should never be
// enqueued for LLM analysis (trash/deleted/spam/junk). Duplicated from the
// syncer's small string checks to avoid an analyzer <-> syncer import cycle.
func FolderIsExcludedFromAnalysis(name, role string) bool {
	r := strings.ToLower(strings.TrimSpace(role))
	switch r {
	case "trash", "spam", "junk":
		return true
	}

	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(n, "trash"),
		strings.Contains(n, "deleted"),
		strings.Contains(n, "spam"),
		strings.Contains(n, "junk"):
		return true
	default:
		return false
	}
}
