package engine

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	pydanticDeprecatedRegex = regexp.MustCompile(`(?i)(PydanticDeprecatedSince\w+|@validator|field_validator|BaseModel)`)
	sqlSyntaxRegex         = regexp.MustCompile(`(?i)(sqlite3|syntax error|operationalerror|injection)`)
	deadlockRegex          = regexp.MustCompile(`(?i)(deadlock|corrupted|race condition|asyncio\.gather|lock)`)
	attributeErrRegex      = regexp.MustCompile(`(?i)(AttributeError:\s*'[a-zA-Z0-9_]+'\s*object has no attribute\s*'([a-zA-Z0-9_]+)')`)
)

// BuildGroundingQuery creates a high-precision search query for Tavily,
// extracting domain-specific breaking change signatures and error patterns.
func BuildGroundingQuery(workDir, targetFile string, archetype ArchetypeAnalysis, lastError string) string {
	ext := filepath.Ext(targetFile)
	lang := "python"
	if ext == ".go" {
		lang = "go"
	} else if ext == ".ts" || ext == ".js" {
		lang = "typescript"
	}

	// 1. Pydantic v2 breaking migration
	if strings.Contains(lastError, "PydanticDeprecatedSince20") || strings.Contains(lastError, "@validator") {
		return "pydantic v2 migrate validator to field_validator official docs"
	}

	// 2. SQL injection / operational error
	if archetype.Archetype == ArchetypeSecurityDefect || sqlSyntaxRegex.MatchString(lastError) {
		return fmt.Sprintf("%s sqlite3 parameterized query execute prevent sql injection", lang)
	}

	// 3. Concurrency / Deadlock
	if archetype.Archetype == ArchetypeConcurrencyRace || deadlockRegex.MatchString(lastError) {
		return fmt.Sprintf("%s asyncio task reentrant lock prevent deadlock race condition", lang)
	}

	// 4. Null / NoneType / AttributeError
	if m := attributeErrRegex.FindStringSubmatch(lastError); len(m) > 2 {
		return fmt.Sprintf("%s fix AttributeError no attribute %s", lang, m[2])
	}
	if strings.Contains(strings.ToLower(lastError), "nonetype") {
		return fmt.Sprintf("%s handle NoneType optional attribute defensive check", lang)
	}

	// 5. Interface breaking API change
	if archetype.Archetype == ArchetypeInterfaceBreaking {
		cleanBase := filepath.Base(targetFile)
		return fmt.Sprintf("%s %s breaking change migration guide official", lang, cleanBase)
	}

	// 6. Fallback: extract decisive error line
	lines := strings.Split(lastError, "\n")
	decisiveLine := ""
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l != "" && !strings.HasPrefix(l, "Traceback") && !strings.HasPrefix(l, "File ") {
			if len(l) > 60 {
				l = l[:60]
			}
			decisiveLine = l
			break
		}
	}

	if decisiveLine != "" {
		return fmt.Sprintf("%s fix %s %s", lang, archetype.Archetype, decisiveLine)
	}

	return fmt.Sprintf("%s how to resolve %s in %s", lang, archetype.Archetype, filepath.Base(targetFile))
}
