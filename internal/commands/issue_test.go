package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/higress-group/issue-spec/internal/model"
)

func TestEnsureIssueBodyMarkerForCreateBodyFile(t *testing.T) {
	body, err := ensureIssueBodyMarker("proposal", "change-name", "# Proposal\n\nReal content.\n")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(body, "<!-- issue-spec:issue=proposal change=change-name version=1 -->\n") {
		t.Fatalf("body missing proposal marker:\n%s", body)
	}
	if !strings.Contains(body, "Real content.") {
		t.Fatalf("body lost content:\n%s", body)
	}
}

func TestEnsureIssueBodyMarkerRejectsWrongIssueClass(t *testing.T) {
	_, err := ensureIssueBodyMarker("proposal", "change-name", "<!-- issue-spec:issue=design change=change-name version=1 -->\n# Proposal\n")
	if err == nil {
		t.Fatal("expected wrong issue marker class to fail")
	}
}

func TestEnsureIssueBodyMarkerIgnoresProseMention(t *testing.T) {
	body, err := ensureIssueBodyMarker("proposal", "change-name", "# Proposal\n\nMention `issue-spec:issue=` in prose.\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(body, "<!-- issue-spec:issue=proposal change=change-name version=1 -->\n") {
		t.Fatalf("body missing prepended marker:\n%s", body)
	}
}

func TestPreserveIssueBodyMarkerForUpdateBodyFile(t *testing.T) {
	existing := "<!-- issue-spec:issue=design change=change-name version=1 -->\n# Design\n\nTBD"
	updated, err := preserveIssueBodyMarker(existing, "# Design\n\nReal design.\n")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(updated, "<!-- issue-spec:issue=design change=change-name version=1 -->\n") {
		t.Fatalf("updated body missing preserved marker:\n%s", updated)
	}
	if strings.Contains(updated, "TBD") {
		t.Fatalf("updated body retained stale placeholder:\n%s", updated)
	}
}

func TestPreserveIssueBodyMarkerRejectsReplacementClassMismatch(t *testing.T) {
	existing := "<!-- issue-spec:issue=design change=change-name version=1 -->\n# Design\n"
	replacement := "<!-- issue-spec:issue=proposal change=change-name version=1 -->\n# Design\n"
	if _, err := preserveIssueBodyMarker(existing, replacement); err == nil {
		t.Fatal("expected replacement marker mismatch to fail")
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

func TestIssueUpdateRejectsSummaryFlagConflict(t *testing.T) {
	var out, errOut bytes.Buffer
	code := Execute([]string{
		"issue", "update",
		"--repo", "o/r",
		"--issue", "1",
		"--title", "new title",
		"--summary", "inline",
		"--summary-file", "summary.md",
	}, strings.NewReader(""), &out, &errOut)

	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stdout=%q stderr=%q", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "--summary and --summary-file cannot both be provided") {
		t.Fatalf("missing conflict error: %s", errOut.String())
	}
}
