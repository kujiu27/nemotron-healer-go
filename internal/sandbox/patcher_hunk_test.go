package sandbox

import (
	"strings"
	"testing"
)

const multiHunkFile = `package main

func A() int {
	return 1
}

func B() int {
	return 2
}
`

// Two hunks touching non-contiguous regions of the same file. The previous
// implementation concatenated both hunks into a single old block, which never
// matched, so every multi-hunk diff fell through the fallback.
const multiHunkDiff = `--- a/main.go
+++ b/main.go
@@ -3,3 +3,3 @@
 func A() int {
-	return 1
+	return 10
 }
@@ -7,3 +7,3 @@
 func B() int {
-	return 2
+	return 20
 }
`

func TestReplaceHunksMultiHunkNonContiguous(t *testing.T) {
	got, ok := replaceHunksInContent(multiHunkFile, multiHunkDiff)
	if !ok {
		t.Fatal("multi-hunk diff should apply")
	}
	if !strings.Contains(got, "return 10") || !strings.Contains(got, "return 20") {
		t.Fatalf("both hunks must apply, got:\n%s", got)
	}
	if strings.Contains(got, "return 1\n") || strings.Contains(got, "return 2\n") {
		t.Fatalf("old lines must be gone, got:\n%s", got)
	}
}

func TestReplaceHunksSingleHunk(t *testing.T) {
	diff := `@@ -3,3 +3,3 @@
 func A() int {
-	return 1
+	return 42
 }
`
	got, ok := replaceHunksInContent(multiHunkFile, diff)
	if !ok || !strings.Contains(got, "return 42") {
		t.Fatalf("single hunk should apply, ok=%v got:\n%s", ok, got)
	}
}

func TestReplaceHunksNoNewlineMarker(t *testing.T) {
	content := "package main\n\nfunc A() int {\n\treturn 1\n}"
	diff := `@@ -4,2 +4,2 @@
-	return 1
-}
+	return 42
+}
\ No newline at end of file
`
	got, ok := replaceHunksInContent(content, diff)
	if !ok || !strings.Contains(got, "return 42") {
		t.Fatalf("no-newline marker must not break matching, ok=%v got:\n%s", ok, got)
	}
}

func TestReplaceHunksMissingContextFails(t *testing.T) {
	diff := `@@ -1,2 +1,2 @@
-does not exist here
+new line
`
	if got, ok := replaceHunksInContent(multiHunkFile, diff); ok {
		t.Fatalf("hunk with absent context must fail, got:\n%s", got)
	}
}

func TestReplaceHunksPureInsertionRejected(t *testing.T) {
	// '+' lines only: no anchor to locate the insertion point.
	diff := `@@ -0,0 +1,1 @@
+import "fmt"
`
	if _, ok := replaceHunksInContent(multiHunkFile, diff); ok {
		t.Fatal("pure insertion without context must be rejected, not guessed")
	}
}
