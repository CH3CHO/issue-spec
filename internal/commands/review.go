package commands

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/higress-group/issue-spec/internal/auth"
	"github.com/higress-group/issue-spec/internal/github"
	"github.com/higress-group/issue-spec/internal/model"
)

func (a *app) runReview(ctx context.Context, args []string) int {
	if len(args) == 0 {
		a.errorf("usage: issue-spec review sync ...\n")
		return 2
	}
	switch args[0] {
	case "sync":
		return a.runReviewSync(ctx, args[1:])
	default:
		a.errorf("unknown review command %q\n", args[0])
		return 2
	}
}

func (a *app) runReviewSync(ctx context.Context, args []string) int {
	fs := newFlagSet("review sync", a.err)
	repoFlag := fs.String("repo", "", "repository owner/name")
	host := fs.String("hostname", "github.com", "GitHub hostname")
	prFlag := fs.Int("pr", 0, "pull request number")
	implementFlag := fs.String("implement", "", "implement issue number or URL")
	id := fs.String("id", "", "REVIEW id to upsert")
	agent := fs.String("agent", "Coordinator", "logical agent identity")
	scope := fs.String("scope", "pr-review", "review scope")
	jsonOut := fs.Bool("json", false, "write JSON output")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	repo, ok := a.validateRepo(*repoFlag)
	if !ok {
		return 2
	}
	if *prFlag <= 0 {
		a.errorf("--pr must be a positive pull request number\n")
		return 2
	}
	implementIssue, err := parseIssueFlag(*implementFlag, "implement")
	if err != nil {
		a.errorf("%v\n", err)
		return 2
	}
	if strings.TrimSpace(*id) == "" {
		a.errorf("--id is required\n")
		return 2
	}
	client, _, err := a.clientFor(ctx, *host)
	if err != nil {
		a.errorf("auth required for review sync on %s: %v\n", auth.NormalizeHost(*host), err)
		return 1
	}
	pr, err := client.GetPullRequest(ctx, repo, *prFlag)
	if err != nil {
		a.errorf("read PR #%d: %v\n", *prFlag, err)
		return 1
	}
	reviewComments, err := client.ListPullRequestReviewComments(ctx, repo, *prFlag)
	if err != nil {
		a.errorf("read PR #%d review comments: %v\n", *prFlag, err)
		return 1
	}
	issueComments, err := client.ListIssueComments(ctx, repo, *prFlag)
	if err != nil {
		a.errorf("read PR #%d issue comments: %v\n", *prFlag, err)
		return 1
	}
	status, err := client.GetCombinedStatus(ctx, repo, pr.Head.SHA)
	if err != nil {
		a.errorf("read PR #%d status contexts: %v\n", *prFlag, err)
		return 1
	}
	checkRuns, err := client.ListCheckRuns(ctx, repo, pr.Head.SHA)
	if err != nil {
		a.errorf("read PR #%d check runs: %v\n", *prFlag, err)
		return 1
	}
	report := buildReviewSyncReport(pr, reviewComments, issueComments, status, checkRuns)
	body, err := renderReviewSyncComment(*id, *agent, *scope, pr.HTMLURL, report)
	if err != nil {
		a.errorf("render review sync comment: %v\n", err)
		return 1
	}
	action, comment, err := upsertTypedComment(ctx, client, repo, implementIssue, "REVIEW", *id, body)
	if err != nil {
		a.errorf("upsert REVIEW %s: %v\n", *id, err)
		return 1
	}
	result := map[string]any{"ok": report.OK, "action": action, "comment_id": comment.ID, "url": comment.HTMLURL, "review": report}
	if *jsonOut {
		if code := a.outputJSON(result); code != 0 {
			return code
		}
		if !report.OK {
			return 1
		}
		return 0
	}
	fmt.Fprintf(a.out, "%s REVIEW %s: %s\n", action, *id, comment.HTMLURL)
	if !report.OK {
		return 1
	}
	return 0
}

type reviewSyncReport struct {
	OK                 bool            `json:"ok"`
	PR                 int             `json:"pr"`
	PRURL              string          `json:"pr_url"`
	RationaleComments  int             `json:"rationale_comments"`
	ActionableFindings []reviewFinding `json:"actionable_findings"`
	IssueComments      int             `json:"issue_comments"`
	FailedChecks       []reviewCheck   `json:"failed_checks"`
	PendingChecks      []reviewCheck   `json:"pending_checks"`
	PassedChecks       []reviewCheck   `json:"passed_checks"`
}

type reviewFinding struct {
	ID       int64  `json:"id"`
	URL      string `json:"url"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
}

type reviewCheck struct {
	Name       string `json:"name"`
	State      string `json:"state"`
	Conclusion string `json:"conclusion,omitempty"`
	URL        string `json:"url,omitempty"`
}

func buildReviewSyncReport(pr github.PullRequest, reviewComments []github.PullRequestReviewComment, issueComments []github.Comment, status github.CombinedStatus, checkRuns []github.CheckRun) reviewSyncReport {
	report := reviewSyncReport{PR: pr.Number, PRURL: pr.HTMLURL, IssueComments: len(issueComments)}
	for _, comment := range reviewComments {
		if _, ok, err := model.FindRationaleMarker(comment.Body); err == nil && ok {
			report.RationaleComments++
			continue
		}
		report.ActionableFindings = append(report.ActionableFindings, reviewFinding{
			ID:       comment.ID,
			URL:      comment.HTMLURL,
			Path:     comment.Path,
			Line:     comment.Line,
			Severity: findingSeverity(comment.Body),
			Summary:  firstLine(comment.Body),
		})
	}
	for _, s := range status.Statuses {
		check := reviewCheck{Name: s.Context, State: s.State, URL: s.TargetURL}
		if s.State == "success" {
			report.PassedChecks = append(report.PassedChecks, check)
		} else if s.State == "pending" {
			report.PendingChecks = append(report.PendingChecks, check)
		} else {
			report.FailedChecks = append(report.FailedChecks, check)
		}
	}
	for _, run := range checkRuns {
		check := reviewCheck{Name: run.Name, State: run.Status, Conclusion: run.Conclusion, URL: firstNonEmpty(run.DetailsURL, run.HTMLURL)}
		if run.Status != "completed" {
			report.PendingChecks = append(report.PendingChecks, check)
			continue
		}
		switch run.Conclusion {
		case "success", "neutral", "skipped":
			report.PassedChecks = append(report.PassedChecks, check)
		default:
			report.FailedChecks = append(report.FailedChecks, check)
		}
	}
	sort.Slice(report.ActionableFindings, func(i, j int) bool { return report.ActionableFindings[i].ID < report.ActionableFindings[j].ID })
	report.OK = len(report.ActionableFindings) == 0 && len(report.FailedChecks) == 0 && len(report.PendingChecks) == 0
	return report
}

func renderReviewSyncComment(id, agent, scope, prURL string, report reviewSyncReport) (string, error) {
	status := "done"
	if !report.OK {
		status = "blocked"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", model.RenderMarker("REVIEW", id, 1))
	b.WriteString(model.RenderHeader("REVIEW", id, model.BodyOptions{
		Agent:  agent,
		Status: status,
		Scope:  scope,
		Links:  map[string][]string{"Implement Issue": {"N/A"}, "PR": {prURL}},
	}))
	b.WriteString("\n\n## Review Sync Summary\n\n")
	fmt.Fprintf(&b, "- PR: %s\n", prURL)
	fmt.Fprintf(&b, "- Rationale comments: %d\n", report.RationaleComments)
	fmt.Fprintf(&b, "- PR issue comments: %d\n", report.IssueComments)
	fmt.Fprintf(&b, "- Actionable findings: %d\n", len(report.ActionableFindings))
	fmt.Fprintf(&b, "- Failed checks: %d\n", len(report.FailedChecks))
	fmt.Fprintf(&b, "- Pending checks: %d\n", len(report.PendingChecks))
	b.WriteString("\n## Actionable Findings\n\n")
	if len(report.ActionableFindings) == 0 {
		b.WriteString("- None.\n")
	} else {
		for _, finding := range report.ActionableFindings {
			fmt.Fprintf(&b, "- %s %s:%d %s %s\n", finding.Severity, finding.Path, finding.Line, finding.URL, finding.Summary)
		}
	}
	b.WriteString("\n## Checks\n\n")
	writeReviewChecks(&b, "Failed", report.FailedChecks)
	writeReviewChecks(&b, "Pending", report.PendingChecks)
	writeReviewChecks(&b, "Passed", report.PassedChecks)
	b.WriteString("\n## Verdict\n\n")
	if report.OK {
		b.WriteString("Review sync passed.\n")
	} else {
		b.WriteString("Review sync blocked.\n")
	}
	return b.String(), nil
}

func writeReviewChecks(b *strings.Builder, label string, checks []reviewCheck) {
	fmt.Fprintf(b, "### %s\n\n", label)
	if len(checks) == 0 {
		b.WriteString("- None.\n")
		return
	}
	for _, check := range checks {
		fmt.Fprintf(b, "- %s state=%s conclusion=%s %s\n", check.Name, check.State, check.Conclusion, check.URL)
	}
}

func findingSeverity(body string) string {
	body = strings.ToUpper(body)
	for _, severity := range []string{"P0", "P1", "P2"} {
		if strings.Contains(body, severity) {
			return severity
		}
	}
	return "P2"
}

func firstLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			if len(line) > 120 {
				return line[:120] + "..."
			}
			return line
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
