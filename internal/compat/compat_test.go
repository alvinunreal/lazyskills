package compat

import (
	"testing"

	"github.com/alvinunreal/lazyskills/internal/model"
)

func TestSanitizeLockDisplayKeepsRawSeparate(t *testing.T) {
	local := model.LocalLockEntry{
		Source:     "owner/\x1b[31mrepo",
		Ref:        "main\nnext",
		SourceType: "github",
		SkillPath:  "skills/\x1b]0;bad\x07demo/SKILL.md",
		ComputedHash: "hash\x1b[31m",
	}
	display := SanitizeLocalLockDisplay(local)
	if local.Source != "owner/\x1b[31mrepo" {
		t.Fatalf("raw local lock was mutated: %#v", local)
	}
	if display.Source != "owner/repo" || display.Ref != "main next" || display.SkillPath != "skills/demo/SKILL.md" {
		t.Fatalf("unexpected sanitized local display: %#v", display)
	}
	if display.ComputedHash != "hash" {
		t.Fatalf("expected sanitized local hash, got %#v", display)
	}

	global := model.GlobalLockEntry{
		Source:     "owner/\x1b[31mrepo",
		SourceURL:  "https://example.com/\x1b[31mrepo",
		Ref:        "main\nnext",
		SkillPath:  "skills/demo\r/SKILL.md",
		PluginName: "plugin\x07name",
		SkillFolderHash: "ghash\x1b[31m",
	}
	gdisplay := SanitizeGlobalLockDisplay(global)
	if global.SourceURL != "https://example.com/\x1b[31mrepo" {
		t.Fatalf("raw global lock was mutated: %#v", global)
	}
	if gdisplay.Source != "owner/repo" || gdisplay.SourceURL != "https://example.com/repo" || gdisplay.Ref != "main next" || gdisplay.SkillPath != "skills/demo/SKILL.md" || gdisplay.PluginName != "pluginname" {
		t.Fatalf("unexpected sanitized global display: %#v", gdisplay)
	}
	if gdisplay.SkillFolderHash != "ghash" {
		t.Fatalf("expected sanitized global hash, got %#v", gdisplay)
	}
}

func TestSanitizePreviewContentPreservesNewlines(t *testing.T) {
	got := SanitizePreviewContent("# Title\n\x1b[31mRed\x1b[0m\nNext")
	want := "# Title\nRed\nNext"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStripTerminalEscapesRemovesBareEsc(t *testing.T) {
	got := StripTerminalEscapes("before\x1b")
	if got != "before" {
		t.Fatalf("got %q", got)
	}
}
