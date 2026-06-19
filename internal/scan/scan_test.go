package scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"lazyskills/internal/model"
	"lazyskills/internal/visibility"
)

func writeSkill(t *testing.T, dir, name, desc string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func withHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	return home
}

func findSkill(t *testing.T, skills any, name string) map[string]any {
	t.Helper()
	list := skills.([]any)
	for _, item := range list {
		m := item.(map[string]any)
		if m["name"] == name {
			return m
		}
	}
	t.Fatalf("skill %q not found", name)
	return nil
}

func TestUniversalProjectVisibilityForUniversalAgents(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".agents", "skills", "build"), "Build Tools", "Use build tools")

	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(res.Skills))
	}
	seen := map[string]bool{}
	for _, p := range res.Skills[0].ObservedPaths {
		seen[p.Agent] = true
	}
	for _, agent := range []string{"universal", "opencode", "cursor", "codex"} {
		if !seen[agent] {
			t.Fatalf("expected %s to observe .agents/skills", agent)
		}
	}
	if seen["claude-code"] {
		t.Fatalf("claude-code should not observe project .agents/skills")
	}
}

func TestClaudeProjectDir(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".claude", "skills", "review"), "Review", "Review code")
	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(res.Skills))
	}
	if got := res.Skills[0].ObservedPaths[0].Agent; got != "claude-code" {
		t.Fatalf("expected claude-code observation, got %s", got)
	}
}

func TestDisabledProjectSkillVisibilityReason(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	localLock := `{"version":1,"skills":{"Deploy":{"source":"owner/repo","sourceType":"github","computedHash":"abc"}}}`
	if err := os.WriteFile(filepath.Join(cwd, "skills-lock.json"), []byte(localLock), 0o644); err != nil {
		t.Fatal(err)
	}
	mutated := filepath.Join(cwd, ".claude", "skills", "deploy")
	state := visibility.NewVisibilityState()
	state.Skills[visibility.StateKey(model.ScopeProject, "claude-code", "deploy", "", mutated)] = visibility.DisabledEntry{Version: 1, Scope: model.ScopeProject, Agent: "claude-code", SkillDisplayName: "Deploy", NormalizedSkillName: "deploy", LockIdentity: visibility.LockIdentity{Source: "owner/repo", SourceType: "github", ComputedHash: "abc"}, MutatedPath: mutated, ObservedStatus: model.StatusSymlink, State: visibility.StateActive}
	if err := visibility.WriteState(visibility.ProjectStatePath(cwd), state); err != nil {
		t.Fatal(err)
	}

	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, sk := range res.Skills {
		if sk.Name != "Deploy" || sk.Scope != model.ScopeProject {
			continue
		}
		for _, v := range sk.Visibility {
			if v.Agent == "claude-code" {
				found = true
				if v.Visible || v.Reason != "disabled_by_lazyskills" || v.Path != mutated {
					t.Fatalf("unexpected claude visibility: %#v", v)
				}
			}
		}
	}
	if !found {
		t.Fatalf("expected disabled claude-code visibility in %#v", res.Skills)
	}
}

func TestDisabledProjectSkillMatchesLockWhenNameDiffers(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	localLock := `{"version":1,"skills":{"deploy-folder":{"source":"owner/repo","sourceType":"github","computedHash":"abc"}}}`
	if err := os.WriteFile(filepath.Join(cwd, "skills-lock.json"), []byte(localLock), 0o644); err != nil {
		t.Fatal(err)
	}
	mutated := filepath.Join(cwd, ".claude", "skills", "deploy-folder")
	state := visibility.NewVisibilityState()
	state.Skills[visibility.StateKey(model.ScopeProject, "claude-code", "pretty-deploy", "", mutated)] = visibility.DisabledEntry{Version: 1, Scope: model.ScopeProject, Agent: "claude-code", SkillDisplayName: "Pretty Deploy", NormalizedSkillName: "pretty-deploy", LockIdentity: visibility.LockIdentity{Source: "owner/repo", SourceType: "github", ComputedHash: "abc"}, MutatedPath: mutated, ObservedStatus: model.StatusSymlink, State: visibility.StateActive}
	if err := visibility.WriteState(visibility.ProjectStatePath(cwd), state); err != nil {
		t.Fatal(err)
	}
	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range res.Skills {
		if sk.Name == "deploy-folder" {
			for _, v := range sk.Visibility {
				if v.Agent == "claude-code" && v.Reason == "disabled_by_lazyskills" {
					return
				}
			}
		}
	}
	t.Fatalf("expected disabled state matched by lock identity, got %#v", res.Skills)
}

func TestUnsafeDisabledStateIgnoredByScan(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	localLock := `{"version":1,"skills":{"Deploy":{"source":"owner/repo","sourceType":"github","computedHash":"abc"}}}`
	if err := os.WriteFile(filepath.Join(cwd, "skills-lock.json"), []byte(localLock), 0o644); err != nil {
		t.Fatal(err)
	}
	state := visibility.NewVisibilityState()
	state.Skills[visibility.StateKey(model.ScopeProject, "claude-code", "deploy", "", "/tmp/outside")] = visibility.DisabledEntry{Version: 1, Scope: model.ScopeProject, Agent: "claude-code", SkillDisplayName: "Deploy", NormalizedSkillName: "deploy", LockIdentity: visibility.LockIdentity{Source: "owner/repo", SourceType: "github", ComputedHash: "abc"}, MutatedPath: "/tmp/outside", ObservedStatus: model.StatusSymlink, State: visibility.StateActive}
	if err := visibility.WriteState(visibility.ProjectStatePath(cwd), state); err != nil {
		t.Fatal(err)
	}
	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range res.Skills {
		for _, v := range sk.Visibility {
			if v.Agent == "claude-code" && v.Reason == "disabled_by_lazyskills" {
				t.Fatalf("unsafe state should not be reported disabled: %#v", v)
			}
		}
	}
	for _, issue := range res.HealthIssues {
		if issue.Type == "invalid_visibility_state" {
			return
		}
	}
	t.Fatalf("expected invalid visibility state issue, got %#v", res.HealthIssues)
}

func TestStaleDisabledStateWithVisiblePathReportsConflict(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	canonical := filepath.Join(cwd, ".agents", "skills", "deploy")
	mutated := filepath.Join(cwd, ".claude", "skills", "deploy")
	writeSkill(t, canonical, "Deploy", "desc")
	if err := os.MkdirAll(filepath.Dir(mutated), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(canonical, mutated); err != nil {
		t.Fatal(err)
	}
	localLock := `{"version":1,"skills":{"Deploy":{"source":"owner/repo","sourceType":"github","computedHash":"abc"}}}`
	if err := os.WriteFile(filepath.Join(cwd, "skills-lock.json"), []byte(localLock), 0o644); err != nil {
		t.Fatal(err)
	}
	state := visibility.NewVisibilityState()
	state.Skills[visibility.StateKey(model.ScopeProject, "claude-code", "deploy", canonical, mutated)] = visibility.DisabledEntry{Version: 1, Scope: model.ScopeProject, Agent: "claude-code", SkillDisplayName: "Deploy", NormalizedSkillName: "deploy", LockIdentity: visibility.LockIdentity{Source: "owner/repo", SourceType: "github", ComputedHash: "abc"}, CanonicalPath: canonical, MutatedPath: mutated, ObservedStatus: model.StatusSymlink, State: visibility.StateActive}
	if err := visibility.WriteState(visibility.ProjectStatePath(cwd), state); err != nil {
		t.Fatal(err)
	}
	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, sk := range res.Skills {
		for _, v := range sk.Visibility {
			if v.Agent == "claude-code" && v.Reason == "disabled_state_conflict" && v.Visible {
				return
			}
		}
	}
	t.Fatalf("expected visible conflict for stale disabled state, got %#v", res.Skills)
}

func TestCorruptVisibilityStateAddsHealthIssue(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(visibility.ProjectStatePath(cwd)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(visibility.ProjectStatePath(cwd), []byte("{bad-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range res.HealthIssues {
		if issue.Type == "corrupt_visibility_state" {
			return
		}
	}
	t.Fatalf("expected corrupt visibility health issue, got %#v", res.HealthIssues)
}

func TestLocksCorrelateBySkillName(t *testing.T) {
	home := withHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".agents", "skills", "pretty-name"), "Pretty Name", "Pretty desc")
	writeSkill(t, filepath.Join(home, ".agents", "skills", "pretty-name"), "Pretty Name", "Pretty desc")
	localLock := `{"version":1,"skills":{"Pretty Name":{"source":"owner/repo","sourceType":"github","computedHash":"abc"}}}`
	if err := os.WriteFile(filepath.Join(cwd, "skills-lock.json"), []byte(localLock), 0o644); err != nil {
		t.Fatal(err)
	}
	globalLockPath := filepath.Join(home, ".local", "state", "skills", ".skill-lock.json")
	if err := os.MkdirAll(filepath.Dir(globalLockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	globalLock := `{"version":3,"skills":{"pretty-name":{"source":"owner/repo","sourceType":"github","sourceUrl":"https://github.com/owner/repo","skillFolderHash":"def","installedAt":"now","updatedAt":"now"}}}`
	if err := os.WriteFile(globalLockPath, []byte(globalLock), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	var sawLocal, sawGlobal bool
	for _, sk := range res.Skills {
		if sk.Scope == "project" && sk.LocalLock != nil {
			sawLocal = true
		}
		if sk.Scope == "global" && sk.GlobalLock != nil {
			sawGlobal = true
		}
	}
	if !sawLocal || !sawGlobal {
		t.Fatalf("expected scoped locks correlated, local=%v global=%v skills=%#v", sawLocal, sawGlobal, res.Skills)
	}
}

func TestGlobalCanonicalVisibleToUniversalAgents(t *testing.T) {
	home := withHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(home, ".agents", "skills", "global-build"), "Global Build", "Global build desc")

	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(res.Skills))
	}
	seen := map[string]bool{}
	for _, p := range res.Skills[0].ObservedPaths {
		if p.Scope == "global" && p.Status == "canonical" {
			seen[p.Agent] = true
		}
	}
	for _, agent := range []string{"universal", "opencode", "cursor", "codex"} {
		if !seen[agent] {
			t.Fatalf("expected %s to observe global canonical .agents/skills", agent)
		}
	}
}

func TestProjectAndGlobalSameNameAreSeparateWithShadowing(t *testing.T) {
	home := withHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".agents", "skills", "same"), "Same", "Project desc")
	writeSkill(t, filepath.Join(home, ".agents", "skills", "same"), "Same", "Global desc")

	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skills) != 2 {
		t.Fatalf("expected separate project/global skills, got %d", len(res.Skills))
	}
	for _, sk := range res.Skills {
		var shadow bool
		for _, issue := range sk.HealthIssues {
			if issue.Type == "project_global_shadowing" {
				shadow = true
			}
		}
		if !shadow {
			t.Fatalf("expected shadowing issue on %#v", sk)
		}
	}
}

func TestScopeSpecificMissingLocks(t *testing.T) {
	home := withHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".agents", "skills", "locked-global-only"), "Locked Global Only", "Project desc")
	globalLockPath := filepath.Join(home, ".local", "state", "skills", ".skill-lock.json")
	if err := os.MkdirAll(filepath.Dir(globalLockPath), 0o755); err != nil {
		t.Fatal(err)
	}
	globalLock := `{"version":3,"skills":{"Locked Global Only":{"source":"owner/repo","sourceType":"github","sourceUrl":"https://github.com/owner/repo","skillFolderHash":"def","installedAt":"now","updatedAt":"now"}}}`
	if err := os.WriteFile(globalLockPath, []byte(globalLock), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skills) != 2 {
		t.Fatalf("expected project skill plus global lock-without-files, got %d", len(res.Skills))
	}
	var missingProject, lockWithoutFiles bool
	for _, sk := range res.Skills {
		for _, issue := range sk.HealthIssues {
			missingProject = missingProject || issue.Type == "missing_project_lock"
			lockWithoutFiles = lockWithoutFiles || issue.Type == "lock_without_files"
		}
	}
	if !missingProject || !lockWithoutFiles {
		t.Fatalf("expected missing project lock and global lock_without_files, got %#v", res.Skills)
	}
}

func TestMissingSkillMDAndMetadataSanitization(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	root := filepath.Join(cwd, ".agents", "skills")
	if err := os.MkdirAll(filepath.Join(root, "missing-md"), 0o755); err != nil {
		t.Fatal(err)
	}
	esc := filepath.Join(root, "escape")
	if err := os.MkdirAll(esc, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: \"Bad\\u001b[31m Name\"\ndescription: \"Line one\\nLine two\"\n---\nbody"
	if err := os.WriteFile(filepath.Join(esc, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	var missing, sanitized bool
	for _, sk := range res.Skills {
		for _, issue := range sk.HealthIssues {
			missing = missing || issue.Type == "missing_skill_md"
		}
		if sk.Name == "Bad Name" && sk.Description == "Line one Line two" {
			sanitized = true
		}
	}
	if !missing || !sanitized {
		t.Fatalf("expected missing_skill_md and sanitized metadata, missing=%v sanitized=%v skills=%#v", missing, sanitized, res.Skills)
	}
}

func TestBrokenSymlinkAndInvalidFrontmatter(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	root := filepath.Join(cwd, ".agents", "skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(cwd, "missing"), filepath.Join(root, "broken")); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(root, "bad")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("---\nname: 123\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	var broken, invalid bool
	for _, sk := range res.Skills {
		for _, issue := range sk.HealthIssues {
			if issue.Type == "broken_symlink" {
				broken = true
			}
			if issue.Type == "invalid_frontmatter" {
				invalid = true
			}
		}
	}
	if !broken || !invalid {
		t.Fatalf("expected broken and invalid issues, broken=%v invalid=%v", broken, invalid)
	}
}

func TestSharedUniversalHealthIssuesAreDeduplicated(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	root := filepath.Join(cwd, ".agents", "skills")
	if err := os.MkdirAll(filepath.Join(root, "missing-md"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(res.Skills))
	}
	var missingCount int
	for _, issue := range res.Skills[0].HealthIssues {
		if issue.Type == "missing_skill_md" {
			missingCount++
		}
	}
	if missingCount != 1 {
		t.Fatalf("expected exactly one missing_skill_md issue, got %d issues=%#v", missingCount, res.Skills[0].HealthIssues)
	}
	seen := map[string]bool{}
	for _, observed := range res.Skills[0].ObservedPaths {
		seen[observed.Agent] = true
	}
	for _, agent := range []string{"universal", "opencode", "cursor", "codex"} {
		if !seen[agent] {
			t.Fatalf("expected observed path for %s, got %#v", agent, res.Skills[0].ObservedPaths)
		}
	}
}

func TestSnapshotBoundary(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".agents", "skills", "build"), "Build", "Build desc")

	res, err := Snapshot(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skills) != 1 || res.Skills[0].Name != "Build" {
		t.Fatalf("unexpected snapshot result: %#v", res.Skills)
	}

	scanner := New(cwd)
	res, err = scanner.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skills) != 1 || res.Skills[0].Name != "Build" {
		t.Fatalf("unexpected scanner snapshot result: %#v", res.Skills)
	}
}

func TestScanIncludesAgentStatesAndVisibilityReasons(t *testing.T) {
	home := withHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".agents", "skills", "build"), "Build", "Build desc")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Agents) < 70 {
		t.Fatalf("expected upstream-compatible agent states, got %d", len(res.Agents))
	}
	if !agentState(res.Agents, "claude-code").Detected {
		t.Fatalf("expected claude-code detected from cwd .claude")
	}
	if agentState(res.Agents, "promptscript").SupportsGlobal {
		t.Fatalf("expected promptscript global unsupported")
	}

	sk := res.Skills[0]
	if got := visibilityReason(sk.Visibility, "opencode"); got != "visible_via_universal_canonical" {
		t.Fatalf("expected opencode universal visibility, got %q", got)
	}
	if got := visibilityReason(sk.Visibility, "claude-code"); got != "missing_agent_link" {
		t.Fatalf("expected claude-code missing link visibility, got %q", got)
	}
	if got := visibilityReason(sk.Visibility, "augment"); got != "agent_not_detected" {
		t.Fatalf("expected augment not detected visibility, got %q", got)
	}
}

func TestUnsupportedGlobalAgentDoesNotGetGlobalCanonicalVisibility(t *testing.T) {
	home := withHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(home, ".agents", "skills", "global-build"), "Global Build", "desc")

	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	sk := res.Skills[0]
	if got := visibilityReason(sk.Visibility, "promptscript"); got != "unsupported_global" {
		t.Fatalf("expected promptscript unsupported global, got %q visibility=%#v", got, sk.Visibility)
	}
	for _, observed := range sk.ObservedPaths {
		if observed.Agent == "promptscript" && observed.Scope == model.ScopeGlobal {
			t.Fatalf("promptscript should not observe global canonical skills: %#v", sk.ObservedPaths)
		}
	}
}

func TestGhostAgentDirectoryWithoutCanonicalDiagnostic(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".claude", "skills", "ghost"), "Ghost", "desc")

	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	sk := res.Skills[0]
	if !hasIssue(sk.HealthIssues, "ghost_agent_skill") {
		t.Fatalf("expected ghost_agent_skill issue, got %#v", sk.HealthIssues)
	}
}

func agentState(states []model.AgentState, name string) model.AgentState {
	for _, state := range states {
		if state.Name == name {
			return state
		}
	}
	return model.AgentState{}
}

func visibilityReason(items []model.SkillVisibility, agent string) string {
	for _, item := range items {
		if item.Agent == agent {
			return item.Reason
		}
	}
	return ""
}

func hasIssue(issues []model.HealthIssue, issueType string) bool {
	for _, issue := range issues {
		if issue.Type == issueType {
			return true
		}
	}
	return false
}

func TestSnapshotLoadsPreviewOnce(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".agents", "skills", "preview"), "Preview", "Preview desc")
	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Skills) != 1 || res.Skills[0].Preview == "" {
		t.Fatalf("expected scanner snapshot to load preview, got %#v", res.Skills)
	}
}

func TestCorruptLockProducesTopLevelWarningAndJSONEncodes(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "skills-lock.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.HealthIssues) == 0 || res.HealthIssues[0].Type != "corrupt_project_lock" {
		t.Fatalf("expected corrupt_project_lock issue, got %#v", res.HealthIssues)
	}
	if _, err := json.Marshal(res); err != nil {
		t.Fatalf("result should JSON encode: %v", err)
	}
}
