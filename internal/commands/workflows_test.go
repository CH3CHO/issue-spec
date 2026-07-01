package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteWorkflowArtifactsUsesCurrentCodexSkillPath(t *testing.T) {
	root := t.TempDir()
	codexHome := filepath.Join(root, "codex-home")
	t.Setenv("CODEX_HOME", codexHome)

	result, err := writeWorkflowArtifacts(root, "owner/repo", "codex,claude", "both")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(result.SkillFiles), 12; got != want {
		t.Fatalf("skill file count = %d, want %d", got, want)
	}
	if got, want := len(result.CommandFiles), 10; got != want {
		t.Fatalf("command file count = %d, want %d", got, want)
	}
	if got := strings.Join(result.CommandsSkipped, ","); got != "" {
		t.Fatalf("commands skipped = %q, want none", got)
	}

	codexSkill := readTestFile(t, filepath.Join(root, ".agents", "skills", "issue-spec-propose", "SKILL.md"))
	for _, want := range []string{
		"name: issue-spec-propose",
		"compatibility: Requires issue-spec CLI.",
		`generatedBy: "issue-spec"`,
		"issue-spec issue create proposal --repo owner/repo",
	} {
		if !strings.Contains(codexSkill, want) {
			t.Fatalf("codex skill missing %q:\n%s", want, codexSkill)
		}
	}

	workflowSkill := readTestFile(t, filepath.Join(root, ".agents", "skills", "issue-spec-workflow", "SKILL.md"))
	for _, want := range []string{
		"native GitHub CLI support",
		"ISSUE_SPEC_GITHUB_BACKEND=gh",
		`ISSUE_SPEC_TOKEN="$(gh auth token)"`,
	} {
		if !strings.Contains(workflowSkill, want) {
			t.Fatalf("workflow skill missing %q:\n%s", want, workflowSkill)
		}
	}

	claudeCommand := readTestFile(t, filepath.Join(root, ".claude", "commands", "issue-spec", "propose.md"))
	for _, want := range []string{
		`name: "Issue Spec: Propose"`,
		`category: "Workflow"`,
		"Use when the user asks for /issue-spec:propose",
	} {
		if !strings.Contains(claudeCommand, want) {
			t.Fatalf("claude command missing %q:\n%s", want, claudeCommand)
		}
	}

	codexCommand := readTestFile(t, filepath.Join(codexHome, "prompts", "issue-spec-propose.md"))
	for _, want := range []string{
		"argument-hint: command arguments",
		"issue-spec issue create proposal --repo owner/repo",
	} {
		if !strings.Contains(codexCommand, want) {
			t.Fatalf("codex command missing %q:\n%s", want, codexCommand)
		}
	}
}

func TestWriteWorkflowArtifactsCommandsOnly(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex-home"))

	result, err := writeWorkflowArtifacts(root, "owner/repo", "codex", "commands")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.SkillFiles) != 0 {
		t.Fatalf("skills generated in commands-only mode: %v", result.SkillFiles)
	}
	if got, want := len(result.CommandFiles), 5; got != want {
		t.Fatalf("command file count = %d, want %d", got, want)
	}
	if _, err := os.Stat(filepath.Join(root, ".codex", "skills")); !os.IsNotExist(err) {
		t.Fatalf("commands-only mode should not create .codex skills, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills")); !os.IsNotExist(err) {
		t.Fatalf("commands-only mode should not create .agents skills, err=%v", err)
	}
}

func TestResolveWorkflowToolsDetectsExistingToolDirs(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}

	tools, err := resolveWorkflowTools(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].ID != "codex" {
		t.Fatalf("detected tools = %#v, want codex", tools)
	}
}

func TestResolveWorkflowToolsRejectsInvalidTool(t *testing.T) {
	_, err := resolveWorkflowTools(t.TempDir(), "codex,agents")
	if err == nil {
		t.Fatal("expected invalid tool to fail")
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
