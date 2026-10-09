package engine

import (
	"strings"
	"testing"

	"github.com/kujiu27/nemotron-healer-go/internal/ast"
)

// Hostile LLM responses and tracebacks feed these parsers; none may panic.
var hostileLLMResponses = []string{
	"", "```", "```diff", "```\n", "text without fences",
	"[TARGET_FILE]", "[TARGET_FILE][/TARGET_FILE]", "[TARGET_FILE]a[/TARGET_FILE][TARGET_FILE]b[/TARGET_FILE]",
	"<thought>", "<thought>unclosed", "</thought>",
	strings.Repeat("```diff\n@@\n", 500),
	strings.Repeat("[TARGET_FILE]", 1000),
	"\x00\xff\xfe invalid utf8 \xc3",
	strings.Repeat("a", 1<<20),
}

func TestHostileLLMParsingNeverPanic(t *testing.T) {
	for _, in := range hostileLLMResponses {
		_ = extractDiffBlock(in)
	}
}

var hostileTraces = []string{
	"",
	"File \"\", line 0",
	"File \"x.py\", line",
	strings.Repeat("File \"a.py\", line 1, in f\n", 2000),
	strings.Repeat(":", 10000),
	"\x00trace",
	"at f (a.ts:1:1)\nat g (b.js:\n",
	strings.Repeat("panic: x\n", 5000),
	"github.com/x/y/z.go:0: other",
}

func TestHostileTraceResolutionNeverPanic(t *testing.T) {
	dir := t.TempDir()
	graph := ast.NewCodeGraph(dir)
	for _, tr := range hostileTraces {
		_ = ResolveTargetLocation(dir, tr, graph)
	}
}
