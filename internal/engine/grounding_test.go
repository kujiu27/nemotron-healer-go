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

	// Rust concurrency case
	q4 := BuildGroundingQuery(".", "lib.rs", archRace, "cannot borrow as mutable deadlock")
	if !strings.Contains(q4, "rust") || !strings.Contains(q4, "Mutex") {
		t.Fatalf("expected Rust concurrency query, got: %s", q4)
	}

	// TypeScript concurrency case
	q5 := BuildGroundingQuery(".", "App.tsx", archRace, "deadlock")
	if !strings.Contains(q5, "typescript") || !strings.Contains(q5, "mutex") {
		t.Fatalf("expected TypeScript concurrency query, got: %s", q5)
	}

	// TypeScript null/undefined error case
	q6 := BuildGroundingQuery(".", "index.ts", ArchetypeAnalysis{Archetype: ArchetypeNullTypeError}, "TypeError: Cannot read properties of undefined")
	if !strings.Contains(q6, "typescript") || !strings.Contains(q6, "optional chaining") {
		t.Fatalf("expected TypeScript optional chaining query, got: %s", q6)
	}

	// Rust Option unwrap error case
	q7 := BuildGroundingQuery(".", "main.rs", ArchetypeAnalysis{Archetype: ArchetypeNullTypeError}, "called `Option::unwrap()` on a `None` value")
	if !strings.Contains(q7, "rust") || !strings.Contains(q7, "Option unwrap") {
		t.Fatalf("expected Rust Option unwrap query, got: %s", q7)
	}
}

func TestResolveGroundingDomains(t *testing.T) {
	goDomains := ResolveGroundingDomains("pkg/service.go")
	if len(goDomains) == 0 || goDomains[0] != "pkg.go.dev" {
		t.Errorf("expected Go domains starting with pkg.go.dev, got: %v", goDomains)
	}

	rsDomains := ResolveGroundingDomains("src/main.rs")
	if len(rsDomains) == 0 || rsDomains[0] != "docs.rs" {
		t.Errorf("expected Rust domains starting with docs.rs, got: %v", rsDomains)
	}

	tsDomains := ResolveGroundingDomains("components/Button.tsx")
	if len(tsDomains) == 0 || tsDomains[0] != "developer.mozilla.org" {
		t.Errorf("expected TS domains starting with developer.mozilla.org, got: %v", tsDomains)
	}

	pyDomains := ResolveGroundingDomains("app/models.py")
	if len(pyDomains) == 0 || pyDomains[0] != "docs.python.org" {
		t.Errorf("expected Python domains starting with docs.python.org, got: %v", pyDomains)
	}
}
