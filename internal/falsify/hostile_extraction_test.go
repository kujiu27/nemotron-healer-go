package falsify

import (
	"strings"
	"testing"
)

// The adversarial-test extractor consumes raw model output; hostile shapes
// must yield (possibly empty) strings, never a panic.
var hostileTestBodies = []string{
	"", "```", "```go", "```go\n", "code without fences",
	"```go\nfunc Test(t *testing.T) {\n```",
	strings.Repeat("```go\n", 300),
	"\x00\xff binary \xc3(",
	strings.Repeat("x", 1<<20),
}

func TestHostileTestExtractionNeverPanic(t *testing.T) {
	lang := detectLanguage("pkg/a.go", "go test ./...")
	for _, in := range hostileTestBodies {
		_ = extractTestCode(in, lang)
	}
}
