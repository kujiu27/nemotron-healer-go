package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectLibraryVersionPydanticV1VsV2(t *testing.T) {
	dir := t.TempDir()
	write := func(req string) {
		_ = os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte(req), 0644)
	}
	write("pydantic==1.10.13\n")
	if v := detectLibraryVersion(dir, "pydantic AttributeError"); v != "pydantic v1" {
		t.Fatalf("v1 pinned but got %q", v)
	}
	write("pydantic>=2.0\n")
	if v := detectLibraryVersion(dir, "pydantic AttributeError"); v != "pydantic v2" {
		t.Fatalf("v2 pinned but got %q", v)
	}
}

func TestBuildGroundingQueryUsesPinnedVersion(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("pydantic==1.10\n"), 0644)
	q := BuildGroundingQuery(dir, "models.py", ClassifyDefect("AttributeError: 'Order' object has no attribute", ""), "AttributeError pydantic")
	if q != "pydantic v1 breaking changes migration guide official docs" {
		t.Fatalf("query must honor pinned v1, got: %q", q)
	}
}
