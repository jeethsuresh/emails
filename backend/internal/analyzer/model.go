package analyzer

import "strings"

type ModelInfo struct {
	ID string
}

func IsChatModel(id string) bool {
	lower := strings.ToLower(id)
	if strings.Contains(lower, "embed") {
		return false
	}
	if strings.Contains(id, "text-embedding") {
		return false
	}
	return true
}

func ResolveModel(preferred string, available []ModelInfo) (string, bool) {
	if len(available) == 0 {
		return "", false
	}

	for _, m := range available {
		if m.ID == preferred && IsChatModel(m.ID) {
			return m.ID, true
		}
	}

	var gemmaE4B string
	var gemmaOther string
	for _, m := range available {
		if !strings.HasPrefix(m.ID, "google/gemma-4") || !IsChatModel(m.ID) {
			continue
		}
		if strings.Contains(m.ID, "e4b") {
			gemmaE4B = m.ID
		} else if gemmaOther == "" {
			gemmaOther = m.ID
		}
	}
	if gemmaE4B != "" {
		return gemmaE4B, true
	}
	if gemmaOther != "" {
		return gemmaOther, true
	}

	for _, m := range available {
		if IsChatModel(m.ID) {
			return m.ID, true
		}
	}

	return "", false
}
