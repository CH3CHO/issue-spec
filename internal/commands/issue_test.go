package commands

import (
	"strings"
	"testing"

	"github.com/higress-group/issue-spec/internal/model"
)

func TestEnsureIssueBodyMarkerForCreateBodyFile(t *testing.T) {
	body := ensureIssueBodyMarker("proposal", "change-name", "# Proposal\n\nReal content.\n")

	if !strings.HasPrefix(body, "<!-- issue-spec:issue=proposal change=change-name version=1 -->\n") {
		t.Fatalf("body missing proposal marker:\n%s", body)
	}
	if !strings.Contains(body, "Real content.") {
		t.Fatalf("body lost content:\n%s", body)
	}
}

func TestPreserveIssueBodyMarkerForUpdateBodyFile(t *testing.T) {
	existing := "<!-- issue-spec:issue=design change=change-name version=1 -->\n# Design\n\nTBD"
	updated := preserveIssueBodyMarker(existing, "# Design\n\nReal design.\n")

	if !strings.HasPrefix(updated, "<!-- issue-spec:issue=design change=change-name version=1 -->\n") {
		t.Fatalf("updated body missing preserved marker:\n%s", updated)
	}
	if strings.Contains(updated, "TBD") {
		t.Fatalf("updated body retained stale placeholder:\n%s", updated)
	}
}

func TestIssueUpdateSummaryIsNotTypedComment(t *testing.T) {
	body := renderIssueUpdateSummary(5, "https://github.com/o/r/issues/5", "Replaced placeholder body with a concrete proposal.")

	if model.IsLikelyTyped(body) {
		t.Fatalf("issue update summary should not be parsed as a typed comment:\n%s", body)
	}
	if !strings.Contains(body, "Replaced placeholder body") {
		t.Fatalf("summary body missing content:\n%s", body)
	}
}
