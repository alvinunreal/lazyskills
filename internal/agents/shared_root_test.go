package agents

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alvinunreal/lazyskills/internal/model"
)

// sharedRootInfo needs real symlinks on a real filesystem (unlike the fake
// string-path + ExistsFunc tests elsewhere in this package), so these tests
// use t.TempDir() throughout.

func TestSharedRootInfoDetectsSymlinkedLeafSkillsDir(t *testing.T) {
	home := t.TempDir()
	claudeSkills := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	codexHome := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	codexSkills := filepath.Join(codexHome, "skills")
	if err := os.Symlink(claudeSkills, codexSkills); err != nil {
		t.Fatal(err)
	}

	link, target, shared := sharedRootInfo(codexSkills, home)
	if !shared {
		t.Fatalf("expected symlinked leaf skills dir to be detected as shared")
	}
	if link != codexSkills {
		t.Fatalf("expected link %q, got %q", codexSkills, link)
	}
	if target != claudeSkills {
		t.Fatalf("expected target %q, got %q", claudeSkills, target)
	}
}

func TestSharedRootInfoDetectsParentLevelSymlink(t *testing.T) {
	home := t.TempDir()
	claudeHome := filepath.Join(home, ".claude")
	if err := os.MkdirAll(filepath.Join(claudeHome, "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	codexHome := filepath.Join(home, ".codex")
	if err := os.Symlink(claudeHome, codexHome); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(codexHome, "skills")
	link, target, shared := sharedRootInfo(root, home)
	if !shared {
		t.Fatalf("expected parent-level symlink to be detected as shared")
	}
	if link != codexHome {
		t.Fatalf("expected link to be the symlinked parent component %q, got %q", codexHome, link)
	}
	if target != claudeHome {
		t.Fatalf("expected target %q, got %q", claudeHome, target)
	}
}

func TestSharedRootInfoIgnoresPlainRealDirectories(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, _, shared := sharedRootInfo(root, home); shared {
		t.Fatalf("expected plain real directories to not be flagged as shared")
	}
}

func TestSharedRootInfoOutsideAnchorRealDirectoryNotShared(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()

	// A real directory root outside anchor: not shared, even though it lives
	// outside the anchor entirely.
	realRoot := filepath.Join(external, "codex", "skills")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, shared := sharedRootInfo(realRoot, home); shared {
		t.Fatalf("expected real directory outside anchor to not be flagged as shared")
	}
}

func TestSharedRootInfoOutsideAnchorDetectsRootSymlink(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()

	// A symlinked root outside anchor (e.g. CODEX_HOME pointing elsewhere):
	// flagged because the root itself is a symlink.
	target := filepath.Join(external, "actual-skills")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	linkedRoot := filepath.Join(external, "codex-linked-skills")
	if err := os.Symlink(target, linkedRoot); err != nil {
		t.Fatal(err)
	}
	link, gotTarget, shared := sharedRootInfo(linkedRoot, home)
	if !shared {
		t.Fatalf("expected symlinked root outside anchor to be flagged as shared")
	}
	if link != linkedRoot {
		t.Fatalf("expected link %q, got %q", linkedRoot, link)
	}
	if gotTarget != target {
		t.Fatalf("expected target %q, got %q", target, gotTarget)
	}
}

// TestSharedRootInfoOutsideAnchorDetectsAncestorSymlink covers the P1 case:
// CODEX_HOME=/alias with /alias -> ~/.claude and /alias/skills an ordinary
// directory must still be flagged shared via the ancestor symlink.
func TestSharedRootInfoOutsideAnchorDetectsAncestorSymlink(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()

	claudeHome := filepath.Join(external, "claude-home")
	claudeSkills := filepath.Join(claudeHome, "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a marker so we can prove canonical content is the link target.
	if err := os.WriteFile(filepath.Join(claudeSkills, ".marker"), []byte("canonical"), 0o644); err != nil {
		t.Fatal(err)
	}

	alias := filepath.Join(external, "alias")
	if err := os.Symlink(claudeHome, alias); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(alias, "skills")
	// root itself is a real directory (reached through the alias symlink).
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("precondition failed: %s should be a real directory, not a symlink", root)
	}

	link, target, shared := sharedRootInfo(root, home)
	if !shared {
		t.Fatalf("expected ancestor symlink /alias to mark %s as shared", root)
	}
	if link != alias {
		t.Fatalf("expected link %q, got %q", alias, link)
	}
	if target != claudeHome {
		t.Fatalf("expected target %q, got %q", claudeHome, target)
	}
}

// TestSharedRootInfoOutsideAnchorMissingDescendantUnderSymlink still flags
// shared when a descendant beneath an earlier symlink does not exist yet
// (e.g. /alias -> target but /alias/skills has not been created).
func TestSharedRootInfoOutsideAnchorMissingDescendantUnderSymlink(t *testing.T) {
	home := t.TempDir()
	external := t.TempDir()

	targetHome := filepath.Join(external, "target-home")
	if err := os.MkdirAll(targetHome, 0o755); err != nil {
		t.Fatal(err)
	}
	// Deliberately do NOT create target-home/skills.
	alias := filepath.Join(external, "alias")
	if err := os.Symlink(targetHome, alias); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(alias, "skills")
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("precondition: skills descendant should be missing, lstat err=%v", err)
	}

	link, gotTarget, shared := sharedRootInfo(root, home)
	if !shared {
		t.Fatalf("expected missing descendant beneath symlink ancestor to still be shared")
	}
	if link != alias {
		t.Fatalf("expected link %q, got %q", alias, link)
	}
	if gotTarget != targetHome {
		t.Fatalf("expected target %q, got %q", targetHome, gotTarget)
	}
}

func TestPathInsideIsBoundaryAware(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "home", "user", ".agents", "skills")
	sibling := root + "-foo"
	child := filepath.Join(root, "my-skill")
	if pathInside(sibling, root) {
		t.Fatalf("expected sibling %q not inside %q", sibling, root)
	}
	if !pathInside(child, root) {
		t.Fatalf("expected child %q inside %q", child, root)
	}
	if !pathInside(root, root) {
		t.Fatalf("expected path inside itself")
	}
}

func TestCheckDestructivePathLiveSharedRoot(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	claudeSkills := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	codexHome := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(claudeSkills, filepath.Join(codexHome, "skills")); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(codexHome, "skills", "demo")
	if err := os.MkdirAll(skillPath, 0o755); err != nil {
		t.Fatal(err)
	}

	e := Env{Home: home, Vars: map[string]string{}, ExistsFunc: pathExists}
	if err := CheckDestructivePathWithEnv(skillPath, cwd, e); err == nil {
		t.Fatal("expected shared-root skill path to be refused")
	}

	// Owned claude path remains usable.
	owned := filepath.Join(claudeSkills, "owned")
	if err := os.MkdirAll(owned, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CheckDestructivePathWithEnv(owned, cwd, e); err != nil {
		t.Fatalf("expected owned path to be allowed, got %v", err)
	}
}

func TestCheckDestructivePathAllowsSkillEntrySymlink(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	claudeSkills := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	realSkill := filepath.Join(home, "elsewhere", "real-skill")
	if err := os.MkdirAll(realSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(claudeSkills, "linked-skill")
	if err := os.Symlink(realSkill, entry); err != nil {
		t.Fatal(err)
	}

	e := Env{Home: home, Vars: map[string]string{}, ExistsFunc: pathExists}
	// Final skill entry may be a symlink; nested parents + root ancestry are gated.
	if err := CheckDestructivePathWithEnv(entry, cwd, e); err != nil {
		t.Fatalf("expected skill-entry symlink under owned root to be allowed, got %v", err)
	}
}

// TestCheckDestructivePathRejectsSymlinkedNestedParent covers a shelf/nested
// parent (e.g. .lazyskills-disabled) that is itself a symlink while the scope
// root remains a plain directory.
func TestCheckDestructivePathRejectsSymlinkedNestedParent(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	claudeSkills := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	// Canonical shelf content elsewhere.
	canonicalShelf := filepath.Join(home, "canonical-disabled")
	if err := os.MkdirAll(filepath.Join(canonicalShelf, "shelved"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonicalShelf, "shelved", "MARKER"), []byte("canonical"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Scope root is real; nested .lazyskills-disabled is a symlink.
	if err := os.Symlink(canonicalShelf, filepath.Join(claudeSkills, ".lazyskills-disabled")); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(claudeSkills, ".lazyskills-disabled", "shelved")
	e := Env{Home: home, Vars: map[string]string{}, ExistsFunc: pathExists}
	if err := CheckDestructivePathWithEnv(path, cwd, e); err == nil {
		t.Fatal("expected symlinked nested shelf parent to be refused")
	}
	// Canonical marker must still be readable (validator is read-only).
	if data, err := os.ReadFile(filepath.Join(canonicalShelf, "shelved", "MARKER")); err != nil || string(data) != "canonical" {
		t.Fatalf("canonical shelf content must remain untouched, data=%q err=%v", data, err)
	}
}

// TestLocationsWithEnvExternalCodexHomeParentAlias is the integration case:
// CODEX_HOME points at an external alias whose parent is a symlink into
// another agent's home; LocationsWithEnv must flag the skills root shared.
func TestLocationsWithEnvExternalCodexHomeParentAlias(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	external := t.TempDir()

	claudeHome := filepath.Join(external, "claude-home")
	claudeSkills := filepath.Join(claudeHome, "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(external, "alias")
	if err := os.Symlink(claudeHome, alias); err != nil {
		t.Fatal(err)
	}
	// CODEX_HOME=/alias → skills at /alias/skills (ordinary dir via parent link).
	codexSkills := filepath.Join(alias, "skills")
	info, err := os.Lstat(codexSkills)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("precondition: skills dir itself must not be a symlink")
	}

	e := Env{
		Home:       home,
		Vars:       map[string]string{"CODEX_HOME": alias},
		ExistsFunc: pathExists,
	}
	locations := LocationsWithEnv(cwd, e)
	found := false
	for _, loc := range locations {
		if loc.AgentName == "codex" && loc.Root == codexSkills && loc.Scope == model.ScopeGlobal && !loc.Canonical {
			found = true
			if !loc.SharedRoot {
				t.Fatalf("expected external CODEX_HOME parent alias to mark shared, got %#v", loc)
			}
			if loc.SharedRootLink != alias {
				t.Fatalf("expected shared link %q, got %#v", alias, loc)
			}
		}
	}
	if !found {
		t.Fatalf("expected codex location at %s, got %#v", codexSkills, locations)
	}

	// Live destructive check on an install path under that root must refuse.
	skillPath := filepath.Join(codexSkills, "demo")
	if err := CheckDestructivePathWithEnv(skillPath, cwd, e); err == nil {
		t.Fatal("expected destructive check to refuse path under aliased external CODEX_HOME")
	}
}

func TestDestructiveSkillInstallPathsIncludesUnobservedRoots(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	e := Env{Home: home, Vars: map[string]string{}, ExistsFunc: pathExists}
	// No skills installed anywhere — paths still enumerate every global root.
	paths, err := DestructiveSkillInstallPathsWithEnv(cwd, "demo", model.ScopeGlobal, nil, e)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 2 {
		t.Fatalf("expected multiple global install paths including unobserved roots, got %#v", paths)
	}
	seenClaude, seenCodex := false, false
	for _, p := range paths {
		if p == filepath.Join(home, ".claude", "skills", "demo") {
			seenClaude = true
		}
		if p == filepath.Join(home, ".codex", "skills", "demo") {
			seenCodex = true
		}
	}
	if !seenClaude || !seenCodex {
		t.Fatalf("expected claude and codex install paths among %#v", paths)
	}
}

func TestCheckDestructivePathUnknownLocationFailsClosed(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	e := Env{Home: home, Vars: map[string]string{}, ExistsFunc: pathExists}
	orphan := filepath.Join(t.TempDir(), "orphan-skill")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CheckDestructivePathWithEnv(orphan, cwd, e); err == nil {
		t.Fatal("expected unknown location to fail closed")
	}
}

func TestSharedRootInfoSiblingNotConfusedWithRoot(t *testing.T) {
	home := t.TempDir()
	anchor := filepath.Join(home, ".agents", "skills")
	if err := os.MkdirAll(anchor, 0o755); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(home, ".agents", "skills-foo")
	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}

	// skills-foo is not "under" skills despite sharing a string prefix; it
	// must be treated as outside the anchor (and here it's a plain real
	// directory, so not shared).
	if _, _, shared := sharedRootInfo(sibling, anchor); shared {
		t.Fatalf("expected ~/.agents/skills-foo to not be confused with ~/.agents/skills")
	}
}

func TestSharedRootInfoMissingComponentIsNotShared(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".codex", "skills")
	// Neither home/.codex nor home/.codex/skills exists.
	if _, _, shared := sharedRootInfo(root, home); shared {
		t.Fatalf("expected missing components to not be flagged as shared")
	}
}

func TestLocationsWithEnvPropagatesSharedRootThroughSymlinkedScopeRoot(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	claudeSkills := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	codexHome := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(claudeSkills, filepath.Join(codexHome, "skills")); err != nil {
		t.Fatal(err)
	}

	e := Env{Home: home, Vars: map[string]string{}, ExistsFunc: pathExists}
	locations := LocationsWithEnv(cwd, e)

	codexSkills := filepath.Join(codexHome, "skills")
	foundCodex, foundClaude := false, false
	for _, loc := range locations {
		if loc.Scope != model.ScopeGlobal {
			continue
		}
		// codex is Universal, so it also has a global-canonical location
		// (~/.agents/skills) in addition to its own GlobalDir; only the
		// latter is reached through the symlink under test.
		if loc.AgentName == "codex" && loc.Root == codexSkills {
			foundCodex = true
			if !loc.SharedRoot {
				t.Fatalf("expected codex global location to be flagged shared, got %#v", loc)
			}
			if loc.SharedRootLink != codexSkills {
				t.Fatalf("unexpected shared root link: %#v", loc)
			}
			if loc.SharedRootTarget != claudeSkills {
				t.Fatalf("unexpected shared root target: %#v", loc)
			}
		}
		if loc.AgentName == "claude-code" && loc.Root == claudeSkills {
			foundClaude = true
			if loc.SharedRoot {
				t.Fatalf("expected claude-code global location to not be flagged shared, got %#v", loc)
			}
		}
	}
	if !foundCodex || !foundClaude {
		t.Fatalf("expected both codex and claude-code global locations, got %#v", locations)
	}
}
