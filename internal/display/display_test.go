package display

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"lazyskills/internal/model"
)

func TestSkillViewSanitizesRenderedFields(t *testing.T) {
	sk := &model.Skill{
		Name:        "Bad\x1b[31m Name",
		Description: "Line one\nLine two",
		Scope:       model.ScopeProject,
		SkillPath:   filepath.Join(t.TempDir(), "SKILL.md"),
		Preview:     "# Hi\n\x1b[31mred",
		ObservedPaths: []model.ObservedPath{{
			Path:   "/tmp/\x1b[31mskill",
			Scope:  model.ScopeProject,
			Agent:  "opencode\x07",
			Status: model.StatusCanonical,
		}},
		LocalLock: &model.LocalLockEntry{Source: "owner/\x1b[31mrepo", SkillPath: "skills/demo\r/SKILL.md"},
		HealthIssues: []model.HealthIssue{{
			Type:    "broken_symlink",
			Message: "bad\x1b[31m message",
			Path:    "/tmp/bad\x07",
		}},
	}
	view := Skill(sk)
	if strings.Contains(view.Name, "\x1b") || strings.Contains(view.Observed[0].Path, "\x1b") || strings.Contains(view.HealthIssues[0].Message, "\x1b") {
		t.Fatalf("expected sanitized view: %#v", view)
	}
	if view.Name != "Bad Name" || view.Description != "Line one Line two" || view.LocalLock.Source != "owner/repo" {
		t.Fatalf("unexpected sanitized fields: %#v", view)
	}
	if view.Preview != "# Hi\nred" {
		t.Fatalf("unexpected preview %q", view.Preview)
	}
}

func TestSkillViewDoesNotReadPreviewFromFilesystem(t *testing.T) {
	dir := t.TempDir()
	skillPath := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("# filesystem content"), 0o644); err != nil {
		t.Fatal(err)
	}
	view := Skill(&model.Skill{Name: "Preview", Description: "desc", Scope: model.ScopeProject, SkillPath: skillPath})
	if view.Preview != "" {
		t.Fatalf("display.Skill should not read preview during render, got %q", view.Preview)
	}
}
