package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
)

func captureStdout(fn func()) string {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestDoctorCmd_JSONPure(t *testing.T) {
	origMock := mockFlag
	origJSON := jsonFlag
	mockFlag = true
	jsonFlag = true
	defer func() {
		mockFlag = origMock
		jsonFlag = origJSON
	}()

	cleanup := initMockServerIfEnabled()
	defer cleanup()

	out := captureStdout(func() {
		_ = doctorCmd.RunE(doctorCmd, []string{})
	})

	var res struct {
		Ready             bool            `json:"ready"`
		MockMode          bool            `json:"mock_mode"`
		InferenceEndpoint string          `json:"inference_endpoint"`
		FastModel         string          `json:"fast_model"`
		ReasoningModel    string          `json:"reasoning_model"`
		EndpointProbe     string          `json:"endpoint_probe"`
		Toolchains        map[string]bool `json:"toolchains"`
	}

	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("doctor --json stdout is not valid pure JSON: %v, raw:\n%s", err, out)
	}

	if !res.Ready {
		t.Errorf("expected ready=true in mock mode")
	}
	if !res.MockMode {
		t.Errorf("expected mock_mode=true")
	}
	if res.Toolchains == nil || !res.Toolchains["git"] {
		t.Errorf("toolchain git expected true")
	}
}

func TestVersionCmd_JSONPure(t *testing.T) {
	origJSON := jsonFlag
	jsonFlag = true
	defer func() {
		jsonFlag = origJSON
	}()

	out := captureStdout(func() {
		versionCmd.Run(versionCmd, []string{})
	})

	var res struct {
		Version  string            `json:"version"`
		Platform string            `json:"platform"`
		Models   map[string]string `json:"models"`
	}

	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("version --json stdout is not valid pure JSON: %v, raw:\n%s", err, out)
	}

	if res.Version != Version {
		t.Errorf("expected version %s, got %s", Version, res.Version)
	}
	if res.Platform != "Nebius Token Factory" {
		t.Errorf("expected platform Nebius Token Factory, got %s", res.Platform)
	}
	if res.Models == nil || res.Models["reasoning"] == "" {
		t.Errorf("expected reasoning model populated")
	}
}
