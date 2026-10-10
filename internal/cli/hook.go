package cli

import (
	"context"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/kujiu27/nemotron-healer-go/internal/engine"
	"github.com/spf13/cobra"
	"encoding/json"
)
var autoHealFlag bool

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage Git Shift-Left pre-push verification and self-healing hooks",
}

var hookInstallCmd = &cobra.Command{
	Use:   "install [target_dir]",
	Short: "Install Git pre-push hook to intercept test regressions before remote push",
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

		hookPath, err := InstallGitHook(absDir, autoHealFlag)
		if err != nil {
			return err
		}

		modeMsg := "Broken commits will now be intercepted prior to `git push`."
		if autoHealFlag {
			modeMsg = "Broken commits will be intercepted and autonomously self-healed in-situ prior to `git push`."
		}
		if jsonFlag {
			payload := map[string]interface{}{
				"status":    "installed",
				"hook_path": hookPath,
				"auto_heal": autoHealFlag,
			}
			data, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render(
			fmt.Sprintf("✅ Shift-Left Git Hook successfully installed at `%s`", hookPath)))
		fmt.Println(modeMsg)
		return nil
	},
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall [target_dir]",
	Short: "Uninstall Git pre-push hook",
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

		err = UninstallGitHook(absDir)
		if err != nil {
			return err
		}

		if jsonFlag {
			payload := map[string]interface{}{
				"status":  "uninstalled",
				"target":  absDir,
				"success": true,
			}
			data, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		fmt.Println("✅ Shift-Left Git Hook successfully uninstalled.")
		return nil
	},
}

var hookRunCmd = &cobra.Command{
	Use:           "run [target_dir]",
	Short:         "Execute pre-push verification and report regression status",
	SilenceUsage:  true,
	SilenceErrors: true,
	Args:          cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := "."
		if len(args) > 0 {
			targetDir = args[0]
		}
		absDir, err := filepath.Abs(targetDir)
		if err != nil {
			return err
		}

		testCmd, eco, err := engine.DetectTestCommand(absDir)
		if err != nil {
			if jsonFlag {
				payload := map[string]interface{}{
					"status":  "skipped",
					"reason":  "no test command detected",
					"success": true,
				}
				data, _ := json.MarshalIndent(payload, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("⚠️ No test command detected in %s; skipping pre-push check.\n", absDir)
			return nil
		}

		humanOut := os.Stdout
		humanErr := os.Stderr
		if jsonFlag {
			humanOut = os.Stderr
		}

		fmt.Fprintf(humanErr, "⚡ [Nemotron-Healer Hook] Running %s check: `%s`...\n", eco, testCmd)
		c := exec.Command("sh", "-c", testCmd)
		c.Dir = absDir
		c.Stdout = humanOut
		c.Stderr = humanErr
		if err := c.Run(); err != nil {
			if autoHealFlag {
				fmt.Fprintln(humanErr, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")).Render(
					"⚡ [Shift-Left Autonomous Self-Healing] Launching in-situ repair before push..."))
				agent := engine.NewAgent(absDir, testCmd, 3, func(event engine.HealingStepEvent) {
					fmt.Fprintf(humanErr, "[%s] %s\n", event.State, event.Summary)
				}, nil)
				session, hErr := agent.Run(context.Background())
				if hErr == nil && session.IsResolved {
					if jsonFlag {
						payload := map[string]interface{}{
							"status":           "healed",
							"success":          true,
							"duration_seconds": session.DurationSeconds,
							"session_id":       session.SessionID,
						}
						data, _ := json.MarshalIndent(payload, "", "  ")
						fmt.Println(string(data))
						return fmt.Errorf("pre-push regression intercepted and healed autonomously; review fix branch before pushing")
					}
					fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render(
						fmt.Sprintf("🎉 Shift-Left Self-Healing SUCCEEDED in %.2fs! Verified fix branch created.", session.DurationSeconds)))
					return fmt.Errorf("pre-push regression intercepted and healed autonomously; review fix branch before pushing")
				}
				fmt.Fprintln(humanErr, lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C")).Render(
					"⚠️ Autonomous repair did not achieve verified pass."))
			}

			if jsonFlag {
				payload := map[string]interface{}{
					"status":  "failed",
					"success": false,
					"command": testCmd,
				}
				data, _ := json.MarshalIndent(payload, "", "  ")
				fmt.Println(string(data))
				return fmt.Errorf("pre-push verification test failed; push blocked")
			}
			fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render(
				"\n❌ Pre-push verification test failed! Push blocked."))
			if !autoHealFlag {
				fmt.Println("💡 Run `nemotron-healer` (or install hook with `--auto-heal`) to autonomously repair this repository before pushing.")
			}
			return fmt.Errorf("pre-push verification test failed; push blocked")
		}

		if jsonFlag {
			payload := map[string]interface{}{
				"status":    "passed",
				"success":   true,
				"command":   testCmd,
				"ecosystem": eco,
			}
			data, _ := json.MarshalIndent(payload, "", "  ")
			fmt.Println(string(data))
			return nil
		}
		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("✅ Pre-push verification passed."))
		return nil
	},
}

const prePushScriptTemplate = `#!/usr/bin/env bash
# [Nemotron-Healer] Autonomous Pre-Push Verification Hook
# Intercepts regressions before remote branch contamination

if command -v nemotron-healer >/dev/null 2>&1; then
    %s
    EXIT_CODE=$?
    if [ $EXIT_CODE -ne 0 ]; then
        exit 1
    fi
else
    echo "⚡ [Nemotron-Healer Hook] nemotron-healer binary not in PATH; skipping hook check."
fi
exit 0
`

// resolveGitHooksDir determines the git hooks directory across standard repositories,
// worktrees (where .git is a file), and submodules.
func resolveGitHooksDir(repoDir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-path", "hooks")
	cmd.Dir = repoDir
	if out, err := cmd.Output(); err == nil {
		p := strings.TrimSpace(string(out))
		if p != "" {
			if !filepath.IsAbs(p) {
				p = filepath.Join(repoDir, p)
			}
			return filepath.Clean(p), nil
		}
	}
	gitDir := filepath.Join(repoDir, ".git")
	if fi, err := os.Stat(gitDir); err == nil && fi.IsDir() {
		return filepath.Join(gitDir, "hooks"), nil
	}
	return "", fmt.Errorf("not a git repository (missing .git in %s)", repoDir)
}

// InstallGitHook creates the executable pre-push hook in the repository's hooks directory
func InstallGitHook(repoDir string, autoHeal ...bool) (string, error) {
	hooksDir, err := resolveGitHooksDir(repoDir)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return "", err
	}

	prePushPath := filepath.Join(hooksDir, "pre-push")
	// Ownership guard: never clobber a foreign pre-push hook (husky, lefthook,
	// custom user scripts). Round-37: this silently destroyed user hooks.
	if existing, err := os.ReadFile(prePushPath); err == nil {
		if !bytes.Contains(existing, []byte(hookOwnershipMarker)) {
			return "", fmt.Errorf("refusing to overwrite existing pre-push hook not owned by nemotron-healer: %s (back it up or remove it first, or merge the 'nemotron-healer hook run .' call into it manually)", prePushPath)
		}
	}
	runCmd := "nemotron-healer hook run ."
	if len(autoHeal) > 0 && autoHeal[0] {
		runCmd = "nemotron-healer hook run . --auto-heal"
	}
	content := fmt.Sprintf(prePushScriptTemplate, runCmd)
	if err := os.WriteFile(prePushPath, []byte(content), 0755); err != nil {
		return "", err
	}
	return prePushPath, nil
}

const hookOwnershipMarker = "[Nemotron-Healer] Autonomous Pre-Push Verification Hook"

// UninstallGitHook removes OUR pre-push hook; a foreign hook is left untouched.
func UninstallGitHook(repoDir string) error {
	hooksDir, err := resolveGitHooksDir(repoDir)
	if err != nil {
		return nil
	}
	prePushPath := filepath.Join(hooksDir, "pre-push")
	existing, err := os.ReadFile(prePushPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !bytes.Contains(existing, []byte(hookOwnershipMarker)) {
		return fmt.Errorf("refusing to remove pre-push hook not owned by nemotron-healer: %s", prePushPath)
	}
	return os.Remove(prePushPath)
}

func init() {
	hookInstallCmd.Flags().BoolVar(&autoHealFlag, "auto-heal", false, "Enable autonomous in-situ self-healing when pre-push verification fails")
	hookRunCmd.Flags().BoolVar(&autoHealFlag, "auto-heal", false, "Launch autonomous in-situ self-healing when pre-push verification fails")
	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	hookCmd.AddCommand(hookRunCmd)
	RootCmd.AddCommand(hookCmd)
}
