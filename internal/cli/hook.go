package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/kujiu27/nemotron-healer-go/internal/engine"
	"github.com/spf13/cobra"
)

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

		hookPath, err := InstallGitHook(absDir)
		if err != nil {
			return err
		}

		fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#50FA7B")).Render(
			fmt.Sprintf("✅ Shift-Left Git Hook successfully installed at `%s`", hookPath)))
		fmt.Println("Broken commits will now be intercepted prior to `git push`.")
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

		fmt.Println("✅ Shift-Left Git Hook successfully uninstalled.")
		return nil
	},
}

var hookRunCmd = &cobra.Command{
	Use:   "run [target_dir]",
	Short: "Execute pre-push verification and report regression status",
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

		testCmd, eco, err := engine.DetectTestCommand(absDir)
		if err != nil {
			fmt.Printf("⚠️ No test command detected in %s; skipping pre-push check.\n", absDir)
			return nil
		}

		fmt.Printf("⚡ [Nemotron-Healer Hook] Running %s check: `%s`...\n", eco, testCmd)
		c := exec.Command("sh", "-c", testCmd)
		c.Dir = absDir
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Println(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5555")).Render(
				"\n❌ Pre-push verification test failed! Push blocked."))
			fmt.Println("💡 Run `nemotron-healer` to autonomously repair this repository before pushing.")
			os.Exit(1)
		}

		fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("✅ Pre-push verification passed."))
		return nil
	},
}

const prePushScriptContent = `#!/usr/bin/env bash
# [Nemotron-Healer] Autonomous Pre-Push Verification Hook
# Intercepts regressions before remote branch contamination

if command -v nemotron-healer >/dev/null 2>&1; then
    nemotron-healer hook run .
    EXIT_CODE=$?
    if [ $EXIT_CODE -ne 0 ]; then
        exit 1
    fi
else
    echo "⚡ [Nemotron-Healer Hook] nemotron-healer binary not in PATH; skipping hook check."
fi
exit 0
`

// InstallGitHook creates the executable pre-push hook in .git/hooks
func InstallGitHook(repoDir string) (string, error) {
	gitDir := filepath.Join(repoDir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		return "", fmt.Errorf("not a git repository (missing .git directory in %s)", repoDir)
	}

	hooksDir := filepath.Join(gitDir, "hooks")
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
	if err := os.WriteFile(prePushPath, []byte(prePushScriptContent), 0755); err != nil {
		return "", err
	}
	return prePushPath, nil
}

const hookOwnershipMarker = "[Nemotron-Healer] Autonomous Pre-Push Verification Hook"

// UninstallGitHook removes OUR pre-push hook; a foreign hook is left untouched.
func UninstallGitHook(repoDir string) error {
	prePushPath := filepath.Join(repoDir, ".git", "hooks", "pre-push")
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
	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	hookCmd.AddCommand(hookRunCmd)
	RootCmd.AddCommand(hookCmd)
}
