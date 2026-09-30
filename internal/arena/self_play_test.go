package arena

import "testing"

func TestDetectArenaLang(t *testing.T) {
	// Python
	cfgPy := detectArenaLang("models.py", "pytest", 1)
	if cfgPy.Name != "Python" {
		t.Fatalf("expected Python, got %s", cfgPy.Name)
	}

	// Go
	cfgGo := detectArenaLang("service.go", "go test ./...", 2)
	if cfgGo.Name != "Go" {
		t.Fatalf("expected Go, got %s", cfgGo.Name)
	}

	// TS
	cfgTS := detectArenaLang("index.ts", "npm test", 1)
	if cfgTS.Name != "TypeScript/JavaScript" {
		t.Fatalf("expected TS/JS, got %s", cfgTS.Name)
	}
}
