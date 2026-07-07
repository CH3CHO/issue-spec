package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	keyring "github.com/zalando/go-keyring"
)

const (
	// serviceName is the OS keyring service namespace used for GitHub
	// credentials. GitLab uses its own namespace (serviceNameGitLab) so
	// the two platforms never share an entry even when the host string
	// collides.
	serviceName            = "issue-spec"
	serviceNameGitLab      = "issue-spec:gitlab"
	GitHubBackendEnv       = "ISSUE_SPEC_GITHUB_BACKEND"
	GitHubBackendAPIURLEnv = "ISSUE_SPEC_API_URL"
	GitHubBackendNameREST  = "rest"
	GitHubBackendNameGH    = "gh"
	GitHubBackendKindREST  = "rest"
	GitHubBackendKindCLI   = "external-cli"
	GitHubBackendModeAuto  = GitHubBackendMode("auto")
	GitHubBackendModeREST  = GitHubBackendMode("rest")
	GitHubBackendModeGH    = GitHubBackendMode("gh")
)

// Platform identifies which code-hosting platform a credential belongs to.
// New platforms (Bitbucket, Gitea, ...) extend this enum in lockstep with
// auth functions so per-platform resolution stays isolated.
type Platform string

const (
	// PlatformGitHub is github.com and GitHub Enterprise instances.
	PlatformGitHub Platform = "github"
	// PlatformGitLab is gitlab.com and self-hosted GitLab instances.
	PlatformGitLab Platform = "gitlab"
)

// EnvTokenNames lists the environment variables a given platform consults
// when resolving a token. The order is highest-priority first; resolution
// short-circuits on the first non-empty value.
func (p Platform) EnvTokenNames() []string {
	switch p {
	case PlatformGitHub:
		return []string{"ISSUE_SPEC_TOKEN", "GH_TOKEN", "GITHUB_TOKEN"}
	case PlatformGitLab:
		return []string{"GITLAB_TOKEN", "GL_TOKEN"}
	default:
		return nil
	}
}

// ParsePlatform accepts the canonical platform string ("github", "gitlab")
// or any case-insensitive / whitespace-padded variant. Unknown values
// return an error so command-level flag validation can reject bad input
// before reaching ResolveToken.
func ParsePlatform(value string) (Platform, error) {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	switch Platform(trimmed) {
	case PlatformGitHub:
		return PlatformGitHub, nil
	case PlatformGitLab:
		return PlatformGitLab, nil
	default:
		return "", fmt.Errorf("invalid --platform %q (want github or gitlab)", value)
	}
}

// serviceFor returns the OS keyring service namespace for the given
// platform. Per-platform services prevent GitHub and GitLab entries from
// colliding when their host strings happen to be identical.
func serviceFor(platform Platform) string {
	switch platform {
	case PlatformGitLab:
		return serviceNameGitLab
	default:
		return serviceName
	}
}

var ErrNoToken = errors.New("no issue-spec token is available")

type GitHubBackendMode string

type Token struct {
	Value   string                    `json:"-"`
	Source  string                    `json:"source"`
	User    string                    `json:"user,omitempty"`
	Scopes  []string                  `json:"scopes,omitempty"`
	Host    string                    `json:"host"`
	Backend *GitHubBackendDiagnostics `json:"backend,omitempty"`
}

type StoredCredential struct {
	Token string `json:"token"`
	Host  string `json:"host,omitempty"`
}

type GitHubBackendDiagnostics struct {
	Mode            string               `json:"mode"`
	Name            string               `json:"name,omitempty"`
	Kind            string               `json:"kind,omitempty"`
	Host            string               `json:"host"`
	SelectionSource string               `json:"selection_source,omitempty"`
	TokenSource     string               `json:"token_source,omitempty"`
	Probes          []GitHubBackendProbe `json:"probes,omitempty"`
}

type GitHubBackendProbe struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type GitHubBackendSelection struct {
	Mode            GitHubBackendMode
	Name            string
	Kind            string
	Host            string
	SelectionSource string
	TokenSource     string
	Probes          []GitHubBackendProbe
	Token           Token
}

type GitHubBackendSelectionOptions struct {
	GHAuthenticated func(context.Context, string) error
	Mode            *GitHubBackendMode
}

type credentialFile struct {
	// Hosts is the pre-PROCESS-004 schema: a map from hostname to a
	// stored token. Kept readable so existing credentials.json files
	// from earlier releases continue to load and resolve as GitHub
	// entries.
	Hosts map[string]StoredCredential `json:"hosts,omitempty"`

	// Token / Host are the very first single-token schema: a single
	// token tied to a single host. Same backward-compat guarantee as
	// Hosts above.
	Token string `json:"token,omitempty"`
	Host  string `json:"host,omitempty"`

	// GitHub / GitLab are the PROCESS-004 per-platform sections. They
	// supersede Hosts/Token once written; legacy entries are still
	// honoured on read so the migration is invisible to existing users.
	GitHub *StoredCredential `json:"github,omitempty"`
	GitLab *StoredCredential `json:"gitlab,omitempty"`
}

func ParseGitHubBackendMode(value string) (GitHubBackendMode, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return GitHubBackendModeAuto, nil
	}
	switch GitHubBackendMode(value) {
	case GitHubBackendModeAuto, GitHubBackendModeREST, GitHubBackendModeGH:
		return GitHubBackendMode(value), nil
	default:
		return "", fmt.Errorf("invalid %s %q (want auto, rest, or gh)", GitHubBackendEnv, value)
	}
}

func GitHubBackendModeFromEnv() (GitHubBackendMode, error) {
	return ParseGitHubBackendMode(os.Getenv(GitHubBackendEnv))
}

func SelectGitHubBackend(ctx context.Context, host string) (GitHubBackendSelection, error) {
	return SelectGitHubBackendWithOptions(ctx, host, GitHubBackendSelectionOptions{})
}

func SelectGitHubBackendWithOptions(ctx context.Context, host string, opts GitHubBackendSelectionOptions) (GitHubBackendSelection, error) {
	host = NormalizeHost(host)
	mode, err := gitHubBackendModeForSelection(opts)
	selection := GitHubBackendSelection{Mode: mode, Host: host}
	if err != nil {
		return selection, err
	}

	if mode == GitHubBackendModeGH {
		selection = selectGHBackend(mode, host, "override:gh")
		if customAPIURLActive() {
			return selection, fmt.Errorf("%s is only supported by the rest GitHub backend; unset it or use %s=rest", GitHubBackendAPIURLEnv, GitHubBackendEnv)
		}
		return selection, nil
	}

	token, err := ResolveToken(ctx, PlatformGitHub, host)
	if err == nil {
		source := "auto:token"
		if mode == GitHubBackendModeREST {
			source = "override:rest"
		}
		return selectRESTBackend(mode, host, source, token), nil
	}
	if !errors.Is(err, ErrNoToken) {
		return selection, err
	}

	selection.Token = Token{Host: host}
	if mode == GitHubBackendModeREST {
		selection = selectRESTBackend(mode, host, "override:rest", Token{Host: host})
		selection.Probes = append(selection.Probes, GitHubBackendProbe{Name: GitHubBackendNameREST, Status: "unavailable", Error: ErrNoToken.Error()})
		return selection, fmt.Errorf("rest GitHub backend selected but %w", err)
	}

	selection.Probes = append(selection.Probes, GitHubBackendProbe{Name: GitHubBackendNameREST, Status: "unavailable", Error: ErrNoToken.Error()})
	if customAPIURLActive() {
		return selection, fmt.Errorf("%w; %s requires a rest token because the gh backend cannot use custom API URLs", err, GitHubBackendAPIURLEnv)
	}
	if opts.GHAuthenticated == nil {
		selection.Probes = append(selection.Probes, GitHubBackendProbe{Name: GitHubBackendNameGH, Status: "not_configured", Error: "gh authentication probe is not configured"})
		return selection, fmt.Errorf("%w; gh authentication probe is not configured", err)
	}
	if probeErr := opts.GHAuthenticated(ctx, host); probeErr != nil {
		selection.Probes = append(selection.Probes, GitHubBackendProbe{Name: GitHubBackendNameGH, Status: "unavailable", Error: probeErr.Error()})
		return selection, fmt.Errorf("%w; gh authentication probe failed for %s: %v", err, host, probeErr)
	}
	return selectGHBackend(mode, host, "auto:gh"), nil
}

func gitHubBackendModeForSelection(opts GitHubBackendSelectionOptions) (GitHubBackendMode, error) {
	if opts.Mode != nil {
		mode := *opts.Mode
		if _, err := ParseGitHubBackendMode(string(mode)); err != nil {
			return mode, err
		}
		return mode, nil
	}
	return GitHubBackendModeFromEnv()
}

func (s GitHubBackendSelection) Diagnostics() GitHubBackendDiagnostics {
	return GitHubBackendDiagnostics{
		Mode:            string(s.Mode),
		Name:            s.Name,
		Kind:            s.Kind,
		Host:            s.Host,
		SelectionSource: s.SelectionSource,
		TokenSource:     s.TokenSource,
		Probes:          s.Probes,
	}
}

func (s GitHubBackendSelection) TokenWithDiagnostics() Token {
	token := s.Token
	if token.Host == "" {
		token.Host = s.Host
	}
	if token.Source == "" && s.Name == GitHubBackendNameGH {
		token.Source = "gh"
	}
	diagnostics := s.Diagnostics()
	token.Backend = &diagnostics
	return token
}

func selectRESTBackend(mode GitHubBackendMode, host, selectionSource string, token Token) GitHubBackendSelection {
	token.Host = host
	return GitHubBackendSelection{
		Mode:            mode,
		Name:            GitHubBackendNameREST,
		Kind:            GitHubBackendKindREST,
		Host:            host,
		SelectionSource: selectionSource,
		TokenSource:     token.Source,
		Token:           token,
	}
}

func selectGHBackend(mode GitHubBackendMode, host, selectionSource string) GitHubBackendSelection {
	return GitHubBackendSelection{
		Mode:            mode,
		Name:            GitHubBackendNameGH,
		Kind:            GitHubBackendKindCLI,
		Host:            host,
		SelectionSource: selectionSource,
		Token:           Token{Source: "gh", Host: host},
	}
}

func customAPIURLActive() bool {
	return strings.TrimSpace(os.Getenv(GitHubBackendAPIURLEnv)) != ""
}

// KeyringBackend is the minimal abstraction over the OS keyring that the
// auth package needs. The default value uses zalando/go-keyring directly;
// tests swap it for a deterministic fake so they can exercise both the
// "keyring beats credentials.json" and "credentials.json only" branches
// without touching real OS state.
type KeyringBackend interface {
	Get(service, key string) (string, error)
	Set(service, key, value string) error
	Delete(service, key string) error
}

// keyringBackend is the package-internal alias kept for backwards
// reference inside the auth package itself. External callers should use
// the exported KeyringBackend interface.
type keyringBackend = KeyringBackend

// osKeyringBackend is the production keyringBackend.
type osKeyringBackend struct{}

func (osKeyringBackend) Get(service, key string) (string, error) {
	return keyring.Get(service, key)
}

func (osKeyringBackend) Set(service, key, value string) error {
	return keyring.Set(service, key, value)
}

func (osKeyringBackend) Delete(service, key string) error {
	return keyring.Delete(service, key)
}

// keyringOps is the active keyring backend. Tests swap it with
// swapKeyring; production code never touches it directly.
var keyringOps keyringBackend = osKeyringBackend{}

// SetKeyringBackend replaces the package-level keyring backend for the
// duration of the calling test and returns a cleanup function. Tests in
// other packages that need to isolate from the real OS keyring (for
// example, command-level integration tests) call this directly.
//
// Production code MUST NOT call this; it exists solely to let tests
// exercise the keyring-vs-credentials.json precedence branches without
// depending on (or polluting) the host's real keyring state.
func SetKeyringBackend(backend keyringBackend) func() {
	prev := keyringOps
	keyringOps = backend
	return func() { keyringOps = prev }
}

// ResolveToken resolves a token for the given platform by consulting, in
// order: the platform's environment variables, the OS keyring, and the
// on-disk credentials.json file. The returned Token.Source string uses
// the canonical forms callers depend on:
//
//   - "env:<VARNAME>"  for an env var match (e.g. "env:GITLAB_TOKEN")
//   - "keyring"        for a keyring hit
//   - "config"         for a credentials.json hit (legacy single-token)
//   - "credentials.json:<platform>" for a per-platform section match
//
// Resolution always honours the platform argument: env vars for one
// platform never satisfy a lookup for another, and the credentials.json
// sections are likewise isolated. Legacy single-token entries are
// honoured as GitHub credentials for backward compatibility.
func ResolveToken(_ context.Context, platform Platform, host string) (Token, error) {
	host = normalizeHostFor(platform, host)
	if envName, value, ok := lookupEnvToken(platform); ok {
		return Token{Value: value, Source: "env:" + envName, Host: host}, nil
	}

	if value, err := keyringOps.Get(serviceFor(platform), host); err == nil && strings.TrimSpace(value) != "" {
		return Token{Value: strings.TrimSpace(value), Source: "keyring", Host: host}, nil
	}

	creds, err := readCredentialFile()
	if err != nil {
		return Token{}, err
	}
	if stored, ok := platformCredential(creds, platform, host); ok && strings.TrimSpace(stored.Token) != "" {
		source := credentialsJSONSource(platform)
		return Token{Value: strings.TrimSpace(stored.Token), Source: source, Host: host}, nil
	}

	return Token{Host: host}, ErrNoToken
}

// lookupEnvToken walks the platform's env var chain and returns the first
// non-empty match along with its name.
func lookupEnvToken(platform Platform) (string, string, bool) {
	for _, name := range platform.EnvTokenNames() {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return name, value, true
		}
	}
	return "", "", false
}

// credentialsJSONSource is the Source label ResolveToken uses for a
// per-platform credentials.json hit. It includes the platform so logs
// can tell which section served the token.
func credentialsJSONSource(platform Platform) string {
	return "credentials.json:" + string(platform)
}

// platformCredential pulls the right entry out of a credentialsFile for
// the given platform/host. Per-platform sections take precedence; legacy
// Hosts and Token fields fall through only when no section exists and
// the platform is GitHub.
func platformCredential(creds credentialFile, platform Platform, host string) (StoredCredential, bool) {
	switch platform {
	case PlatformGitLab:
		if creds.GitLab != nil && strings.TrimSpace(creds.GitLab.Host) == host {
			return *creds.GitLab, true
		}
		return StoredCredential{}, false
	default:
		if creds.GitHub != nil && strings.TrimSpace(creds.GitHub.Host) == host {
			return *creds.GitHub, true
		}
		if stored, ok := creds.Hosts[host]; ok {
			return stored, true
		}
		if strings.TrimSpace(creds.Host) == host && strings.TrimSpace(creds.Token) != "" {
			return StoredCredential{Token: creds.Token}, true
		}
		return StoredCredential{}, false
	}
}

// StoreToken stores a token for the given platform/host. When
// insecureStorage is false the token is written to the OS keyring; when
// true it is appended to credentials.json under the platform's section.
// The returned source label is "keyring" or "credentials.json:<platform>".
func StoreToken(_ context.Context, platform Platform, host, token string, insecureStorage bool) (string, error) {
	host = normalizeHostFor(platform, host)
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("token is empty")
	}

	if !insecureStorage {
		if err := keyringOps.Set(serviceFor(platform), host, token); err != nil {
			return "", fmt.Errorf("store token in OS keyring for %s: %w; rerun with --insecure-storage to use explicit plaintext fallback", host, err)
		}
		return "keyring", nil
	}

	creds, err := readCredentialFile()
	if err != nil {
		return "", err
	}
	entry := StoredCredential{Token: token, Host: host}
	switch platform {
	case PlatformGitLab:
		creds.GitLab = &entry
	default:
		creds.GitHub = &entry
	}
	if err := writeCredentialFile(creds); err != nil {
		return "", err
	}
	return credentialsJSONSource(platform), nil
}

// DeleteToken removes every credential associated with the given
// platform/host pair: both the OS keyring entry (under the platform's
// service namespace) and the matching credentials.json section. Other
// platforms' entries are left untouched.
func DeleteToken(_ context.Context, platform Platform, host string) error {
	host = normalizeHostFor(platform, host)
	var errs []error
	if err := keyringOps.Delete(serviceFor(platform), host); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		errs = append(errs, err)
	}

	creds, err := readCredentialFile()
	if err != nil {
		errs = append(errs, err)
		return errors.Join(errs...)
	}
	changed := false
	switch platform {
	case PlatformGitLab:
		if creds.GitLab != nil && strings.TrimSpace(creds.GitLab.Host) == host {
			creds.GitLab = nil
			changed = true
		}
	default:
		if creds.GitHub != nil && strings.TrimSpace(creds.GitHub.Host) == host {
			creds.GitHub = nil
			changed = true
		}
		if _, ok := creds.Hosts[host]; ok {
			delete(creds.Hosts, host)
			changed = true
		}
		if strings.TrimSpace(creds.Host) == host {
			creds.Token = ""
			creds.Host = ""
			changed = true
		}
	}
	if changed {
		if err := writeCredentialFile(creds); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// EnvTokenActive returns the env var name currently set for the given
// platform, or "" if no env var resolves a token. Cross-platform env
// vars are deliberately ignored: GITLAB_TOKEN does not satisfy a GitHub
// lookup, and vice-versa, so the warning surfaced on logout is
// platform-accurate.
func EnvTokenActive(platform Platform) string {
	for _, envName := range platform.EnvTokenNames() {
		if strings.TrimSpace(os.Getenv(envName)) != "" {
			return envName
		}
	}
	return ""
}

func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return "github.com"
	}
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	return host
}

// NormalizeGitLabHost is the GitLab counterpart of NormalizeHost. It
// defaults to "gitlab.com" when given an empty value so callers that
// pass "" get the canonical GitLab instance without having to remember
// the default.
func NormalizeGitLabHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return "gitlab.com"
	}
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	return host
}

// normalizeHostFor routes a host string through the right normalizer for
// the given platform. Callers that already know which platform they are
// resolving a token for should use this rather than guessing.
func normalizeHostFor(platform Platform, host string) string {
	if platform == PlatformGitLab {
		return NormalizeGitLabHost(host)
	}
	return NormalizeHost(host)
}

func ConfigDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("ISSUE_SPEC_CONFIG_DIR")); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "issue-spec"), nil
}

func credentialPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "credentials.json"), nil
}

func readCredentialFile() (credentialFile, error) {
	path, err := credentialPath()
	if err != nil {
		return credentialFile{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return credentialFile{Hosts: map[string]StoredCredential{}}, nil
	}
	if err != nil {
		return credentialFile{}, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return credentialFile{Hosts: map[string]StoredCredential{}}, nil
	}
	var creds credentialFile
	if err := json.Unmarshal(data, &creds); err != nil {
		return credentialFile{}, fmt.Errorf("read issue-spec credentials %s: %w", path, err)
	}
	if creds.Hosts == nil {
		creds.Hosts = map[string]StoredCredential{}
	}
	return creds, nil
}

func writeCredentialFile(creds credentialFile) error {
	path, err := credentialPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(creds, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
