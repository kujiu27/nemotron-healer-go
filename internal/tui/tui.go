package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kujiu27/nemotron-healer-go/internal/engine"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	badgeStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#04B575")).
			MarginRight(1)

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#999999"))

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(1)
)

type EventMsg engine.HealingStepEvent
type TokenMsg string

type Model struct {
	Agent   *engine.Agent
	Session *engine.HealingSession
	Logs    []string
	Diff    string
	Width   int
	Height  int
	Done    bool
}

func NewModel(agent *engine.Agent) Model {
	return Model{
		Agent:   agent,
		Session: agent.Session,
		Logs:    make([]string, 0),
		Diff:    "",
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
	case EventMsg:
		ev := engine.HealingStepEvent(msg)
		badge := fmt.Sprintf("[%s]", ev.State)
		m.Logs = append(m.Logs, fmt.Sprintf("%s %s", badge, ev.Summary))
		if ev.State == engine.StateSucceeded || ev.State == engine.StateFailed {
			m.Done = true
		}
	case TokenMsg:
		m.Diff += string(msg)
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("⚡ NEMOTRON-HEALER (Go Edition) - Autonomous Self-Healing CLI"))
	b.WriteString("\n\n")

	// Render from a locked snapshot: the agent goroutine keeps mutating the
	// live session (data race found in review).
	snap := m.Agent.SessionSnapshot()
	status := fmt.Sprintf("State: %s | Turn: %d/%d | Duration: %.1fs",
		snap.CurrentState, snap.CurrentTurn, snap.MaxTurns, snap.DurationSeconds)
	b.WriteString(badgeStyle.Render("● ACTIVE") + infoStyle.Render(status) + "\n\n")

	// Render last 8 event logs
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Execution Pipeline Trace:\n"))
	startIdx := 0
	if len(m.Logs) > 8 {
		startIdx = len(m.Logs) - 8
	}
	for _, l := range m.Logs[startIdx:] {
		b.WriteString("  " + l + "\n")
	}

	// Render Token Ledger
	ledger := snap.TokenLedger
	ledgerBox := fmt.Sprintf(
		"Tokens: %d in / %d out | TTFT: %.2fs | TPS: %.1f | Cost: $%.6f | Saved: %.1f%%",
		ledger.PromptTokens, ledger.CompletionTokens, ledger.TTFTSeconds, ledger.MeasuredTPS, ledger.EstimatedCostUSD, ledger.SavingsPercentage,
	)
	b.WriteString("\n" + borderStyle.Render(ledgerBox) + "\n")

	if m.Done {
		b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render("Press 'q' to exit.") + "\n")
	}

	return b.String()
}
