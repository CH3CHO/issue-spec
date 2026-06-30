package templates

import (
	"strings"
	"testing"
)

func TestIssueSpecSkillAndCommandTemplates(t *testing.T) {
	skills := IssueSpecSkills("owner/repo")
	if got, want := len(skills), 6; got != want {
		t.Fatalf("skills = %d, want %d", got, want)
	}
	if !strings.Contains(skills[0].Content, `generatedBy: "issue-spec"`) {
		t.Fatalf("skill missing generatedBy:\n%s", skills[0].Content)
	}

	commands := IssueSpecCommandContents("owner/repo")
	if got, want := len(commands), 5; got != want {
		t.Fatalf("commands = %d, want %d", got, want)
	}
	if commands[0].ID != "propose" {
		t.Fatalf("first command ID = %q, want propose", commands[0].ID)
	}
	if !strings.Contains(commands[0].Body, "issue-spec issue create proposal --repo owner/repo") {
		t.Fatalf("command body missing repo-specific issue-spec usage:\n%s", commands[0].Body)
	}
}
