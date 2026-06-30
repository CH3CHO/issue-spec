package model

import (
	"errors"
	"fmt"
	"strings"
)

func AddRelatedCommentLink(body, url string) (string, bool, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return "", false, fmt.Errorf("related comment URL is empty")
	}
	tc := ParseTypedComment(body)
	if len(tc.Errors) > 0 {
		return "", false, errors.New(strings.Join(tc.Errors, "; "))
	}
	lines := strings.Split(body, "\n")
	linksIndex := -1
	relatedIndex := -1
	linkBlockEnd := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "Links:" {
			linksIndex = i
			linkBlockEnd = i + 1
			for j := i + 1; j < len(lines); j++ {
				next := strings.TrimSpace(lines[j])
				if strings.HasPrefix(next, "- ") {
					name, _, ok := parseLinkLine(next)
					if ok && name == "Related Comments" {
						relatedIndex = j
					}
					linkBlockEnd = j + 1
					continue
				}
				break
			}
			break
		}
	}
	if linksIndex == -1 {
		return "", false, fmt.Errorf("typed comment is missing Links block")
	}

	if relatedIndex >= 0 {
		name, value, _ := parseLinkLine(strings.TrimSpace(lines[relatedIndex]))
		values := splitLinkValues(value)
		for _, existing := range values {
			if NormalizeURL(existing) == NormalizeURL(url) {
				return body, false, nil
			}
		}
		values = append(values, url)
		lines[relatedIndex] = fmt.Sprintf("- %s: %s", name, strings.Join(values, ", "))
		return strings.Join(lines, "\n"), true, nil
	}

	newLine := "- Related Comments: " + url
	if linkBlockEnd <= linksIndex {
		linkBlockEnd = linksIndex + 1
	}
	lines = append(lines[:linkBlockEnd], append([]string{newLine}, lines[linkBlockEnd:]...)...)
	return strings.Join(lines, "\n"), true, nil
}
