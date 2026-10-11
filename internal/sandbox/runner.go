package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var (
	goCompileRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-./\\]+\.go:\d+(?::\d+)?:`)
	tsCompileRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-./\\]+\.(?:ts|tsx|js|jsx):\d+:\d+\s+-\s+error\s+TS\d+:`)
)

type ExecutionResult struct {
	Command      string   `json:"command"`
	ExitCode     int      `json:"exit_code"`
	Stdout       string   `json:"stdout"`
	Stderr       string   `json:"stderr"`
	DurationMs   int64    `json:"duration_ms"`
	IsSuccess    bool     `json:"is_success"`
	ParsedErrors []string `json:"parsed_errors"`
}

type Runner struct {
	WorkDir string
	Timeout time.Duration
}

func NewRunner(workDir string, timeout time.Duration) *Runner {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Runner{
		WorkDir: workDir,
		Timeout: timeout,
	}
}

func (r *Runner) Run(cmdStr string) (*ExecutionResult, error) {
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), r.Timeout)
	defer cancel()

	// Use shell execution with sanitized environment to prevent credential exfiltration
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = r.WorkDir
	cmd.Env = sanitizeEnvironment()
	cmd.WaitDelay = 1 * time.Second
	setupProcessGroup(cmd)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	duration := time.Since(startTime).Milliseconds()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	stdout := stdoutBuf.String()
	stderr := stderrBuf.String()

	if ctx.Err() == context.DeadlineExceeded {
		timeoutMsg := fmt.Sprintf("Command timed out after %v: %s", r.Timeout, cmdStr)
		if stderr == "" {
			stderr = timeoutMsg
		} else {
			stderr += "\n" + timeoutMsg
		}
	}

	errors := parseTracebacks(stdout, stderr)
	if ctx.Err() == context.DeadlineExceeded && len(errors) == 0 {
		errors = []string{fmt.Sprintf("Command timed out after %v: %s", r.Timeout, cmdStr)}
	}
	return &ExecutionResult{
		Command:      cmdStr,
		ExitCode:     exitCode,
		Stdout:       stdout,
		Stderr:       stderr,
		DurationMs:   duration,
		IsSuccess:    exitCode == 0,
		ParsedErrors: errors,
	}, nil
}

func parseTracebacks(stdout, stderr string) []string {
	var traces []string
	combined := stdout + "\n" + stderr
	lines := strings.Split(combined, "\n")

	inTrace := false
	var currentTrace []string

	isStart := func(line, trimmed string) bool {
		if strings.HasPrefix(trimmed, "Traceback (most recent call last):") ||
			strings.HasPrefix(trimmed, "FAIL:") ||
			strings.HasPrefix(trimmed, "--- FAIL:") ||
			strings.HasPrefix(trimmed, "ERROR:") ||
			strings.HasPrefix(trimmed, "error:") ||
			strings.HasPrefix(trimmed, "error[") ||
			strings.Contains(trimmed, "FAILED") ||
			strings.Contains(trimmed, "panic:") ||
			strings.Contains(trimmed, "panicked at") ||
			strings.Contains(trimmed, "WARNING: DATA RACE") ||
			strings.HasPrefix(trimmed, "● ") ||
			strings.HasPrefix(trimmed, "AssertionError:") {
			return true
		}
		// Unindented compiler error diagnostics (e.g. ./server.go:42:10: or src/index.ts:15:7 - error TS...)
		// Guard against indented test output lines like "    sample_test.go:10: assert failed"
		if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
			return goCompileRegex.MatchString(trimmed) || tsCompileRegex.MatchString(trimmed)
		}
		return false
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if isStart(line, trimmed) {
			if inTrace && len(currentTrace) > 0 {
				traces = append(traces, strings.Join(currentTrace, "\n"))
			}
			inTrace = true
			currentTrace = []string{line}
			continue
		}

		if inTrace {
			if strings.HasPrefix(trimmed, "=== RUN ") {
				traces = append(traces, strings.Join(currentTrace, "\n"))
				inTrace = false
				currentTrace = nil
				continue
			}

			currentTrace = append(currentTrace, line)
			if strings.HasPrefix(trimmed, "E   ") ||
				strings.Contains(line, "Error:") ||
				strings.Contains(line, "Exception:") ||
				strings.HasPrefix(trimmed, "FAIL\t") ||
				trimmed == "FAIL" ||
				strings.Contains(trimmed, "test result: FAILED") ||
				strings.Contains(trimmed, "could not compile") {
				traces = append(traces, strings.Join(currentTrace, "\n"))
				inTrace = false
				currentTrace = nil
			}
		}
	}

	if inTrace && len(currentTrace) > 0 {
		traces = append(traces, strings.Join(currentTrace, "\n"))
	}

	return traces
}

// sanitizeEnvironment builds the subprocess environment from an explicit
// ALLOWLIST (default-deny). The previous keyword DENYLIST leaked DSN-style
// secrets (DATABASE_URL, REDIS_URL, SENTRY_DSN...) into every test subprocess.
func sanitizeEnvironment() []string {
	allow := map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "LOGNAME": true, "SHELL": true,
		"LANG": true, "LC_ALL": true, "LC_CTYPE": true, "TERM": true, "TMPDIR": true,
		"VIRTUAL_ENV": true, "PYTHONPATH": true, "PYTHONUNBUFFERED": true,
		"GOPATH": true, "GOROOT": true, "GOFLAGS": true, "GOPROXY": true,
		"GOSUMDB": true, "GONOSUMDB": true, "GO111MODULE": true, "GOTOOLCHAIN": true,
		"GOPRIVATE": true, "GOINSECURE": true,
		"GOMEMLIMIT": true, "GOCACHE": true, "GOTMPDIR": true, "GOENV": true, "GODEBUG": true,
		"NODE_PATH": true, "NODE_ENV": true, "CARGO_HOME": true, "RUSTUP_HOME": true,
		"CI": true, "GITHUB_ACTIONS": true,
		// Windows platform system invariants (case-insensitive lookup below)
		"SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true,
		"TEMP": true, "TMP": true, "USERPROFILE": true, "LOCALAPPDATA": true, "APPDATA": true,
		"PROGRAMFILES": true, "PROGRAMFILES(X86)": true, "COMMONPROGRAMFILES": true, "SYSTEMDRIVE": true,
	}
	var safeEnv []string
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 && allow[strings.ToUpper(parts[0])] {
			safeEnv = append(safeEnv, entry)
		}
	}
	return safeEnv
}
