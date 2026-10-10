package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
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
		TavilyProbe       string          `json:"tavily_probe"`
		TavilyRTTMS       int64           `json:"tavily_rtt_ms"`
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
	if res.TavilyProbe == "" {
		t.Errorf("expected tavily_probe to be populated")
	}
}

func TestDoctorCmd_TavilyAuthFailClosed(t *testing.T) {
	origMock := mockFlag
	origJSON := jsonFlag
	mockFlag = false
	jsonFlag = true
	defer func() {
		mockFlag = origMock
		jsonFlag = origJSON
	}()

	t.Setenv("NEBIUS_BASE_URL", "http://127.0.0.1:9")
	t.Setenv("TAVILY_BASE_URL", "http://127.0.0.1:9")
	t.Setenv("NEBIUS_API_KEY", "dummy-key")
	t.Setenv("TAVILY_API_KEY", "bad-key")

	out := captureStdout(func() {
		_ = doctorCmd.RunE(doctorCmd, []string{})
	})

	var res struct {
		Ready          bool     `json:"ready"`
		FailureReasons []string `json:"failure_reasons"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("stdout not valid JSON: %v, raw:\n%s", err, out)
	}
	if res.Ready {
		t.Errorf("expected ready=false when Tavily is unreachable")
	}
	foundTavily := false
	for _, reason := range res.FailureReasons {
		if strings.Contains(reason, "Tavily API") {
			foundTavily = true
			break
		}
	}
	if !foundTavily {
		t.Errorf("expected failure_reasons to include Tavily API error, got: %v", res.FailureReasons)
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
