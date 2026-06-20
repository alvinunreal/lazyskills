package locks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skills-lock.json")
	content := `{"version":1,"extra":"keep","skills":{"a":{"source":"o/r"},"b":{"source":"o/r2"}}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := RemoveEntry(path, "a"); err != nil {
		t.Fatalf("RemoveEntry: %v", err)
	}

	lock, err := ReadLocal(path)
	if err != nil {
		t.Fatalf("ReadLocal after prune: %v", err)
	}
	if _, gone := lock.Skills["a"]; gone {
		t.Error("expected entry 'a' to be removed")
	}
	if _, kept := lock.Skills["b"]; !kept {
		t.Error("expected entry 'b' to be preserved")
	}

	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), `"extra"`) {
		t.Errorf("expected unknown top-level field to be preserved, got %s", raw)
	}

	if err := RemoveEntry(path, "missing"); err == nil {
		t.Error("expected an error removing a missing key")
	}
}

func TestRemoveEntryIfExistsIgnoresMissingFileAndKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skills-lock.json")

	removed, err := RemoveEntryIfExists(path, "ghost")
	if err != nil || removed {
		t.Fatalf("expected missing lock to be ignored, removed=%v err=%v", removed, err)
	}

	content := `{"version":1,"extra":"keep","skills":{"a":{"source":"o/r"},"b":{"source":"o/r2"}}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, err = RemoveEntryIfExists(path, "missing")
	if err != nil || removed {
		t.Fatalf("expected missing key to be ignored, removed=%v err=%v", removed, err)
	}

	removed, err = RemoveEntryIfExists(path, "a")
	if err != nil || !removed {
		t.Fatalf("expected existing key to be removed, removed=%v err=%v", removed, err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), `"a"`) || !strings.Contains(string(raw), `"b"`) || !strings.Contains(string(raw), `"extra"`) {
		t.Fatalf("expected a pruned while preserving b and extra, got %s", raw)
	}
}
