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

func TestSharedRootInfoOutsideAnchorOnlyChecksRootItself(t *testing.T) {
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
