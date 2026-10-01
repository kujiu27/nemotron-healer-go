package engine

import (
	"strings"
)

type DefectArchetype string

const (
	ArchetypeConcurrencyRace      DefectArchetype = "ConcurrencyRace"
	ArchetypeInterfaceBreaking    DefectArchetype = "InterfaceBreaking"
	ArchetypeSecurityDefect       DefectArchetype = "SecurityDefect"
	ArchetypeResourceLeak         DefectArchetype = "ResourceLeak"
	ArchetypeNullTypeError        DefectArchetype = "NullTypeError"
	ArchetypeGeneralAssertion     DefectArchetype = "GeneralAssertion"
)

type ArchetypeAnalysis struct {
	Archetype       DefectArchetype `json:"archetype"`
	Severity        string          `json:"severity"` // "CRITICAL", "HIGH", "MEDIUM"
	Description     string          `json:"description"`
	Constraints     []string        `json:"constraints"`
	ExemplarPattern string          `json:"exemplar_pattern,omitempty"`
}

// ClassifyDefect uses deterministic rules (inspired by Alibaba Open Code Review)
// to categorize failures before LLM inference, injecting domain-specific negative constraints and few-shot exemplars.
func ClassifyDefect(trace string, sourceContext string) ArchetypeAnalysis {
	lowerTrace := strings.ToLower(trace)
	lowerSource := strings.ToLower(sourceContext)

	// 1. Concurrency & Race Condition
	if strings.Contains(lowerTrace, "corrupted") ||
		strings.Contains(lowerTrace, "race") ||
		strings.Contains(lowerTrace, "concurrent") ||
		strings.Contains(lowerTrace, "deadlock") ||
		(strings.Contains(lowerSource, "asyncio.gather") && strings.Contains(lowerTrace, "assertionerror")) {
		return ArchetypeAnalysis{
			Archetype:   ArchetypeConcurrencyRace,
			Severity:    "CRITICAL",
			Description: "Non-atomic shared state mutation under concurrent async/threaded execution.",
			Constraints: []string{
				"MUST initialize an atomic synchronization primitive (e.g. asyncio.Lock / sync.Mutex / sync.RWMutex).",
				"MUST enclose shared mutable state mutations inside synchronous/async lock scopes (RAII pattern).",
				"DO NOT remove concurrency or delete concurrent task gatherers to fake a test pass.",
			},
			ExemplarPattern: `[ARCHITECTURAL PATTERN EXEMPLAR (CONCURRENCY)]
Go:
    type SafeState struct {
        mu sync.RWMutex
        store map[string]int
    }
    func (s *SafeState) Update(k string, v int) {
        s.mu.Lock()
        defer s.mu.Unlock()
        s.store[k] = v
    }
Python:
    class SafeStore:
        def __init__(self):
            self._lock = asyncio.Lock()
            self._data = {}
        async def update(self, k, v):
            async with self._lock:
                self._data[k] = v`,
		}
	}

	// 2. Breaking Interface & Deprecated API
	if strings.Contains(lowerTrace, "deprecated") ||
		strings.Contains(lowerTrace, "removed in") ||
		strings.Contains(lowerTrace, "migration") ||
		strings.Contains(lowerTrace, "has been renamed") ||
		strings.Contains(lowerTrace, "validationerror") {
		return ArchetypeAnalysis{
			Archetype:   ArchetypeInterfaceBreaking,
			Severity:    "HIGH",
			Description: "Interface incompatibility caused by library upgrade or deprecated signature.",
			Constraints: []string{
				"MUST follow modern upstream migration guide retrieved via Tavily.",
				"DO NOT downgrade dependencies or suppress deprecation warnings with warnings.filterwarnings.",
				"MUST preserve backward-compatible field aliases where expected.",
			},
			ExemplarPattern: `[ARCHITECTURAL PATTERN EXEMPLAR (API MIGRATION)]
Pydantic v1 -> v2:
    from pydantic import field_validator # replaces deprecated @validator
    @field_validator('my_field')
    @classmethod
    def validate_field(cls, v):
        return v.strip()`,
		}
	}

	// 3. Security Vulnerability (Injection / Sanitization)
	if strings.Contains(lowerTrace, "injection") ||
		strings.Contains(lowerTrace, "syntax error at or near") ||
		strings.Contains(lowerSource, "execute(") && strings.Contains(lowerSource, "%") {
		return ArchetypeAnalysis{
			Archetype:   ArchetypeSecurityDefect,
			Severity:    "CRITICAL",
			Description: "Untrusted input concatenated into SQL/Command string without parameterization.",
			Constraints: []string{
				"MUST use parameterized queries or prepared statements (e.g. %s, ? or positional bindings).",
				"NEVER use f-strings or format() to construct raw queries.",
				"MUST validate and sanitize all external arguments before execution.",
			},
			ExemplarPattern: `[ARCHITECTURAL PATTERN EXEMPLAR (PARAMETRIZED SECURITY)]
SQL:
    # Use tuple parameter bindings instead of string formatting
    cursor.execute("SELECT * FROM users WHERE username = %s AND role = %s", (username, role))`,
		}
	}

	// 4. Resource Leak / Unclosed handles
	if strings.Contains(lowerTrace, "unclosed") ||
		strings.Contains(lowerTrace, "resourcewarning") ||
		strings.Contains(lowerTrace, "file descriptor") ||
		strings.Contains(lowerTrace, "connection pool exhausted") {
		return ArchetypeAnalysis{
			Archetype:   ArchetypeResourceLeak,
			Severity:    "HIGH",
			Description: "File handle, socket, or database connection not properly closed.",
			Constraints: []string{
				"MUST use context managers (with / async with) or defer cleanup.",
				"MUST ensure connection release in finally / defer blocks.",
			},
			ExemplarPattern: `[ARCHITECTURAL PATTERN EXEMPLAR (RESOURCE CLEANUP)]
Go:
    f, err := os.Open(path)
    if err != nil { return err }
    defer f.Close()
Python:
    async with aiohttp.ClientSession() as session:
        async with session.get(url) as resp:
            data = await resp.text()`,
		}
	}

	// 5. Null Pointer / Type Error
	if strings.Contains(lowerTrace, "nonetype") ||
		strings.Contains(lowerTrace, "nil pointer") ||
		strings.Contains(lowerTrace, "nullpointerexception") ||
		strings.Contains(lowerTrace, "attributeerror") {
		return ArchetypeAnalysis{
			Archetype:   ArchetypeNullTypeError,
			Severity:    "MEDIUM",
			Description: "Accessing attribute or method on uninitialized or null reference.",
			Constraints: []string{
				"MUST introduce null-check guard clauses before property access.",
				"Provide sensible fallback default values if reference is null.",
			},
			ExemplarPattern: `[ARCHITECTURAL PATTERN EXEMPLAR (GUARD CLAUSE)]
Go:
    if item == nil {
        return DefaultItem()
    }
Python:
    if obj is None:
        return default_val`,
		}
	}

	// Default: General Assertion
	return ArchetypeAnalysis{
		Archetype:   ArchetypeGeneralAssertion,
		Severity:    "MEDIUM",
		Description: "Logical invariant broken in unit test assertion.",
		Constraints: []string{
			"Repair the calculation logic without modifying the test assertion values.",
			"Ensure mathematical symmetry and boundary safety.",
		},
	}
}
