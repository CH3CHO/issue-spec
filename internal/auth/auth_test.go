package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

func TestParseGitHubBackendMode(t *testing.T) {
	tests := []struct {
		value string
		want  GitHubBackendMode
		ok    bool
	}{
		{want: GitHubBackendModeAuto, ok: true},
		{value: "auto", want: GitHubBackendModeAuto, ok: true},
		{value: "REST", want: GitHubBackendModeREST, ok: true},
		{value: " gh ", want: GitHubBackendModeGH, ok: true},
		{value: "cli", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := ParseGitHubBackendMode(tt.value)
			if tt.ok && err != nil {
				t.Fatalf("ParseGitHubBackendMode returned error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("ParseGitHubBackendMode succeeded, want error")
			}
			if got != tt.want {
				t.Fatalf("mode = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSelectGitHubBackendPrefersTokenInAuto(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("ISSUE_SPEC_TOKEN", "issue-token")

	selection, err := SelectGitHubBackendWithOptions(context.Background(), "github.com", GitHubBackendSelectionOptions{
		GHAuthenticated: func(context.Context, string) error {
			t.Fatal("gh probe should not run when an explicit token exists")
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != GitHubBackendNameREST {
		t.Fatalf("backend = %q, want rest", selection.Name)
	}
	if selection.SelectionSource != "auto:token" {
		t.Fatalf("selection source = %q", selection.SelectionSource)
	}
	if selection.TokenSource != "env:ISSUE_SPEC_TOKEN" {
		t.Fatalf("token source = %q", selection.TokenSource)
	}
	if selection.Token.Value != "issue-token" {
		t.Fatal("selected REST token value was not preserved for backend construction")
	}
}

func TestSelectGitHubBackendForcedRESTRequiresToken(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv(GitHubBackendEnv, "rest")
	t.Setenv("ISSUE_SPEC_CONFIG_DIR", t.TempDir())

	selection, err := SelectGitHubBackendWithOptions(context.Background(), "example.invalid", GitHubBackendSelectionOptions{})
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("error = %v, want ErrNoToken", err)
	}
	if selection.Name != GitHubBackendNameREST {
		t.Fatalf("backend = %q, want rest", selection.Name)
	}
	if selection.SelectionSource != "override:rest" {
		t.Fatalf("selection source = %q", selection.SelectionSource)
	}
}

func TestSelectGitHubBackendAutoUsesAuthenticatedGHWithoutToken(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("ISSUE_SPEC_CONFIG_DIR", t.TempDir())
	var probedHost string

	selection, err := SelectGitHubBackendWithOptions(context.Background(), "https://example.invalid/", GitHubBackendSelectionOptions{
		GHAuthenticated: func(_ context.Context, host string) error {
			probedHost = host
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != GitHubBackendNameGH || selection.Kind != GitHubBackendKindCLI {
		t.Fatalf("backend = %q/%q, want gh/external-cli", selection.Name, selection.Kind)
	}
	if selection.SelectionSource != "auto:gh" {
		t.Fatalf("selection source = %q", selection.SelectionSource)
	}
	if probedHost != "example.invalid" {
		t.Fatalf("probed host = %q, want example.invalid", probedHost)
	}
}

func TestSelectGitHubBackendAutoPrefersStoredTokenAndRedactsDiagnostics(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("ISSUE_SPEC_CONFIG_DIR", t.TempDir())
	const secret = "stored-secret-token"
	if _, err := StoreToken(context.Background(), PlatformGitHub, "stored.example.com", secret, true); err != nil {
		t.Fatal(err)
	}

	selection, err := SelectGitHubBackendWithOptions(context.Background(), "stored.example.com", GitHubBackendSelectionOptions{
		GHAuthenticated: func(context.Context, string) error {
			t.Fatal("gh probe should not run when a stored token exists")
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != GitHubBackendNameREST || selection.SelectionSource != "auto:token" || selection.TokenSource != "credentials.json:github" {
		t.Fatalf("unexpected selection: %+v", selection)
	}
	if selection.Token.Value != secret {
		t.Fatal("stored token value was not preserved for REST backend construction")
	}
	data, err := json.Marshal(selection.TokenWithDiagnostics())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("stored token leaked in JSON diagnostics: %s", data)
	}
}

func TestCompatibilitySelectGitHubBackendAutoKeepsRESTTokenWhenCustomAPIURLIsSet(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv(GitHubBackendAPIURLEnv, "https://api.example.test/custom/")
	t.Setenv("GH_TOKEN", "rest-token")

	selection, err := SelectGitHubBackendWithOptions(context.Background(), "ghe.example.com", GitHubBackendSelectionOptions{
		GHAuthenticated: func(context.Context, string) error {
			t.Fatal("gh probe should not run when a REST token and custom API URL are configured")
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if selection.Name != GitHubBackendNameREST || selection.Kind != GitHubBackendKindREST {
		t.Fatalf("backend = %q/%q, want rest/rest", selection.Name, selection.Kind)
	}
	if selection.SelectionSource != "auto:token" || selection.TokenSource != "env:GH_TOKEN" {
		t.Fatalf("selection diagnostics = %+v", selection)
	}
	if selection.Token.Value != "rest-token" || selection.Host != "ghe.example.com" {
		t.Fatalf("selection token/host = %+v", selection)
	}
}

func TestSelectGitHubBackendAutoReportsGHProbeFailure(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("ISSUE_SPEC_CONFIG_DIR", t.TempDir())

	selection, err := SelectGitHubBackendWithOptions(context.Background(), "github.com", GitHubBackendSelectionOptions{
		GHAuthenticated: func(context.Context, string) error {
			return errors.New("not logged in")
		},
	})
	if err == nil {
		t.Fatal("selection succeeded, want gh probe failure")
	}
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("error = %v, want ErrNoToken context", err)
	}
	if len(selection.Probes) != 2 {
		t.Fatalf("probes = %+v, want rest and gh probe records", selection.Probes)
	}
	if selection.Probes[0].Name != GitHubBackendNameREST || selection.Probes[0].Status != "unavailable" {
		t.Fatalf("rest probe = %+v", selection.Probes[0])
	}
	if selection.Probes[1].Name != GitHubBackendNameGH || selection.Probes[1].Status != "unavailable" || !strings.Contains(selection.Probes[1].Error, "not logged in") {
		t.Fatalf("gh probe = %+v", selection.Probes[1])
	}
	if !strings.Contains(err.Error(), "gh authentication probe failed for github.com") {
		t.Fatalf("error missing probe context: %v", err)
	}
}

func TestSelectGitHubBackendForcedGHRejectsCustomAPIURL(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv(GitHubBackendEnv, "gh")
	t.Setenv(GitHubBackendAPIURLEnv, "https://api.example.test")

	selection, err := SelectGitHubBackendWithOptions(context.Background(), "github.com", GitHubBackendSelectionOptions{})
	if err == nil {
		t.Fatal("forced gh with custom API URL succeeded, want error")
	}
	if selection.Name != GitHubBackendNameGH {
		t.Fatalf("backend = %q, want gh", selection.Name)
	}
	if !strings.Contains(err.Error(), GitHubBackendAPIURLEnv) {
		t.Fatalf("error does not mention %s: %v", GitHubBackendAPIURLEnv, err)
	}
}

func TestSelectGitHubBackendInvalidOverride(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv(GitHubBackendEnv, "bad")

	_, err := SelectGitHubBackendWithOptions(context.Background(), "github.com", GitHubBackendSelectionOptions{})
	if err == nil {
		t.Fatal("invalid backend override succeeded, want error")
	}
	if !strings.Contains(err.Error(), GitHubBackendEnv) {
		t.Fatalf("error does not mention %s: %v", GitHubBackendEnv, err)
	}
}

func clearAuthEnv(t *testing.T) {
	t.Helper()
	t.Setenv(GitHubBackendEnv, "")
	t.Setenv(GitHubBackendAPIURLEnv, "")
	t.Setenv("ISSUE_SPEC_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GL_TOKEN", "")
}

// -----------------------------------------------------------------------------
// PROCESS-004: per-platform token resolution and credential storage
// -----------------------------------------------------------------------------

func TestParsePlatform(t *testing.T) {
	tests := []struct {
		value string
		want  Platform
		ok    bool
	}{
		{value: "github", want: PlatformGitHub, ok: true},
		{value: "GITHUB", want: PlatformGitHub, ok: true},
		{value: " GitHub ", want: PlatformGitHub, ok: true},
		{value: "gitlab", want: PlatformGitLab, ok: true},
		{value: "GitLab", want: PlatformGitLab, ok: true},
		{value: " bitbucket ", ok: false},
		{value: "", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := ParsePlatform(tt.value)
			if tt.ok && err != nil {
				t.Fatalf("ParsePlatform returned error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("ParsePlatform succeeded, want error")
			}
			if got != tt.want {
				t.Fatalf("platform = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTokenGitHubEnvISSUE_SPEC_TOKEN(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("ISSUE_SPEC_TOKEN", "issue-token")

	token, err := ResolveToken(context.Background(), PlatformGitHub, "github.com")
	if err != nil {
		t.Fatal(err)
	}
	if token.Value != "issue-token" {
		t.Fatalf("token.Value = %q", token.Value)
	}
	if token.Source != "env:ISSUE_SPEC_TOKEN" {
		t.Fatalf("token.Source = %q", token.Source)
	}
	if token.Host != "github.com" {
		t.Fatalf("token.Host = %q", token.Host)
	}
}

func TestResolveTokenGitHubEnvGH_TOKEN(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("GH_TOKEN", "gh-token")

	token, err := ResolveToken(context.Background(), PlatformGitHub, "github.com")
	if err != nil {
		t.Fatal(err)
	}
	if token.Value != "gh-token" {
		t.Fatalf("token.Value = %q", token.Value)
	}
	if token.Source != "env:GH_TOKEN" {
		t.Fatalf("token.Source = %q", token.Source)
	}
}

func TestResolveTokenGitLabEnvGITLAB_TOKEN(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("GITLAB_TOKEN", "glpat-primary")

	token, err := ResolveToken(context.Background(), PlatformGitLab, "gitlab.com")
	if err != nil {
		t.Fatal(err)
	}
	if token.Value != "glpat-primary" {
		t.Fatalf("token.Value = %q", token.Value)
	}
	if token.Source != "env:GITLAB_TOKEN" {
		t.Fatalf("token.Source = %q", token.Source)
	}
	if token.Host != "gitlab.com" {
		t.Fatalf("token.Host = %q", token.Host)
	}
}

func TestResolveTokenGitLabEnvGL_TOKEN(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("GL_TOKEN", "glpat-fallback")

	token, err := ResolveToken(context.Background(), PlatformGitLab, "gitlab.com")
	if err != nil {
		t.Fatal(err)
	}
	if token.Value != "glpat-fallback" {
		t.Fatalf("token.Value = %q", token.Value)
	}
	if token.Source != "env:GL_TOKEN" {
		t.Fatalf("token.Source = %q", token.Source)
	}
}

func TestResolveTokenGitLabKeyringBeatsCredentials(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("ISSUE_SPEC_CONFIG_DIR", t.TempDir())

	// Seed credentials.json with a GitLab entry; the keyring probe should
	// outrank it because the OS keyring is checked before the plaintext file.
	writeGitLabCredentialsFixture(t, filepath.Join(mustConfigDir(t), "credentials.json"), "credentials-token")

	// Inject a keyring probe via setenv; the real OS keyring may not be
	// writable in CI, so we hook it via a process-level swap.
	keyringProbe := &probeKeyring{value: "keyring-token"}
	swapKeyring(t, keyringProbe)

	token, err := ResolveToken(context.Background(), PlatformGitLab, "gitlab.com")
	if err != nil {
		t.Fatal(err)
	}
	if token.Value != "keyring-token" {
		t.Fatalf("token.Value = %q, want keyring-token", token.Value)
	}
	if !strings.Contains(token.Source, "keyring") {
		t.Fatalf("token.Source = %q, want a keyring source", token.Source)
	}
}

func TestResolveTokenGitLabCredentialsFileOnly(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("ISSUE_SPEC_CONFIG_DIR", t.TempDir())

	writeGitLabCredentialsFixture(t, filepath.Join(mustConfigDir(t), "credentials.json"), "credentials-token")

	// Make sure the keyring probe returns nothing.
	swapKeyring(t, &probeKeyring{})

	token, err := ResolveToken(context.Background(), PlatformGitLab, "gitlab.com")
	if err != nil {
		t.Fatal(err)
	}
	if token.Value != "credentials-token" {
		t.Fatalf("token.Value = %q, want credentials-token", token.Value)
	}
	if !strings.Contains(token.Source, "credentials.json") {
		t.Fatalf("token.Source = %q, want a credentials.json source", token.Source)
	}
}

func TestCredentialsFilePerPlatformIsolation(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("ISSUE_SPEC_CONFIG_DIR", t.TempDir())
	swapKeyring(t, &probeKeyring{})

	// Log in to both platforms using plaintext storage so we exercise the
	// new per-platform credentials.json schema.
	if _, err := StoreToken(context.Background(), PlatformGitHub, "github.com", "gh-token", true); err != nil {
		t.Fatal(err)
	}
	if _, err := StoreToken(context.Background(), PlatformGitLab, "gitlab.com", "gl-token", true); err != nil {
		t.Fatal(err)
	}

	// Logging out of GitLab must not touch the GitHub section.
	if err := DeleteToken(context.Background(), PlatformGitLab, "gitlab.com"); err != nil {
		t.Fatal(err)
	}

	ghToken, err := ResolveToken(context.Background(), PlatformGitHub, "github.com")
	if err != nil {
		t.Fatalf("github token removed by gitlab logout: %v", err)
	}
	if ghToken.Value != "gh-token" {
		t.Fatalf("github token.Value = %q", ghToken.Value)
	}

	_, err = ResolveToken(context.Background(), PlatformGitLab, "gitlab.com")
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("gitlab logout did not clear credentials.json entry: err=%v", err)
	}
}

func TestEnvTokenActiveGitLab(t *testing.T) {
	clearAuthEnv(t)
	// Both env vars empty: no token active.
	if name := EnvTokenActive(PlatformGitLab); name != "" {
		t.Fatalf("EnvTokenActive(gitlab) = %q, want empty", name)
	}
	// GITLAB_TOKEN set: it should be the reported name.
	t.Setenv("GITLAB_TOKEN", "glpat-x")
	if name := EnvTokenActive(PlatformGitLab); name != "GITLAB_TOKEN" {
		t.Fatalf("EnvTokenActive(gitlab) = %q, want GITLAB_TOKEN", name)
	}
	// Only GL_TOKEN set: should report the fallback alias.
	t.Setenv("GITLAB_TOKEN", "")
	t.Setenv("GL_TOKEN", "glpat-y")
	if name := EnvTokenActive(PlatformGitLab); name != "GL_TOKEN" {
		t.Fatalf("EnvTokenActive(gitlab) = %q, want GL_TOKEN", name)
	}
	// GitHub env vars do not satisfy a GitLab lookup.
	t.Setenv("GL_TOKEN", "")
	t.Setenv("GH_TOKEN", "gh-x")
	if name := EnvTokenActive(PlatformGitLab); name != "" {
		t.Fatalf("EnvTokenActive(gitlab) = %q, want empty (GH_TOKEN must not count)", name)
	}
}

func TestCredentialsFileLegacyFormatMigration(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("ISSUE_SPEC_CONFIG_DIR", t.TempDir())
	swapKeyring(t, &probeKeyring{})

	// Write a legacy single-token credentials.json file (the format shipped
	// before PROCESS-004). ReadToken must surface it as the GitHub entry.
	writeLegacyCredentialsFixture(t, filepath.Join(mustConfigDir(t), "credentials.json"), "legacy-token")

	token, err := ResolveToken(context.Background(), PlatformGitHub, "github.com")
	if err != nil {
		t.Fatal(err)
	}
	if token.Value != "legacy-token" {
		t.Fatalf("token.Value = %q, want legacy-token", token.Value)
	}
	if !strings.Contains(token.Source, "credentials.json") {
		t.Fatalf("token.Source = %q, want a credentials.json source", token.Source)
	}

	// A GitLab lookup on the same file must NOT see the GitHub legacy entry.
	_, err = ResolveToken(context.Background(), PlatformGitLab, "gitlab.com")
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("gitlab lookup leaked into legacy github entry: err=%v", err)
	}
}

func TestNormalizeGitLabHost(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "gitlab.com"},
		{"   ", "gitlab.com"},
		{"https://gitlab.example.com/", "gitlab.example.com"},
		{"http://gitlab.example.com", "gitlab.example.com"},
		{"gitlab.example.com", "gitlab.example.com"},
		{"GITLAB.COM", "GITLAB.COM"},
	}
	for _, tt := range cases {
		t.Run(tt.in, func(t *testing.T) {
			if got := NormalizeGitLabHost(tt.in); got != tt.want {
				t.Fatalf("NormalizeGitLabHost(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEnvTokenActiveGitHubStillWorks(t *testing.T) {
	clearAuthEnv(t)
	if name := EnvTokenActive(PlatformGitHub); name != "" {
		t.Fatalf("EnvTokenActive(github) = %q, want empty", name)
	}
	t.Setenv("ISSUE_SPEC_TOKEN", "issue-x")
	if name := EnvTokenActive(PlatformGitHub); name != "ISSUE_SPEC_TOKEN" {
		t.Fatalf("EnvTokenActive(github) = %q, want ISSUE_SPEC_TOKEN", name)
	}
	// GitLab env vars must not satisfy a GitHub lookup.
	t.Setenv("GITLAB_TOKEN", "gl-x")
	if name := EnvTokenActive(PlatformGitHub); name != "ISSUE_SPEC_TOKEN" {
		t.Fatalf("EnvTokenActive(github) = %q, want ISSUE_SPEC_TOKEN (GITLAB_TOKEN must not count)", name)
	}
}

// -----------------------------------------------------------------------------
// Test helpers for PROCESS-004
// -----------------------------------------------------------------------------

func mustConfigDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("ISSUE_SPEC_CONFIG_DIR")
	if dir == "" {
		t.Fatal("ISSUE_SPEC_CONFIG_DIR is empty")
	}
	return dir
}

func writeGitLabCredentialsFixture(t *testing.T, path, token string) {
	t.Helper()
	body := map[string]any{
		"gitlab": map[string]any{
			"token": token,
			"host":  "gitlab.com",
		},
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeLegacyCredentialsFixture(t *testing.T, path, token string) {
	t.Helper()
	// Old single-token format used by issue-spec releases before PROCESS-004.
	body := map[string]any{
		"token": token,
		"host":  "github.com",
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

// probeKeyring is a fake keyringBackend used by PROCESS-004 tests. Set
// Value to a non-empty string to make Get behave as if a keyring entry
// exists; leave it empty to make Get behave as if no entry is present.
type probeKeyring struct {
	value string
	err   error
}

func (p *probeKeyring) Get(_, _ string) (string, error) {
	if p.err != nil {
		return "", p.err
	}
	return p.value, nil
}

func (*probeKeyring) Set(_, _, _ string) error { return nil }

func (*probeKeyring) Delete(_, _ string) error { return keyring.ErrNotFound }

// swapKeyring swaps the package-level keyringOps for the duration of the
// test, restoring the OS keyring on cleanup. Tests that exercise the
// keyring-vs-credentials.json precedence MUST call this so they do not
// depend on (or pollute) the real OS keyring.
func swapKeyring(t *testing.T, backend keyringBackend) {
	t.Helper()
	restore := SetKeyringBackend(backend)
	t.Cleanup(restore)
}
