package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	"github.com/kujiu27/nemotron-healer-go/internal/mock"
	"github.com/spf13/cobra"
)

// Version is overridden at build time via -X github.com/kujiu27/nemotron-healer-go/internal/cli.Version=...
var Version = "v0.7.35"

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
	mockFlag       bool
	mockCleanupFunc func()
)

func initMockServerIfEnabled() func() {
	if !mockFlag {
		return func() {}
	}
	server := mock.StartServer()
	_ = os.Setenv("NEBIUS_BASE_URL", server.URL+"/v1")
	_ = os.Setenv("TAVILY_BASE_URL", server.URL)
	if os.Getenv("NEBIUS_API_KEY") == "" {
		_ = os.Setenv("NEBIUS_API_KEY", "mock")
	}
	if os.Getenv("TAVILY_API_KEY") == "" {
		_ = os.Setenv("TAVILY_API_KEY", "mock")
	}
	return func() {
		server.Close()
	}
}

var RootCmd = &cobra.Command{
	Use:   "nemotron-healer [target_dir]",
	Short: "Autonomous Code Self-Healing & Diagnostic Agent powered by NVIDIA Nemotron on Nebius Token Factory & Tavily Search",
	SilenceUsage: true,
	SilenceErrors: true,
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

		if mockFlag && !jsonFlag {
			fmt.Fprintln(human, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFB86C")).Render("⚡ Running in offline MOCK mode (X-Nemotron-Healer: MOCK) — zero API keys required"))
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

			origBranchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
			origBranchCmd.Dir = absDir
			origBranchOut, _ := origBranchCmd.Output()
			origBranch := strings.TrimSpace(string(origBranchOut))

			session, err := agent.Run(context.Background())
			if err != nil {
				return err
			}
			return handlePostSession(session, agent, absDir, origBranch, human)
		}

		// Interactive TUI Mode
		origBranchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
		origBranchCmd.Dir = absDir
		origBranchOut, _ := origBranchCmd.Output()
		origBranch := strings.TrimSpace(string(origBranchOut))

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

		var runSession *engine.HealingSession
		var runErr error
		agentDone := make(chan struct{})

		// Run agent in background goroutine
		go func() {
			runSession, runErr = agent.Run(context.Background())
			close(agentDone)
		}()

		if _, err := p.Run(); err != nil {
			return err
		}
		<-agentDone
		if runErr != nil {
			return runErr
		}
		if runSession != nil {
			return handlePostSession(runSession, agent, absDir, origBranch, human)
		}

		return nil
	},
}

func handlePostSession(session *engine.HealingSession, agent *engine.Agent, absDir, origBranch string, human io.Writer) error {
	if jsonFlag {
		data, _ := json.MarshalIndent(session, "", "  ")
		fmt.Println(string(data))
	}
	if sarifFlag != "" {
		if sErr := ExportSarif(session, sarifFlag); sErr == nil {
			fmt.Fprintf(human, "📊 Exported SARIF 2.1.0 security report to `%s`\n", sarifFlag)
		} else {
			fmt.Fprintf(human, "⚠️ Failed to export SARIF report to `%s`: %v\n", sarifFlag, sErr)
		}
	}
	if session.IsResolved && len(session.AppliedPatches) > 0 && !jsonFlag {
		fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render("\nProposed Verified Surgical Patch:"))
		latestPatch := session.AppliedPatches[len(session.AppliedPatches)-1]
		fmt.Print(RenderColorizedDiff(latestPatch))

		if session.PatchDigest != "" {
			fmt.Print(RenderPatchProvenance(session.PatchDigest, agent.Nebius.Model, "Nebius Token Factory"))
		}

		inCIEnv := os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("CI") != "" || ciFlag
		if !inCIEnv && !autoAcceptFlag {
			confirmMsg := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFB86C")).Render("\n? Stay on the verified fix branch (patch committed to it)? [Y/n]: ")
			confirmed := PromptConfirmation(confirmMsg, nil)
			if !confirmed {
				if origBranch != "" && origBranch != "HEAD" {
					checkoutCmd := exec.Command("git", "checkout", origBranch)
					checkoutCmd.Dir = absDir
					_ = checkoutCmd.Run()
				}
				fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render(fmt.Sprintf("Declined — restored to '%s'; fix branch remains available for review.", origBranch)))
			} else {
				fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("Fix branch retained with the verified patch."))
			}
		}
	}
	// Emit GitHub Actions step outputs if running inside GitHub Actions runner
	if outputFile := os.Getenv("GITHUB_OUTPUT"); outputFile != "" {
		if f, oErr := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); oErr == nil {
			branchName := ""
			if session.IsResolved && session.SessionID != "" {
				branchName = fmt.Sprintf("fix/nemotron-heal-%s", session.SessionID)
			}
			patchCount := len(session.AppliedPatches)
			lines := []string{
				fmt.Sprintf("is_resolved=%t\n", session.IsResolved),
				fmt.Sprintf("branch_name=%s\n", branchName),
				fmt.Sprintf("turns_used=%d\n", session.CurrentTurn),
				fmt.Sprintf("patch_digest=%s\n", session.PatchDigest),
				fmt.Sprintf("defect_archetype=%s\n", session.DefectArchetype),
				fmt.Sprintf("duration_seconds=%.2f\n", session.DurationSeconds),
				fmt.Sprintf("patches_count=%d\n", patchCount),
			}
			for _, line := range lines {
				_, _ = f.WriteString(line)
			}
			_ = f.Close()
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

var doctorCmd = &cobra.Command{
	Use:          "doctor",
	Short:        "Verify environment, APIs, and dependencies",
	SilenceErrors: true,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		human := os.Stdout
		if jsonFlag {
			human = os.Stderr
		}

		fmt.Fprintln(human, lipgloss.NewStyle().Bold(true).Render("Nemotron Healer Environment Check (Go Runtime)"))
		fmt.Fprintln(human, "--------------------------------------------------")

		ready := true
		var failureReasons []string

		nebius := client.NewNebiusClient()
		fmt.Fprintf(human, "• Inference Endpoint: %s\n", nebius.BaseURL)
		fmt.Fprintf(human, "• Fast Triage Model:  %s (Tier-1 <150ms)\n", nebius.FastModel)
		fmt.Fprintf(human, "• Reasoning Brain:    %s\n", nebius.ReasoningModel)
		fmt.Fprintf(human, "• Catalog Pricing:    $1.00/1M in, $3.00/1M out\n")

		if mockFlag {
			fmt.Fprintln(human, "• Mode:               [OFFLINE MOCK] Zero API keys required (X-Nemotron-Healer: MOCK)")
		}
		probeLabel := "UNTESTED"

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
			probeLabel = "ONLINE"
			if mockFlag {
				probeLabel = "ONLINE (MOCK)"
			}
			fmt.Fprintf(human, "• Endpoint Probe:     [%s] Authenticated (RTT: %dms)\n", probeLabel, nebiusRTT)
			resp.Body.Close()
		} else if err == nil {
			probeLabel = fmt.Sprintf("HTTP %d", resp.StatusCode)
			fmt.Fprintf(human, "• Endpoint Probe:     [%s] %s (RTT: %dms)\n", probeLabel, resp.Status, nebiusRTT)
			resp.Body.Close()
			ready = false
			failureReasons = append(failureReasons, fmt.Sprintf("Inference endpoint rejected authentication (HTTP %d %s). Please set a valid NEBIUS_API_KEY.", resp.StatusCode, resp.Status))
		} else {
			probeLabel = "UNREACHABLE"
			fmt.Fprintf(human, "• Endpoint Probe:     [UNREACHABLE] (Error: %v)\n", err)
			ready = false
			failureReasons = append(failureReasons, fmt.Sprintf("Inference endpoint unreachable (%v). Check network or set NEBIUS_BASE_URL.", err))
		}

		if nebius.APIKey == "" && !mockFlag {
			ready = false
			failureReasons = append(failureReasons, "NEBIUS_API_KEY is not set. Export your key: export NEBIUS_API_KEY=\"...\" (tokenfactory.nebius.com)")
		}

		tavily := client.NewTavilyClient()
		if tavily.APIKey != "" {
			tavilyLabel := "Configured (Online Grounding)"
			if mockFlag {
				tavilyLabel = "Configured (Mock Grounding)"
			}
			fmt.Fprintf(human, "• Tavily Search API:  %s\n", tavilyLabel)
		} else {
			fmt.Fprintln(human, "• Tavily Search API:  Not configured (grounding disabled, baseline ungrounded mode)")
		}

		// Toolchain checks
		toolchains := []string{"git", "python3", "pytest", "go"}
		toolchainMap := make(map[string]bool)
		for _, tc := range toolchains {
			if path, err := exec.LookPath(tc); err == nil {
				toolchainMap[tc] = true
				fmt.Fprintf(human, "• Toolchain %-8s: Available (%s)\n", tc, path)
			} else {
				toolchainMap[tc] = false
				if tc == "git" {
					ready = false
					failureReasons = append(failureReasons, "Toolchain git is required for transactional rollback and branch creation, but was not found in PATH.")
					fmt.Fprintf(human, "• Toolchain %-8s: [MISSING - REQUIRED]\n", tc)
				} else {
					fmt.Fprintf(human, "• Toolchain %-8s: Not found in PATH (optional, needed if target repo uses it)\n", tc)
				}
			}
		}

		fmt.Fprintln(human, "--------------------------------------------------")
		if jsonFlag {
			payload := map[string]interface{}{
				"ready":              ready,
				"mock_mode":          mockFlag,
				"failure_reasons":    failureReasons,
				"inference_endpoint": nebius.BaseURL,
				"fast_model":         nebius.FastModel,
				"reasoning_model":    nebius.ReasoningModel,
				"endpoint_probe":     probeLabel,
				"endpoint_rtt_ms":    nebiusRTT,
				"tavily_configured":  tavily.APIKey != "",
				"toolchains":         toolchainMap,
			}
			data, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(data))
			if !ready {
				return fmt.Errorf("environment check failed: %d issue(s) detected", len(failureReasons))
			}
			return nil
		}
		fmt.Fprintln(human, "--------------------------------------------------")
		if ready {
			fmt.Fprintln(human, lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("System ready for autonomous self-healing."))
			return nil
		}

		fmt.Fprintln(human, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render("System NOT ready for live cloud healing:"))
		for _, reason := range failureReasons {
			fmt.Fprintf(human, "  ✘ %s\n", reason)
		}
		fmt.Fprintln(human, lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C")).Render("\nTip: To test the full self-healing pipeline offline with zero API keys:"))
		fmt.Fprintln(human, "     nemotron-healer doctor --mock")
		fmt.Fprintln(human, "     nemotron-healer samples/external_gjson --mock --command \"go test . -run TestEmptyValueQuery -count=1\"")
		return fmt.Errorf("environment check failed: %d issue(s) detected", len(failureReasons))
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version info",
	Run: func(cmd *cobra.Command, args []string) {
		if jsonFlag {
			payload := map[string]interface{}{
				"version":   Version,
				"platform":  "Nebius Token Factory",
				"models": map[string]string{
					"reasoning": "nvidia/Nemotron-3-Ultra-550b-a55b",
					"triage":    "nvidia/Nemotron-3-Nano-30B-A3B",
				},
			}
			data, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(data))
			return
		}
		fmt.Printf("nemotron-healer-go %s (NVIDIA Nemotron 3 Ultra + Tavily)\n", Version)
	},
}

func init() {
	RootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		mockCleanupFunc = initMockServerIfEnabled()
		return nil
	}
	RootCmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
		if mockCleanupFunc != nil {
			mockCleanupFunc()
		}
	}

	RootCmd.PersistentFlags().StringVarP(&testCmdFlag, "command", "c", "", "Test command to run (default: auto-detected from ecosystem: go test, pytest, npm test, cargo test, make test)")
	RootCmd.PersistentFlags().IntVarP(&turnsFlag, "turns", "t", 5, "Maximum healing attempts")
	RootCmd.PersistentFlags().BoolVar(&noTUIFlag, "no-tui", false, "Disable TUI and output plain text (for CI / GitHub Actions)")
	RootCmd.PersistentFlags().BoolVar(&ciFlag, "ci", false, "Enable headless CI mode with GitHub Actions annotations and step summary")
	RootCmd.PersistentFlags().StringVar(&sarifFlag, "sarif", "", "Export diagnostic and healing results in standard SARIF 2.1.0 format to path")
	RootCmd.PersistentFlags().BoolVar(&searchFlag, "search", false, "Enable Test-Time Compute (TTC) divergent hypothesis search (parallel best-of-N branches + adversarial depth-2 refinement)")
	RootCmd.PersistentFlags().BoolVar(&arenaFlag, "arena", false, "Enable Red-Blue Adversarial Self-Play Arena (attack/defend rounds)")
	RootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Output machine-readable telemetry JSON to stdout")
	RootCmd.PersistentFlags().BoolVarP(&autoAcceptFlag, "yes", "y", false, "Automatically accept and commit without interactive prompt (default in CI)")
	RootCmd.PersistentFlags().BoolVar(&mockFlag, "mock", false, "Run in offline mock mode using built-in labeled mock server (zero API keys required)")

	RootCmd.AddCommand(doctorCmd)
	RootCmd.AddCommand(versionCmd)
}
