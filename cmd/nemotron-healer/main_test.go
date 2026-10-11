package main

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestSignalExitCode(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := tmpDir + "/test-signal-bin"
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	if err := buildCmd.Run(); err != nil {
		t.Skipf("cannot build binary: %v", err)
	}

	// 1. Test SIGTERM -> exit 143
	cmdTerm := exec.Command(binPath)
	outPipe1, err := cmdTerm.StdoutPipe()
	if err != nil {
		t.Fatalf("failed getting stdout pipe: %v", err)
	}
	if err := cmdTerm.Start(); err != nil {
		t.Fatalf("failed to start binary: %v", err)
	}

	// Wait for process to start and emit its first byte (guarantees signal.Notify is active)
	buf1 := make([]byte, 1)
	_, _ = outPipe1.Read(buf1)
	time.Sleep(50 * time.Millisecond)

	_ = cmdTerm.Process.Signal(syscall.SIGTERM)
	errTerm := cmdTerm.Wait()
	if exitErr, ok := errTerm.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 143 {
			t.Errorf("expected exit code 143 on SIGTERM, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected exit error on SIGTERM, got: %v", errTerm)
	}

	// 2. Test SIGINT -> exit 130
	cmdInt := exec.Command(binPath)
	outPipe2, err := cmdInt.StdoutPipe()
	if err != nil {
		t.Fatalf("failed getting stdout pipe: %v", err)
	}
	if err := cmdInt.Start(); err != nil {
		t.Fatalf("failed to start binary: %v", err)
	}

	buf2 := make([]byte, 1)
	_, _ = outPipe2.Read(buf2)
	time.Sleep(50 * time.Millisecond)

	_ = cmdInt.Process.Signal(os.Interrupt)
	errInt := cmdInt.Wait()
	if exitErr, ok := errInt.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 130 {
			t.Errorf("expected exit code 130 on SIGINT, got %d", exitErr.ExitCode())
		}
	} else {
		t.Errorf("expected exit error on SIGINT, got: %v", errInt)
	}
}
