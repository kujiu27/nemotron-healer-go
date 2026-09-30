package falsify

import "testing"

func TestDetectLanguage(t *testing.T) {
	cfgPy := detectLanguage("account.py", "pytest")
	if cfgPy.Name != "Python" {
		t.Fatalf("expected Python, got %s", cfgPy.Name)
	}

	cfgGo := detectLanguage("main.go", "go test ./...")
	if cfgGo.Name != "Go" {
		t.Fatalf("expected Go, got %s", cfgGo.Name)
	}

	cfgTS := detectLanguage("component.tsx", "vitest run")
	if cfgTS.Name != "TypeScript/JavaScript" {
		t.Fatalf("expected TS/JS, got %s", cfgTS.Name)
	}
}
