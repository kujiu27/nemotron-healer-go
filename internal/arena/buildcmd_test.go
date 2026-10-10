package arena

import "testing"

func TestBuildTestCmdPreservesPytestModuleForm(t *testing.T) {
	a := &Arena{TestCommand: "python3 -m pytest"}
	got := a.buildTestCmd(ArenaLangConfig{Name: "Python"}, "test__red_attack_r1.py")
	if got != "python3 -m pytest test__red_attack_r1.py" {
		t.Fatalf("module form dropped: %q", got)
	}
	a2 := &Arena{TestCommand: "pytest"}
	if got2 := a2.buildTestCmd(ArenaLangConfig{Name: "Python"}, "t.py"); got2 != "pytest t.py" {
		t.Fatalf("plain form broken: %q", got2)
	}
	a3 := &Arena{TestCommand: "go test ./..."}
	if got3 := a3.buildTestCmd(ArenaLangConfig{Name: "Go"}, "x_test.go"); got3 != "go test -v -run TestRedAttack ." {
		t.Fatalf("go form broken: %q", got3)
	}
	a4 := &Arena{TestCommand: "cargo test"}
	if got4 := a4.buildTestCmd(ArenaLangConfig{Name: "Rust"}, "arena_red_attack_r1_test.rs"); got4 != "cargo test --test arena_red_attack_r1_test" {
		t.Fatalf("rust form broken: %q", got4)
	}
	a5 := &Arena{TestCommand: "npm test"}
	if got5 := a5.buildTestCmd(ArenaLangConfig{Name: "TypeScript/JavaScript"}, "red_attack_r1.test.ts", "src"); got5 != "npm test -- src/red_attack_r1.test.ts" {
		t.Fatalf("npm test form broken: %q", got5)
	}
}
