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

func TestTokenLedgerLifecycle_AddTokens(t *testing.T) {
	agent := NewAgent(".", "go test", 3, nil, nil)
	// Initial ledger
	if agent.Session.TokenLedger.TotalTokens != 0 {
		t.Fatalf("expected initial 0 tokens, got %d", agent.Session.TokenLedger.TotalTokens)
	}

	// 1. Stage 1: Fast triage (Nemotron Nano @ $0.06/$0.24 per 1M)
	agent.addFastTokens(150, 40)
	if agent.Session.TokenLedger.TotalTokens != 190 {
		t.Errorf("expected 190 tokens after triage, got %d", agent.Session.TokenLedger.TotalTokens)
	}
	if agent.Session.TokenLedger.FastPromptTokens != 150 || agent.Session.TokenLedger.FastCompTokens != 40 {
		t.Errorf("fast token counters mismatch: %d, %d", agent.Session.TokenLedger.FastPromptTokens, agent.Session.TokenLedger.FastCompTokens)
	}

	// 2. Stage 2: Synthesis (Nemotron 3 Ultra)
	agent.addTokens(1200, 350)
	if agent.Session.TokenLedger.TotalTokens != 1740 {
		t.Errorf("expected 1740 tokens after synthesis (190 fast + 1550 ultra), got %d", agent.Session.TokenLedger.TotalTokens)
	}

	// 3. Stage 3: Adversarial Falsification stress test
	agent.addTokens(400, 180)
	if agent.Session.TokenLedger.TotalTokens != 2320 {
		t.Errorf("expected 2320 tokens after falsification, got %d", agent.Session.TokenLedger.TotalTokens)
	}

	// 4. Stage 4: Red-Blue Arena (2 rounds)
	agent.addTokens(800, 300)
	if agent.Session.TokenLedger.TotalTokens != 3420 {
		t.Errorf("expected 3420 tokens after arena, got %d", agent.Session.TokenLedger.TotalTokens)
	}

	// Verify prompt/completion totals (Ultra models)
	if agent.Session.TokenLedger.PromptTokens != (1200 + 400 + 800) {
		t.Errorf("prompt tokens mismatch: got %d", agent.Session.TokenLedger.PromptTokens)
	}
	if agent.Session.TokenLedger.CompletionTokens != (350 + 180 + 300) {
		t.Errorf("completion tokens mismatch: got %d", agent.Session.TokenLedger.CompletionTokens)
	}
	if agent.Session.TokenLedger.EstimatedCostUSD <= 0 {
		t.Errorf("expected non-zero estimated cost")
	}
}
