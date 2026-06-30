package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientCreatesAndListsComments(t *testing.T) {
	var createdBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization header = %q", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues/1/comments":
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			createdBody = req["body"]
			json.NewEncoder(w).Encode(Comment{ID: 10, HTMLURL: "https://github.com/o/r/issues/1#issuecomment-10", URL: serverURL(r) + "/repos/o/r/issues/comments/10", Body: createdBody})
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/issues/1/comments":
			if r.URL.Query().Get("per_page") != "100" {
				t.Fatalf("missing pagination query: %s", r.URL.RawQuery)
			}
			json.NewEncoder(w).Encode([]Comment{{ID: 10, HTMLURL: "https://github.com/o/r/issues/1#issuecomment-10", Body: createdBody}})
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	client := NewClientWithBaseURL("github.com", server.URL, "token", server.Client())
	created, err := client.CreateComment(context.Background(), "o/r", 1, "body")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != 10 || createdBody != "body" {
		t.Fatalf("unexpected create result: %+v body=%q", created, createdBody)
	}
	comments, err := client.ListIssueComments(context.Background(), "o/r", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].ID != 10 {
		t.Fatalf("unexpected comments: %+v", comments)
	}
}

func TestParseIssueNumberFromURL(t *testing.T) {
	n, err := ParseIssueNumber("https://github.com/o/r/issues/123")
	if err != nil {
		t.Fatal(err)
	}
	if n != 123 {
		t.Fatalf("number = %d", n)
	}
}

func serverURL(r *http.Request) string {
	return "http://" + strings.TrimSuffix(r.Host, "/")
}
