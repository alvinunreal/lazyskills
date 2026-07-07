package actions

import (
	"os"
	"strings"
	"testing"

	"github.com/alvinunreal/lazyskills/internal/model"
)

func init() {
	LookPath = func(name string) (string, error) {
		return "/mock/bin/" + name, nil
	}
}

func TestForSkillPruneLockAction(t *testing.T) {
	orphaned := &model.Skill{
		Name:         "ghost",
		Scope:        model.ScopeProject,
		LocalLock:    &model.LocalLockEntry{Source: "owner/repo"},
		HealthIssues: []model.HealthIssue{{Type: "lock_without_files", Severity: "warning"}},
	}
	previews := ForSkill(orphaned)
	var prune *CommandPreview
	for i := range previews {
		if previews[i].ID == "prune_lock" {
			prune = &previews[i]
		}
	}
	if prune == nil {
		t.Fatal("expected a prune_lock action for an orphaned-lock skill")
	}
	if !prune.Available || prune.ConfirmValue != "ghost" || prune.Exec.Internal != "prune_project_lock" {
		t.Fatalf("unexpected prune preview: %+v", *prune)
	}

	healthy := &model.Skill{Name: "ok", Scope: model.ScopeProject, CanonicalPath: "/tmp/ok", LocalLock: &model.LocalLockEntry{Source: "owner/repo"}}
	for _, a := range ForSkill(healthy) {
		if a.ID == "prune_lock" {
			t.Fatal("did not expect a prune_lock action for a healthy skill")
		}
	}
}

func TestDeleteBrokenSymlinkAction(t *testing.T) {
	// Skill with a broken symlink should include delete_broken_symlink.
	withBroken := &model.Skill{
		Name:  "broken-skill",
		Scope: model.ScopeProject,
		ObservedPaths: []model.ObservedPath{
			{Path: "/tmp/link-to-nowhere", Status: model.StatusBrokenSymlink, TargetPath: "/tmp/gone"},
			{Path: "/tmp/working-link", Status: model.StatusSymlink, TargetPath: "/tmp/real"},
		},
	}
	previews := ForSkill(withBroken)
	var deleteAct *CommandPreview
	for i := range previews {
		if previews[i].ID == "delete_broken_symlink" {
			deleteAct = &previews[i]
		}
	}
	if deleteAct == nil {
		t.Fatal("expected a delete_broken_symlink action for skill with broken symlink")
	}
	if !deleteAct.Available || !deleteAct.Dangerous || !deleteAct.RequiresConfirm {
		t.Fatalf("expected dangerous+confirm action: %+v", *deleteAct)
	}
	if deleteAct.ConfirmValue != "broken-skill" || deleteAct.Exec.Internal != "delete_broken_symlink" {
		t.Fatalf("unexpected delete preview: %+v", *deleteAct)
	}
	if len(deleteAct.Exec.Args) != 2 || deleteAct.Exec.Args[0] != string(model.ScopeProject) || deleteAct.Exec.Args[1] != "broken-skill" {
		t.Fatalf("expected scoped delete args, got %+v", deleteAct.Exec.Args)
	}
	if !strings.Contains(deleteAct.Title, "broken symlink") {
		t.Fatalf("expected title to mention broken symlinks, got %q", deleteAct.Title)
	}

	// Skill WITHOUT any broken symlink should NOT include delete_broken_symlink.
	healthy := &model.Skill{
		Name:  "ok",
		Scope: model.ScopeProject,
		ObservedPaths: []model.ObservedPath{
			{Path: "/tmp/working-link", Status: model.StatusSymlink, TargetPath: "/tmp/real"},
		},
		CanonicalPath: "/tmp/ok",
	}
	for _, a := range ForSkill(healthy) {
		if a.ID == "delete_broken_symlink" {
			t.Fatal("did not expect delete_broken_symlink action for a healthy skill")
		}
	}
}

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

func TestAppLevelActions(t *testing.T) {
	previews := AppLevelActions()
	if len(previews) != 4 {
		t.Fatalf("expected 4 app-level actions, got %d", len(previews))
	}
	initAct := previewByTitle(t, previews, "Initialize skills in project")
	if initAct.ID != "skills_init" || !initAct.Mutates || !initAct.RequiresConfirm {
		t.Errorf("unexpected skills init preview: %+v", initAct)
	}

	findAct := previewByTitle(t, previews, "Find new skills (interactive)")
	if findAct.ID != "skills_find" || !findAct.Mutates || findAct.RequiresConfirm || !findAct.Exec.Interactive {
		t.Errorf("unexpected skills find preview: %+v", findAct)
	}

	updateAct := previewByTitle(t, previews, "Update project-local skills")
	if updateAct.ID != "skills_update" || !updateAct.Mutates || !updateAct.RequiresConfirm {
		t.Errorf("unexpected skills update preview: %+v", updateAct)
	}

	registryAct := previewByTitle(t, previews, "Find new skills from skills.sh")
	if registryAct.ID != "find_new_skills" || registryAct.Exec.Internal != "find_new_skills" || !registryAct.Available {
		t.Errorf("unexpected registry preview: %+v", registryAct)
	}
}

func TestMissingDepsDisablesActions(t *testing.T) {
	oldLookPath := LookPath
	defer func() {
		LookPath = oldLookPath
		ResetActionCaches()
	}()

	ResetActionCaches()
	// Simulate missing dependencies
	LookPath = func(name string) (string, error) {
		return "", os.ErrNotExist
	}
	ResetActionCaches()

	sk := &model.Skill{
		Name:          "Deploy Skill",
		Scope:         model.ScopeGlobal,
		CanonicalPath: "/tmp/deploy-skill",
		GlobalLock:    &model.GlobalLockEntry{Source: "owner/repo", SkillPath: "skills/deploy/SKILL.md", Ref: "v1"},
	}

	previews := ForSkill(sk)
	add := previewByTitle(t, previews, "Reinstall/update selected skill")
	if add.Available {
		t.Errorf("expected reinstall action to be unavailable when deps are missing")
	}
	if !strings.Contains(add.Reason, "neither 'skills' nor 'npx'") {
		t.Errorf("expected missing dependency reason, got %q", add.Reason)
	}

	appPreviews := AppLevelActions()
	appInit := previewByTitle(t, appPreviews, "Initialize skills in project")
	if appInit.Available {
		t.Errorf("expected app-level init to be unavailable when deps are missing")
	}
}

func TestForAvailableSkill(t *testing.T) {
	ResetActionCaches()
	originalLookPath := LookPath
	defer func() {
		LookPath = originalLookPath
		ResetActionCaches()
	}()
	LookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	ResetActionCaches()
	previews := ForAvailableSkill("owner/repo", "test-skill")
	if len(previews) != 1 {
		t.Fatalf("expected 1 install preview, got %d", len(previews))
	}
	inst := previews[0]
	if inst.ID != "install_skill" || !inst.Mutates || !inst.RequiresConfirm {
		t.Errorf("unexpected install preview: %+v", inst)
	}
	if !containsArg(inst.Exec.Args, "owner/repo") || !containsArg(inst.Exec.Args, "test-skill") {
		t.Errorf("expected source and skill name in args, got %+v", inst.Exec.Args)
	}
	if containsArg(inst.Exec.Args, "--global") {
		t.Errorf("expected project install by default, got %+v", inst.Exec.Args)
	}

	globalPreviews := ForAvailableSkillWithResolver("owner/repo", "test-skill", true, ResolveSkillsCommand)
	if len(globalPreviews) != 1 {
		t.Fatalf("expected 1 global install preview, got %d", len(globalPreviews))
	}
	if !containsArg(globalPreviews[0].Exec.Args, "--global") {
		t.Errorf("expected global install args to include --global, got %+v", globalPreviews[0].Exec.Args)
	}
}

func TestForAvailableSkillUnsafeRejection(t *testing.T) {
	cases := []struct {
		source string
		name   string
	}{
		{"--bad-source", "name"},
		{"source", "--bad-name"},
		{"", "name"},
		{"source", ""},
		{"source\x1b", "name"},
		{"source", "name\n"},
		{"bad\x00source", "name"},
		{"source", "name\r"},
	}
	for _, tc := range cases {
		previews := ForAvailableSkill(tc.source, tc.name)
		if len(previews) != 1 {
			t.Fatalf("expected 1 preview, got %d", len(previews))
		}
		if previews[0].Available {
			t.Errorf("expected preview to be unavailable for source=%q name=%q", tc.source, tc.name)
		}
	}
}

func TestResolveSkillsCommandMutationIsolation(t *testing.T) {
	oldLookPath := LookPath
	defer func() {
		LookPath = oldLookPath
		ResetActionCaches()
	}()

	ResetActionCaches()
	// Force it to return npx fallback with args
	LookPath = func(name string) (string, error) {
		return "", os.ErrNotExist
	}
	prog, args := ResolveSkillsCommand()
	if prog != "npx" || len(args) != 2 || args[0] != "--yes" || args[1] != "skills" {
		t.Fatalf("expected npx fallback, got prog=%q args=%#v", prog, args)
	}

	// Mutate returned slice
	args[0] = "mutated"

	// Retrieve again and verify it is isolated from the mutation
	prog2, args2 := ResolveSkillsCommand()
	if prog2 != "npx" || len(args2) != 2 || args2[0] != "--yes" || args2[1] != "skills" {
		t.Fatalf("expected npx fallback after mutation, got prog=%q args=%#v", prog2, args2)
	}

	// Now try when LookPath finds skills and it returns nil args
	ResetActionCaches()
	LookPath = func(name string) (string, error) {
		return "/usr/bin/skills", nil
	}
	prog3, args3 := ResolveSkillsCommand()
	if prog3 != "skills" || args3 != nil {
		t.Fatalf("expected skills and nil args, got prog=%q args=%#v", prog3, args3)
	}
}

func TestForAvailableSkillWithOptions(t *testing.T) {
	LookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}

	// 1. Display name differs from slug: check title and args
	previews := ForAvailableSkillWithOptions("owner/repo", InstallOptions{
		DisplayName: "My Display Name",
		Slug:        "my-slug-name",
		Global:      false,
	})
	if len(previews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(previews))
	}
	p := previews[0]
	if !p.Available {
		t.Fatalf("expected preview to be available, got unavailable with reason: %s", p.Reason)
	}
	if p.Title != "Install My Display Name to project" {
		t.Errorf("expected project title, got %q", p.Title)
	}
	if !containsArg(p.Exec.Args, "owner/repo") || !containsArg(p.Exec.Args, "my-slug-name") {
		t.Errorf("expected source and slug-name in args, got %+v", p.Exec.Args)
	}
	if containsArg(p.Exec.Args, "My Display Name") {
		t.Errorf("did not expect display name in command args: %+v", p.Exec.Args)
	}
	if containsArg(p.Exec.Args, "--global") {
		t.Errorf("did not expect global --global flag when Global=false")
	}

	// 2. Global --global flag added only when Global is true
	globalPreviews := ForAvailableSkillWithOptions("owner/repo", InstallOptions{
		DisplayName: "My Display Name",
		Slug:        "my-slug-name",
		Global:      true,
	})
	if len(globalPreviews) != 1 {
		t.Fatalf("expected 1 preview, got %d", len(globalPreviews))
	}
	gp := globalPreviews[0]
	if gp.Title != "Install My Display Name globally" {
		t.Errorf("expected global title, got %q", gp.Title)
	}
	if !containsArg(gp.Exec.Args, "--global") {
		t.Errorf("expected global --global flag in args, got %+v", gp.Exec.Args)
	}

	// 3. Unsafe source/slug rejection checks
	unsafeCases := []struct {
		desc   string
		source string
		opts   InstallOptions
	}{
		{"unsafe source", "source\x1b", InstallOptions{DisplayName: "Name", Slug: "slug"}},
		{"unsafe slug", "source", InstallOptions{DisplayName: "Name", Slug: "slug\n"}},
		{"empty slug", "source", InstallOptions{DisplayName: "Name", Slug: ""}},
		{"empty source", "", InstallOptions{DisplayName: "Name", Slug: "slug"}},
		{"option source", "-g", InstallOptions{DisplayName: "Name", Slug: "slug"}},
		{"option slug", "source", InstallOptions{DisplayName: "Name", Slug: "-g"}},
	}
	for _, tc := range unsafeCases {
		t.Run(tc.desc, func(t *testing.T) {
			previews := ForAvailableSkillWithOptions(tc.source, tc.opts)
			if len(previews) != 1 {
				t.Fatalf("expected 1 preview, got %d", len(previews))
			}
			if previews[0].Available {
				t.Errorf("expected preview to be unavailable for %+v", tc)
			}
		})
	}
}

func TestForAvailableSkills(t *testing.T) {
	ResetActionCaches()
	originalLookPath := LookPath
	defer func() {
		LookPath = originalLookPath
		ResetActionCaches()
	}()
	LookPath = func(name string) (string, error) {
		return "/usr/bin/" + name, nil
	}
	ResetActionCaches()

	skills := []AvailableSkillInstall{
		{Source: "owner/one", DisplayName: "One", Slug: "one"},
		{Source: "owner/two", DisplayName: "Two", Slug: "two"},
	}

	// Project install
	preview := ForAvailableSkills(skills, false)
	if !preview.Available || preview.ID != "bulk_install_skills" {
		t.Fatalf("expected project bulk install preview available, got %+v", preview)
	}
	if len(preview.Exec.Batch) != 2 {
		t.Fatalf("expected batch size 2, got %d", len(preview.Exec.Batch))
	}

	// Global install
	globalPreview := ForAvailableSkills(skills, true)
	if !globalPreview.Available || globalPreview.ID != "bulk_install_skills" {
		t.Fatalf("expected global bulk install preview available, got %+v", globalPreview)
	}
	if len(globalPreview.Exec.Batch) != 2 {
		t.Fatalf("expected global batch size 2, got %d", len(globalPreview.Exec.Batch))
	}
}
