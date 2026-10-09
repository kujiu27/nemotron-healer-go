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
	"github.com/kujiu27/nemotron-healer-go/internal/client"
	"github.com/kujiu27/nemotron-healer-go/internal/engine"
	"github.com/kujiu27/nemotron-healer-go/internal/tui"
	"github.com/spf13/cobra"
)

// Version is overridden at build time via -X github.com/kujiu27/nemotron-healer-go/internal/cli.Version=...
var Version = "v0.6.1"

var (
	testCmdFlag    string
	turnsFlag      int
	noTUIFlag      bool
	ciFlag         bool
	sarifFlag      string
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
		// Under --json, stdout carries ONLY the machine-readable document;
		// every human line (banner, sniffer, progress, annotations) goes to stderr.
		human := os.Stdout
		if jsonFlag {
			human = os.Stderr
		}

		fmt.Fprintln(human, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Render("⚡ Nemotron-Healer (Go Edition)"))

		// Zero-Config Ecosystem Sniffer: Auto-detect test runner if not specified
		if testCmdFlag == "" {
			detectedCmd, ecosystem, err := engine.DetectTestCommand(absDir)
			if err != nil {
				return fmt.Errorf("no test command specified and could not auto-detect ecosystem: %w (please specify with -c or --command)", err)
			}
			testCmdFlag = detectedCmd
			fmt.Fprintln(human, lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render(
				fmt.Sprintf("🔍 Zero-Config Ecosystem Sniffer: Auto-detected %s -> `%s`", ecosystem, testCmdFlag)))
		}

		fmt.Fprintf(human, "Target: %s\nCommand: `%s`\nMax Turns: %d\n\n", absDir, testCmdFlag, turnsFlag)

		isCI := noTUIFlag || ciFlag || os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("CI") != ""

		if isCI {
			// Plain CLI logging mode (ideal for CI / GitHub Actions)
			agent := engine.NewAgent(absDir, testCmdFlag, turnsFlag, func(event engine.HealingStepEvent) {
				fmt.Fprintf(human, "[%s] %s\n", event.State, event.Summary)
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
			if sarifFlag != "" {
				if sErr := ExportSarif(session, sarifFlag); sErr == nil {
					fmt.Fprintf(human, "📊 Exported SARIF 2.1.0 security report to `%s`\n", sarifFlag)
				}
			}
			if session.IsResolved && len(session.AppliedPatches) > 0 && !jsonFlag {
				fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render("\nProposed Verified Surgical Patch:"))
				latestPatch := session.AppliedPatches[len(session.AppliedPatches)-1]
				fmt.Print(RenderColorizedDiff(latestPatch))

				if session.PatchDigest != "" {
					fmt.Print(RenderPatchProvenance(session.PatchDigest, "nvidia/Nemotron-3-Ultra-550b-a55b", "Nebius Token Factory"))
				}

				inCIEnv := os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("CI") != "" || ciFlag
				if !inCIEnv && !autoAcceptFlag {
					confirmMsg := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFB86C")).Render("\n? Keep the verified fix branch (patch already committed to it)? [Y/n]: ")
					confirmed := PromptConfirmation(confirmMsg, nil)
					if !confirmed {
						fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("Declined — fix branch remains available for review; nothing further applied."))
					} else {
						fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("Fix branch retained with the verified patch."))
					}
				}
			}

			if session.IsResolved {
				fmt.Fprintf(human, "::notice title=Nemotron Self-Healing Succeeded::Verified fix generated in %.2fs (Turn %d)\n", session.DurationSeconds, session.CurrentTurn)
			} else {
				fmt.Fprintf(human, "::error title=Nemotron Self-Healing Failed::Could not verify fix within %d turns\n", session.MaxTurns)
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

		m := tui.NewModel(agent)
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
		fmt.Printf("nemotron-healer-go %s (NVIDIA Nemotron 3 Ultra + Tavily)\n", Version)
	},
}

func init() {
	RootCmd.PersistentFlags().StringVarP(&testCmdFlag, "command", "c", "", "Test command to run (default: auto-detected from ecosystem: go test, pytest, npm test, cargo test, make test)")
	RootCmd.PersistentFlags().IntVarP(&turnsFlag, "turns", "t", 5, "Maximum healing attempts")
	RootCmd.PersistentFlags().BoolVar(&noTUIFlag, "no-tui", false, "Disable TUI and output plain text (for CI / GitHub Actions)")
	RootCmd.PersistentFlags().BoolVar(&ciFlag, "ci", false, "Enable headless CI mode with GitHub Actions annotations and step summary")
	RootCmd.PersistentFlags().StringVar(&sarifFlag, "sarif", "", "Export diagnostic and healing results in standard SARIF 2.1.0 format to path")
	RootCmd.PersistentFlags().BoolVar(&searchFlag, "search", false, "Enable Test-Time Compute (TTC) divergent hypothesis search (parallel best-of-N branches + adversarial depth-2 refinement)")
	RootCmd.PersistentFlags().BoolVar(&arenaFlag, "arena", false, "Enable Red-Blue Adversarial Self-Play Arena (attack/defend rounds)")
	RootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Output machine-readable telemetry JSON to stdout")
	RootCmd.PersistentFlags().BoolVarP(&autoAcceptFlag, "yes", "y", false, "Automatically accept and commit without interactive prompt (default in CI)")

	RootCmd.AddCommand(doctorCmd)
	RootCmd.AddCommand(versionCmd)
}
