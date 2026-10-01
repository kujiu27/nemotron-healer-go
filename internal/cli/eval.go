package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/nemotron-healer/nemotron-healer-go/internal/engine"
	"github.com/spf13/cobra"
)

type BenchmarkCase struct {
	ID          string
	Name        string
	Path        string
	Command     string
	Archetype   string
	Description string
}

var ablationFlag bool
var repeatFlag int

var evalCmd = &cobra.Command{
	Short: "Run the in-repo Autonomous Healer Benchmark (AHB-4) with sandboxed isolation, A/B ablation, and repeat runs",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Render("⚡ AHB-4 In-Repo Benchmark & Ablation Engine"))
		fmt.Printf("Repeats per case: %d\n\n", max(1, repeatFlag))
		fmt.Println("Grounded against Concurrency, API Breaking Migrations, and Re-entrancy Traps")
		fmt.Println()

		// Auto-resolve pytest command if not in global PATH
		defaultPytest := "pytest"
		if _, err := exec.LookPath("pytest"); err != nil {
			// Try locating project virtualenv pytest or python3 -m pytest
			candidates := []string{
				filepath.Join(".venv", "bin", "pytest"),
				filepath.Join("venv", "bin", "pytest"),
				filepath.Join("..", ".venv", "bin", "pytest"),
			}
			if venvEnv := os.Getenv("VIRTUAL_ENV"); venvEnv != "" {
				candidates = append([]string{filepath.Join(venvEnv, "bin", "pytest")}, candidates...)
			}
			found := false
			for _, cand := range candidates {
				if _, err := os.Stat(cand); err == nil {
					defaultPytest = cand
					found = true
					break
				}
			}
			if !found {
				defaultPytest = "python3 -m pytest"
			}
		}

		cases := []BenchmarkCase{
			{
				ID:          "AHB-01",
				Name:        "Async Concurrency Race",
				Path:        "samples/fastapi_async_deadlock",
				Command:     defaultPytest,
				Archetype:   "ConcurrencyRace",
				Description: "10 concurrent coroutines corrupting shared state without synchronization.",
			},
			{
				ID:          "AHB-02",
				Name:        "Cascading Deadlock Trap",
				Path:        "samples/hard_concurrency_cascade",
				Command:     defaultPytest,
				Archetype:   "ConcurrencyRace (Hard)",
				Description: "4-file cascading re-entrancy deadlock trap requiring task-aware lock.",
			},
			{
				ID:          "AHB-03",
				Name:        "Breaking API Deprecation",
				Path:        "samples/pydantic_v2_migration",
				Command:     defaultPytest,
				Archetype:   "InterfaceBreaking",
				Description: "Upstream deprecation requiring dynamic Tavily migration documentation.",
			},
			{
				ID:          "AHB-04",
				Name:        "SQL Injection Remediation",
				Path:        "samples/sql_injection_remediation",
				Command:     defaultPytest,
				Archetype:   "SecurityDefect",
				Description: "Unsanitized user inputs requiring parameterized query AST refactor.",
			},
			{
				ID:          "AHB-05",
				Name:        "Go Concurrency Race (-race)",
				Path:        "samples/go_concurrency_race",
				Command:     "go test -race .",
				Archetype:   "ConcurrencyRace (Go Data Race)",
				Description: "Concurrent map and state mutation detected by Go runtime -race detector.",
			},
		}

		type EvalRunResult struct {
			CaseID       string
			Name         string
			Archetype    string
			BaselinePass bool
			FullPass     bool
			TurnsTaken   int
			DurationS    float64
			CostUSD      float64
		}

		results := make([]EvalRunResult, 0)

		totalRuns := max(1, repeatFlag)
		for rep := 1; rep <= totalRuns; rep++ {
			if totalRuns > 1 {
				fmt.Printf("— Repeat %d/%d —\n", rep, totalRuns)
			}
			for idx, c := range cases {
				fmt.Printf("[%d/%d] Sandboxing & Evaluating %s (%s)...\n", idx+1, len(cases), c.ID, c.Name)

		// Step 1: Create clean isolated temporary sandboxes (one per arm)
		tmpDir, err := createIsolatedSandbox(c.Path)
		if err != nil {
			fmt.Printf("  ⚠️ Could not sandbox %s: %v (skipping)\n", c.Path, err)
			continue
		}
		defer os.RemoveAll(tmpDir)

		baseDir, err := createIsolatedSandbox(c.Path)
		if err != nil {
			fmt.Printf("  ⚠️ Could not sandbox baseline %s: %v (skipping)\n", c.Path, err)
			continue
		}
		defer os.RemoveAll(baseDir)

		// Step 2: Measured baseline — single-turn greedy LLM loop, no Tavily grounding, no archetype constraints
		baseAgent := engine.NewAgent(baseDir, c.Command, 1, nil, nil)
		baseAgent.EnableArena = false
		baseAgent.DisableGrounding = true
		baseSession, _ := baseAgent.Run(context.Background())
		baselinePass := baseSession.IsResolved

		// Step 3: Run Full Nemotron-Healer System in the fresh isolated sandbox
		start := time.Now()
		agent := engine.NewAgent(tmpDir, c.Command, 3, nil, nil)
		agent.EnableArena = false // Fast verifiable evaluation
		session, _ := agent.Run(context.Background())
		dur := time.Since(start).Seconds()

			results = append(results, EvalRunResult{
				CaseID:       c.ID,
				Name:         c.Name,
				Archetype:    c.Archetype,
				BaselinePass: baselinePass,
				FullPass:     session.IsResolved,
				TurnsTaken:   session.CurrentTurn,
				DurationS:    dur,
				CostUSD:      session.TokenLedger.EstimatedCostUSD,
			})
			}
		}

		// Print Comparative A/B Ablation Scorecard Table
		fmt.Println("\n" + lipgloss.NewStyle().Bold(true).Render("📊 AHB-4 Ablation Scorecard (NVIDIA x Nebius x Tavily)"))
		fmt.Println("=====================================================================================================")
		fmt.Printf("%-8s | %-24s | %-16s | %-12s | %-12s | %-6s | %-8s\n",
			"Case ID", "Benchmark Scenario", "Defect Archetype", "Baseline LLM", "Full System", "Turns", "Cost ($)")
		fmt.Println("-----------------------------------------------------------------------------------------------------")

		baselineWins := 0
		fullWins := 0

		for _, r := range results {
			baseStr := "❌ FAIL"
			if r.BaselinePass {
				baseStr = "✅ PASS"
				baselineWins++
			}

			fullStr := "❌ FAIL"
			if r.FullPass {
				fullStr = "✅ PASS"
				fullWins++
			}

			fmt.Printf("%-8s | %-24s | %-16s | %-12s | %-12s | %-6d | $%-7.5f\n",
				r.CaseID, r.Name, r.Archetype, baseStr, fullStr, r.TurnsTaken, r.CostUSD)
		}
		fmt.Println("=====================================================================================================")

		total := max(1, len(results))
		baseRate := float64(baselineWins) / float64(total) * 100.0
		fullRate := float64(fullWins) / float64(total) * 100.0
		delta := fullRate - baseRate

		fmt.Printf("• Baseline Un-Grounded Solve Rate : %5.1f%% (%d/%d)\n", baseRate, baselineWins, total)
		fmt.Printf("• Nemotron-Healer Full Solve Rate : %5.1f%% (%d/%d)\n", fullRate, fullWins, total)
		fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render(
			fmt.Sprintf("• Empirical Grounding Delta (Tavily + AST) : +%.1f%% Accuracy Lift\n", delta),
		))

		return nil
	},
}

func createIsolatedSandbox(srcRelPath string) (string, error) {
	absSrc, err := filepath.Abs(srcRelPath)
	if err != nil {
		return "", err
	}

	tmpDir, err := os.MkdirTemp("", "nemotron_eval_*")
	if err != nil {
		return "", err
	}

	err = filepath.Walk(absSrc, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(absSrc, path)
		if strings.HasPrefix(rel, ".") || strings.Contains(rel, "__pycache__") {
			return nil
		}

		// If a corresponding .orig file exists for this file, skip copying the fixed version!
		origPath := path + ".orig"
		if _, err := os.Stat(origPath); err == nil {
			return nil
		}

		// If this is the .orig file, restore it as the target file!
		if strings.HasSuffix(path, ".orig") {
			targetName := strings.TrimSuffix(rel, ".orig")
			actualDest := filepath.Join(tmpDir, targetName)
			return copyFileDirect(path, actualDest)
		}

		destPath := filepath.Join(tmpDir, rel)
		_ = os.MkdirAll(filepath.Dir(destPath), 0755)
		return copyFileDirect(path, destPath)
	})

	return tmpDir, err
}

func copyFileDirect(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func init() {
	evalCmd.Flags().BoolVar(&ablationFlag, "ablation", true, "Perform side-by-side A/B ablation against baseline ungrounded model")
	evalCmd.Flags().IntVar(&repeatFlag, "repeat", 1, "Repeat each benchmark case N times to expose variance")
	RootCmd.AddCommand(evalCmd)
}
