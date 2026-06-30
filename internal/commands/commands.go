package commands

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/higress-group/issue-spec/internal/auth"
	"github.com/higress-group/issue-spec/internal/github"
	"github.com/higress-group/issue-spec/internal/model"
)

type app struct {
	in  io.Reader
	out io.Writer
	err io.Writer
}

type commandFunc func(context.Context, []string) int

func Execute(args []string, in io.Reader, out io.Writer, errOut io.Writer) int {
	a := &app{in: in, out: out, err: errOut}
	ctx := context.Background()
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		a.printUsage()
		return 0
	}
	switch args[0] {
	case "auth":
		return a.runAuth(ctx, args[1:])
	case "init":
		return a.runInit(ctx, args[1:])
	case "issue":
		return a.runIssue(ctx, args[1:])
	case "comment":
		return a.runComment(ctx, args[1:])
	case "question":
		return a.runQuestion(ctx, args[1:])
	case "pr":
		return a.runPR(ctx, args[1:])
	case "archive":
		return a.runArchive(ctx, args[1:])
	case "link":
		return a.runLink(ctx, args[1:])
	case "status":
		return a.runStatus(ctx, args[1:])
	case "verify":
		return a.runVerify(ctx, args[1:])
	case "verify-links":
		return a.runVerifyLinks(ctx, args[1:])
	default:
		a.errorf("unknown command %q\n", args[0])
		a.printUsage()
		return 2
	}
}

func (a *app) printUsage() {
	fmt.Fprintln(a.out, `issue-spec manages issue-native OpenSpec artifacts.

Usage:
  issue-spec auth status|login|logout|token
  issue-spec init --repo owner/repo [--create-labels]
  issue-spec issue create proposal|design|implement --repo owner/repo --change name
  issue-spec comment upsert --repo owner/repo --issue N --type SPEC --id SPEC-001 --body-file file.md
  issue-spec comment list --repo owner/repo --issue N [--type SPEC] [--json]
  issue-spec question create --repo owner/repo --issue N --id QUESTION-001 --question "..."
  issue-spec question resolve --repo owner/repo --issue N --id QUESTION-001 --resolution-file file.md
  issue-spec pr rationale --repo owner/repo --pr N --path file.go --line 42 --process PROCESS-001 --spec SPEC-001 --spec-url URL --body "why"
  issue-spec archive durable-spec --repo owner/repo --proposal N --capability my-capability
  issue-spec link --repo owner/repo --from SPEC-001 --from-issue N --to TASK-001 --to-issue M
  issue-spec status --repo owner/repo --proposal N [--design N] [--implement N]
  issue-spec verify --repo owner/repo --proposal N --design N --implement N [--durable-spec path]
  issue-spec verify-links --repo owner/repo --proposal N --design N --implement N`)
}

func newFlagSet(name string, errOut io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(errOut)
	return fs
}

func (a *app) clientFor(ctx context.Context, host string) (*github.Client, auth.Token, error) {
	host = auth.NormalizeHost(host)
	token, err := auth.ResolveToken(ctx, host)
	if err != nil {
		return nil, auth.Token{Host: host}, err
	}
	return github.NewClient(host, token.Value), token, nil
}

func (a *app) validateRepo(repo string) (string, bool) {
	parsed, err := github.ParseRepo(repo)
	if err != nil {
		a.errorf("%v\n", err)
		return "", false
	}
	return parsed, true
}

func (a *app) readBodyFile(path string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		a.errorf("--body-file is required\n")
		return "", false
	}
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(a.in)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		a.errorf("read body file %s: %v\n", path, err)
		return "", false
	}
	return string(data), true
}

func (a *app) outputJSON(value any) int {
	enc := json.NewEncoder(a.out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		a.errorf("write JSON: %v\n", err)
		return 1
	}
	return 0
}

func (a *app) errorf(format string, args ...any) {
	fmt.Fprintf(a.err, format, args...)
}

func issueNumberFlag(value string) (int, error) {
	return github.ParseIssueNumber(value)
}

func parseIssueFlag(value string, name string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, fmt.Errorf("--%s is required", name)
	}
	return issueNumberFlag(value)
}

func collectArtifacts(ctx context.Context, client *github.Client, repo string, issueNumbers ...int) ([]model.Artifact, error) {
	var artifacts []model.Artifact
	for _, issueNumber := range issueNumbers {
		if issueNumber == 0 {
			continue
		}
		comments, err := client.ListIssueComments(ctx, repo, issueNumber)
		if err != nil {
			return nil, err
		}
		for _, comment := range comments {
			if !model.IsLikelyTyped(comment.Body) {
				continue
			}
			tc := model.ParseTypedComment(comment.Body)
			artifacts = append(artifacts, model.Artifact{
				Issue:     issueNumber,
				CommentID: comment.ID,
				URL:       comment.HTMLURL,
				APIURL:    comment.URL,
				Comment:   tc,
			})
		}
	}
	return artifacts, nil
}

func findArtifactByID(ctx context.Context, client *github.Client, repo string, issueNumber int, id string) (model.Artifact, string, error) {
	comments, err := client.ListIssueComments(ctx, repo, issueNumber)
	if err != nil {
		return model.Artifact{}, "", err
	}
	for _, comment := range comments {
		tc := model.ParseTypedComment(comment.Body)
		if tc.ID == id {
			return model.Artifact{
				Issue:     issueNumber,
				CommentID: comment.ID,
				URL:       comment.HTMLURL,
				APIURL:    comment.URL,
				Comment:   tc,
			}, comment.Body, nil
		}
	}
	return model.Artifact{}, "", fmt.Errorf("typed comment %s not found on issue %d", id, issueNumber)
}

func upsertTypedComment(ctx context.Context, client *github.Client, repo string, issueNumber int, commentType, id, body string) (string, github.Comment, error) {
	comments, err := client.ListIssueComments(ctx, repo, issueNumber)
	if err != nil {
		return "", github.Comment{}, err
	}
	for _, comment := range comments {
		tc := model.ParseTypedComment(comment.Body)
		if tc.Type == strings.ToUpper(commentType) && tc.ID == id {
			updated, err := client.UpdateComment(ctx, repo, comment.ID, body)
			return "updated", updated, err
		}
	}
	created, err := client.CreateComment(ctx, repo, issueNumber, body)
	return "created", created, err
}

func hasBlockedQuestion(artifacts []model.Artifact) bool {
	for _, artifact := range artifacts {
		tc := artifact.Comment
		if tc.Type == "QUESTION" && tc.Status == "blocked" {
			return true
		}
	}
	return false
}

func countType(artifacts []model.Artifact, commentType string) int {
	count := 0
	for _, artifact := range artifacts {
		if artifact.Comment.Type == commentType {
			count++
		}
	}
	return count
}

func parseIntFlag(value string, name string) (int, error) {
	if strings.TrimSpace(value) == "" {
		return 0, fmt.Errorf("--%s is required", name)
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("--%s must be a positive integer", name)
	}
	return n, nil
}

func isNoToken(err error) bool {
	return errors.Is(err, auth.ErrNoToken)
}
