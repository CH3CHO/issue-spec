package model

import (
	"errors"
	"fmt"
	"strings"
)

func AddPRLink(body, url string) (string, bool, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", false, fmt.Errorf("PR URL is empty")
	}
	tc := ParseTypedComment(body)
	if len(tc.Errors) > 0 {
		return "", false, errors.New(strings.Join(tc.Errors, "; "))
	}
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- PR:") {
			continue
		}
		name, value, ok := parseLinkLine(trimmed)
		if !ok {
			return "", false, fmt.Errorf("malformed PR link line")
		}
		values := splitLinkValues(value)
		for _, existing := range values {
			if NormalizeURL(existing) == NormalizeURL(url) {
				return body, false, nil
			}
		}
		values = append(values, url)
		lines[i] = fmt.Sprintf("- %s: %s", name, strings.Join(values, ", "))
		return strings.Join(lines, "\n"), true, nil
	}
	return "", false, fmt.Errorf("typed comment is missing PR link line")
}
