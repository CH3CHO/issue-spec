package commands

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/higress-group/issue-spec/internal/auth"
	"github.com/higress-group/issue-spec/internal/github"
)

// defaultPlatform is the platform assumed when --platform is omitted.
// PROCESS-004 keeps the GitHub default so existing scripts continue to
// work unchanged; GitLab users opt in explicitly with --platform gitlab.
const defaultPlatform = auth.PlatformGitHub

// resolvePlatformFlag parses --platform into an auth.Platform. Empty
// input falls back to defaultPlatform so the flag is optional.
func resolvePlatformFlag(value string) (auth.Platform, error) {
	if strings.TrimSpace(value) == "" {
		return defaultPlatform, nil
	}
	return auth.ParsePlatform(value)
}

// resolveHostForPlatform normalizes a hostname through the right
// platform-specific helper. The flag default is "github.com" for
// backward compatibility; callers that pass an empty string to a
// GitLab subcommand still get the canonical "gitlab.com" via
// auth.NormalizeGitLabHost.
func resolveHostForPlatform(platform auth.Platform, host string) string {
	if platform == auth.PlatformGitLab {
		return auth.NormalizeGitLabHost(host)
	}
	return auth.NormalizeHost(host)
}

func (a *app) runAuth(ctx context.Context, args []string) int {
	if len(args) == 0 {
		a.errorf("usage: issue-spec auth status|login|logout|token\n")
		return 2
	}
	switch args[0] {
	case "status":
		return a.runAuthStatus(ctx, args[1:])
	case "login":
		return a.runAuthLogin(ctx, args[1:])
	case "logout":
		return a.runAuthLogout(ctx, args[1:])
	case "token":
		return a.runAuthToken(ctx, args[1:])
	default:
		a.errorf("unknown auth command %q\n", args[0])
		return 2
	}
}

func (a *app) runAuthStatus(ctx context.Context, args []string) int {
	fs := newFlagSet("auth status", a.err)
	host := fs.String("hostname", "", "GitHub or GitLab hostname (defaults to the platform's canonical host)")
	platformFlag := fs.String("platform", string(defaultPlatform), "platform to query (github or gitlab)")
	jsonOut := fs.Bool("json", false, "write JSON output")
	includeToken := fs.Bool("include-token", false, "include token value in JSON output")
	if ok, code := a.parseFlagSet(fs, args); !ok {
		return code
	}
	platform, err := resolvePlatformFlag(*platformFlag)
	if err != nil {
		a.errorf("%v\n", err)
		return 2
	}
	hostName := resolveHostForPlatform(platform, *host)
	if platform == auth.PlatformGitLab {
		return a.runAuthStatusGitLab(ctx, hostName, *jsonOut, *includeToken)
	}
	client, token, err := a.clientFor(ctx, hostName)
	if err != nil {
		if *jsonOut {
			return a.outputJSON(authErrorResult(token, err))
		}
		a.errorf("not authenticated for %s: %v\n", hostName, err)
		return 1
	}
	user, scopes, err := client.GetUser(ctx)
	if err != nil {
		if *jsonOut {
			return a.outputJSON(authErrorResult(token, err))
		}
		a.errorf("validate token for %s from %s: %v\n", token.Host, token.Source, err)
		return 1
	}
	token.User = user.Login
	token.Scopes = scopes
	if *jsonOut {
		return a.outputJSON(map[string]any{"ok": true, "auth": token, "backend": token.Backend})
	}
	fmt.Fprintf(a.out, "github host: %s\nuser: %s\ntoken source: %s\n", token.Host, token.User, token.Source)
	if token.Backend != nil {
		fmt.Fprintf(a.out, "github backend: %s (%s)\n", token.Backend.Name, token.Backend.SelectionSource)
	}
	if len(token.Scopes) > 0 {
		fmt.Fprintf(a.out, "scopes: %s\n", strings.Join(token.Scopes, ", "))
	}
	return 0
}

// runAuthStatusGitLab implements the GitLab branch of `auth status`.
// It reuses auth.ResolveToken directly because GitLab commands in
// PROCESS-003 will consume the same Token shape; until then we surface
// only what the GitLab CLI backend would need (host + token source).
func (a *app) runAuthStatusGitLab(ctx context.Context, host string, jsonOut, includeToken bool) int {
	token, err := auth.ResolveToken(ctx, auth.PlatformGitLab, host)
	if err != nil {
		result := map[string]any{"ok": false, "host": host, "platform": string(auth.PlatformGitLab), "error": err.Error()}
		if jsonOut {
			return a.outputJSON(result)
		}
		a.errorf("not authenticated for %s: %v\n", host, err)
		return 1
	}
	if jsonOut {
		out := map[string]any{
			"ok":       true,
			"platform": string(auth.PlatformGitLab),
			"auth":     token,
		}
		if includeToken {
			out["token"] = token.Value
		}
		return a.outputJSON(out)
	}
	fmt.Fprintf(a.out, "gitlab host: %s\ntoken source: %s\n", token.Host, token.Source)
	return 0
}

func (a *app) runAuthLogin(ctx context.Context, args []string) int {
	fs := newFlagSet("auth login", a.err)
	host := fs.String("hostname", "", "GitHub or GitLab hostname (defaults to the platform's canonical host)")
	platformFlag := fs.String("platform", string(defaultPlatform), "platform to authenticate with (github or gitlab)")
	withToken := fs.Bool("with-token", false, "read token from stdin")
	insecure := fs.Bool("insecure-storage", false, "store token in issue-spec plaintext config when keyring is unavailable or undesired")
	jsonOut := fs.Bool("json", false, "write JSON output")
	if ok, code := a.parseFlagSet(fs, args); !ok {
		return code
	}
	platform, err := resolvePlatformFlag(*platformFlag)
	if err != nil {
		a.errorf("%v\n", err)
		return 2
	}
	if platform == auth.PlatformGitLab {
		return a.runAuthLoginGitLab(ctx, fs, host, withToken, insecure, jsonOut)
	}
	if !*withToken {
		return a.runAuthLoginAdvice(ctx, *host, *jsonOut)
	}
	data, err := io.ReadAll(a.in)
	if err != nil {
		a.errorf("read token from stdin: %v\n", err)
		return 1
	}
	tokenValue := strings.TrimSpace(string(data))
	if tokenValue == "" {
		a.errorf("stdin token is empty\n")
		return 1
	}
	hostName := auth.NormalizeHost(*host)
	user, scopes, err := github.NewClient(hostName, tokenValue).GetUser(ctx)
	if err != nil {
		a.errorf("validate token for %s: %v\n", hostName, err)
		return 1
	}
	source, err := auth.StoreToken(ctx, platform, hostName, tokenValue, *insecure)
	if err != nil {
		a.errorf("%v\n", err)
		return 1
	}
	result := map[string]any{"ok": true, "platform": string(platform), "host": hostName, "user": user.Login, "source": source, "scopes": scopes}
	if *jsonOut {
		return a.outputJSON(result)
	}
	if *insecure {
		fmt.Fprintln(a.err, "warning: token stored in issue-spec plaintext config because --insecure-storage was set")
	}
	fmt.Fprintf(a.out, "logged in to %s as %s using %s storage\n", hostName, user.Login, source)
	return 0
}

// runAuthLoginGitLab implements the GitLab branch of `auth login`. The
// GitLab REST API does not expose a lightweight token-validation
// endpoint the way GitHub does, so the validation step is deferred to
// the first command that actually needs the token (matching the glab
// CLI's own behaviour). For now we persist the token, surface its
// source, and let downstream commands fail loudly if it is invalid.
func (a *app) runAuthLoginGitLab(ctx context.Context, fs *flag.FlagSet, host *string, withToken *bool, insecure *bool, jsonOut *bool) int {
	if !*withToken {
		a.errorf("auth login --platform gitlab requires --with-token; glab-based browser login is not wired in PROCESS-004\n")
		return 2
	}
	data, err := io.ReadAll(a.in)
	if err != nil {
		a.errorf("read token from stdin: %v\n", err)
		return 1
	}
	tokenValue := strings.TrimSpace(string(data))
	if tokenValue == "" {
		a.errorf("stdin token is empty\n")
		return 1
	}
	hostName := auth.NormalizeGitLabHost(*host)
	source, err := auth.StoreToken(ctx, auth.PlatformGitLab, hostName, tokenValue, *insecure)
	if err != nil {
		a.errorf("%v\n", err)
		return 1
	}
	result := map[string]any{"ok": true, "platform": string(auth.PlatformGitLab), "host": hostName, "source": source}
	if *jsonOut {
		return a.outputJSON(result)
	}
	if *insecure {
		fmt.Fprintln(a.err, "warning: token stored in issue-spec plaintext config because --insecure-storage was set")
	}
	fmt.Fprintf(a.out, "logged in to %s (gitlab) using %s storage\n", hostName, source)
	return 0
}

type authLoginAdvice struct {
	OK               bool     `json:"ok"`
	Host             string   `json:"host"`
	Backend          string   `json:"backend"`
	Mode             string   `json:"mode"`
	GitHubCLI        ghAdvice `json:"github_cli"`
	Message          string   `json:"message"`
	NextSteps        []string `json:"next_steps"`
	RESTLoginCommand string   `json:"rest_login_command,omitempty"`
	GHLoginCommand   string   `json:"gh_login_command,omitempty"`
	GHDownloadURL    string   `json:"gh_download_url,omitempty"`
}

type ghAdvice struct {
	Installed     bool   `json:"installed"`
	Authenticated bool   `json:"authenticated"`
	Error         string `json:"error,omitempty"`
}

const ghDownloadURL = "https://cli.github.com/"
const ghNotAuthenticatedError = "not_authenticated"

func (a *app) runAuthLoginAdvice(ctx context.Context, host string, jsonOut bool) int {
	advice := buildAuthLoginAdvice(ctx, host)
	if jsonOut {
		return a.outputJSON(advice)
	}
	fmt.Fprintln(a.out, advice.Message)
	for _, step := range advice.NextSteps {
		fmt.Fprintf(a.out, "  %s\n", step)
	}
	return 0
}

func buildAuthLoginAdvice(ctx context.Context, host string) authLoginAdvice {
	host = auth.NormalizeHost(host)
	restLoginCommand := issueSpecAuthLoginWithTokenCommand(host)
	statusCommand := issueSpecAuthStatusJSONCommand(host)
	ghLoginCommand := ghAuthLoginCommand(host)
	if _, err := ghLookPath("gh"); err != nil {
		return authLoginAdvice{
			OK:               true,
			Host:             host,
			Backend:          auth.GitHubBackendNameREST,
			Mode:             "rest-fallback",
			GitHubCLI:        ghAdvice{Installed: false},
			Message:          fmt.Sprintf("GitHub CLI was not found. issue-spec is using the fallback REST token login mode for %s.", host),
			NextSteps:        []string{restLoginCommand, "Install GitHub CLI from " + ghDownloadURL + " for the complete local workflow experience."},
			RESTLoginCommand: restLoginCommand,
			GHDownloadURL:    ghDownloadURL,
		}
	}

	if err := ghAuthenticated(ctx, host); err != nil {
		return authLoginAdvice{
			OK:               true,
			Host:             host,
			Backend:          auth.GitHubBackendNameGH,
			Mode:             "gh-needs-auth",
			GitHubCLI:        ghAdvice{Installed: true, Authenticated: false, Error: ghNotAuthenticatedError},
			Message:          fmt.Sprintf("GitHub CLI is installed but is not authenticated for %s. Authenticate gh first, then issue-spec can reuse that login.", host),
			NextSteps:        []string{ghLoginCommand, statusCommand, "For the REST token storage path instead, run: " + restLoginCommand},
			RESTLoginCommand: restLoginCommand,
			GHLoginCommand:   ghLoginCommand,
		}
	}

	return authLoginAdvice{
		OK:               true,
		Host:             host,
		Backend:          auth.GitHubBackendNameGH,
		Mode:             "gh-reuse",
		GitHubCLI:        ghAdvice{Installed: true, Authenticated: true},
		Message:          fmt.Sprintf("GitHub CLI is installed and authenticated for %s. issue-spec can reuse your gh CLI login directly; no issue-spec token login is required.", host),
		NextSteps:        []string{statusCommand, "For the REST token storage path instead, run: " + restLoginCommand},
		RESTLoginCommand: restLoginCommand,
	}
}

func issueSpecAuthLoginWithTokenCommand(host string) string {
	if isDefaultGitHubHost(host) {
		return "issue-spec auth login --with-token"
	}
	return fmt.Sprintf("issue-spec auth login --hostname %s --with-token", host)
}

func issueSpecAuthStatusJSONCommand(host string) string {
	if isDefaultGitHubHost(host) {
		return "issue-spec auth status --json"
	}
	return fmt.Sprintf("issue-spec auth status --hostname %s --json", host)
}

func ghAuthLoginCommand(host string) string {
	if isDefaultGitHubHost(host) {
		return "gh auth login"
	}
	return fmt.Sprintf("gh auth login --hostname %s", host)
}

func isDefaultGitHubHost(host string) bool {
	return strings.EqualFold(auth.NormalizeHost(host), "github.com")
}

func (a *app) runAuthLogout(ctx context.Context, args []string) int {
	fs := newFlagSet("auth logout", a.err)
	host := fs.String("hostname", "", "GitHub or GitLab hostname (defaults to the platform's canonical host)")
	platformFlag := fs.String("platform", string(defaultPlatform), "platform to log out (github or gitlab)")
	jsonOut := fs.Bool("json", false, "write JSON output")
	if ok, code := a.parseFlagSet(fs, args); !ok {
		return code
	}
	platform, err := resolvePlatformFlag(*platformFlag)
	if err != nil {
		a.errorf("%v\n", err)
		return 2
	}
	hostName := resolveHostForPlatform(platform, *host)
	err = auth.DeleteToken(ctx, platform, hostName)
	envActive := auth.EnvTokenActive(platform)
	if err != nil {
		a.errorf("logout %s: %v\n", hostName, err)
		return 1
	}
	result := map[string]any{"ok": true, "platform": string(platform), "host": hostName, "env_token_active": envActive}
	if *jsonOut {
		return a.outputJSON(result)
	}
	fmt.Fprintf(a.out, "removed persisted issue-spec token for %s\n", hostName)
	if envActive != "" {
		fmt.Fprintf(a.out, "environment token %s is still active and was not unset\n", envActive)
	}
	return 0
}

func (a *app) runAuthToken(ctx context.Context, args []string) int {
	fs := newFlagSet("auth token", a.err)
	host := fs.String("hostname", "", "GitHub or GitLab hostname (defaults to the platform's canonical host)")
	platformFlag := fs.String("platform", string(defaultPlatform), "platform to query (github or gitlab)")
	plain := fs.Bool("plain", false, "print token in plain text")
	jsonOut := fs.Bool("json", false, "write JSON output")
	includeToken := fs.Bool("include-token", false, "include token in JSON output")
	if ok, code := a.parseFlagSet(fs, args); !ok {
		return code
	}
	platform, err := resolvePlatformFlag(*platformFlag)
	if err != nil {
		a.errorf("%v\n", err)
		return 2
	}
	if platform == auth.PlatformGitLab {
		return a.runAuthTokenGitLab(ctx, *host, *plain, *jsonOut, *includeToken)
	}
	selection, err := a.selectBackend(ctx, *host)
	if err != nil {
		token := selection.TokenWithDiagnostics()
		if *jsonOut {
			return a.outputJSON(authErrorResult(token, err))
		}
		if errors.Is(err, auth.ErrNoToken) {
			a.errorf("not authenticated for %s\n", auth.NormalizeHost(*host))
		} else {
			a.errorf("resolve token: %v\n", err)
		}
		return 1
	}
	token := selection.TokenWithDiagnostics()
	if !*plain && !*jsonOut {
		a.errorf("refusing to print token without --plain\n")
		return 2
	}
	if *jsonOut {
		out := map[string]any{"platform": string(platform), "host": token.Host, "source": token.Source, "backend": token.Backend}
		if *includeToken {
			tokenValue, err := a.tokenForSelection(ctx, selection)
			if err != nil {
				return a.outputJSON(authErrorResult(token, err))
			}
			out["token"] = tokenValue
		}
		return a.outputJSON(out)
	}
	tokenValue, err := a.tokenForSelection(ctx, selection)
	if err != nil {
		a.errorf("resolve token: %v\n", err)
		return 1
	}
	fmt.Fprintln(a.out, tokenValue)
	return 0
}

// runAuthTokenGitLab implements the GitLab branch of `auth token`. It
// goes straight through auth.ResolveToken because GitLab token retrieval
// does not need backend selection until a command actually executes
// against the API (PROCESS-003 territory).
func (a *app) runAuthTokenGitLab(ctx context.Context, host string, plain, jsonOut, includeToken bool) int {
	hostName := auth.NormalizeGitLabHost(host)
	token, err := auth.ResolveToken(ctx, auth.PlatformGitLab, hostName)
	if err != nil {
		result := map[string]any{"ok": false, "platform": string(auth.PlatformGitLab), "host": hostName, "error": err.Error()}
		if jsonOut {
			return a.outputJSON(result)
		}
		if errors.Is(err, auth.ErrNoToken) {
			a.errorf("not authenticated for %s\n", hostName)
		} else {
			a.errorf("resolve token: %v\n", err)
		}
		return 1
	}
	if !plain && !jsonOut {
		a.errorf("refusing to print token without --plain\n")
		return 2
	}
	if jsonOut {
		out := map[string]any{"platform": string(auth.PlatformGitLab), "host": token.Host, "source": token.Source}
		if includeToken {
			out["token"] = token.Value
		}
		return a.outputJSON(out)
	}
	fmt.Fprintln(a.out, token.Value)
	return 0
}

var _ = flag.ContinueOnError

func authErrorResult(token auth.Token, err error) map[string]any {
	result := map[string]any{
		"ok":    false,
		"host":  token.Host,
		"error": err.Error(),
	}
	if token.Source != "" {
		result["source"] = token.Source
	}
	if token.Backend != nil {
		result["backend"] = token.Backend
	}
	return result
}
