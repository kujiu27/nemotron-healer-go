package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEvalCmd_AuthFailClosed(t *testing.T) {
	origKey := os.Getenv("NEBIUS_API_KEY")
	_ = os.Unsetenv("NEBIUS_API_KEY")
	defer func() {
		if origKey != "" {
			_ = os.Setenv("NEBIUS_API_KEY", origKey)
		}
	}()

	origMock := mockFlag
	mockFlag = false
	defer func() { mockFlag = origMock }()

	err := evalCmd.RunE(evalCmd, []string{})
	if err == nil {
		t.Fatalf("expected error when NEBIUS_API_KEY is unset and --mock is false, got nil")
	}
	if !strings.Contains(err.Error(), "NEBIUS_API_KEY is not set") {
		t.Errorf("expected error message to mention NEBIUS_API_KEY, got %v", err)
	}
}

func TestEvalCmd_MockRun(t *testing.T) {
	origMock := mockFlag
	mockFlag = true
	defer func() { mockFlag = origMock }()

	cleanup := initMockServerIfEnabled()
	defer cleanup()

	origCase := caseFilterFlag
	caseFilterFlag = "AHB-08"
	defer func() { caseFilterFlag = origCase }()

	origRepeat := repeatFlag
	repeatFlag = 1
	defer func() { repeatFlag = origRepeat }()

	err := evalCmd.RunE(evalCmd, []string{})
	if err != nil {
		t.Fatalf("evalCmd with --mock on AHB-08 failed: %v", err)
	}
}

func TestEvalCmd_NestedExportDirs(t *testing.T) {
	origMock := mockFlag
	mockFlag = true
	defer func() { mockFlag = origMock }()

	cleanup := initMockServerIfEnabled()
	defer cleanup()

	origCase := caseFilterFlag
	caseFilterFlag = "AHB-08"
	defer func() { caseFilterFlag = origCase }()

	origRepeat := repeatFlag
	repeatFlag = 1
	defer func() { repeatFlag = origRepeat }()

	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "sub", "reports", "eval.json")
	mdPath := filepath.Join(tmpDir, "sub", "scorecards", "eval.md")

	origJSON := exportJSONFlag
	origMD := exportMDFlag
	exportJSONFlag = jsonPath
	exportMDFlag = mdPath
	defer func() {
		exportJSONFlag = origJSON
		exportMDFlag = origMD
	}()

	err := evalCmd.RunE(evalCmd, []string{})
	if err != nil {
		t.Fatalf("evalCmd nested export failed: %v", err)
	}

	if _, err := os.Stat(jsonPath); err != nil {
		t.Fatalf("expected export json created at %s, got: %v", jsonPath, err)
	}
	if _, err := os.Stat(mdPath); err != nil {
		t.Fatalf("expected export md created at %s, got: %v", mdPath, err)
	}
}

func TestEvalCmd_JSONPure(t *testing.T) {
	origMock := mockFlag
	mockFlag = true
	defer func() { mockFlag = origMock }()

	cleanup := initMockServerIfEnabled()
	defer cleanup()

	origCase := caseFilterFlag
	caseFilterFlag = "AHB-08"
	defer func() { caseFilterFlag = origCase }()

	origRepeat := repeatFlag
	repeatFlag = 1
	defer func() { repeatFlag = origRepeat }()

	origJSON := jsonFlag
	jsonFlag = true
	defer func() { jsonFlag = origJSON }()

	out := captureStdout(func() {
		_ = evalCmd.RunE(evalCmd, []string{})
	})

	var res struct {
		Benchmark         string  `json:"benchmark"`
		TotalRuns         int     `json:"total_runs"`
		BaselineSolveRate float64 `json:"baseline_solve_rate"`
		FullSolveRate     float64 `json:"full_solve_rate"`
		EmpiricalLift     float64 `json:"empirical_lift"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("eval --json stdout not pure JSON: %v, raw:\n%s", err, out)
	}
	if res.Benchmark != "AHB-9" || res.TotalRuns != 1 {
		t.Errorf("unexpected eval json payload: %+v", res)
	}
}

func TestEvalCmd_StepSummaryPublishesScorecardWithoutJunk(t *testing.T) {
	origMock := mockFlag
	mockFlag = true
	defer func() { mockFlag = origMock }()

	cleanup := initMockServerIfEnabled()
	defer cleanup()

	origCase := caseFilterFlag
	caseFilterFlag = "AHB-08"
	defer func() { caseFilterFlag = origCase }()

	origRepeat := repeatFlag
	repeatFlag = 1
	defer func() { repeatFlag = origRepeat }()

	tmpDir := t.TempDir()
	summaryFile := filepath.Join(tmpDir, "step_summary.md")
	t.Setenv("GITHUB_STEP_SUMMARY", summaryFile)

	err := evalCmd.RunE(evalCmd, []string{})
	if err != nil {
		t.Fatalf("evalCmd run failed: %v", err)
	}

	data, err := os.ReadFile(summaryFile)
	if err != nil {
		t.Fatalf("failed reading GITHUB_STEP_SUMMARY: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "## 📊 AHB-9 Benchmark & Ablation Scorecard") {
		t.Errorf("expected GITHUB_STEP_SUMMARY to contain scorecard header, got:\n%s", content)
	}
	if !strings.Contains(content, "AHB-08") {
		t.Errorf("expected GITHUB_STEP_SUMMARY to contain AHB-08 row, got:\n%s", content)
	}
	// Verify NO intermediate sandbox failure/success reports polluted the summary
	if strings.Contains(content, "## ⚡ Nemotron-Healer Failure Diagnostic Report") {
		t.Errorf("intermediate baseline failure report leaked into GITHUB_STEP_SUMMARY:\n%s", content)
	}
	if strings.Contains(content, "## ⚡ Nemotron-Healer Autonomous Verification Report") {
		t.Errorf("intermediate healing report leaked into GITHUB_STEP_SUMMARY:\n%s", content)
	}
}

func TestCreateIsolatedSandbox_InitializesGitRepo(t *testing.T) {
	sandboxDir, err := createIsolatedSandbox("samples/external_gjson")
	if err != nil {
		t.Fatalf("createIsolatedSandbox failed: %v", err)
	}
	defer os.RemoveAll(sandboxDir)

	// Verify .git directory exists
	gitDir := filepath.Join(sandboxDir, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		t.Errorf("expected .git directory in isolated sandbox, got error: %v", err)
	}

	// Verify git status runs cleanly
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = sandboxDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("git status in sandbox failed: %v, out: %s", err, out)
	}
	if len(strings.TrimSpace(string(out))) != 0 {
		t.Errorf("expected clean git tree in sandbox, got uncommitted files: %s", out)
	}
}

func TestEvalCmd_JSONPurity(t *testing.T) {
	origMock := mockFlag
	mockFlag = true
	defer func() { mockFlag = origMock }()

	cleanup := initMockServerIfEnabled()
	defer cleanup()

	origCase := caseFilterFlag
	caseFilterFlag = "AHB-08"
	defer func() { caseFilterFlag = origCase }()

	origRepeat := repeatFlag
	repeatFlag = 1
	defer func() { repeatFlag = origRepeat }()

	origJSON := jsonFlag
	jsonFlag = true
	defer func() { jsonFlag = origJSON }()

	tmpDir := t.TempDir()
	jsonExport := filepath.Join(tmpDir, "export.json")
	mdExport := filepath.Join(tmpDir, "export.md")
	exportJSONFlag = jsonExport
	exportMDFlag = mdExport
	defer func() {
		exportJSONFlag = ""
		exportMDFlag = ""
	}()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := evalCmd.RunE(evalCmd, []string{})

	_ = w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("evalCmd with --json failed: %v", err)
	}

	var buf [4096]byte
	n, _ := r.Read(buf[:])
	_ = r.Close()
	stdoutStr := string(buf[:n])

	var payload map[string]interface{}
	if jErr := json.Unmarshal([]byte(stdoutStr), &payload); jErr != nil {
		t.Fatalf("stdout was not pure JSON, unmarshal error: %v, raw stdout:\n%s", jErr, stdoutStr)
	}
	if payload["benchmark"] != "AHB-9" {
		t.Errorf("expected benchmark AHB-9 in json payload, got %v", payload["benchmark"])
	}
}

func TestEvalCmd_AblationGroundingDelta(t *testing.T) {
	origMock := mockFlag
	mockFlag = true
	defer func() { mockFlag = origMock }()

	cleanup := initMockServerIfEnabled()
	defer cleanup()

	origCase := caseFilterFlag
	caseFilterFlag = "AHB-08"
	defer func() { caseFilterFlag = origCase }()

	origRepeat := repeatFlag
	repeatFlag = 1
	defer func() { repeatFlag = origRepeat }()

	origAblation := ablationFlag
	ablationFlag = true
	defer func() { ablationFlag = origAblation }()

	origJSON := jsonFlag
	jsonFlag = true
	defer func() { jsonFlag = origJSON }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := evalCmd.RunE(evalCmd, []string{})

	_ = w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("evalCmd with --ablation failed: %v", err)
	}

	var buf [4096]byte
	n, _ := r.Read(buf[:])
	_ = r.Close()
	stdoutStr := string(buf[:n])

	var payload map[string]interface{}
	if jErr := json.Unmarshal([]byte(stdoutStr), &payload); jErr != nil {
		t.Fatalf("unmarshal error: %v, raw:\n%s", jErr, stdoutStr)
	}

	lift, ok := payload["empirical_lift"].(float64)
	if !ok || lift <= 0 {
		t.Errorf("expected empirical lift > 0 demonstrating grounding value, got: %v", payload["empirical_lift"])
	}
}
