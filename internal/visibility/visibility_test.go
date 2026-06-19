package visibility

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lazyskills/internal/model"
)

func withTestHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	t.Setenv("AppData", filepath.Join(home, "AppData"))
	t.Setenv("LocalAppData", filepath.Join(home, "LocalAppData"))
	return home
}

func TestProjectStatePathAndGitignoreCreation(t *testing.T) {
	cwd := t.TempDir()
	statePath := ProjectStatePath(cwd)
	expectedStatePath := filepath.Join(cwd, ".lazyskills", "visibility.json")
	if statePath != expectedStatePath {
		t.Fatalf("expected state path %q, got %q", expectedStatePath, statePath)
	}

	state := NewVisibilityState()
	err := WriteState(statePath, state)
	if err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	gitignorePath := filepath.Join(cwd, ".lazyskills", ".gitignore")
	data, err := os.ReadFile(gitignorePath)
	if err != nil {
		t.Fatalf("failed to read gitignore: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "*") || !strings.Contains(content, "!.gitignore") {
		t.Fatalf("unexpected gitignore contents: %q", content)
	}
}

func TestGlobalStatePath(t *testing.T) {
	home := withTestHome(t)
	globalPath, err := GlobalStatePath()
	if err != nil {
		t.Fatalf("failed to get global state path: %v", err)
	}

	if !strings.Contains(globalPath, home) {
		t.Fatalf("expected global path %q to contain temp home %q", globalPath, home)
	}
	if !strings.HasSuffix(globalPath, filepath.Join("lazyskills", "visibility.json")) {
		t.Fatalf("expected path ending in lazyskills/visibility.json, got %q", globalPath)
	}
}

func TestAtomicStateRoundtrip(t *testing.T) {
	cwd := t.TempDir()
	statePath := ProjectStatePath(cwd)

	state := NewVisibilityState()
	entry := DisabledEntry{
		Version:             1,
		Scope:               model.ScopeProject,
		Agent:               "claude-code",
		SkillDisplayName:    "Test Skill",
		NormalizedSkillName: "test-skill",
		CanonicalPath:       "/some/canonical/path",
		MutatedPath:         "/some/mutated/path",
		ObservedStatus:      model.StatusSymlink,
		RawSymlinkTarget:    "/some/canonical/path",
		Timestamp:           time.Now().UTC(),
		State:               StateActive,
	}
	key := StateKey(model.ScopeProject, "claude-code", "test-skill", entry.CanonicalPath, entry.MutatedPath)
	state.Skills[key] = entry

	err := WriteState(statePath, state)
	if err != nil {
		t.Fatalf("failed to write state: %v", err)
	}

	readState, err := ReadState(statePath)
	if err != nil {
		t.Fatalf("failed to read state: %v", err)
	}

	readEntry, ok := readState.Skills[key]
	if !ok {
		t.Fatalf("expected state entry for key %q", key)
	}

	if readEntry.SkillDisplayName != entry.SkillDisplayName {
		t.Fatalf("expected display name %q, got %q", entry.SkillDisplayName, readEntry.SkillDisplayName)
	}
}

func TestReadCorruptStateReturnsUsableFilePlusDiagnostics(t *testing.T) {
	cwd := t.TempDir()
	statePath := ProjectStatePath(cwd)
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("{invalid-json"), 0644); err != nil {
		t.Fatal(err)
	}

	state, err := ReadState(statePath)
	if err == nil {
		t.Fatalf("expected corrupt state read to return an error")
	}
	if !strings.Contains(err.Error(), "corrupt visibility state JSON") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if state == nil || state.Skills == nil {
		t.Fatalf("expected initialized state even on corruption")
	}
}

func TestTwoPhasePendingDisableHook(t *testing.T) {
	home := withTestHome(t)
	cwd := t.TempDir()

	// Set up expected directories and paths for agent 'claude-code' project scope
	// project_dir is .claude/skills
	agentProjectDir := filepath.Join(cwd, ".claude", "skills")
	err := os.MkdirAll(agentProjectDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	canonicalPath := filepath.Join(home, "my-canonical-skill")
	err = os.MkdirAll(canonicalPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	skillMDPath := filepath.Join(canonicalPath, "SKILL.md")
	err = os.WriteFile(skillMDPath, []byte("---\nname: my-skill\n---\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	mutatedPath := filepath.Join(agentProjectDir, "my-skill")
	err = os.Symlink(canonicalPath, mutatedPath)
	if err != nil {
		t.Fatal(err)
	}

	skill := &model.Skill{
		Name:          "my-skill",
		Scope:         model.ScopeProject,
		CanonicalPath: canonicalPath,
		ObservedPaths: []model.ObservedPath{
			{
				Path:       mutatedPath,
				Scope:      model.ScopeProject,
				Agent:      "claude-code",
				Status:     model.StatusSymlink,
				TargetPath: canonicalPath,
			},
		},
	}

	var hookCalled bool
	var observedStateInHook State
	HookPostPendingDisableWrite = func() {
		hookCalled = true
		statePath := ProjectStatePath(cwd)
		s, err := ReadState(statePath)
		if err == nil {
			key := StateKey(model.ScopeProject, "claude-code", "my-skill", canonicalPath, mutatedPath)
			if entry, ok := s.Skills[key]; ok {
				observedStateInHook = entry.State
			}
		}
	}
	defer func() { HookPostPendingDisableWrite = nil }()

	err = Disable(cwd, skill, "claude-code", model.ScopeProject)
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}

	if !hookCalled {
		t.Fatalf("expected HookPostPendingDisableWrite to be called")
	}
	if observedStateInHook != StatePendingDisable {
		t.Fatalf("expected hook to observe StatePendingDisable, got %q", observedStateInHook)
	}

	// State should now be active
	s, err := ReadState(ProjectStatePath(cwd))
	if err != nil {
		t.Fatal(err)
	}
	key := StateKey(model.ScopeProject, "claude-code", "my-skill", canonicalPath, mutatedPath)
	if entry, ok := s.Skills[key]; !ok || entry.State != StateActive {
		t.Fatalf("expected state to transition to active, got: %#v", entry)
	}
}

func TestDisableRemovesOnlySymlinkSafePathValidation(t *testing.T) {
	home := withTestHome(t)
	cwd := t.TempDir()

	agentProjectDir := filepath.Join(cwd, ".claude", "skills")
	err := os.MkdirAll(agentProjectDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	canonicalPath := filepath.Join(home, "my-canonical-skill")
	err = os.MkdirAll(canonicalPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(canonicalPath, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	mutatedPath := filepath.Join(agentProjectDir, "my-skill")
	err = os.Symlink(canonicalPath, mutatedPath)
	if err != nil {
		t.Fatal(err)
	}

	skill := &model.Skill{
		Name:          "my-skill",
		Scope:         model.ScopeProject,
		CanonicalPath: canonicalPath,
		ObservedPaths: []model.ObservedPath{
			{
				Path:       mutatedPath,
				Scope:      model.ScopeProject,
				Agent:      "claude-code",
				Status:     model.StatusSymlink,
				TargetPath: canonicalPath,
			},
		},
	}

	err = Disable(cwd, skill, "claude-code", model.ScopeProject)
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}

	// Symlink should be removed
	_, err = os.Lstat(mutatedPath)
	if !os.IsNotExist(err) {
		t.Fatalf("expected symlink to be unlinked, stat returned: %v", err)
	}

	// Target/canonical directory should NOT be removed/affected
	_, err = os.Stat(canonicalPath)
	if err != nil {
		t.Fatalf("expected canonical path to remain untouched, got: %v", err)
	}
}

func TestEnableRestoresRawSymlinkTargetAndRemovesState(t *testing.T) {
	home := withTestHome(t)
	cwd := t.TempDir()

	agentProjectDir := filepath.Join(cwd, ".claude", "skills")
	err := os.MkdirAll(agentProjectDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	canonicalPath := filepath.Join(home, "my-canonical-skill")
	err = os.MkdirAll(canonicalPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(canonicalPath, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	mutatedPath := filepath.Join(agentProjectDir, "my-skill")
	err = os.Symlink(canonicalPath, mutatedPath)
	if err != nil {
		t.Fatal(err)
	}

	skill := &model.Skill{
		Name:          "my-skill",
		Scope:         model.ScopeProject,
		CanonicalPath: canonicalPath,
		ObservedPaths: []model.ObservedPath{
			{
				Path:       mutatedPath,
				Scope:      model.ScopeProject,
				Agent:      "claude-code",
				Status:     model.StatusSymlink,
				TargetPath: canonicalPath,
			},
		},
	}

	// 1. Disable
	err = Disable(cwd, skill, "claude-code", model.ScopeProject)
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}

	// 2. Enable
	err = Enable(cwd, skill, "claude-code", model.ScopeProject)
	if err != nil {
		t.Fatalf("Enable failed: %v", err)
	}

	// 3. Verify symlink is restored and points to canonical target
	info, err := os.Lstat(mutatedPath)
	if err != nil {
		t.Fatalf("expected symlink to be restored, got error: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("restored path %q is not a symlink", mutatedPath)
	}
	target, err := os.Readlink(mutatedPath)
	if err != nil {
		t.Fatal(err)
	}
	if target != canonicalPath {
		t.Fatalf("expected symlink target %q, got %q", canonicalPath, target)
	}

	// State entry should be deleted
	s, err := ReadState(ProjectStatePath(cwd))
	if err != nil {
		t.Fatal(err)
	}
	key := StateKey(model.ScopeProject, "claude-code", "my-skill", canonicalPath, mutatedPath)
	if _, ok := s.Skills[key]; ok {
		t.Fatalf("expected disabled state entry to be deleted after Enable")
	}
}

func TestEnableFailsIfDestinationExists(t *testing.T) {
	home := withTestHome(t)
	cwd := t.TempDir()

	agentProjectDir := filepath.Join(cwd, ".claude", "skills")
	err := os.MkdirAll(agentProjectDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	canonicalPath := filepath.Join(home, "my-canonical-skill")
	err = os.MkdirAll(canonicalPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(canonicalPath, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	mutatedPath := filepath.Join(agentProjectDir, "my-skill")
	err = os.Symlink(canonicalPath, mutatedPath)
	if err != nil {
		t.Fatal(err)
	}

	skill := &model.Skill{
		Name:          "my-skill",
		Scope:         model.ScopeProject,
		CanonicalPath: canonicalPath,
		ObservedPaths: []model.ObservedPath{
			{
				Path:       mutatedPath,
				Scope:      model.ScopeProject,
				Agent:      "claude-code",
				Status:     model.StatusSymlink,
				TargetPath: canonicalPath,
			},
		},
	}

	// Disable it
	err = Disable(cwd, skill, "claude-code", model.ScopeProject)
	if err != nil {
		t.Fatalf("Disable failed: %v", err)
	}

	// Now recreate something at mutatedPath (e.g. a regular file or broken symlink)
	err = os.WriteFile(mutatedPath, []byte("conflict"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Enable should fail
	err = Enable(cwd, skill, "claude-code", model.ScopeProject)
	if err == nil {
		t.Fatalf("expected Enable to fail because destination exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected conflict error message, got: %v", err)
	}

	// Test with broken symlink
	err = os.Remove(mutatedPath)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink(filepath.Join(cwd, "non-existent-target"), mutatedPath)
	if err != nil {
		t.Fatal(err)
	}

	// Enable should fail even with a broken symlink
	err = Enable(cwd, skill, "claude-code", model.ScopeProject)
	if err == nil {
		t.Fatalf("expected Enable to fail because broken symlink exists at destination")
	}
}

func TestSharedDirectoryDetection(t *testing.T) {
	withTestHome(t)
	cwd := t.TempDir()

	// qoder and qoder-cn are non-universal agents sharing a directory structure or similar.
	// Let's verify qoder / qoder-cn shared directory detection.
	// Looking at Registry:
	// qoder maps ProjectDir to: `.qoder/skills`
	// qoder-cn maps ProjectDir to: `.qoder/skills`
	// So for ScopeProject under cwd, they share the directory `cwd/.qoder/skills`.
	sharingDirs, err := SharedDirectoryAgents("qoder", model.ScopeProject, cwd)
	if err != nil {
		t.Fatalf("failed to query sharing agents: %v", err)
	}
	if len(sharingDirs) == 0 {
		t.Fatalf("expected qoder-cn to be sharing qoder's project directory")
	}
	var foundQoderCN bool
	for _, s := range sharingDirs {
		if s == "qoder-cn" {
			foundQoderCN = true
		}
	}
	if !foundQoderCN {
		t.Fatalf("expected qoder-cn to share qoder directory, got %q", sharingDirs)
	}

	// Try disabling on qoder - it should fail because of shared directory check
	skill := &model.Skill{
		Name:  "my-skill",
		Scope: model.ScopeProject,
		ObservedPaths: []model.ObservedPath{
			{
				Path:   filepath.Join(cwd, ".qoder", "skills", "my-skill"),
				Scope:  model.ScopeProject,
				Agent:  "qoder",
				Status: model.StatusSymlink,
			},
		},
	}
	err = Disable(cwd, skill, "qoder", model.ScopeProject)
	if err == nil {
		t.Fatalf("expected Disable to fail on shared directory agent qoder")
	}
	if !strings.Contains(err.Error(), "shares directory with other agent") {
		t.Fatalf("expected shared directory error message, got: %v", err)
	}
}

func TestDisableRejectsNonSymlinks(t *testing.T) {
	withTestHome(t)
	cwd := t.TempDir()

	agentProjectDir := filepath.Join(cwd, ".claude", "skills")
	err := os.MkdirAll(agentProjectDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	mutatedPath := filepath.Join(agentProjectDir, "my-skill")
	err = os.MkdirAll(mutatedPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(mutatedPath, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	skill := &model.Skill{
		Name:  "my-skill",
		Scope: model.ScopeProject,
		ObservedPaths: []model.ObservedPath{
			{
				Path:   mutatedPath,
				Scope:  model.ScopeProject,
				Agent:  "claude-code",
				Status: model.StatusCopy, // NOT a symlink
			},
		},
	}

	err = Disable(cwd, skill, "claude-code", model.ScopeProject)
	if err == nil {
		t.Fatalf("expected disable to fail for StatusCopy")
	}
	if !strings.Contains(err.Error(), "only supported for symlinks") {
		t.Fatalf("expected symlink support error, got: %v", err)
	}
}

func TestDisableRejectsPathsOutsideExpectedAgentDir(t *testing.T) {
	home := withTestHome(t)
	cwd := t.TempDir()

	// Mutated path is outside expected agent project directory `.claude/skills`
	mutatedPath := filepath.Join(cwd, "some-secret-dir", "my-skill")
	err := os.MkdirAll(filepath.Dir(mutatedPath), 0755)
	if err != nil {
		t.Fatal(err)
	}

	canonicalPath := filepath.Join(home, "my-canonical-skill")
	err = os.MkdirAll(canonicalPath, 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(canonicalPath, "SKILL.md"), []byte("---\nname: my-skill\n---\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Symlink(canonicalPath, mutatedPath)
	if err != nil {
		t.Fatal(err)
	}

	skill := &model.Skill{
		Name:          "my-skill",
		Scope:         model.ScopeProject,
		CanonicalPath: canonicalPath,
		ObservedPaths: []model.ObservedPath{
			{
				Path:       mutatedPath,
				Scope:      model.ScopeProject,
				Agent:      "claude-code",
				Status:     model.StatusSymlink,
				TargetPath: canonicalPath,
			},
		},
	}

	err = Disable(cwd, skill, "claude-code", model.ScopeProject)
	if err == nil {
		t.Fatalf("expected Disable to fail because path is outside expected agent dir")
	}
	if !strings.Contains(err.Error(), "path safety validation failed") {
		t.Fatalf("expected path safety validation error, got: %v", err)
	}

	// Reject path that is nested too deeply under expected agent dir (not a direct child)
	agentDir := filepath.Join(cwd, ".claude", "skills")
	deepPath := filepath.Join(agentDir, "nested-dir", "my-skill")
	err = os.MkdirAll(filepath.Dir(deepPath), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Symlink(canonicalPath, deepPath)
	if err != nil {
		t.Fatal(err)
	}

	skill.ObservedPaths[0].Path = deepPath
	err = Disable(cwd, skill, "claude-code", model.ScopeProject)
	if err == nil {
		t.Fatalf("expected Disable to fail because path is a nested child, not a direct child")
	}
}

func TestFindEntryForSkillRequiresAuthoritativeIdentity(t *testing.T) {
	state := NewVisibilityState()
	state.Skills[StateKey(model.ScopeProject, "claude-code", "deploy", "", "/repo/.claude/skills/deploy")] = DisabledEntry{Version: 1, Scope: model.ScopeProject, Agent: "claude-code", SkillDisplayName: "Deploy", NormalizedSkillName: "deploy", LockIdentity: LockIdentity{Source: "owner/old", SourceType: "github", ComputedHash: "old"}, MutatedPath: "/repo/.claude/skills/deploy", ObservedStatus: model.StatusSymlink, State: StateActive}

	noLockSameName := &model.Skill{Name: "Deploy", Scope: model.ScopeProject}
	if _, _, ok := FindEntryForSkill(state, noLockSameName, "claude-code", model.ScopeProject); ok {
		t.Fatalf("no-lock skill must not match lock-backed state by name only")
	}

	differentLockSameName := &model.Skill{Name: "Deploy", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/new", SourceType: "github", ComputedHash: "new"}}
	if _, _, ok := FindEntryForSkill(state, differentLockSameName, "claude-code", model.ScopeProject); ok {
		t.Fatalf("different lock identity must not match by name only")
	}

	sameLockDifferentName := &model.Skill{Name: "deploy-folder", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/old", SourceType: "github", ComputedHash: "old"}}
	if _, _, ok := FindEntryForSkill(state, sameLockDifferentName, "claude-code", model.ScopeProject); !ok {
		t.Fatalf("matching lock identity should match even when synthetic lock name differs")
	}
}
