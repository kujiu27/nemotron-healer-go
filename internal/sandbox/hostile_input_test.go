package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hostileDiffCorpus feeds the LLM-output-facing parsers and the tier-3 applier
// malformed input of every shape observed from real model output (and worse).
// Contract: never panic; verdicts are (bool, string) either way.
var hostileDiffCorpus = []string{
	"",
	"```diff",
	"```\n```",
	"```diff\n@@ -1 +1 @@\n-a\n+b\n```",
	"--- a/x.go\n+++ b/x.go",
	"--- a/x.go\n+++ b/x.go\n@@ garbage header\n-no context\n+new",
	"@@ -0,0 +0,0 @@",
	"@@ -1,2 +1,2 @@\n+only additions\n",
	"@@ -1,2 +1,2 @@\n-only deletions\n",
	"\\ No newline at end of file",
	"\x00\x01\x02 binary \xff\xfe",
	strings.Repeat("@@ -1 +1 @@\n-a\n+b\n", 1000),
	"--- /dev/null\n+++ b/new.go\n@@ -0,0 +1 @@\n+created",
	"--- a/x\n--- a/y\n--- a/z\n+++ b/x\n@@ -1 +1 @@\n-a\n+b",
	"[TARGET_FILE]../../etc/passwd[/TARGET_FILE]",
	"[TARGET_FILE][/TARGET_FILE]",
	"[TARGET_FILE]no closing",
	"```diff\n@@ -1 +1 @@\n-a\n+" + strings.Repeat("x", 1<<16) + "\n```",
}

func TestHostileDiffParsersNeverPanic(t *testing.T) {
	for _, in := range hostileDiffCorpus {
		_ = ExtractModifiedFiles(in)
		_ = splitDiffIntoFileChunks(in)
		_, _ = replaceHunksInContent("line1\nline2\nline3\n", in)
		p := NewPatcher(t.TempDir())
		_, _ = p.ApplyPatch(in, "target.go")
	}
}

func FuzzApplyPatch(f *testing.F) {
	for _, seed := range hostileDiffCorpus {
		f.Add(seed)
	}
	f.Add("--- a/f.go\n+++ b/f.go\n@@ -1,2 +1,2 @@\n line1\n-old\n+new\n")
	f.Fuzz(func(t *testing.T, diff string) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "f.go"), []byte("line1\nold\nline3\n"), 0644); err != nil {
			t.Skip()
		}
		p := NewPatcher(dir)
		_, _ = p.ApplyPatch(diff, "f.go") // must not panic; any verdict fine
	})
}
