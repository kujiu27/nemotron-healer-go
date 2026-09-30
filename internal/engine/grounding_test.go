package engine

import (
	"strings"
	"testing"
)

func TestBuildGroundingQuery(t *testing.T) {
	// Pydantic case
	pydanticErr := "PydanticDeprecatedSince20: Pydantic V1 style `@validator` validators are deprecated"
	arch := ArchetypeAnalysis{Archetype: ArchetypeInterfaceBreaking}
	q1 := BuildGroundingQuery(".", "models.py", arch, pydanticErr)
	if !strings.Contains(q1, "pydantic v2") {
		t.Fatalf("expected query to contain 'pydantic v2', got: %s", q1)
	}

	// Concurrency case
	deadlockErr := "AssertionError: Corrupted balance: expected 1000, got 750"
	archRace := ArchetypeAnalysis{Archetype: ArchetypeConcurrencyRace}
	q2 := BuildGroundingQuery(".", "account.py", archRace, deadlockErr)
	if !strings.Contains(q2, "asyncio") || !strings.Contains(q2, "race condition") {
		t.Fatalf("expected concurrency query, got: %s", q2)
	}

	// SQL injection case
	sqlErr := "sqlite3.OperationalError: near 'syntax error'"
	archSec := ArchetypeAnalysis{Archetype: ArchetypeSecurityDefect}
	q3 := BuildGroundingQuery(".", "repo.py", archSec, sqlErr)
	if !strings.Contains(q3, "parameterized query") {
		t.Fatalf("expected sql parameterized query, got: %s", q3)
	}
}
