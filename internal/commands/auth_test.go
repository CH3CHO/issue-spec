package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/higress-group/issue-spec/internal/auth"
	"github.com/higress-group/issue-spec/internal/github"
	"github.com/higress-group/issue-spec/internal/model"
)

func TestAuthStatusJSONIncludesBackendDiagnosticsWithoutToken(t *testing.T) {
	const secret = "secret-token-value"
	var out, errOut bytes.Buffer
	app := newApp(strings.NewReader(""), &out, &errOut)
	app.selectGitHubBackend = func(context.Context, string) (auth.GitHubBackendSelection, error) {
		return auth.GitHubBackendSelection{
			Mode:            auth.GitHubBackendModeAuto,
			Name:            auth.GitHubBackendNameREST,
			Kind:            auth.GitHubBackendKindREST,
			Host:            "github.com",
			SelectionSource: "auto:token",
			TokenSource:     "env:ISSUE_SPEC_TOKEN",
			Token:           auth.Token{Value: secret, Source: "env:ISSUE_SPEC_TOKEN", Host: "github.com"},
		}, nil
	}
	app.newGitHubBackend = func(_ context.Context, selection auth.GitHubBackendSelection) (github.Backend, error) {
		return fakeGitHubBackend{
			info:   github.BackendInfo{Name: selection.Name, Kind: selection.Kind, Host: selection.Host},
			user:   github.User{Login: "octocat"},
			scopes: []string{"repo"},
		}, nil
	}

	code := app.runAuthStatus(context.Background(), []string{"--json"})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	if strings.Contains(out.String(), secret) || strings.Contains(errOut.String(), secret) {
		t.Fatalf("token leaked in output: stdout=%q stderr=%q", out.String(), errOut.String())
	}
	var got struct {
		OK      bool                          `json:"ok"`
		Auth    auth.Token                    `json:"auth"`
		Backend auth.GitHubBackendDiagnostics `json:"backend"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK {
		t.Fatalf("ok = false in %s", out.String())
	}
	if got.Auth.Source != "env:ISSUE_SPEC_TOKEN" || got.Auth.User != "octocat" || got.Auth.Host != "github.com" {
		t.Fatalf("unexpected auth metadata: %+v", got.Auth)
	}
	if got.Backend.Name != "rest" || got.Backend.SelectionSource != "auto:token" || got.Backend.TokenSource != "env:ISSUE_SPEC_TOKEN" {
		t.Fatalf("unexpected backend diagnostics: %+v", got.Backend)
	}
}

func TestAuthTokenJSONForGHDoesNotFetchTokenUnlessIncluded(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(strings.NewReader(""), &out, &errOut)
	app.selectGitHubBackend = func(context.Context, string) (auth.GitHubBackendSelection, error) {
		return auth.GitHubBackendSelection{
			Mode:            auth.GitHubBackendModeGH,
			Name:            auth.GitHubBackendNameGH,
			Kind:            auth.GitHubBackendKindCLI,
			Host:            "github.com",
			SelectionSource: "override:gh",
			Token:           auth.Token{Source: "gh", Host: "github.com"},
		}, nil
	}
	app.gitHubBackendToken = func(context.Context, auth.GitHubBackendSelection) (string, error) {
		t.Fatal("gh token provider should not run without --include-token")
		return "", nil
	}

	code := app.runAuthToken(context.Background(), []string{"--json"})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	if strings.Contains(out.String(), "token") {
		t.Fatalf("unexpected token field in output: %s", out.String())
	}
	var got struct {
		Host    string                        `json:"host"`
		Source  string                        `json:"source"`
		Backend auth.GitHubBackendDiagnostics `json:"backend"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Source != "gh" || got.Backend.Name != "gh" || got.Backend.Kind != "external-cli" {
		t.Fatalf("unexpected gh token metadata: %+v", got)
	}
}

func TestAuthTokenPlainForGHUsesExplicitTokenProvider(t *testing.T) {
	const secret = "gh-secret-token"
	var out, errOut bytes.Buffer
	app := newApp(strings.NewReader(""), &out, &errOut)
	app.selectGitHubBackend = func(context.Context, string) (auth.GitHubBackendSelection, error) {
		return auth.GitHubBackendSelection{
			Mode:            auth.GitHubBackendModeGH,
			Name:            auth.GitHubBackendNameGH,
			Kind:            auth.GitHubBackendKindCLI,
			Host:            "github.com",
			SelectionSource: "override:gh",
			Token:           auth.Token{Source: "gh", Host: "github.com"},
		}, nil
	}
	app.gitHubBackendToken = func(context.Context, auth.GitHubBackendSelection) (string, error) {
		return secret, nil
	}

	code := app.runAuthToken(context.Background(), []string{"--plain"})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	if strings.TrimSpace(out.String()) != secret {
		t.Fatalf("plain token output = %q", out.String())
	}
}

func TestAuthTokenJSONIncludeTokenForGHUsesExplicitTokenProvider(t *testing.T) {
	const secret = "gh-secret-token"
	var out, errOut bytes.Buffer
	app := newApp(strings.NewReader(""), &out, &errOut)
	app.selectGitHubBackend = func(context.Context, string) (auth.GitHubBackendSelection, error) {
		return auth.GitHubBackendSelection{
			Mode:            auth.GitHubBackendModeGH,
			Name:            auth.GitHubBackendNameGH,
			Kind:            auth.GitHubBackendKindCLI,
			Host:            "github.com",
			SelectionSource: "override:gh",
			Token:           auth.Token{Source: "gh", Host: "github.com"},
		}, nil
	}
	app.gitHubBackendToken = func(context.Context, auth.GitHubBackendSelection) (string, error) {
		return secret, nil
	}

	code := app.runAuthToken(context.Background(), []string{"--json", "--include-token"})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	var got struct {
		Token   string                        `json:"token"`
		Backend auth.GitHubBackendDiagnostics `json:"backend"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Token != secret || got.Backend.Name != "gh" || got.Backend.SelectionSource != "override:gh" {
		t.Fatalf("unexpected token JSON: %+v", got)
	}
}

func TestInitJSONIncludesBackendDiagnostics(t *testing.T) {
	t.Chdir(t.TempDir())
	var out, errOut bytes.Buffer
	app := newApp(strings.NewReader(""), &out, &errOut)
	app.selectGitHubBackend = func(context.Context, string) (auth.GitHubBackendSelection, error) {
		return auth.GitHubBackendSelection{
			Mode:            auth.GitHubBackendModeAuto,
			Name:            auth.GitHubBackendNameREST,
			Kind:            auth.GitHubBackendKindREST,
			Host:            "github.com",
			SelectionSource: "auto:token",
			TokenSource:     "config",
			Token:           auth.Token{Value: "stored-secret", Source: "config", Host: "github.com"},
		}, nil
	}
	app.newGitHubBackend = func(_ context.Context, selection auth.GitHubBackendSelection) (github.Backend, error) {
		return fakeGitHubBackend{
			info: github.BackendInfo{Name: selection.Name, Kind: selection.Kind, Host: selection.Host},
			user: github.User{Login: "octocat"},
		}, nil
	}

	code := app.runInit(context.Background(), []string{"--repo", "o/r", "--tools", "none", "--json"})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	if strings.Contains(out.String(), "stored-secret") || strings.Contains(errOut.String(), "stored-secret") {
		t.Fatalf("token leaked in init output: stdout=%q stderr=%q", out.String(), errOut.String())
	}
	var got struct {
		OK      bool                          `json:"ok"`
		Backend auth.GitHubBackendDiagnostics `json:"backend"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Backend.Name != "rest" || got.Backend.TokenSource != "config" {
		t.Fatalf("unexpected init result: %+v", got)
	}
}

func TestIssueCreateUsesSelectedGHBackend(t *testing.T) {
	var out, errOut bytes.Buffer
	app := newApp(strings.NewReader(""), &out, &errOut)
	app.selectGitHubBackend = ghSelection
	var created bool
	app.newGitHubBackend = func(_ context.Context, selection auth.GitHubBackendSelection) (github.Backend, error) {
		if selection.Name != auth.GitHubBackendNameGH {
			t.Fatalf("backend = %q, want gh", selection.Name)
		}
		return fakeGitHubBackend{
			info: github.BackendInfo{Name: selection.Name, Kind: selection.Kind, Host: selection.Host},
			createIssue: func(_ context.Context, repo, title, body string, labels []string) (github.Issue, error) {
				created = true
				if repo != "o/r" || !strings.Contains(title, "gh-proxy") || !strings.Contains(body, "Proposal: gh-proxy") {
					t.Fatalf("unexpected issue create args repo=%q title=%q body=%q", repo, title, body)
				}
				if len(labels) != 1 || labels[0] != "issue-spec/proposal" {
					t.Fatalf("labels = %#v", labels)
				}
				return github.Issue{Number: 9, HTMLURL: "https://github.com/o/r/issues/9", Title: title}, nil
			},
		}, nil
	}

	code := app.runIssueCreate(context.Background(), "proposal", []string{"--repo", "o/r", "--change", "gh-proxy", "--json"})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	if !created {
		t.Fatal("CreateIssue was not called")
	}
	var got struct {
		OK     bool `json:"ok"`
		Number int  `json:"number"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Number != 9 {
		t.Fatalf("unexpected issue create output: %+v", got)
	}
}

func TestCommentListUsesSelectedGHBackend(t *testing.T) {
	body, err := model.EnsureTypedBody("SPEC", "SPEC-001", "## Requirement: X\n\nX MUST work.\n", model.BodyOptions{Status: "confirmed", Scope: "backend"})
	if err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	app := newApp(strings.NewReader(""), &out, &errOut)
	app.selectGitHubBackend = ghSelection
	app.newGitHubBackend = func(_ context.Context, selection auth.GitHubBackendSelection) (github.Backend, error) {
		return fakeGitHubBackend{
			info: github.BackendInfo{Name: selection.Name, Kind: selection.Kind, Host: selection.Host},
			listIssueComments: func(_ context.Context, repo string, issueNumber int) ([]github.Comment, error) {
				if repo != "o/r" || issueNumber != 9 {
					t.Fatalf("unexpected comment list args repo=%q issue=%d", repo, issueNumber)
				}
				return []github.Comment{{ID: 101, HTMLURL: "https://github.com/o/r/issues/9#issuecomment-101", URL: "https://api.github.com/repos/o/r/issues/comments/101", Body: body}}, nil
			},
		}, nil
	}

	code := app.runCommentList(context.Background(), []string{"--repo", "o/r", "--issue", "9", "--type", "SPEC", "--json"})
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%q", code, errOut.String())
	}
	var got struct {
		OK       bool `json:"ok"`
		Comments []struct {
			Comment struct {
				ID string `json:"id"`
			} `json:"comment"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || len(got.Comments) != 1 || got.Comments[0].Comment.ID != "SPEC-001" {
		t.Fatalf("unexpected comment list output: %+v", got)
	}
}

func TestDefaultNewGitHubBackendConstructsGHBackend(t *testing.T) {
	backend, err := defaultNewGitHubBackend(context.Background(), auth.GitHubBackendSelection{
		Name: auth.GitHubBackendNameGH,
		Kind: auth.GitHubBackendKindCLI,
		Host: "https://ghe.example.com/",
	})
	if err != nil {
		t.Fatal(err)
	}
	info := backend.BackendInfo()
	if info.Name != "gh" || info.Kind != "external-cli" || info.Host != "ghe.example.com" {
		t.Fatalf("backend info = %+v", info)
	}
}

func TestDefaultGitHubBackendTokenForGHUsesProvider(t *testing.T) {
	old := ghAuthToken
	t.Cleanup(func() { ghAuthToken = old })
	var gotHost string
	ghAuthToken = func(_ context.Context, host string) (string, error) {
		gotHost = host
		return "gh-token", nil
	}

	token, err := defaultGitHubBackendToken(context.Background(), auth.GitHubBackendSelection{
		Name: auth.GitHubBackendNameGH,
		Host: "ghe.example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if token != "gh-token" || gotHost != "ghe.example.com" {
		t.Fatalf("token = %q host = %q", token, gotHost)
	}
}

func ghSelection(context.Context, string) (auth.GitHubBackendSelection, error) {
	return auth.GitHubBackendSelection{
		Mode:            auth.GitHubBackendModeGH,
		Name:            auth.GitHubBackendNameGH,
		Kind:            auth.GitHubBackendKindCLI,
		Host:            "github.com",
		SelectionSource: "override:gh",
		Token:           auth.Token{Source: "gh", Host: "github.com"},
	}, nil
}

type fakeGitHubBackend struct {
	info              github.BackendInfo
	user              github.User
	scopes            []string
	createIssue       func(context.Context, string, string, string, []string) (github.Issue, error)
	listIssueComments func(context.Context, string, int) ([]github.Comment, error)
}

func (f fakeGitHubBackend) BackendInfo() github.BackendInfo {
	return f.info
}

func (f fakeGitHubBackend) GetUser(context.Context) (github.User, []string, error) {
	return f.user, f.scopes, nil
}

func (f fakeGitHubBackend) CreateIssue(ctx context.Context, repo, title, body string, labels []string) (github.Issue, error) {
	if f.createIssue != nil {
		return f.createIssue(ctx, repo, title, body, labels)
	}
	return github.Issue{}, errors.New("unused")
}

func (fakeGitHubBackend) GetIssue(context.Context, string, int) (github.Issue, error) {
	return github.Issue{}, errors.New("unused")
}

func (fakeGitHubBackend) UpdateIssue(context.Context, string, int, github.UpdateIssueOptions) (github.Issue, error) {
	return github.Issue{}, errors.New("unused")
}

func (f fakeGitHubBackend) ListIssueComments(ctx context.Context, repo string, issueNumber int) ([]github.Comment, error) {
	if f.listIssueComments != nil {
		return f.listIssueComments(ctx, repo, issueNumber)
	}
	return nil, errors.New("unused")
}

func (fakeGitHubBackend) CreateComment(context.Context, string, int, string) (github.Comment, error) {
	return github.Comment{}, errors.New("unused")
}

func (fakeGitHubBackend) UpdateComment(context.Context, string, int64, string) (github.Comment, error) {
	return github.Comment{}, errors.New("unused")
}

func (fakeGitHubBackend) CreateLabel(context.Context, string, string, string, string) (github.LabelResult, error) {
	return github.LabelResult{}, errors.New("unused")
}

func (fakeGitHubBackend) GetPullRequest(context.Context, string, int) (github.PullRequest, error) {
	return github.PullRequest{}, errors.New("unused")
}

func (fakeGitHubBackend) CreatePullRequest(context.Context, string, github.CreatePullRequestOptions) (github.PullRequest, error) {
	return github.PullRequest{}, errors.New("unused")
}

func (fakeGitHubBackend) ListPullRequestFiles(context.Context, string, int) ([]github.PullRequestFile, error) {
	return nil, errors.New("unused")
}

func (fakeGitHubBackend) ListPullRequestReviewComments(context.Context, string, int) ([]github.PullRequestReviewComment, error) {
	return nil, errors.New("unused")
}

func (fakeGitHubBackend) CreatePullRequestReviewComment(context.Context, string, int, string, string, string, int, string) (github.PullRequestReviewComment, error) {
	return github.PullRequestReviewComment{}, errors.New("unused")
}

func (fakeGitHubBackend) ReplyPullRequestReviewComment(context.Context, string, int, int64, string) (github.PullRequestReviewComment, error) {
	return github.PullRequestReviewComment{}, errors.New("unused")
}

func (fakeGitHubBackend) GetCombinedStatus(context.Context, string, string) (github.CombinedStatus, error) {
	return github.CombinedStatus{}, errors.New("unused")
}

func (fakeGitHubBackend) ListCheckRuns(context.Context, string, string) ([]github.CheckRun, error) {
	return nil, errors.New("unused")
}
