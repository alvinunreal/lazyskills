package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/model"
)

func TestVisibilityRepairWizardPreview(t *testing.T) {
	home := setupVisibilityRepairEnv(t)
	cwd := t.TempDir()
	source := filepath.Join(cwd, ".agents", "skills", "build")
	writeSkillDoc(t, source, "Build", "Build desc")

	m := newVisibilityRepairTestModel(cwd, &model.Skill{
		Name:          "Build",
		Scope:         model.ScopeProject,
		CanonicalPath: source,
		LocalLock:     &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/build/SKILL.md"},
		ObservedPaths: []model.ObservedPath{{Agent: "opencode", Scope: model.ScopeProject, Path: source, Status: model.StatusCanonical}},
		Visibility: []model.SkillVisibility{
			{Agent: "opencode", Display: "OpenCode", Visible: true, Reason: "visible_via_universal_canonical", Path: source, Status: model.StatusCanonical},
			{Agent: "claude-code", Display: "Claude Code", Visible: false, Reason: "missing_agent_link"},
			{Agent: "augment", Display: "Augment", Visible: false, Reason: "agent_not_detected"},
			{Agent: "promptscript", Display: "PromptScript", Visible: false, Reason: "unsupported_global"},
		},
	}, []model.AgentState{
		{Name: "opencode", Display: "OpenCode", Supported: true, Detected: true, Universal: true, SupportsGlobal: true, ProjectDir: filepath.Join(cwd, ".agents", "skills"), GlobalDir: filepath.Join(home, ".config", "opencode", "skills"), ProjectDirExists: true, GlobalDirExists: true},
		{Name: "claude-code", Display: "Claude Code", Supported: true, Detected: true, Universal: false, SupportsGlobal: true, ProjectDir: filepath.Join(home, ".claude", "skills"), GlobalDir: filepath.Join(home, ".claude", "skills"), ProjectDirExists: true, GlobalDirExists: true},
		{Name: "augment", Display: "Augment", Supported: true, Detected: false, Universal: false, SupportsGlobal: true, ProjectDir: filepath.Join(home, ".augment", "skills"), GlobalDir: filepath.Join(home, ".augment", "skills"), ProjectDirExists: true, GlobalDirExists: true},
		{Name: "promptscript", Display: "PromptScript", Supported: true, Detected: true, Universal: true, SupportsGlobal: false, ProjectDir: filepath.Join(cwd, ".agents", "skills"), ProjectDirExists: true},
	})

	acts := m.currentActions()
	preview := actionByTitle(t, acts, "Fix visibility…")
	if !preview.Available {
		t.Fatalf("expected repair action to be available, got %#v", preview)
	}

	m.commands = true
	m.action = actionIndex(t, m, "Fix visibility…")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if cmd != nil {
		t.Fatalf("expected no command when opening visibility repair wizard, got %T", cmd)
	}
	if !m.visibilityRepairModal {
		t.Fatal("expected visibility repair wizard to open")
	}
	view := m.View()
	for _, want := range []string{"Visibility Repair Wizard", "Claude Code", "missing", "Augment", "not detected", "PromptScript", "unsupported", "ln -s"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expected wizard view to contain %q, got:\n%s", want, view)
		}
	}
}

func TestVisibilityRepairWizardCopyPreview(t *testing.T) {
	home := setupVisibilityRepairEnv(t)
	cwd := t.TempDir()
	source := filepath.Join(cwd, ".agents", "skills", "build")
	writeSkillDoc(t, source, "Build", "Build desc")

	m := newVisibilityRepairTestModel(cwd, &model.Skill{
		Name:          "Build",
		Scope:         model.ScopeProject,
		CanonicalPath: source,
		LocalLock:     &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/build/SKILL.md"},
		ObservedPaths: []model.ObservedPath{
			{Agent: "opencode", Scope: model.ScopeProject, Path: source, Status: model.StatusCanonical},
			{Agent: "copy-agent", Scope: model.ScopeProject, Path: filepath.Join(cwd, ".copies", "build"), Status: model.StatusCopy},
		},
		Visibility: []model.SkillVisibility{
			{Agent: "opencode", Display: "OpenCode", Visible: true, Reason: "visible_via_universal_canonical", Path: source, Status: model.StatusCanonical},
			{Agent: "copy-agent", Display: "Copy Agent", Visible: true, Reason: "visible_via_copy", Path: filepath.Join(cwd, ".copies", "build"), Status: model.StatusCopy},
			{Agent: "claude-code", Display: "Claude Code", Visible: false, Reason: "missing_agent_link"},
		},
	}, []model.AgentState{
		{Name: "opencode", Display: "OpenCode", Supported: true, Detected: true, Universal: true, SupportsGlobal: true, ProjectDir: filepath.Join(cwd, ".agents", "skills"), GlobalDir: filepath.Join(home, ".config", "opencode", "skills"), ProjectDirExists: true, GlobalDirExists: true},
		{Name: "copy-agent", Display: "Copy Agent", Supported: true, Detected: true, Universal: false, SupportsGlobal: true, ProjectDir: filepath.Join(home, ".copy-agent", "skills"), GlobalDir: filepath.Join(home, ".copy-agent", "skills"), ProjectDirExists: true, GlobalDirExists: true},
		{Name: "claude-code", Display: "Claude Code", Supported: true, Detected: true, Universal: false, SupportsGlobal: true, ProjectDir: filepath.Join(home, ".claude", "skills"), GlobalDir: filepath.Join(home, ".claude", "skills"), ProjectDirExists: true, GlobalDirExists: true},
	})

	m.commands = true
	m.action = actionIndex(t, m, "Fix visibility…")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if cmd != nil {
		t.Fatalf("expected no command when opening visibility repair wizard, got %T", cmd)
	}
	if !m.visibilityRepairModal {
		t.Fatal("expected visibility repair wizard to open")
	}
	view := m.View()
	if !strings.Contains(view, "cp -R") {
		t.Fatalf("expected copy strategy preview, got:\n%s", view)
	}
}

func TestVisibilityRepairWizardApplyAndRescan(t *testing.T) {
	for _, tc := range []struct {
		name        string
		copyMode    bool
		wantReason  string
		wantSymlink bool
	}{
		{name: "symlink", copyMode: false, wantReason: "visible_via_symlink", wantSymlink: true},
		{name: "copy", copyMode: true, wantReason: "visible_via_copy", wantSymlink: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := setupVisibilityRepairEnv(t)
			cwd := t.TempDir()
			source := filepath.Join(cwd, ".agents", "skills", "build")
			writeSkillDoc(t, source, "Build", "Build desc")

			skill := &model.Skill{
				Name:          "Build",
				Scope:         model.ScopeProject,
				CanonicalPath: source,
				LocalLock:     &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/build/SKILL.md"},
				ObservedPaths: []model.ObservedPath{{Agent: "opencode", Scope: model.ScopeProject, Path: source, Status: model.StatusCanonical}},
				Visibility: []model.SkillVisibility{
					{Agent: "opencode", Display: "OpenCode", Visible: true, Reason: "visible_via_universal_canonical", Path: source, Status: model.StatusCanonical},
					{Agent: "claude-code", Display: "Claude Code", Visible: false, Reason: "missing_agent_link"},
				},
			}
			if tc.copyMode {
				skill.ObservedPaths = append(skill.ObservedPaths, model.ObservedPath{Agent: "copy-agent", Scope: model.ScopeProject, Path: filepath.Join(cwd, ".copies", "build"), Status: model.StatusCopy})
				skill.Visibility = append(skill.Visibility, model.SkillVisibility{Agent: "copy-agent", Display: "Copy Agent", Visible: true, Reason: "visible_via_copy", Path: filepath.Join(cwd, ".copies", "build"), Status: model.StatusCopy})
			}

			m := newVisibilityRepairTestModel(cwd, skill, []model.AgentState{
				{Name: "opencode", Display: "OpenCode", Supported: true, Detected: true, Universal: true, SupportsGlobal: true, ProjectDir: filepath.Join(cwd, ".agents", "skills"), GlobalDir: filepath.Join(home, ".config", "opencode", "skills"), ProjectDirExists: true, GlobalDirExists: true},
				{Name: "claude-code", Display: "Claude Code", Supported: true, Detected: true, Universal: false, SupportsGlobal: true, ProjectDir: filepath.Join(home, ".claude", "skills"), GlobalDir: filepath.Join(home, ".claude", "skills"), ProjectDirExists: true, GlobalDirExists: true},
			})
			if tc.copyMode {
				m.result.Agents = append(m.result.Agents, model.AgentState{Name: "copy-agent", Display: "Copy Agent", Supported: true, Detected: true, Universal: false, SupportsGlobal: true, ProjectDir: filepath.Join(home, ".copy-agent", "skills"), GlobalDir: filepath.Join(home, ".copy-agent", "skills"), ProjectDirExists: true, GlobalDirExists: true})
			}

			m.commands = true
			m.action = actionIndex(t, m, "Fix visibility…")
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(appModel)
			if cmd != nil {
				t.Fatalf("expected no command when opening visibility repair wizard, got %T", cmd)
			}
			if !m.visibilityRepairModal {
				t.Fatal("expected visibility repair wizard to open")
			}

			preview, ok := m.visibilityRepairSelectionPreview()
			if !ok || !preview.Available {
				t.Fatalf("expected selectable visibility repair preview, got %#v ok=%v", preview, ok)
			}

			modelTmp, cmd := m.applyVisibilityRepairPreview()
			m = modelTmp.(appModel)
			if cmd != nil || !m.confirming {
				t.Fatalf("expected apply preview to open confirmation without command, confirming=%v cmd=%T", m.confirming, cmd)
			}
			if out := m.View(); !strings.Contains(out, "Repair visibility for") || !strings.Contains(out, "Type y or yes to confirm") || strings.Contains(out, "Visibility Repair Wizard") {
				t.Fatalf("expected visible confirmation overlay above wizard, got:\n%s", out)
			}
			m.confirmInput = "yes"
			confirmed, cmd := m.confirmAction()
			m = confirmed.(appModel)
			if cmd == nil {
				t.Fatal("expected repair execution command")
			}
			if m.visibilityRepairModal {
				t.Fatal("expected visibility repair wizard to close after applying")
			}

			msg := cmd()
			updated, _ = m.Update(msg)
			m = updated.(appModel)

			dest := filepath.Join(home, ".claude", "skills", "build")
			info, err := os.Lstat(dest)
			if err != nil {
				t.Fatalf("expected repaired destination to exist: %v", err)
			}
			if tc.wantSymlink {
				if info.Mode()&os.ModeSymlink == 0 {
					t.Fatalf("expected symlink at %s, got mode %v", dest, info.Mode())
				}
				resolved, err := filepath.EvalSymlinks(dest)
				if err != nil {
					t.Fatalf("expected symlink to resolve: %v", err)
				}
				if resolved != source {
					t.Fatalf("expected symlink to resolve to %s, got %s", source, resolved)
				}
			} else {
				if info.Mode()&os.ModeSymlink != 0 {
					t.Fatalf("expected copied directory at %s, got symlink", dest)
				}
				if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
					t.Fatalf("expected copied skill contents, got %v", err)
				}
			}

			if got := visibilityReasonByAgent(m.result.Skills, "claude-code"); got != tc.wantReason {
				t.Fatalf("expected claude-code visibility %q, got %q", tc.wantReason, got)
			}
		})
	}
}

func TestVisibilityRepairWizardNoOpWhenNothingFixable(t *testing.T) {
	home := setupVisibilityRepairEnv(t)
	cwd := t.TempDir()
	source := filepath.Join(cwd, ".agents", "skills", "build")
	writeSkillDoc(t, source, "Build", "Build desc")

	m := newVisibilityRepairTestModel(cwd, &model.Skill{
		Name:          "Build",
		Scope:         model.ScopeProject,
		CanonicalPath: source,
		LocalLock:     &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/build/SKILL.md"},
		ObservedPaths: []model.ObservedPath{{Agent: "opencode", Scope: model.ScopeProject, Path: source, Status: model.StatusCanonical}},
		Visibility: []model.SkillVisibility{
			{Agent: "opencode", Display: "OpenCode", Visible: true, Reason: "visible_via_universal_canonical", Path: source, Status: model.StatusCanonical},
			{Agent: "augment", Display: "Augment", Visible: false, Reason: "agent_not_detected"},
			{Agent: "promptscript", Display: "PromptScript", Visible: false, Reason: "unsupported_global"},
		},
	}, []model.AgentState{
		{Name: "opencode", Display: "OpenCode", Supported: true, Detected: true, Universal: true, SupportsGlobal: true, ProjectDir: filepath.Join(cwd, ".agents", "skills"), GlobalDir: filepath.Join(home, ".config", "opencode", "skills"), ProjectDirExists: true, GlobalDirExists: true},
		{Name: "augment", Display: "Augment", Supported: true, Detected: false, Universal: false, SupportsGlobal: true, ProjectDir: filepath.Join(home, ".augment", "skills"), GlobalDir: filepath.Join(home, ".augment", "skills"), ProjectDirExists: true, GlobalDirExists: true},
		{Name: "promptscript", Display: "PromptScript", Supported: true, Detected: true, Universal: true, SupportsGlobal: false, ProjectDir: filepath.Join(cwd, ".agents", "skills"), ProjectDirExists: true},
	})

	preview := actionByTitle(t, m.currentActions(), "Fix visibility…")
	if preview.Available {
		t.Fatalf("expected repair action to be unavailable, got %#v", preview)
	}
	m.commands = true
	m.action = actionIndex(t, m, "Fix visibility…")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if cmd != nil {
		t.Fatalf("expected no command for unavailable repair action, got %T", cmd)
	}
	if m.visibilityRepairModal {
		t.Fatal("did not expect wizard to open when nothing is fixable")
	}
}

func newVisibilityRepairTestModel(cwd string, skill *model.Skill, agents []model.AgentState) appModel {
	return appModel{
		cwd:             cwd,
		width:           120,
		height:          32,
		selected:        1,
		collapsedGroups: map[string]bool{},
		result: model.ScanResult{
			Agents: agents,
			Skills: []*model.Skill{skill},
		},
	}
}

func setupVisibilityRepairEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home
}

func writeSkillDoc(t *testing.T, dir, name, desc string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func visibilityReasonByAgent(skills []*model.Skill, agent string) string {
	for _, skill := range skills {
		for _, visibility := range skill.Visibility {
			if visibility.Agent == agent {
				return visibility.Reason
			}
		}
	}
	return ""
}

func actionByTitle(t *testing.T, previews []actions.CommandPreview, title string) actions.CommandPreview {
	t.Helper()
	for _, action := range previews {
		if action.Title == title {
			return action
		}
	}
	t.Fatalf("action %q not found in %#v", title, previews)
	return actions.CommandPreview{}
}
