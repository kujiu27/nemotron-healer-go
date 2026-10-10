package cli

import (
	"os"
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
