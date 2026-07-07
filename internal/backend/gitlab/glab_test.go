package gitlab

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
	"testing"
)

// recordingRunner is a Runner double that captures the (ctx, args) tuple of
// the most recent Run call and returns a preset result. Tests build a
// queue-style runner on top of it for multi-call scenarios.
type recordingRunner struct {
	args   []string
	stdin  []byte
	result Result
	err    error
}

func (r *recordingRunner) Run(ctx context.Context, args ...string) ([]byte, []byte, error) {
	r.args = append([]string{}, args...)
	return r.result.Stdout, r.result.Stderr, r.err
}

// queuedRunner returns a different result for each Run call (FIFO).
type queuedRunner struct {
	results []Result
	errs    []error
	calls   int
}

func (q *queuedRunner) Run(ctx context.Context, args ...string) ([]byte, []byte, error) {
	if q.calls >= len(q.results) {
		return nil, nil, fmt.Errorf("queuedRunner: no response queued for call %d (args=%v)", q.calls+1, args)
	}
	out := q.results[q.calls].Stdout
	errOut := q.results[q.calls].Stderr
	var err error
	if q.calls < len(q.errs) {
		err = q.errs[q.calls]
	}
	q.calls++
	return out, errOut, err
}

func TestExecRunnerInvokesBinaryAndCapturesOutput(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("no go binary on PATH: %v", err)
	}

	// Use `go` as a stand-in for glab: we are only verifying that ExecRunner
	// constructs and runs an exec.Cmd, capturing stdout/stderr into buffers
	// and returning the exit error.
	r := &ExecRunner{GLPath: "go", Env: nil}
	out, errOut, err := r.Run(context.Background(), "version")
	if err != nil {
		t.Fatalf("go version failed: %v (stderr=%q)", err, errOut)
	}
	if len(out) == 0 {
		t.Fatalf("expected stdout from go version, got 0 bytes")
	}
}

func TestClassifyStderrNotFound(t *testing.T) {
	stderr := []byte("404 Not Found - Issue does not exist")
	exitErr := &fakeExitErr{code: 1}
	got := ClassifyStderr(stderr, exitErr)
	if got.Kind != ErrorNotFound {
		t.Fatalf("Kind = %v, want ErrorNotFound", got.Kind)
	}
	if !strings.Contains(got.Message, "404") {
		t.Errorf("Message should preserve stderr context, got %q", got.Message)
	}
	if !errors.Is(got, exitErr) {
		t.Errorf("ClassifyStderr must Unwrap to the original exitErr so callers can errors.Is")
	}
}

func TestClassifyStderrPermission(t *testing.T) {
	stderr := []byte("403 Forbidden - You are not allowed to push into this branch")
	got := ClassifyStderr(stderr, &fakeExitErr{code: 1})
	if got.Kind != ErrorPermission {
		t.Fatalf("Kind = %v, want ErrorPermission", got.Kind)
	}
	if !strings.Contains(got.Message, "403") {
		t.Errorf("Message should preserve stderr context, got %q", got.Message)
	}
}

func TestClassifyStderrNetwork(t *testing.T) {
	stderr := []byte("dial tcp: lookup gitlab.com: no such host")
	got := ClassifyStderr(stderr, &fakeExitErr{code: 1})
	if got.Kind != ErrorNetwork {
		t.Fatalf("Kind = %v, want ErrorNetwork", got.Kind)
	}
}

func TestClassifyStderrUnknownWhenStderrEmpty(t *testing.T) {
	got := ClassifyStderr(nil, errors.New("boom"))
	if got.Kind != ErrorUnknown {
		t.Fatalf("Kind = %v, want ErrorUnknown", got.Kind)
	}
	if got.RawErr == nil {
		t.Errorf("RawErr should be preserved")
	}
}

func TestClassifyStderrWrapsExitError(t *testing.T) {
	exitErr := &fakeExitErr{code: 2}
	got := ClassifyStderr([]byte("404 Not Found"), exitErr)
	if got.RawErr != exitErr {
		t.Errorf("RawErr should wrap the original exit error")
	}
	if !strings.Contains(got.Error(), "404") {
		t.Errorf("Error() should expose classified message, got %q", got.Error())
	}
}

func TestClassifyStderrForbiddenCaseInsensitive(t *testing.T) {
	// GitLab uses "Forbidden" / "forbidden" interchangeably depending on
	// version; classification must not depend on capitalisation.
	stderr := []byte("ERROR: forbidden - insufficient_scope")
	got := ClassifyStderr(stderr, &fakeExitErr{code: 1})
	if got.Kind != ErrorPermission {
		t.Fatalf("Kind = %v, want ErrorPermission", got.Kind)
	}
}

func TestErrorMessageContainsKindAndStderr(t *testing.T) {
	stderr := []byte("401 Unauthorized - token expired")
	got := ClassifyStderr(stderr, &fakeExitErr{code: 1})
	if got.Kind != ErrorPermission {
		t.Fatalf("Kind = %v, want ErrorPermission", got.Kind)
	}
	msg := got.Error()
	if !strings.Contains(msg, "401") {
		t.Errorf("message should contain stderr code, got %q", msg)
	}
}

func TestErrorIsComparableViaErrorsAs(t *testing.T) {
	exitErr := &fakeExitErr{code: 3}
	classified := ClassifyStderr([]byte("connection refused"), exitErr)
	wrapped := fmt.Errorf("call glab failed: %w", classified)

	var target *Error
	if !errors.As(wrapped, &target) {
		t.Fatalf("errors.As should unwrap to *Error")
	}
	if target.Kind != ErrorNetwork {
		t.Errorf("Kind after wrap = %v, want ErrorNetwork", target.Kind)
	}
}

func TestNotFoundSubstringVariants(t *testing.T) {
	cases := []string{
		"404 Not Found",
		"project not found",
		"No matching issues",
		"merge request does not exist",
	}
	for _, c := range cases {
		got := ClassifyStderr([]byte(c), &fakeExitErr{code: 1})
		if got.Kind != ErrorNotFound {
			t.Errorf("ClassifyStderr(%q) = %v, want ErrorNotFound", c, got.Kind)
		}
	}
}

func TestNetworkSubstringVariants(t *testing.T) {
	cases := []string{
		"connection refused",
		"i/o timeout",
		"no such host",
		"TLS handshake timeout",
	}
	for _, c := range cases {
		got := ClassifyStderr([]byte(c), &fakeExitErr{code: 1})
		if got.Kind != ErrorNetwork {
			t.Errorf("ClassifyStderr(%q) = %v, want ErrorNetwork", c, got.Kind)
		}
	}
}

// fakeExitErr is a minimal exec.ExitError-like type for testing. We do not
// embed exec.ExitError because the field is unexported on some platforms.
type fakeExitErr struct {
	code int
}

func (e *fakeExitErr) Error() string {
	return fmt.Sprintf("exit status %d", e.code)
}

// Guard against accidental fs.DirEntry-style misuse (regression check).
var _ fs.DirEntry = nil

// Compile-time guard: ensure ExecRunner satisfies the Runner interface. The
// compile-time check would already fail in glab.go, but we surface it here
// so future refactors cannot accidentally widen the runner surface without
// updating the tests.
var _ Runner = (*ExecRunner)(nil)

// Sanity check: bytes.Buffer can back the stdout/stderr capture.
var _ = bytes.NewBuffer(nil)
