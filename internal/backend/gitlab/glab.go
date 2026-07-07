// Package gitlab implements the internal/backend.Backend interface for
// GitLab by wrapping the glab CLI (>= 1.30). The package is split into
// two layers:
//
//   - glab.go (this file): the exec.Cmd abstraction + error classification.
//     Tests in glab_test.go exercise these primitives with a fake runner.
//   - backend.go: the platform.Backend implementation. It composes a Runner
//     with the JSON shapes returned by glab and glab api.
//
// The package exposes Runner as the only seam between the two layers so
// tests can swap in a fake without spawning a real glab process.
package gitlab

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Result is a Runner-friendly stand-in for the (stdout, stderr, err)
// triplet returned by Run. Backend code never sees exec.ExitError directly
// — it always inspects stderr and the classified Error.
type Result struct {
	Stdout []byte
	Stderr []byte
}

// Runner abstracts the underlying glab invocation so tests can swap in a
// fake. Implementations MUST honour context cancellation; the production
// ExecRunner does so via exec.CommandContext.
type Runner interface {
	// Run executes glab (or a stand-in) with the given args and returns
	// captured stdout, stderr and an exit error. A non-nil error means
	// glab exited non-zero; the implementation is responsible for
	// honouring context cancellation as a non-zero exit.
	Run(ctx context.Context, args ...string) (stdout []byte, stderr []byte, err error)
}

// ExecRunner is the production Runner that shells out to a glab binary.
// The binary path is configurable so callers can override it for
// self-hosted glab installs or for tests.
type ExecRunner struct {
	// GLPath is the glab executable. Empty defaults to "glab".
	GLPath string
	// Env is the environment passed to glab; nil inherits os.Environ().
	Env []string
}

// Run shells out to the configured glab binary with exec.CommandContext,
// capturing stdout and stderr into byte buffers. Returns the underlying
// exit error verbatim; callers wrap it through ClassifyStderr.
func (e *ExecRunner) Run(ctx context.Context, args ...string) ([]byte, []byte, error) {
	bin := strings.TrimSpace(e.GLPath)
	if bin == "" {
		bin = "glab"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = e.Env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// -----------------------------------------------------------------------------
// Error classification
// -----------------------------------------------------------------------------

// ErrorKind distinguishes failure modes so command code can pick a useful
// recovery path (retry on network, prompt for auth on permission, surface
// a friendly error on not-found).
type ErrorKind int

const (
	// ErrorUnknown is the zero value and means we could not classify the
	// failure.
	ErrorUnknown ErrorKind = iota
	// ErrorNotFound covers 404s and "does not exist" messages.
	ErrorNotFound
	// ErrorPermission covers 401/403 and auth-related failures.
	ErrorPermission
	// ErrorNetwork covers DNS / dial / TLS / timeout errors.
	ErrorNetwork
)

// String renders a human-readable label for the kind. Useful in error
// messages and log lines.
func (k ErrorKind) String() string {
	switch k {
	case ErrorNotFound:
		return "not_found"
	case ErrorPermission:
		return "permission"
	case ErrorNetwork:
		return "network"
	default:
		return "unknown"
	}
}

// Error is the typed error returned by every Backend method that hits glab.
// It carries enough information for callers to classify failures with
// errors.As without re-parsing stderr.
type Error struct {
	// Kind classifies the failure mode.
	Kind ErrorKind
	// Message is the human-readable description. Always non-empty.
	Message string
	// RawErr is the underlying exec error (typically *exec.ExitError).
	// Callers can errors.Is(err, &exec.ExitError{}) to inspect exit
	// metadata. May be nil for synthetic errors.
	RawErr error
}

// Error renders the message. The Kind is deliberately omitted from the
// default string so log lines stay compact; callers wanting the kind should
// use errors.As.
func (e *Error) Error() string { return e.Message }

// Unwrap exposes RawErr to errors.Is / errors.As chains.
func (e *Error) Unwrap() error { return e.RawErr }

// notFoundMarkers / permissionMarkers / networkMarkers are the substrings
// that ClassifyStderr matches case-insensitively. The lists are kept short
// on purpose; classification should be conservative — an unknown failure
// surfaces as ErrorUnknown so the user sees the original stderr.
var (
	notFoundMarkers = []string{
		"404",
		"not found",
		"does not exist",
		"no matching",
	}
	permissionMarkers = []string{
		"401",
		"403",
		"forbidden",
		"unauthorized",
		"insufficient_scope",
	}
	networkMarkers = []string{
		"connection refused",
		"connection reset",
		"i/o timeout",
		"no such host",
		"tls handshake",
		"network is unreachable",
	}
)

// ClassifyStderr inspects glab's stderr (and the optional exit error) and
// returns a typed *Error. The original error is always preserved via
// RawErr so errors.Is works.
//
// The classification is deliberately keyword-based rather than HTTP-status
// based because glab sometimes wraps the status in a human-readable
// message ("merge request does not exist") and because non-API commands
// (auth status, mr note resolve) never return HTTP at all.
func ClassifyStderr(stderr []byte, exitErr error) *Error {
	raw := strings.ToLower(string(stderr))
	kind := ErrorUnknown
	for _, m := range notFoundMarkers {
		if strings.Contains(raw, m) {
			kind = ErrorNotFound
			break
		}
	}
	if kind == ErrorUnknown {
		for _, m := range permissionMarkers {
			if strings.Contains(raw, m) {
				kind = ErrorPermission
				break
			}
		}
	}
	if kind == ErrorUnknown {
		for _, m := range networkMarkers {
			if strings.Contains(raw, m) {
				kind = ErrorNetwork
				break
			}
		}
	}

	msg := strings.TrimSpace(string(stderr))
	if msg == "" && exitErr != nil {
		msg = exitErr.Error()
	}
	if msg == "" {
		msg = "glab command failed"
	}
	return &Error{
		Kind:    kind,
		Message: fmt.Sprintf("glab: %s: %s", kind, msg),
		RawErr:  exitErr,
	}
}
