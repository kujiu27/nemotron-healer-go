package engine

import (
	"fmt"
	"path/filepath"
)

type MCTSHypothesis struct {
	Name     string
	Guidance string
}

// GenerateMCTSHypotheses produces divergent, archetype-specific architectural repair hypotheses
// for Test-Time Compute (TTC) Monte Carlo Tree Search.
func GenerateMCTSHypotheses(archetype ArchetypeAnalysis, targetFile, targetSym, failingOutput string) []MCTSHypothesis {
	baseFile := filepath.Base(targetFile)
	symDesc := ""
	if targetSym != "" {
		symDesc = fmt.Sprintf("in symbol `%s` of %s", targetSym, baseFile)
	} else {
		symDesc = fmt.Sprintf("in %s", baseFile)
	}

	switch archetype.Archetype {
	case ArchetypeConcurrencyRace:
		return []MCTSHypothesis{
			{
				Name:     "Hypothesis A: Re-entrant Task-Aware Synchronization",
				Guidance: fmt.Sprintf("Implement task-aware or re-entrant locking %s to prevent race conditions and deadlocks under concurrent coroutine execution.", symDesc),
			},
			{
				Name:     "Hypothesis B: Decoupled Critical Scope Isolation",
				Guidance: fmt.Sprintf("Decouple background logging and external I/O outside the critical locked section %s to eliminate blocking contention.", symDesc),
			},
			{
				Name:     "Hypothesis C: Atomic Invariant Guard with Mutex",
				Guidance: fmt.Sprintf("Guard mutable shared state %s strictly using an atomic context manager or mutex before modifying attributes.", symDesc),
			},
		}

	case ArchetypeInterfaceBreaking:
		return []MCTSHypothesis{
			{
				Name:     "Hypothesis A: Modern Upstream API Migration",
				Guidance: fmt.Sprintf("Migrate deprecated syntax %s to the official modern API specification (e.g., modern validators or signature conventions) according to official docs.", symDesc),
			},
			{
				Name:     "Hypothesis B: Backward-Compatible Adapter Layer",
				Guidance: fmt.Sprintf("Implement an adapter or compatibility wrapper %s that supports both updated and legacy invocation contracts safely.", symDesc),
			},
			{
				Name:     "Hypothesis C: Schema Transformation & Field Normalization",
				Guidance: fmt.Sprintf("Normalize parameter models and attribute decorators %s to satisfy modern type validation without regressions.", symDesc),
			},
		}

	case ArchetypeSecurityDefect:
		return []MCTSHypothesis{
			{
				Name:     "Hypothesis A: Parameterized / Prepared Statement Binding",
				Guidance: fmt.Sprintf("Replace string concatenation/interpolation %s with strictly parameterized query placeholders (?, $1, :arg) across database execution.", symDesc),
			},
			{
				Name:     "Hypothesis B: Strict Input Sanitization & Allowlist Validation",
				Guidance: fmt.Sprintf("Enforce canonicalization, character allowlists, and boundary checks %s before executing operations.", symDesc),
			},
			{
				Name:     "Hypothesis C: Type-Safe Escaping & Principle of Least Privilege",
				Guidance: fmt.Sprintf("Wrap arguments in type-safe binders %s and apply defense-in-depth sanitization at the API boundary.", symDesc),
			},
		}

	case ArchetypeResourceLeak:
		return []MCTSHypothesis{
			{
				Name:     "Hypothesis A: Deterministic Context Management (with / defer)",
				Guidance: fmt.Sprintf("Wrap resource acquisition %s in deterministic cleanup constructs (with-statement, try/finally, or defer) ensuring release on all exit paths.", symDesc),
			},
			{
				Name:     "Hypothesis B: Resource Pool Lifecycle Boundary",
				Guidance: fmt.Sprintf("Enforce bounded resource pooling %s and release handles immediately upon operation completion.", symDesc),
			},
			{
				Name:     "Hypothesis C: Graceful Error-Path Disposal",
				Guidance: fmt.Sprintf("Implement explicit error handlers %s that close active sockets, files, or connections during failure unwinding.", symDesc),
			},
		}

	case ArchetypeNullTypeError:
		return []MCTSHypothesis{
			{
				Name:     "Hypothesis A: Defensive Precondition Check & Safe Fallback",
				Guidance: fmt.Sprintf("Add explicit nil/None guards %s with sane fallback defaults before accessing attributes or indices.", symDesc),
			},
			{
				Name:     "Hypothesis B: Optional Chaining & Safe Dereferencing",
				Guidance: fmt.Sprintf("Narrow type contracts %s and handle missing or empty payloads defensively without throwing AttributeError/NullPointer.", symDesc),
			},
			{
				Name:     "Hypothesis C: Invariant Validation at Function Entry",
				Guidance: fmt.Sprintf("Validate preconditions and argument types %s at the public API boundary to guarantee non-null invariants.", symDesc),
			},
		}

	default:
		return []MCTSHypothesis{
			{
				Name:     "Hypothesis A: Invariant Correction Aligning with Test Assertion",
				Guidance: fmt.Sprintf("Correct the core algorithmic calculation %s to satisfy the failing test assertions directly.", symDesc),
			},
			{
				Name:     "Hypothesis B: Boundary & Extreme Value Hardening",
				Guidance: fmt.Sprintf("Handle edge cases %s (empty collections, zero limits, boundary conditions) that trigger the test failure.", symDesc),
			},
			{
				Name:     "Hypothesis C: Surgical Contract Refactoring",
				Guidance: fmt.Sprintf("Refactor the offending block %s while strictly preserving existing downstream interfaces and call graph invariants.", symDesc),
			},
		}
	}
}
