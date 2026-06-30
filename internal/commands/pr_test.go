package commands

import (
	"context"
	"testing"

	"github.com/higress-group/issue-spec/internal/github"
	"github.com/higress-group/issue-spec/internal/model"
)

func TestChangedRightLines(t *testing.T) {
	patch := `@@ -1,3 +1,5 @@
 package main
+import "fmt"
 
 func main() {
-	println("x")
+	fmt.Println("x")
 }`
	got := changedRightLines(patch)
	want := []int{2, 5}
	if len(got) != len(want) {
		t.Fatalf("lines = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("lines = %v, want %v", got, want)
		}
	}
}

func TestCreateRationaleIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client := &fakePRClient{
		files: []github.PullRequestFile{{
			Filename: "internal/foo.go",
			Patch: `@@ -1,2 +1,3 @@
 package foo
+var X = 1
`,
		}},
		pr: github.PullRequest{Number: 7},
	}
	client.pr.Head.SHA = "abc123"
	result, err := createRationale(ctx, client, "o/r", 7, "internal/foo.go", 2, "PROCESS-001", "SPEC-001", "https://github.com/o/r/issues/1#issuecomment-1", "Worker Agent A", "Explain why.")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || result.CommentID == 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	result, err = createRationale(ctx, client, "o/r", 7, "internal/foo.go", 2, "PROCESS-001", "SPEC-001", "https://github.com/o/r/issues/1#issuecomment-1", "Worker Agent A", "Explain why.")
	if err != nil {
		t.Fatal(err)
	}
	if result.Created {
		t.Fatalf("expected idempotent existing result: %+v", result)
	}
}

type fakePRClient struct {
	pr       github.PullRequest
	files    []github.PullRequestFile
	comments []github.PullRequestReviewComment
}

func (f *fakePRClient) GetPullRequest(context.Context, string, int) (github.PullRequest, error) {
	return f.pr, nil
}

func (f *fakePRClient) ListPullRequestFiles(context.Context, string, int) ([]github.PullRequestFile, error) {
	return f.files, nil
}

func (f *fakePRClient) ListPullRequestReviewComments(context.Context, string, int) ([]github.PullRequestReviewComment, error) {
	return f.comments, nil
}

func (f *fakePRClient) CreatePullRequestReviewComment(_ context.Context, _ string, _ int, body, commitID, path string, line int, side string) (github.PullRequestReviewComment, error) {
	if commitID == "" || path == "" || line == 0 || side != "RIGHT" {
		panic("invalid create review comment args")
	}
	comment := github.PullRequestReviewComment{
		ID:       int64(len(f.comments) + 1),
		HTMLURL:  "https://github.com/o/r/pull/7#discussion_r1",
		Body:     body,
		Path:     path,
		Line:     line,
		CommitID: commitID,
	}
	marker, ok, err := model.FindRationaleMarker(body)
	if err != nil || !ok || marker.Process != "PROCESS-001" || marker.Spec != "SPEC-001" {
		panic("missing rationale marker")
	}
	f.comments = append(f.comments, comment)
	return comment, nil
}
