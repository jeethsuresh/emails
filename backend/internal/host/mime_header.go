package host

import "strings"

func mimeHeaderGet(raw, name string) string {
	var parts []string
	collecting := false
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			break
		}
		if line[0] == ' ' || line[0] == '\t' {
			if collecting {
				if continuation := strings.TrimSpace(line); continuation != "" {
					parts = append(parts, continuation)
				}
			}
			continue
		}
		if collecting {
			break
		}
		if len(line) >= len(name)+1 && strings.EqualFold(line[:len(name)+1], name+":") {
			parts = append(parts, strings.TrimSpace(line[len(name)+1:]))
			collecting = true
		}
	}
	return strings.Join(parts, " ")
}
