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

func TestArenaBuildTestCmd_SubpackageColocation(t *testing.T) {
	a := &Arena{TestCommand: "go test ./..."}
	langGo := detectArenaLang("diffmatchpatch/operation.go", "go test ./...", 1)

	// 1. Subpackage Go
	cmdGo := a.buildTestCmd(langGo, "red_attack_r1_test.go", "diffmatchpatch")
	if cmdGo != "go test -v -run TestRedAttack ./diffmatchpatch" {
		t.Errorf("expected targeted subpackage test command, got %q", cmdGo)
	}

	// 2. Subpackage Python
	aPy := &Arena{TestCommand: "python3 -m pytest"}
	langPy := detectArenaLang("service/handler.py", "python3 -m pytest", 1)
	cmdPy := aPy.buildTestCmd(langPy, "test__red_attack_r1.py", "service")
	if cmdPy != "python3 -m pytest service/test__red_attack_r1.py" {
		t.Errorf("expected colocated pytest path, got %q", cmdPy)
	}
}
