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

	// Rust
	cfgRust := detectArenaLang("src/main.rs", "cargo test", 1)
	if cfgRust.Name != "Rust" {
		t.Fatalf("expected Rust, got %s", cfgRust.Name)
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

	// 3. Rust
	aRust := &Arena{TestCommand: "cargo test"}
	langRust := detectArenaLang("src/lib.rs", "cargo test", 1)
	cmdRust := aRust.buildTestCmd(langRust, "red_attack_r1_test.rs", "src")
	if cmdRust != "cargo test" {
		t.Errorf("expected 'cargo test', got %q", cmdRust)
	}
}

func TestIsAttackSyntaxOrImportError(t *testing.T) {
	if !isAttackSyntaxOrImportError("SyntaxError: invalid syntax", "Python") {
		t.Errorf("expected Python SyntaxError detected")
	}
	if !isAttackSyntaxOrImportError("# test\n[build failed]\n./x.go:1:2: undefined: Foo", "Go") {
		t.Errorf("expected Go build failure detected")
	}
	if isAttackSyntaxOrImportError("FAIL: assert balance == 100", "Python") {
		t.Errorf("assertion failure must not be treated as syntax error")
	}
}
