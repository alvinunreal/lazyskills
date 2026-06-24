package scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alvinunreal/lazyskills/internal/model"
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

func TestOpenCodeNativeProjectDir(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()

	t.Run("native path is tracked", func(t *testing.T) {
		writeSkill(t, filepath.Join(cwd, ".opencode", "skills", "build"), "Build", "Build code")

		res, err := Run(cwd)
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Skills) != 1 {
			t.Fatalf("expected 1 skill, got %d", len(res.Skills))
		}
		sk := res.Skills[0]
		seen := map[string]bool{}
		for _, p := range sk.ObservedPaths {
			if p.Scope == model.ScopeProject {
				seen[p.Agent] = true
			}
		}
		if !seen["opencode"] {
			t.Fatalf("expected opencode to observe native .opencode/skills, got %#v", sk.ObservedPaths)
		}
		if !agentState(res.Agents, "opencode").ProjectDirExists {
			t.Fatalf("expected opencode project dir to be marked present, got %#v", agentState(res.Agents, "opencode"))
		}
		if got := agentState(res.Agents, "opencode").ProjectDir; got != filepath.Join(cwd, ".opencode", "skills") {
			t.Fatalf("expected opencode project dir to point at native path, got %q", got)
		}
	})

	t.Run("legacy-only path does not imply native path exists", func(t *testing.T) {
		legacyOnlyCwd := t.TempDir()
		writeSkill(t, filepath.Join(legacyOnlyCwd, ".agents", "skills", "build"), "Build", "Legacy build")

		res, err := Run(legacyOnlyCwd)
		if err != nil {
			t.Fatal(err)
		}
		if agentState(res.Agents, "opencode").ProjectDirExists {
			t.Fatalf("expected opencode native project dir existence to ignore legacy-only path, got %#v", agentState(res.Agents, "opencode"))
		}
	})
}

func TestOpenCodeNativeRecordOverridesLegacyAliasMetadata(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()
	writeSkill(t, filepath.Join(cwd, ".agents", "skills", "build"), "Build", "Legacy build")
	writeSkill(t, filepath.Join(cwd, ".opencode", "skills", "build"), "Build", "Native build")

	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	var sk *model.Skill
	for i := range res.Skills {
		if res.Skills[i].Name == "Build" && res.Skills[i].Scope == model.ScopeProject {
			sk = &res.Skills[i]
			break
		}
	}
	if sk == nil {
		t.Fatalf("expected merged Build skill, got %#v", res.Skills)
	}
	if sk.CanonicalPath != filepath.Join(cwd, ".opencode", "skills", "build") {
		t.Fatalf("expected canonical path to prefer native OpenCode, got %q", sk.CanonicalPath)
	}
	if sk.SkillPath != filepath.Join(cwd, ".opencode", "skills", "build", "SKILL.md") {
		t.Fatalf("expected skill path to prefer native OpenCode, got %q", sk.SkillPath)
	}
	if !strings.Contains(sk.Preview, "Native build") || strings.Contains(sk.Preview, "Legacy build") {
		t.Fatalf("expected preview to come from native OpenCode record, got %q", sk.Preview)
	}
}

func TestOpenCodeNativeDiagnosticsSurviveLegacyAlias(t *testing.T) {
	tests := []struct {
		name       string
		setupNative func(t *testing.T, cwd string)
		issueType  string
	}{
		{
			name: "missing_skill_md",
			setupNative: func(t *testing.T, cwd string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(cwd, ".opencode", "skills", "build"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			issueType: "missing_skill_md",
		},
		{
			name: "invalid_frontmatter",
			setupNative: func(t *testing.T, cwd string) {
				t.Helper()
				dir := filepath.Join(cwd, ".opencode", "skills", "build")
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: 123\n---\nbody"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			issueType: "invalid_frontmatter",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withHome(t)
			cwd := t.TempDir()
			writeSkill(t, filepath.Join(cwd, ".agents", "skills", "build"), "Build", "Legacy build")
			tc.setupNative(t, cwd)

			res, err := Run(cwd)
			if err != nil {
				t.Fatal(err)
			}
			var sk *model.Skill
			for i := range res.Skills {
				if res.Skills[i].Name == "Build" && res.Skills[i].Scope == model.ScopeProject {
					sk = &res.Skills[i]
					break
				}
			}
			if sk == nil {
				t.Fatalf("expected merged Build skill, got %#v", res.Skills)
			}
			if !hasIssue(sk.HealthIssues, tc.issueType) {
				t.Fatalf("expected %s issue, got %#v", tc.issueType, sk.HealthIssues)
			}
		})
	}
}

func TestHiddenSkillDirectoriesAreIgnored(t *testing.T) {
	home := withHome(t)
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".codex", "skills", ".system"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, filepath.Join(home, ".codex", "skills", "review"), "Review", "Review code")
	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, skill := range res.Skills {
		if skill.Name == ".system" {
			t.Fatalf("expected hidden .system directory to be ignored, got %#v", skill)
		}
	}
	if len(res.Skills) != 1 || res.Skills[0].Name != "Review" {
		t.Fatalf("expected only normal skill, got %#v", res.Skills)
	}
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

func TestPreflightChecking(t *testing.T) {
	oldLookPath := LookPath
	defer func() { LookPath = oldLookPath }()

	LookPath = func(name string) (string, error) {
		if name == "skills" {
			return "/usr/local/bin/skills", nil
		}
		if name == "node" {
			return "/usr/local/bin/node", nil
		}
		return "", os.ErrNotExist
	}

	withHome(t)
	cwd := t.TempDir()
	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}

	if res.Preflight == nil {
		t.Fatal("expected Preflight to not be nil")
	}

	if !res.Preflight.CanRunSkills {
		t.Error("expected CanRunSkills to be true when skills exists")
	}

	if !res.Preflight.Tools["skills"].Exists || res.Preflight.Tools["skills"].Path != "/usr/local/bin/skills" {
		t.Errorf("expected skills tool status to reflect exists/path, got %+v", res.Preflight.Tools["skills"])
	}

	if res.Preflight.Tools["npx"].Exists {
		t.Error("expected npx to not exist")
	}

	// Now check when skills does not exist, but npx/node/npm exist
	LookPath = func(name string) (string, error) {
		if name == "npx" {
			return "/usr/local/bin/npx", nil
		}
		if name == "node" {
			return "/usr/local/bin/node", nil
		}
		if name == "npm" {
			return "/usr/local/bin/npm", nil
		}
		return "", os.ErrNotExist
	}

	res, err = Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Preflight.CanRunSkills {
		t.Error("expected CanRunSkills to be true when npx, node, npm exist but skills does not")
	}

	// Now check when skills does not exist and npx exists, but node is missing
	LookPath = func(name string) (string, error) {
		if name == "npx" {
			return "/usr/local/bin/npx", nil
		}
		if name == "npm" {
			return "/usr/local/bin/npm", nil
		}
		return "", os.ErrNotExist
	}
	res, err = Run(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if res.Preflight.CanRunSkills {
		t.Error("expected CanRunSkills to be false when npx exists but node is missing")
	}
}

func TestScanDisabledSkills(t *testing.T) {
	withHome(t)
	cwd := t.TempDir()

	// 1. Create a disabled skill inside project-local aider-desk agent root
	disabledDir := filepath.Join(cwd, ".aider-desk", "skills", ".lazyskills-disabled", "review")
	writeSkill(t, disabledDir, "Review", "Review code")

	// Run scan
	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}

	// Verify we found the disabled skill
	found := false
	for _, sk := range res.Skills {
		if sk.Name == "Review" {
			found = true
			if !sk.Disabled {
				t.Errorf("expected skill 'Review' to be disabled, got %#v", sk)
			}
			if len(sk.ObservedPaths) != 1 {
				t.Fatalf("expected 1 observed path, got %d", len(sk.ObservedPaths))
			}
			obs := sk.ObservedPaths[0]
			if obs.Status != model.StatusDisabled {
				t.Errorf("expected status 'disabled', got %s", obs.Status)
			}
			expectedTarget := filepath.Join(cwd, ".aider-desk", "skills", "review")
			if obs.TargetPath != expectedTarget {
				t.Errorf("expected target path %q, got %q", expectedTarget, obs.TargetPath)
			}
		}
	}
	if !found {
		t.Fatal("expected to find disabled skill 'Review'")
	}
}

func TestScanDisabledRelativeSymlinkDoesNotReportBroken(t *testing.T) {
	home := withHome(t)
	cwd := t.TempDir()

	targetDir := filepath.Join(home, "source-skills", "cloudflare")
	writeSkill(t, targetDir, "Cloudflare", "Cloudflare skills")

	disabledRoot := filepath.Join(home, ".claude", "skills", ".lazyskills-disabled")
	if err := os.MkdirAll(disabledRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// This is the kind of relative symlink that works from ~/.claude/skills,
	// but becomes broken according to the OS after it is moved into the disabled
	// shelf. Disabled scans should resolve it relative to the active root and not
	// report a false broken_symlink health issue.
	if err := os.Symlink(filepath.Join("..", "..", "source-skills", "cloudflare"), filepath.Join(disabledRoot, "cloudflare")); err != nil {
		t.Fatal(err)
	}

	res, err := Run(cwd)
	if err != nil {
		t.Fatal(err)
	}

	var found *model.Skill
	for _, sk := range res.Skills {
		if sk.Name == "Cloudflare" {
			found = sk
			break
		}
	}
	if found == nil {
		t.Fatal("expected to find disabled symlink skill 'Cloudflare'")
	}
	if !found.Disabled {
		t.Fatalf("expected Cloudflare skill to be disabled, got %#v", found)
	}
	for _, issue := range found.HealthIssues {
		if issue.Type == "broken_symlink" {
			t.Fatalf("disabled symlink should not report broken_symlink: %#v", issue)
		}
	}
	if found.Preview == "" {
		t.Fatal("expected disabled symlink target preview to be parsed")
	}
}
