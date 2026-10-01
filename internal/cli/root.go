package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nemotron-healer/nemotron-healer-go/internal/client"
	"github.com/nemotron-healer/nemotron-healer-go/internal/engine"
	"github.com/nemotron-healer/nemotron-healer-go/internal/tui"
	"github.com/spf13/cobra"
)

var (
	testCmdFlag    string
	turnsFlag      int
	noTUIFlag      bool
	searchFlag     bool
	arenaFlag      bool
	jsonFlag       bool
	autoAcceptFlag bool
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

		// Zero-Config Ecosystem Sniffer: Auto-detect test runner if not specified
		if testCmdFlag == "" {
			detectedCmd, ecosystem, err := engine.DetectTestCommand(absDir)
			if err != nil {
				return fmt.Errorf("no test command specified and could not auto-detect ecosystem: %w (please specify with -c or --command)", err)
			}
			testCmdFlag = detectedCmd
			fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(
				fmt.Sprintf("🔍 Zero-Config Ecosystem Sniffer: Auto-detected %s -> `%s`", ecosystem, testCmdFlag)))
		}

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
			if jsonFlag {
				data, _ := json.MarshalIndent(session, "", "  ")
				fmt.Println(string(data))
			}
			if session.IsResolved && len(session.AppliedPatches) > 0 && !autoAcceptFlag && !jsonFlag {
				fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render("\nProposed Verified Surgical Patch:"))
				fmt.Println(session.AppliedPatches[len(session.AppliedPatches)-1])
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
		fmt.Printf("• Fast Triage Model:  %s (Tier-1 <150ms)\n", nebius.FastModel)
		fmt.Printf("• Reasoning Brain:    %s\n", nebius.ReasoningModel)
		fmt.Printf("• Catalog Pricing:    $1.00/1M in, $3.00/1M out\n")

		// Active Network RTT Probe against Inference Endpoint
		nebiusStart := time.Now()
		modelsURL := strings.TrimRight(nebius.BaseURL, "/") + "/models"
		req, _ := http.NewRequest("GET", modelsURL, nil)
		if nebius.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+nebius.APIKey)
		}
		probeClient := &http.Client{Timeout: 4 * time.Second}
		resp, err := probeClient.Do(req)
		nebiusRTT := time.Since(nebiusStart).Milliseconds()
		if err == nil && resp.StatusCode == http.StatusOK {
			fmt.Printf("• Endpoint Probe:     [ONLINE] Authenticated (RTT: %dms)\n", nebiusRTT)
			resp.Body.Close()
		} else if err == nil {
			fmt.Printf("• Endpoint Probe:     [HTTP %d] %s (RTT: %dms)\n", resp.StatusCode, resp.Status, nebiusRTT)
			resp.Body.Close()
		} else {
			fmt.Printf("• Endpoint Probe:     [UNREACHABLE] (Error: %v)\n", err)
		}

		tavily := client.NewTavilyClient()
		if tavily.APIKey != "" {
			fmt.Println("• Tavily Search API:  Configured (Online Grounding)")
		} else {
			fmt.Println("• Tavily Search API:  Not configured (grounding disabled, no simulated results)")
		}

		// Toolchain checks
		toolchains := []string{"git", "python3", "pytest", "go"}
		for _, tc := range toolchains {
			if path, err := exec.LookPath(tc); err == nil {
				fmt.Printf("• Toolchain %-8s: Available (%s)\n", tc, path)
			} else {
				fmt.Printf("• Toolchain %-8s: Not found in PATH\n", tc)
			}
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
	RootCmd.PersistentFlags().StringVarP(&testCmdFlag, "command", "c", "", "Test command to run (default: auto-detected from ecosystem: go test, pytest, npm test, cargo test, make test)")
	RootCmd.PersistentFlags().IntVarP(&turnsFlag, "turns", "t", 5, "Maximum healing attempts")
	RootCmd.PersistentFlags().BoolVar(&noTUIFlag, "no-tui", false, "Disable TUI and output plain text (for CI / GitHub Actions)")
	RootCmd.PersistentFlags().BoolVar(&searchFlag, "search", false, "Enable Test-Time Compute (TTC) MCTS multi-branch search")
	RootCmd.PersistentFlags().BoolVar(&arenaFlag, "arena", false, "Enable Red-Blue Adversarial Self-Play Arena (attack/defend rounds)")
	RootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Output machine-readable telemetry JSON to stdout")
	RootCmd.PersistentFlags().BoolVarP(&autoAcceptFlag, "yes", "y", false, "Automatically accept and commit without interactive prompt (default in CI)")

	RootCmd.AddCommand(doctorCmd)
	RootCmd.AddCommand(versionCmd)
}
