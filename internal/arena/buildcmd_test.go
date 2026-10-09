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
}
