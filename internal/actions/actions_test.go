package actions

import (
	"strings"
	"testing"

	"lazyskills/internal/model"
)

func TestForSkillBuildsStructuredSanitizedCommandPreviews(t *testing.T) {
	t.Setenv("EDITOR", "")
	sk := &model.Skill{
		Name:          "Deploy Skill",
		Scope:         model.ScopeGlobal,
		CanonicalPath: "/tmp/deploy-skill",
		GlobalLock:    &model.GlobalLockEntry{Source: "owner/repo", SkillPath: "skills/deploy/SKILL.md", Ref: "v1"},
	}
	previews := ForSkill(sk)
	if len(previews) < 3 {
		t.Fatalf("expected command previews, got %#v", previews)
	}
	for _, preview := range previews {
		if strings.ContainsRune(preview.Command, '\x1b') {
			t.Fatalf("command contains escape: %#v", preview)
		}
		if preview.Available && preview.Program == "" && preview.Exec.Internal == "" {
			t.Fatalf("available preview missing program: %#v", preview)
		}
	}
	add := previewByTitle(t, previews, "Reinstall/update selected skill")
	if !add.Available || add.Exec.Program == "" || len(add.Exec.Args) == 0 {
		t.Fatalf("expected structured add preview, got %#v", add)
	}
	if !containsArg(add.Exec.Args, "--yes") {
		t.Fatalf("expected --yes in add args: %#v", add.Exec.Args)
	}
	if !strings.Contains(add.Command, "owner/repo/skills/deploy#v1") {
		t.Fatalf("expected ref and skill path preserved, got %q", add.Command)
	}
}

func TestBulkActionsBuildBatchCommands(t *testing.T) {
	skills := []*model.Skill{
		{Name: "One", Scope: model.ScopeProject, CanonicalPath: "/tmp/one", LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
		{Name: "Two", Scope: model.ScopeProject, CanonicalPath: "/tmp/two", LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
	}
	previews := ForSkillsWithResolver(skills, func() (string, []string) { return "skills", nil })
	if len(previews) != 2 {
		t.Fatalf("expected update and remove bulk actions, got %#v", previews)
	}
	if previews[0].ConfirmValue != "update 2 skills" || len(previews[0].Exec.Batch) != 2 {
		t.Fatalf("unexpected bulk update preview: %#v", previews[0])
	}
	if previews[1].ConfirmValue != "remove 2 skills" || !previews[1].Dangerous || len(previews[1].Exec.Batch) != 2 {
		t.Fatalf("unexpected bulk remove preview: %#v", previews[1])
	}
}

func TestOpenEditorActionUsesSafeEditorAndSkillPath(t *testing.T) {
	t.Setenv("EDITOR", "vim -n")
	previews := ForSkill(&model.Skill{Name: "Deploy", Scope: model.ScopeProject, SkillPath: "/tmp/deploy/SKILL.md"})
	open := previewByTitle(t, previews, "Open selected skill")
	if !open.Available || !open.Exec.Interactive || open.Exec.Program != "vim" {
		t.Fatalf("expected interactive editor action, got %#v", open)
	}
	if len(open.Exec.Args) != 2 || open.Exec.Args[0] != "-n" || open.Exec.Args[1] != "/tmp/deploy/SKILL.md" {
		t.Fatalf("unexpected editor args: %#v", open.Exec.Args)
	}
}

func TestOpenEditorRejectsUnsafeEditor(t *testing.T) {
	t.Setenv("EDITOR", "vim;rm")
	open := previewByTitle(t, ForSkill(&model.Skill{Name: "Deploy", Scope: model.ScopeProject, SkillPath: "/tmp/deploy/SKILL.md"}), "Open selected skill")
	if open.Available {
		t.Fatalf("expected unsafe editor unavailable: %#v", open)
	}
}

func TestUnsafeRawValuesSuppressExecActions(t *testing.T) {
	previews := ForSkill(&model.Skill{Name: "Deploy\x1b[31m", Scope: model.ScopeProject, CanonicalPath: "/tmp/deploy", LocalLock: &model.LocalLockEntry{Source: "owner/repo"}})
	add := previewByTitle(t, previews, "Reinstall/update selected skill")
	if add.Available {
		t.Fatalf("expected unsafe raw name to suppress executable add: %#v", add)
	}
}

func TestUnsafeRawLockFieldsSuppressExecActions(t *testing.T) {
	cases := []struct {
		name  string
		skill *model.Skill
	}{
		{"local source", &model.Skill{Name: "Deploy", Scope: model.ScopeProject, CanonicalPath: "/tmp/deploy", LocalLock: &model.LocalLockEntry{Source: "owner/\x1b[31mrepo"}}},
		{"local ref", &model.Skill{Name: "Deploy", Scope: model.ScopeProject, CanonicalPath: "/tmp/deploy", LocalLock: &model.LocalLockEntry{Source: "owner/repo", Ref: "main\x1b"}}},
		{"local skill path", &model.Skill{Name: "Deploy", Scope: model.ScopeProject, CanonicalPath: "/tmp/deploy", LocalLock: &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/\x1bdeploy/SKILL.md"}}},
		{"global source url", &model.Skill{Name: "Deploy", Scope: model.ScopeGlobal, CanonicalPath: "/tmp/deploy", GlobalLock: &model.GlobalLockEntry{SourceURL: "https://example.com/\x1bskills"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			add := previewByTitle(t, ForSkill(tc.skill), "Reinstall/update selected skill")
			if add.Available {
				t.Fatalf("expected unsafe raw lock field to suppress action: %#v", add)
			}
		})
	}
}

func TestResolverFallbackUsesNpxYesSkills(t *testing.T) {
	previews := ForSkillWithResolver(&model.Skill{Name: "Deploy", Scope: model.ScopeProject, CanonicalPath: "/tmp/deploy", LocalLock: &model.LocalLockEntry{Source: "owner/repo"}}, func() (string, []string) {
		return "npx", []string{"--yes", "skills"}
	})
	add := previewByTitle(t, previews, "Reinstall/update selected skill")
	if add.Exec.Program != "npx" || len(add.Exec.Args) < 3 || add.Exec.Args[0] != "--yes" || add.Exec.Args[1] != "skills" {
		t.Fatalf("expected npx --yes skills fallback, got %#v", add.Exec)
	}
}

func TestRemoveRequiresInstalledDirectoryIdentity(t *testing.T) {
	previews := ForSkill(&model.Skill{Name: "Display Name", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}})
	remove := previewByTitle(t, previews, "Remove selected skill")
	if remove.Available {
		t.Fatalf("expected remove unavailable without installed path identity: %#v", remove)
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

func TestRemoveUsesExactInstallBasenameWithoutSanitizing(t *testing.T) {
	previews := ForSkill(&model.Skill{Name: "Display Name", Scope: model.ScopeProject, CanonicalPath: "/tmp/Exact_Name.1", LocalLock: &model.LocalLockEntry{Source: "owner/repo"}})
	remove := previewByTitle(t, previews, "Remove selected skill")
	if !remove.Available {
		t.Fatalf("expected remove available: %#v", remove)
	}
	if remove.ConfirmValue != "Exact_Name.1" || !containsArg(remove.Exec.Args, "Exact_Name.1") {
		t.Fatalf("expected exact basename target, got confirm=%q args=%#v", remove.ConfirmValue, remove.Exec.Args)
	}
}

func TestRemoveRejectsUnsafeInstallBasename(t *testing.T) {
	previews := ForSkill(&model.Skill{Name: "Display Name", Scope: model.ScopeProject, CanonicalPath: "/tmp/bad\x1bname", LocalLock: &model.LocalLockEntry{Source: "owner/repo"}})
	remove := previewByTitle(t, previews, "Remove selected skill")
	if remove.Available {
		t.Fatalf("expected unsafe exact basename to suppress remove: %#v", remove)
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

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
