package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	pydanticDeprecatedRegex = regexp.MustCompile(`(?i)(PydanticDeprecatedSince\w+|@validator|field_validator|BaseModel)`)
	sqlSyntaxRegex          = regexp.MustCompile(`(?i)(sqlite3|syntax error|operationalerror|injection)`)
	deadlockRegex           = regexp.MustCompile(`(?i)(deadlock|corrupted|race condition|asyncio\.gather|lock)`)
	attributeErrRegex       = regexp.MustCompile(`(?i)(AttributeError:\s*'[a-zA-Z0-9_]+'\s*object has no attribute\s*'([a-zA-Z0-9_]+)')`)
)

// BuildGroundingQuery creates a high-precision search query for Tavily,
// extracting domain-specific breaking change signatures, manifest dependencies, and error patterns.
func BuildGroundingQuery(workDir, targetFile string, archetype ArchetypeAnalysis, lastError string) string {
	ext := filepath.Ext(targetFile)
	lang := "python"
	if ext == ".go" {
		lang = "go"
	} else if ext == ".ts" || ext == ".js" {
		lang = "typescript"
	}

	lowerErr := strings.ToLower(lastError)

	// Check if any major library version is indicated in error or manifest
	libVersion := detectLibraryVersion(workDir, lastError)

	// 1. Pydantic v2 breaking migration
	if strings.Contains(lastError, "PydanticDeprecatedSince20") || strings.Contains(lastError, "@validator") || strings.Contains(lowerErr, "pydantic") {
		versionTag := "v2"
		if libVersion != "" && strings.Contains(libVersion, "pydantic") {
			versionTag = libVersion
		}
		if strings.Contains(lastError, "@validator") {
			return fmt.Sprintf("pydantic %s migrate validator to field_validator official docs", versionTag)
		}
		return fmt.Sprintf("pydantic %s breaking changes migration guide official docs", versionTag)
	}

	// 2. SQL injection / operational error
	if archetype.Archetype == ArchetypeSecurityDefect || sqlSyntaxRegex.MatchString(lastError) {
		dbEngine := "sqlite3"
		if strings.Contains(lowerErr, "postgres") || strings.Contains(lowerErr, "psycopg") {
			dbEngine = "postgresql"
		} else if strings.Contains(lowerErr, "mysql") {
			dbEngine = "mysql"
		}
		return fmt.Sprintf("%s %s parameterized query execute prevent sql injection", lang, dbEngine)
	}

	// 3. Concurrency / Deadlock
	if archetype.Archetype == ArchetypeConcurrencyRace || deadlockRegex.MatchString(lastError) {
		if lang == "go" {
			return "go sync.Mutex RWMutex prevent race condition deadlock"
		}
		return fmt.Sprintf("%s asyncio task reentrant lock prevent deadlock race condition", lang)
	}

	// 4. Null / NoneType / AttributeError
	if m := attributeErrRegex.FindStringSubmatch(lastError); len(m) > 2 {
		return fmt.Sprintf("%s fix AttributeError no attribute %s", lang, m[2])
	}
	if strings.Contains(lowerErr, "nonetype") || strings.Contains(lowerErr, "nil pointer") {
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

func detectLibraryVersion(workDir, lastError string) string {
	lowerErr := strings.ToLower(lastError)

	// Manifest paths to inspect
	manifests := []string{
		filepath.Join(workDir, "pyproject.toml"),
		filepath.Join(workDir, "requirements.txt"),
		filepath.Join(workDir, "go.mod"),
		filepath.Join(workDir, "package.json"),
	}

	for _, m := range manifests {
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		content := strings.ToLower(string(data))

		if strings.Contains(lowerErr, "pydantic") && strings.Contains(content, "pydantic") {
			if strings.Contains(content, "pydantic>=2") || strings.Contains(content, "pydantic==2") || strings.Contains(content, "pydantic ~=") {
				return "v2"
			}
			return "v2"
		}
		if strings.Contains(lowerErr, "fastapi") && strings.Contains(content, "fastapi") {
			return "fastapi"
		}
		if strings.Contains(lowerErr, "sqlalchemy") && strings.Contains(content, "sqlalchemy") {
			if strings.Contains(content, "sqlalchemy>=2") || strings.Contains(content, "sqlalchemy==2") {
				return "v2"
			}
		}
	}

	return ""
}
