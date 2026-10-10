package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kujiu27/nemotron-healer-go/internal/engine"
)

func TestTUIModel_LifecycleAndBadges(t *testing.T) {
	agent := engine.NewAgent(".", "go test", 3, nil, nil)
	m := NewModel(agent)

	if m.Init() != nil {
		t.Errorf("expected Init() to return nil")
	}

	// 1. Initial State: IDLE
	viewIdle := m.View()
	if !strings.Contains(viewIdle, "IDLE") {
		t.Errorf("expected view to contain IDLE, got:\n%s", viewIdle)
	}

	// 2. Active State
	agent.Session.CurrentState = engine.StateSynthesizingPatch
	viewActive := m.View()
	if !strings.Contains(viewActive, "ACTIVE") {
		t.Errorf("expected view to contain ACTIVE, got:\n%s", viewActive)
	}

	// 3. Resolved State
	agent.Session.CurrentState = engine.StateSucceeded
	viewResolved := m.View()
	if !strings.Contains(viewResolved, "RESOLVED") {
		t.Errorf("expected view to contain RESOLVED, got:\n%s", viewResolved)
	}

	// 4. Failed State
	agent.Session.CurrentState = engine.StateFailed
	viewFailed := m.View()
	if !strings.Contains(viewFailed, "FAILED") {
		t.Errorf("expected view to contain FAILED, got:\n%s", viewFailed)
	}
}

func TestTUIModel_HeterogeneousTokenLedgerView(t *testing.T) {
	agent := engine.NewAgent(".", "go test", 3, nil, nil)
	m := NewModel(agent)

	// Add both fast and ultra tokens
	agent.Session.TokenLedger.PromptTokens = 1200
	agent.Session.TokenLedger.CompletionTokens = 350
	agent.Session.TokenLedger.FastPromptTokens = 150
	agent.Session.TokenLedger.FastCompTokens = 40
	agent.Session.TokenLedger.CalculateCost()

	view := m.View()
	if !strings.Contains(view, "Ultra: 1200/350") {
		t.Errorf("expected Ultra token count in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Nano: 150/40") {
		t.Errorf("expected Nano token count in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Tokens: 1740 total") {
		t.Errorf("expected 1740 total tokens in view, got:\n%s", view)
	}
}

func TestTUIModel_Updates(t *testing.T) {
	agent := engine.NewAgent(".", "go test", 3, nil, nil)
	m := NewModel(agent)

	// Window resize
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd != nil {
		t.Errorf("expected nil cmd on window resize")
	}
	mod := updated.(Model)
	if mod.Width != 120 || mod.Height != 40 {
		t.Errorf("window size not updated: %dx%d", mod.Width, mod.Height)
	}

	// EventMsg
	ev := EventMsg(engine.HealingStepEvent{
		State:   engine.StateDiagnosing,
		Summary: "AST graph built",
	})
	updated2, _ := mod.Update(ev)
	mod2 := updated2.(Model)
	if len(mod2.Logs) != 1 || !strings.Contains(mod2.Logs[0], "AST graph built") {
		t.Errorf("event log not recorded: %+v", mod2.Logs)
	}

	// TokenMsg
	_, _ = mod2.Update(TokenMsg("chunk"))

	// Quit key
	_, quitCmd := mod2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if quitCmd == nil {
		t.Errorf("expected quit cmd on 'q'")
	}
}
