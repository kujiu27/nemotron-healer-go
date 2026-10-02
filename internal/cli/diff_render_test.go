package cli

import (
	"bufio"
	"strings"
	"testing"
)

func TestRenderColorizedDiff(t *testing.T) {
	diff := `--- a/math.py
+++ b/math.py
@@ -1,3 +1,3 @@
 def add(a, b):
-    return a - b
+    return a + b
`
	rendered := RenderColorizedDiff(diff)
	if rendered == "" {
		t.Fatalf("expected non-empty rendered diff")
	}

	// Verify all parts of the diff are preserved
	if !strings.Contains(rendered, "--- a/math.py") {
		t.Errorf("rendered output missing header line")
	}
	if !strings.Contains(rendered, "@@ -1,3 +1,3 @@") {
		t.Errorf("rendered output missing hunk line")
	}
	if !strings.Contains(rendered, "-    return a - b") {
		t.Errorf("rendered output missing deleted line")
	}
	if !strings.Contains(rendered, "+    return a + b") {
		t.Errorf("rendered output missing added line")
	}

	// Empty diff handling
	if RenderColorizedDiff("") != "" {
		t.Errorf("expected empty string for empty input")
	}
}

func TestRenderPatchProvenance(t *testing.T) {
	digest := "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	model := "nvidia/Nemotron-3-Ultra-550b-a55b"
	platform := "Nebius Token Factory"

	box := RenderPatchProvenance(digest, model, platform)
	if !strings.Contains(box, digest) {
		t.Errorf("provenance box missing digest")
	}
	if !strings.Contains(box, model) {
		t.Errorf("provenance box missing model")
	}
	if !strings.Contains(box, "X-Nemotron-Audit") {
		t.Errorf("provenance box missing trailer reference")
	}

	// Empty digest handling
	if RenderPatchProvenance("", model, platform) != "" {
		t.Errorf("expected empty string when digest is empty")
	}
}

func TestPromptConfirmation(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"\n", true},
		{"y\n", true},
		{"Y\n", true},
		{"yes\n", true},
		{"YES\n", true},
		{"n\n", false},
		{"N\n", false},
		{"no\n", false},
		{"cancel\n", false},
	}

	for _, tt := range tests {
		reader := bufio.NewReader(strings.NewReader(tt.input))
		res := PromptConfirmation("Confirm? [Y/n]: ", reader)
		if res != tt.expected {
			t.Errorf("input %q: expected %v, got %v", tt.input, tt.expected, res)
		}
	}
}
