package commands

import (
	"context"
	"fmt"

	"github.com/higress-group/issue-spec/internal/auth"
	"github.com/higress-group/issue-spec/internal/model"
	"github.com/higress-group/issue-spec/internal/templates"
)

func (a *app) runIssue(ctx context.Context, args []string) int {
	if len(args) < 1 || args[0] != "create" {
		a.errorf("usage: issue-spec issue create proposal|design|implement --repo owner/repo --change name\n")
		return 2
	}
	if len(args) < 2 {
		a.errorf("issue class is required: proposal, design, or implement\n")
		return 2
	}
	return a.runIssueCreate(ctx, args[1], args[2:])
}

func (a *app) runIssueCreate(ctx context.Context, kind string, args []string) int {
	fs := newFlagSet("issue create "+kind, a.err)
	repoFlag := fs.String("repo", "", "repository owner/name")
	host := fs.String("hostname", "github.com", "GitHub hostname")
	change := fs.String("change", "", "change name")
	proposal := fs.String("proposal", "", "proposal issue number or URL")
	design := fs.String("design", "", "design issue number or URL")
	jsonOut := fs.Bool("json", false, "write JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *change == "" {
		a.errorf("--change is required\n")
		return 2
	}
	repo, ok := a.validateRepo(*repoFlag)
	if !ok {
		return 2
	}
	client, _, err := a.clientFor(ctx, *host)
	if err != nil {
		a.errorf("auth required for issue create on %s: %v\n", auth.NormalizeHost(*host), err)
		return 1
	}

	var title, body string
	var labels []string
	switch kind {
	case "proposal":
		title, body, labels = templates.ProposalIssue(*change)
	case "design":
		if *proposal == "" {
			a.errorf("--proposal is required for design issues\n")
			return 2
		}
		proposalIssue, err := parseIssueFlag(*proposal, "proposal")
		if err != nil {
			a.errorf("%v\n", err)
			return 2
		}
		artifacts, err := collectArtifacts(ctx, client, repo, proposalIssue)
		if err != nil {
			a.errorf("read proposal issue comments: %v\n", err)
			return 1
		}
		if countType(artifacts, "SPEC") == 0 {
			a.errorf("design gate blocked: proposal issue %d has no SPEC comments\n", proposalIssue)
			return 1
		}
		if hasBlockedQuestion(artifacts) {
			a.errorf("design gate blocked: proposal issue %d has open blocking QUESTION comments\n", proposalIssue)
			return 1
		}
		title, body, labels = templates.DesignIssue(*change, *proposal)
	case "implement":
		if *design == "" {
			a.errorf("--design is required for implement issues\n")
			return 2
		}
		designIssue, err := parseIssueFlag(*design, "design")
		if err != nil {
			a.errorf("%v\n", err)
			return 2
		}
		artifacts, err := collectArtifacts(ctx, client, repo, designIssue)
		if err != nil {
			a.errorf("read design issue comments: %v\n", err)
			return 1
		}
		if countType(artifacts, "TASK") == 0 {
			a.errorf("implement gate blocked: design issue %d has no TASK comments\n", designIssue)
			return 1
		}
		if hasBlockedQuestion(artifacts) {
			a.errorf("implement gate blocked: design issue %d has open blocking QUESTION comments\n", designIssue)
			return 1
		}
		if *proposal != "" {
			proposalIssue, err := parseIssueFlag(*proposal, "proposal")
			if err != nil {
				a.errorf("%v\n", err)
				return 2
			}
			fullArtifacts, err := collectArtifacts(ctx, client, repo, proposalIssue, designIssue)
			if err != nil {
				a.errorf("read proposal/design issue comments: %v\n", err)
				return 1
			}
			report := model.VerifyTraceability(fullArtifacts)
			if !report.OK {
				a.errorf("implement gate blocked: proposal/design traceability errors:\n")
				for _, msg := range report.Errors {
					a.errorf("- %s\n", msg)
				}
				return 1
			}
		} else {
			for _, artifact := range artifacts {
				if artifact.Comment.Type == "TASK" && len(model.RelatedCommentURLs(artifact.Comment)) == 0 {
					a.errorf("implement gate blocked: %s has no Related Comments links; pass --proposal for full SPEC backlink verification\n", artifact.Comment.ID)
					return 1
				}
			}
		}
		title, body, labels = templates.ImplementIssue(*change, *design)
	default:
		a.errorf("unknown issue class %q\n", kind)
		return 2
	}

	issue, err := client.CreateIssue(ctx, repo, title, body, labels)
	if err != nil {
		a.errorf("create %s issue: %v\n", kind, err)
		return 1
	}
	result := map[string]any{"ok": true, "type": kind, "number": issue.Number, "url": issue.HTMLURL, "title": issue.Title}
	if *jsonOut {
		return a.outputJSON(result)
	}
	fmt.Fprintf(a.out, "created %s issue #%d: %s\n", kind, issue.Number, issue.HTMLURL)
	return 0
}
