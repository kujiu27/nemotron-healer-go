package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	diffFileHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#8BE9FD"))
	diffHunkHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#BD93F9"))
	diffAddStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B"))
	diffDelStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555"))
	diffContextStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6272A4"))
	provenanceBoxStyle  = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#7D56F4")).
				Padding(0, 1).
				Margin(1, 0)
)

// RenderColorizedDiff parses a standard unified diff and applies terminal ANSI styling.
func RenderColorizedDiff(diff string) string {
	if strings.TrimSpace(diff) == "" {
		return ""
	}

	var sb strings.Builder
	lines := strings.Split(diff, "\n")
	for _, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		switch {
		case strings.HasPrefix(trimmed, "--- ") || strings.HasPrefix(trimmed, "+++ "):
			sb.WriteString(diffFileHeaderStyle.Render(trimmed))
		case strings.HasPrefix(trimmed, "@@"):
			sb.WriteString(diffHunkHeaderStyle.Render(trimmed))
		case strings.HasPrefix(trimmed, "+"):
			sb.WriteString(diffAddStyle.Render(trimmed))
		case strings.HasPrefix(trimmed, "-"):
			sb.WriteString(diffDelStyle.Render(trimmed))
		default:
			sb.WriteString(diffContextStyle.Render(trimmed))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

// RenderPatchProvenance renders an audit verification box with cryptographic signature and model metadata.
func RenderPatchProvenance(patchDigest, model, platform string) string {
	if patchDigest == "" {
		return ""
	}
	content := fmt.Sprintf(
		"🔒 Cryptographic Patch Provenance (Non-Repudiation Audit)\n"+
			"• Digest:   %s\n"+
			"• Model:    %s\n"+
			"• Platform: %s\n"+
			"• Audit:    Signed into Git Commit Trailer [X-Nemotron-Audit]",
		patchDigest, model, platform,
	)
	return provenanceBoxStyle.Render(content)
}

// PromptConfirmation asks the user for interactive terminal confirmation [Y/n].
// If reader is nil, it reads from os.Stdin. Returns true if accepted or empty (default yes).
func PromptConfirmation(promptMsg string, reader *bufio.Reader) bool {
	if reader == nil {
		reader = bufio.NewReader(os.Stdin)
	}
	fmt.Print(promptMsg)
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	answer = strings.TrimSpace(strings.ToLower(answer))
	return answer == "" || answer == "y" || answer == "yes"
}
