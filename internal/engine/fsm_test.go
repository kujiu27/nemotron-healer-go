package engine

import (
	"testing"
)

func TestTokenLedgerCalculation(t *testing.T) {
	ledger := TokenLedger{
		PromptTokens:     10000,
		CompletionTokens: 2000,
	}
	ledger.CalculateCost()

	if ledger.TotalTokens != 12000 {
		t.Fatalf("expected 12000 total tokens, got %d", ledger.TotalTokens)
	}

	if ledger.EstimatedCostUSD <= 0 {
		t.Fatalf("expected positive estimated cost, got %f", ledger.EstimatedCostUSD)
	}

	if ledger.SavingsPercentage <= 90.0 {
		t.Fatalf("expected >90%% savings vs human engineer, got %f", ledger.SavingsPercentage)
	}
}

func TestHealingSessionTransitions(t *testing.T) {
	session := NewHealingSession("test-1", ".", "pytest", 5)
	if session.CurrentState != StateIdle {
		t.Fatalf("expected initial StateIdle, got %s", session.CurrentState)
	}

	session.TransitionTo(StateReproducing, "running tests", nil)
	if session.CurrentState != StateReproducing {
		t.Fatalf("expected StateReproducing, got %s", session.CurrentState)
	}
	if len(session.History) != 1 {
		t.Fatalf("expected 1 history event, got %d", len(session.History))
	}
}

func TestHealingSessionPatchDigest(t *testing.T) {
	session := NewHealingSession("test-digest", ".", "go test ./...", 3)
	session.PatchDigest = "sha256:abcd1234ef5678"
	if session.PatchDigest != "sha256:abcd1234ef5678" {
		t.Fatalf("expected patch digest to be stored, got %s", session.PatchDigest)
	}
}

