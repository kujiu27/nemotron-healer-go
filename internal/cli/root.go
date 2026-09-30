package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nemotron-healer/nemotron-healer-go/internal/client"
	"github.com/nemotron-healer/nemotron-healer-go/internal/engine"
	"github.com/nemotron-healer/nemotron-healer-go/internal/tui"
	"github.com/spf13/cobra"
)

var (
	testCmdFlag string
	turnsFlag   int
	noTUIFlag   bool
	searchFlag  bool
	arenaFlag   bool
)

var RootCmd = &cobra.Command{
	Use:   "nemotron-healer [target_dir]",
	Short: "Autonomous Code Self-Healing & Diagnostic Agent powered by NVIDIA Nemotron on Nebius Token Factory & Tavily Search",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := "."
		if len(args) > 0 {
			targetDir = args[0]
		}
		absDir, err := filepath.Abs(targetDir)
		if err != nil {
			return err
		}

		fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Render("⚡ Nemotron-Healer (Go Edition)"))
		fmt.Printf("Target: %s\nCommand: `%s`\nMax Turns: %d\n\n", absDir, testCmdFlag, turnsFlag)

		if noTUIFlag {
			// Plain CLI logging mode (ideal for CI / GitHub Actions)
			agent := engine.NewAgent(absDir, testCmdFlag, turnsFlag, func(event engine.HealingStepEvent) {
				fmt.Printf("[%s] %s\n", event.State, event.Summary)
			}, nil)
			agent.EnableSearch = searchFlag
			agent.EnableArena = arenaFlag

			session, err := agent.Run(context.Background())
			if err != nil {
				return err
			}
			if !session.IsResolved {
				os.Exit(1)
			}
			return nil
		}

		// Interactive TUI Mode
		var p *tea.Program
		agent := engine.NewAgent(absDir, testCmdFlag, turnsFlag, func(event engine.HealingStepEvent) {
			if p != nil {
				p.Send(tui.EventMsg(event))
			}
		}, func(token string) {
			if p != nil {
				p.Send(tui.TokenMsg(token))
			}
		})
		agent.EnableSearch = searchFlag
		agent.EnableArena = arenaFlag

		m := tui.NewModel(agent.Session)
		p = tea.NewProgram(m)

		// Run agent in background goroutine
		go func() {
			_, _ = agent.Run(context.Background())
		}()

		if _, err := p.Run(); err != nil {
			return err
		}

		return nil
	},
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Verify environment, APIs, and dependencies",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(lipgloss.NewStyle().Bold(true).Render("Nemotron Healer Environment Check (Go Runtime)"))
		fmt.Println("--------------------------------------------------")

		nebius := client.NewNebiusClient()
		fmt.Printf("• Inference Endpoint: %s\n", nebius.BaseURL)
		fmt.Printf("• Active Model:       %s\n", nebius.Model)
		if nebius.APIKey != "" {
			fmt.Printf("• Nebius API Key:     Configured (%s...)\n", nebius.APIKey[:8])
		} else {
			fmt.Println("• Nebius API Key:     [Using Local / Fallback Inference]")
		}

		tavily := client.NewTavilyClient()
		if tavily.APIKey != "" {
			fmt.Println("• Tavily Search API:  Configured (Online)")
		} else {
			fmt.Println("• Tavily Search API:  Simulated / Fallback Mode")
		}

		// Git binary check
		if _, err := exec.LookPath("git"); err == nil {
			fmt.Println("• Git CLI:            Available")
		} else {
			fmt.Println("• Git CLI:            NOT FOUND (Required for patch/checkpoint)")
		}

		fmt.Println("--------------------------------------------------")
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("System ready for autonomous self-healing."))
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("nemotron-healer-go v0.2.0 (NVIDIA Nemotron 3 Ultra + Tavily)")
	},
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&testCmdFlag, "command", "c", "pytest", "Test command to run for reproduction and verification")
	RootCmd.PersistentFlags().IntVarP(&turnsFlag, "turns", "t", 5, "Maximum healing attempts")
	RootCmd.PersistentFlags().BoolVar(&noTUIFlag, "no-tui", false, "Disable TUI and output plain text (for CI / GitHub Actions)")
	RootCmd.PersistentFlags().BoolVar(&searchFlag, "search", false, "Enable Test-Time Compute (TTC) MCTS multi-branch search")
	RootCmd.PersistentFlags().BoolVar(&arenaFlag, "arena", false, "Enable Red-Blue Minimax Adversarial Self-Play Arena")

	RootCmd.AddCommand(doctorCmd)
	RootCmd.AddCommand(versionCmd)
}
