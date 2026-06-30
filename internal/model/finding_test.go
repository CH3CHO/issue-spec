package model

import (
	"strings"
	"testing"
)

func TestRenderAndFindFindingMarker(t *testing.T) {
	body, err := RenderFindingBody("Review", "FINDING-001", "p1", "PROCESS-001", "SPEC-001", "https://github.com/o/r/issues/1#issuecomment-1", "Fix this.", "unknown", "internal/foo.go", 12)
	if err != nil {
		t.Fatal(err)
	}
	marker, ok, err := FindFindingMarker(body)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("missing finding marker")
	}
	if marker.ID != "FINDING-001" || marker.Severity != "P1" || marker.Status != "open" || marker.Path != "internal/foo.go" || marker.Line != 12 {
		t.Fatalf("unexpected marker: %+v", marker)
	}
	if !strings.Contains(body, "Type: FINDING") || !strings.Contains(body, "Status: open") {
		t.Fatalf("unexpected body:\n%s", body)
	}
}

func TestRenderAndFindFindingReplyMarker(t *testing.T) {
	body, err := RenderFindingReplyBody("Worker", "FINDING-001", "PROCESS-001", "fixed", "Fixed.")
	if err != nil {
		t.Fatal(err)
	}
	marker, ok, err := FindFindingReplyMarker(body)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("missing finding reply marker")
	}
	if marker.Finding != "FINDING-001" || marker.Process != "PROCESS-001" || marker.Status != "fixed" {
		t.Fatalf("unexpected marker: %+v", marker)
	}
	if !IsTerminalFindingStatus(marker.Status) {
		t.Fatalf("expected terminal status: %+v", marker)
	}
}
