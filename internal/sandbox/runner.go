package sandbox

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
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

	// Use shell execution
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = r.WorkDir

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

	errors := parseTracebacks(stdout, stderr)

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

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Traceback (most recent call last):") ||
			strings.HasPrefix(trimmed, "FAIL:") ||
			strings.HasPrefix(trimmed, "ERROR:") ||
			strings.Contains(trimmed, "FAILED") ||
			strings.Contains(trimmed, "panic:") {
			inTrace = true
			currentTrace = []string{line}
			continue
		}

		if inTrace {
			currentTrace = append(currentTrace, line)
			if strings.HasPrefix(trimmed, "E   ") ||
				strings.Contains(line, "Error:") ||
				strings.Contains(line, "Exception:") {
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
