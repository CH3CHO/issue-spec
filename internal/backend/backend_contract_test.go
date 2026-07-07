package backend

import (
	"reflect"
	"testing"
)

// TestBackendMethodCount verifies the interface stays small. Adding a
// method is a breaking change because both platform implementations must
// update in lockstep (PROCESS-002 adds the GitLab backend, the existing
// internal/github/* already satisfies the interface). If this test fails
// after a deliberate refactor, update both platform implementations in the
// same change and bump the expected count here.
func TestBackendMethodCount(t *testing.T) {
	got := methodNamesOfBackend()
	want := 17
	if len(got) != want {
		t.Errorf("Backend method count drifted: got %d, want %d (%v)", len(got), want, got)
	}
}

// TestBackendMethodNames pins the exact method set so a rename is also a
// compile error elsewhere, not just a count drift. The order is not
// significant for the contract; the test only checks the set.
func TestBackendMethodNames(t *testing.T) {
	want := map[string]struct{}{
		// Issues
		"CreateIssue":          {},
		"GetIssue":             {},
		"ListIssueNotes":       {},
		"CreateIssueNote":      {},
		"UpdateIssueNote":      {},
		"ReplyIssueDiscussion": {},
		// Merge requests
		"GetMergeRequest":     {},
		"CreateMergeRequest":  {},
		"UpdateMergeRequest":  {},
		"ListMRFiles":         {},
		"ListMRDiscussions":   {},
		"CreateMRDiscussion":  {},
		"ReplyMRDiscussion":   {},
		"ResolveMRDiscussion": {},
		// CI
		"GetMRPipelineStatus": {},
		"ListMRPipelineJobs":  {},
		// Labels
		"CreateLabel": {},
	}
	got := methodNamesOfBackend()
	gotSet := make(map[string]struct{}, len(got))
	for _, name := range got {
		gotSet[name] = struct{}{}
	}
	for name := range want {
		if _, ok := gotSet[name]; !ok {
			t.Errorf("Backend is missing expected method %q", name)
		}
	}
	for name := range gotSet {
		if _, ok := want[name]; !ok {
			t.Errorf("Backend has unexpected method %q (drift from contract)", name)
		}
	}
}

// methodNamesOfBackend returns the Backend interface's method names in
// declaration order. Reflection on interfaces is the only way to enumerate
// methods in Go; the helper is intentionally unexported so tests in other
// packages cannot accidentally rely on it.
func methodNamesOfBackend() []string {
	t := reflect.TypeOf((*Backend)(nil)).Elem()
	names := make([]string, t.NumMethod())
	for i := 0; i < t.NumMethod(); i++ {
		names[i] = t.Method(i).Name
	}
	return names
}
