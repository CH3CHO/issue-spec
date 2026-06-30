package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/higress-group/issue-spec/internal/auth"
	"github.com/higress-group/issue-spec/internal/templates"
)

func (a *app) runArchive(ctx context.Context, args []string) int {
	if len(args) < 1 {
		a.errorf("usage: issue-spec archive durable-spec ...\n")
		return 2
	}
	switch args[0] {
	case "durable-spec":
		return a.runArchiveDurableSpec(ctx, args[1:])
	default:
		a.errorf("unknown archive command %q\n", args[0])
		return 2
	}
}

func (a *app) runArchiveDurableSpec(ctx context.Context, args []string) int {
	fs := newFlagSet("archive durable-spec", a.err)
	repoFlag := fs.String("repo", "", "repository owner/name")
	host := fs.String("hostname", "github.com", "GitHub hostname")
	proposalFlag := fs.String("proposal", "", "proposal issue number or URL")
	capability := fs.String("capability", "", "durable spec capability name")
	output := fs.String("output", "", "output spec path")
	purpose := fs.String("purpose", "", "durable spec purpose text")
	jsonOut := fs.Bool("json", false, "write JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	repo, ok := a.validateRepo(*repoFlag)
	if !ok {
		return 2
	}
	if strings.TrimSpace(*capability) == "" {
		a.errorf("--capability is required\n")
		return 2
	}
	proposalIssue, err := parseIssueFlag(*proposalFlag, "proposal")
	if err != nil {
		a.errorf("%v\n", err)
		return 2
	}
	client, _, err := a.clientFor(ctx, *host)
	if err != nil {
		a.errorf("auth required for archive durable-spec on %s: %v\n", auth.NormalizeHost(*host), err)
		return 1
	}
	issue, err := client.GetIssue(ctx, repo, proposalIssue)
	if err != nil {
		a.errorf("read proposal issue #%d: %v\n", proposalIssue, err)
		return 1
	}
	artifacts, err := collectArtifacts(ctx, client, repo, proposalIssue)
	if err != nil {
		a.errorf("read proposal comments: %v\n", err)
		return 1
	}
	var specs []templates.SpecSource
	for _, artifact := range artifacts {
		tc := artifact.Comment
		if tc.Type != "SPEC" || tc.Status == "superseded" {
			continue
		}
		specs = append(specs, templates.SpecSource{
			ID:   tc.ID,
			URL:  artifact.URL,
			Body: tc.Body,
		})
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	if len(specs) == 0 {
		a.errorf("proposal issue #%d has no active SPEC comments\n", proposalIssue)
		return 1
	}
	outputPath := strings.TrimSpace(*output)
	if outputPath == "" {
		outputPath = filepath.Join("openspec", "specs", *capability, "spec.md")
	}
	existing, err := os.ReadFile(outputPath)
	if err != nil && !os.IsNotExist(err) {
		a.errorf("read existing durable spec %s: %v\n", outputPath, err)
		return 1
	}
	rendered, err := templates.DurableSpec(templates.DurableSpecOptions{
		Capability:        *capability,
		Purpose:           *purpose,
		ProposalIssueURL:  issue.HTMLURL,
		ExistingSpecBody:  string(existing),
		SpecificationList: specs,
	})
	if err != nil {
		a.errorf("render durable spec: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		a.errorf("create durable spec directory: %v\n", err)
		return 1
	}
	if err := os.WriteFile(outputPath, []byte(rendered), 0o644); err != nil {
		a.errorf("write durable spec %s: %v\n", outputPath, err)
		return 1
	}
	result := map[string]any{"ok": true, "capability": *capability, "output": outputPath, "proposal": issue.HTMLURL, "spec_count": len(specs)}
	if *jsonOut {
		return a.outputJSON(result)
	}
	fmt.Fprintf(a.out, "wrote durable spec draft for %s to %s\n", *capability, outputPath)
	return 0
}
