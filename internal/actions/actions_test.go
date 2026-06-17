package actions

import (
	"strings"
	"testing"

	"lazyskills/internal/model"
)

func TestForSkillBuildsStructuredSanitizedCommandPreviews(t *testing.T) {
	sk := &model.Skill{
		Name:          "Deploy Skill\x1b[31m",
		Scope:         model.ScopeGlobal,
		CanonicalPath: "/tmp/deploy-skill",
		GlobalLock:    &model.GlobalLockEntry{Source: "owner/repo\x1b[31m", SkillPath: "skills/deploy/SKILL.md", Ref: "v1"},
	}
	previews := ForSkill(sk)
	if len(previews) < 3 {
		t.Fatalf("expected command previews, got %#v", previews)
	}
	for _, preview := range previews {
		if strings.ContainsRune(preview.Command, '\x1b') {
			t.Fatalf("command contains escape: %#v", preview)
		}
		if preview.Available && preview.Program == "" {
			t.Fatalf("available preview missing program: %#v", preview)
		}
	}
	add := previewByTitle(t, previews, "Reinstall/update selected skill")
	if !add.Available || add.Program != "npx" || len(add.Args) == 0 {
		t.Fatalf("expected structured add preview, got %#v", add)
	}
	if !strings.Contains(add.Command, "owner/repo/skills/deploy#v1") {
		t.Fatalf("expected ref and skill path preserved, got %q", add.Command)
	}
}

func TestOptionLookingSkillNameSuppressesMutatingPreviews(t *testing.T) {
	previews := ForSkill(&model.Skill{Name: "--all", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}})
	add := previewByTitle(t, previews, "Reinstall/update selected skill")
	remove := previewByTitle(t, previews, "Remove selected skill")
	if add.Available || remove.Available {
		t.Fatalf("expected mutating previews unavailable, add=%#v remove=%#v", add, remove)
	}
}

func TestOptionLookingSourceSuppressesAddPreview(t *testing.T) {
	previews := ForSkill(&model.Skill{Name: "Deploy", Scope: model.ScopeProject, CanonicalPath: "/tmp/deploy", LocalLock: &model.LocalLockEntry{Source: "--all"}})
	add := previewByTitle(t, previews, "Reinstall/update selected skill")
	remove := previewByTitle(t, previews, "Remove selected skill")
	if add.Available {
		t.Fatalf("expected add preview unavailable for option-like source: %#v", add)
	}
	if !remove.Available || !strings.Contains(remove.Command, "deploy") {
		t.Fatalf("expected safe remove preview from install identity, got %#v", remove)
	}
}

func TestRemoveUsesInstallDirectoryIdentityNotDisplayName(t *testing.T) {
	previews := ForSkill(&model.Skill{Name: "Display Name", Scope: model.ScopeProject, CanonicalPath: "/tmp/display-name", LocalLock: &model.LocalLockEntry{Source: "owner/repo"}})
	remove := previewByTitle(t, previews, "Remove selected skill")
	if !remove.Available {
		t.Fatalf("expected remove preview available: %#v", remove)
	}
	if strings.Contains(remove.Command, "Display Name") || !strings.Contains(remove.Command, "display-name") {
		t.Fatalf("expected install directory target, got %q", remove.Command)
	}
}

func TestGenericGitSourceKeepsRefButDoesNotAppendSkillPath(t *testing.T) {
	previews := ForSkill(&model.Skill{Name: "Deploy", Scope: model.ScopeGlobal, CanonicalPath: "/tmp/deploy", GlobalLock: &model.GlobalLockEntry{Source: "git@github.com:owner/repo.git", Ref: "main", SkillPath: "skills/deploy/SKILL.md"}})
	add := previewByTitle(t, previews, "Reinstall/update selected skill")
	if !add.Available {
		t.Fatalf("expected add preview available: %#v", add)
	}
	if !strings.Contains(add.Command, "git@github.com:owner/repo.git#main") || strings.Contains(add.Command, "skills/deploy") {
		t.Fatalf("expected ref without appended subpath, got %q", add.Command)
	}
}

func TestGlobalNoSkillPathPrefersSourceURL(t *testing.T) {
	previews := ForSkill(&model.Skill{Name: "Deploy", Scope: model.ScopeGlobal, CanonicalPath: "/tmp/deploy", GlobalLock: &model.GlobalLockEntry{Source: "example.com", SourceURL: "https://example.com/.well-known/agent-skills/index.json", Ref: "v1"}})
	add := previewByTitle(t, previews, "Reinstall/update selected skill")
	if !add.Available {
		t.Fatalf("expected add preview available: %#v", add)
	}
	if !strings.Contains(add.Command, "https://example.com/.well-known/agent-skills/index.json#v1") {
		t.Fatalf("expected sourceUrl with ref when skillPath missing, got %q", add.Command)
	}
	if strings.Contains(add.Command, "example.com#v1") {
		t.Fatalf("did not expect source host fallback when sourceUrl exists, got %q", add.Command)
	}
}

func TestGlobalWithSkillPathPrefersSourceForSubpath(t *testing.T) {
	previews := ForSkill(&model.Skill{Name: "Deploy", Scope: model.ScopeGlobal, CanonicalPath: "/tmp/deploy", GlobalLock: &model.GlobalLockEntry{Source: "owner/repo", SourceURL: "https://github.com/owner/repo", Ref: "v1", SkillPath: "skills/deploy/SKILL.md"}})
	add := previewByTitle(t, previews, "Reinstall/update selected skill")
	if !add.Available {
		t.Fatalf("expected add preview available: %#v", add)
	}
	if !strings.Contains(add.Command, "owner/repo/skills/deploy#v1") {
		t.Fatalf("expected source with appended skill folder and ref, got %q", add.Command)
	}
}

func previewByTitle(t *testing.T, previews []CommandPreview, title string) CommandPreview {
	t.Helper()
	for _, preview := range previews {
		if preview.Title == title {
			return preview
		}
	}
	t.Fatalf("preview %q not found in %#v", title, previews)
	return CommandPreview{}
}
