package engine

import (
	"testing"
)

func TestClassifyDefect(t *testing.T) {
	cases := []struct {
		name        string
		trace       string
		src         string
		expected    DefectArchetype
	}{
		{
			name:     "Python Concurrency Deadlock",
			trace:    "AssertionError: deadlock occurred corrupted balance",
			expected: ArchetypeConcurrencyRace,
		},
		{
			name:     "Rust Concurrency Borrow Mut",
			trace:    "thread 'main' panicked at 'cannot borrow as mutable', src/lib.rs:10",
			expected: ArchetypeConcurrencyRace,
		},
		{
			name:     "Python Pydantic Deprecated",
			trace:    "PydanticUserError: validator is deprecated, removed in V2",
			expected: ArchetypeInterfaceBreaking,
		},
		{
			name:     "Node Module Not Found",
			trace:    "Error: Cannot find module 'lodash'",
			expected: ArchetypeInterfaceBreaking,
		},
		{
			name:     "SQL Injection Syntax",
			trace:    "syntax error at or near 'SELECT'",
			expected: ArchetypeSecurityDefect,
		},
		{
			name:     "Go Too Many Open Files",
			trace:    "open /tmp/file: too many open files",
			expected: ArchetypeResourceLeak,
		},
		{
			name:     "Go Nil Pointer",
			trace:    "panic: runtime error: invalid memory address or nil pointer dereference",
			expected: ArchetypeNullTypeError,
		},
		{
			name:     "JS Cannot Read Property Undefined",
			trace:    "TypeError: Cannot read properties of undefined (reading 'status')",
			expected: ArchetypeNullTypeError,
		},
		{
			name:     "Rust Unwrap on None",
			trace:    "thread 'main' panicked at 'called `Option::unwrap()` on a `None` value', src/main.rs:4:5",
			expected: ArchetypeNullTypeError,
		},
		{
			name:     "General Assertion",
			trace:    "assert x == 42",
			expected: ArchetypeGeneralAssertion,
		},
		{
			name:     "Go Test Timeout",
			trace:    "panic: test timed out after 30s\ngoroutine 18 [chan receive]:",
			expected: ArchetypeConcurrencyRace,
		},
		{
			name:     "Command Timeout Injection",
			trace:    "Command timed out after 60s: go test -race .",
			expected: ArchetypeConcurrencyRace,
		},
		{
			name:     "Goroutines Asleep Deadlock",
			trace:    "fatal error: all goroutines are asleep - deadlock!",
			expected: ArchetypeConcurrencyRace,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := ClassifyDefect(tc.trace, tc.src)
			if res.Archetype != tc.expected {
				t.Fatalf("expected archetype %s, got %s", tc.expected, res.Archetype)
			}
		})
	}
}
