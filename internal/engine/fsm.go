package engine

import (
	"fmt"
	"time"
)

type HealingState string

const (
	StateIdle               HealingState = "IDLE"
	StateReproducing        HealingState = "REPRODUCING"
	StateDiagnosing         HealingState = "DIAGNOSING"
	StateSearchingKnowledge HealingState = "SEARCHING_KNOWLEDGE"
	StateSynthesizingPatch  HealingState = "SYNTHESIZING_PATCH"
	StateVerifyingSandbox   HealingState = "VERIFYING_SANDBOX"
	StateRollingBack        HealingState = "ROLLING_BACK"
	StateSucceeded          HealingState = "SUCCEEDED"
	StateFailed             HealingState = "FAILED"
)

type TokenLedger struct {
	PromptTokens      int     `json:"prompt_tokens"`
	CompletionTokens  int     `json:"completion_tokens"`
	TotalTokens       int     `json:"total_tokens"`
	TTFTSeconds       float64 `json:"ttft_seconds"`
	MeasuredTPS       float64 `json:"measured_tps"`
	EstimatedCostUSD  float64 `json:"estimated_cost_usd"`
	SavingsPercentage float64 `json:"savings_percentage"`
}

// Nemotron-3-Ultra-550b-a55b Token Factory catalog prices (USD per 1M tokens).
const (
	PricePromptPerMillion     = 1.00
	PriceCompletionPerMillion = 3.00
)

func (l *TokenLedger) CalculateCost() {
	l.TotalTokens = l.PromptTokens + l.CompletionTokens
	cost := (float64(l.PromptTokens)*PricePromptPerMillion + float64(l.CompletionTokens)*PriceCompletionPerMillion) / 1_000_000.0
	l.EstimatedCostUSD = cost
	// Reference point only: 30min engineer triage @ $50/hr.
	humanCost := 25.0
	if cost < humanCost {
		l.SavingsPercentage = ((humanCost - cost) / humanCost) * 100.0
	}
}

type HealingStepEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	State     HealingState           `json:"state"`
	Summary   string                 `json:"summary"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

type HealingSession struct {
	SessionID       string             `json:"session_id"`
	TargetDir       string             `json:"target_dir"`
	TestCommand     string             `json:"test_command"`
	CurrentState    HealingState       `json:"current_state"`
	CurrentTurn     int                `json:"current_turn"`
	MaxTurns        int                `json:"max_turns"`
	InitialError    string             `json:"initial_error"`
	LastError       string             `json:"last_error"`
	AppliedPatches  []string           `json:"applied_patches"`
	TavilyQueries   []string           `json:"tavily_queries"`
	TokenLedger     TokenLedger        `json:"token_ledger"`
	IsResolved      bool               `json:"is_resolved"`
	DurationSeconds float64            `json:"duration_seconds"`
	History         []HealingStepEvent `json:"history"`
}

func NewHealingSession(sessionID, targetDir, testCommand string, maxTurns int) *HealingSession {
	return &HealingSession{
		SessionID:    sessionID,
		TargetDir:    targetDir,
		TestCommand:  testCommand,
		CurrentState: StateIdle,
		CurrentTurn:  0,
		MaxTurns:     maxTurns,
		TokenLedger:  TokenLedger{SavingsPercentage: 99.9},
		History:      make([]HealingStepEvent, 0),
	}
}

func (s *HealingSession) TransitionTo(state HealingState, summary string, details map[string]interface{}) HealingStepEvent {
	s.CurrentState = state
	event := HealingStepEvent{
		Timestamp: time.Now(),
		State:     state,
		Summary:   summary,
		Details:   details,
	}
	s.History = append(s.History, event)
	return event
}

func (s *HealingSession) FormatSummary() string {
	return fmt.Sprintf("Session %s | State: %s | Turn: %d/%d | Cost: $%.6f (Saved %.1f%%)",
		s.SessionID, s.CurrentState, s.CurrentTurn, s.MaxTurns, s.TokenLedger.EstimatedCostUSD, s.TokenLedger.SavingsPercentage)
}
