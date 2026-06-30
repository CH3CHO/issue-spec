package model

import (
	"fmt"
	"strings"
)

func SetTypedCommentStatus(body, status string) (string, error) {
	status = strings.TrimSpace(status)
	if !AllowedStatuses[status] {
		return "", fmt.Errorf("unsupported status %s", status)
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Status:") {
			lines[i] = "Status: " + status
			return strings.Join(lines, "\n"), nil
		}
		if trimmed == "Links:" {
			break
		}
	}
	return "", fmt.Errorf("typed comment is missing Status header")
}

func AppendResolutionLog(body, resolution string) string {
	resolution = strings.TrimSpace(resolution)
	if resolution == "" {
		return body
	}
	entry := "- " + strings.ReplaceAll(resolution, "\n", "\n  ")
	trimmed := strings.TrimRight(body, "\n")
	if strings.Contains(trimmed, "\n## Resolution Log\n") || strings.HasSuffix(trimmed, "\n## Resolution Log") {
		return trimmed + "\n" + entry + "\n"
	}
	return trimmed + "\n\n## Resolution Log\n\n" + entry + "\n"
}
