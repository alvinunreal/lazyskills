package tui

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/display"
	"github.com/alvinunreal/lazyskills/internal/model"
	"github.com/alvinunreal/lazyskills/internal/runner"
)

func TestFilteredSkillsUsesSanitizedSearch(t *testing.T) {
	m := appModel{
		search: "bad name",
		result: model.ScanResult{Skills: []*model.Skill{{
			Name:        "Bad\x1b[31m Name",
			Description: "desc",
			Scope:       model.ScopeProject,
		}}},
	}
	items := m.filteredSkills()
	if len(items) != 1 {
		t.Fatalf("expected sanitized search to match, got %d", len(items))
	}
}

func TestErrorViewIsSanitized(t *testing.T) {
	m := appModel{err: errors.New("bad\x1b[31m path")}
	out := m.View()
	if strings.Contains(out, "\x1b[31m") || strings.Contains(out, "\x1b") {
		t.Fatalf("expected sanitized error view, got %q", out)
	}
	if !strings.Contains(out, "bad path") {
		t.Fatalf("expected sanitized error text, got %q", out)
	}
}

func TestViewRendersWithoutFooter(t *testing.T) {
	m := appModel{width: 100, height: 30, result: model.ScanResult{Skills: []*model.Skill{{Name: "Build", Description: "Build desc", Scope: model.ScopeProject}}}}
	out := m.View()
	if strings.Contains(out, "LazySkills v1") || strings.Contains(out, "actions are guarded") || !strings.Contains(out, "Build") {
		t.Fatalf("unexpected view: %s", out)
	}
}

func TestAgentFilterLimitsVisibleSkills(t *testing.T) {
	m := appModel{agent: "opencode", result: model.ScanResult{Skills: []*model.Skill{
		{Name: "OpenCode Skill", Description: "desc", Scope: model.ScopeProject, ObservedPaths: []model.ObservedPath{{Agent: "opencode"}}},
		{Name: "Claude Skill", Description: "desc", Scope: model.ScopeProject, ObservedPaths: []model.ObservedPath{{Agent: "claude-code"}}},
	}}}
	items := m.filteredSkills()
	if len(items) != 1 || items[0].Name != "OpenCode Skill" {
		t.Fatalf("unexpected filtered skills: %#v", items)
	}
}

func TestListTitleReflectsAgentFilter(t *testing.T) {
	m := appModel{width: 100, height: 20, result: model.ScanResult{
		Agents: []model.AgentState{{Name: "opencode", Display: "OpenCode"}},
		Skills: []*model.Skill{{Name: "Build", Scope: model.ScopeProject}},
	}}
	if title := m.listTitle(); !strings.Contains(title, "1 Inventory") {
		t.Fatalf("expected all-skills title, got %q", title)
	}
	out := m.View()
	if !strings.Contains(out, "1 Inventory") {
		t.Fatalf("expected view to contain all-skills title in border, got %q", out)
	}
	m.agent = "opencode"
	if title := m.listTitle(); !strings.Contains(title, "1 Inventory (OpenCode)") {
		t.Fatalf("expected agent-specific title, got %q", title)
	}
	out = m.View()
	if !strings.Contains(out, "1 Inventory (OpenCode)") {
		t.Fatalf("expected view to contain agent-specific title in border, got %q", out)
	}
}

func TestNextAgentFilterCyclesThroughDetectedAgents(t *testing.T) {
	m := appModel{result: model.ScanResult{Agents: []model.AgentState{
		{Name: "opencode", Detected: true},
		{Name: "cursor", Detected: true},
	}}}
	first := m.nextAgentFilter()
	if first != "cursor" {
		t.Fatalf("expected first detected agent cursor, got %q", first)
	}
	m.agent = first
	second := m.nextAgentFilter()
	if second != "opencode" {
		t.Fatalf("expected second detected agent opencode, got %q", second)
	}
	m.agent = second
	if got := m.nextAgentFilter(); got != "" {
		t.Fatalf("expected cycle back to all, got %q", got)
	}
}

func TestAgentFilterCyclesDetectedAgentsNoSupportedFallback(t *testing.T) {
	m := appModel{result: model.ScanResult{Agents: []model.AgentState{
		{Name: "opencode", Display: "OpenCode", Detected: true},
		{Name: "cursor", Display: "Cursor"},
		{Name: "claude-code", Display: "Claude Code", Detected: true},
	}}}
	if got := m.agentFilters(); strings.Join(got, ",") != "claude-code,opencode" {
		t.Fatalf("expected only detected agents in rotation, got %#v", got)
	}
	m = appModel{result: model.ScanResult{Agents: []model.AgentState{{Name: "opencode", Display: "OpenCode"}}}}
	if got := m.agentFilters(); len(got) != 0 {
		t.Fatalf("expected no fallback to supported agents when none detected, got %#v", got)
	}
}

func TestAgentClearedOnRefreshIfNoLongerDetected(t *testing.T) {
	m := appModel{
		agent: "opencode",
		result: model.ScanResult{
			Agents: []model.AgentState{
				{Name: "opencode", Detected: true},
			},
		},
	}
	// Simulate a snapshotMsg where opencode is no longer detected (or not present in Agents)
	updated, _ := m.Update(snapshotMsg{
		result: model.ScanResult{
			Agents: []model.AgentState{
				{Name: "opencode", Detected: false},
			},
		},
	})
	next := updated.(appModel)
	if next.agent != "" {
		t.Fatalf("expected agent to be cleared when no longer detected after refresh, got %q", next.agent)
	}
}

func TestAgentFilterCanSelectSupportedAgentWithNoSkills(t *testing.T) {
	m := appModel{width: 100, height: 30, agent: "claude-code", result: model.ScanResult{
		Agents: []model.AgentState{{Name: "claude-code", Display: "Claude Code"}},
		Skills: []*model.Skill{
			{Name: "OpenCode Skill", Description: "desc", Scope: model.ScopeProject, ObservedPaths: []model.ObservedPath{{Agent: "opencode"}}},
		},
	}}
	items := m.filteredSkills()
	if len(items) != 0 {
		t.Fatalf("expected no skills for claude-code, got %#v", items)
	}
	out := m.View()
	if !strings.Contains(out, "Claude Code") || !strings.Contains(out, "No skills matched") {
		t.Fatalf("expected zero-skill agent empty state, got %q", out)
	}
}

func TestAgentFilterCanResetDirectlyToAll(t *testing.T) {
	m := appModel{width: 100, height: 30, agent: "claude-code", result: model.ScanResult{Skills: []*model.Skill{
		{Name: "OpenCode Skill", Description: "desc", Scope: model.ScopeProject, ObservedPaths: []model.ObservedPath{{Agent: "opencode"}}},
	}}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	next := updated.(appModel)
	if next.agent != "" {
		t.Fatalf("expected A to reset agent filter, got %q", next.agent)
	}
	updated, _ = appModel{agent: "claude-code", result: m.result}.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next = updated.(appModel)
	if next.agent != "" {
		t.Fatalf("expected esc to reset agent filter when no mode is active, got %q", next.agent)
	}
}

func TestAgentFilterPreservesSelectedSkill(t *testing.T) {
	m := appModel{width: 100, height: 30, selected: 2, result: model.ScanResult{
		Agents: []model.AgentState{{Name: "claude-code", Detected: true}, {Name: "opencode", Detected: true}},
		Skills: []*model.Skill{
			{Name: "OpenOnly", Scope: model.ScopeProject, ObservedPaths: []model.ObservedPath{{Agent: "opencode"}}},
			{Name: "Shared", Scope: model.ScopeProject, ObservedPaths: []model.ObservedPath{{Agent: "opencode"}, {Agent: "claude-code"}}},
			{Name: "ClaudeOnly", Scope: model.ScopeProject, ObservedPaths: []model.ObservedPath{{Agent: "claude-code"}}},
		},
	}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	next := updated.(appModel)
	items := next.filteredSkills()
	if next.agent != "claude-code" || len(items) <= next.selected || items[next.selected].Name != "ClaudeOnly" {
		t.Fatalf("expected agent switch to preserve ClaudeOnly, agent=%q selected=%d items=%#v", next.agent, next.selected, items)
	}
}

func TestClearingAgentFilterPreservesSelectedSkill(t *testing.T) {
	m := appModel{width: 100, height: 30, agent: "claude-code", selected: 1, result: model.ScanResult{
		Agents: []model.AgentState{{Name: "claude-code", Detected: true}},
		Skills: []*model.Skill{
			{Name: "OpenOnly", Scope: model.ScopeProject, ObservedPaths: []model.ObservedPath{{Agent: "opencode"}}},
			{Name: "Shared", Scope: model.ScopeProject, ObservedPaths: []model.ObservedPath{{Agent: "opencode"}, {Agent: "claude-code"}}},
			{Name: "ClaudeOnly", Scope: model.ScopeProject, ObservedPaths: []model.ObservedPath{{Agent: "claude-code"}}},
		},
	}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	next := updated.(appModel)
	items := next.filteredSkills()
	if next.agent != "" || len(items) <= next.selected || items[next.selected].Name != "ClaudeOnly" {
		t.Fatalf("expected clearing agent to preserve ClaudeOnly, agent=%q selected=%d items=%#v", next.agent, next.selected, items)
	}
}

func TestTopLevelScanHealthIsRenderedSanitized(t *testing.T) {
	m := appModel{width: 100, height: 30, result: model.ScanResult{HealthIssues: []model.HealthIssue{{Type: "corrupt_lock", Message: "bad\x1b[31m lock"}}}}
	out := m.View()
	if !strings.Contains(out, "Scan health") || !strings.Contains(out, "bad") || !strings.Contains(out, "lock") {
		t.Fatalf("expected scan health in output: %q", out)
	}
	if strings.Contains(out, "\x1b[31m") || strings.ContainsRune(out, '\x1b') {
		t.Fatalf("expected sanitized scan health: %q", out)
	}
}

func TestCommandPreviewModeRendersWithoutExecuting(t *testing.T) {
	m := appModel{width: 120, height: 40, selected: 1, commands: true, result: model.ScanResult{Skills: []*model.Skill{{
		Name:      "Deploy Skill",
		Scope:     model.ScopeProject,
		LocalLock: &model.LocalLockEntry{Source: "owner/repo"},
	}}}}
	out := m.View()
	if !strings.Contains(out, "Actions") || !strings.Contains(out, "Reinstall/update selected skill") || !strings.Contains(out, "skills add") || !strings.Contains(out, "--yes") {
		t.Fatalf("expected actions in output: %q", out)
	}
	if strings.Contains(out, "Refresh LazySkills") {
		t.Fatalf("refresh belongs on r key, not action list: %q", out)
	}
}

func TestActiveAgentVisibilityReasonIsRendered(t *testing.T) {
	m := appModel{width: 120, height: 32, selected: 1, agent: "claude-code", result: model.ScanResult{Skills: []*model.Skill{{
		Name:          "Build",
		Description:   "desc",
		Scope:         model.ScopeProject,
		CanonicalPath: "/tmp/build",
		Visibility:    []model.SkillVisibility{{Agent: "claude-code", Display: "Claude Code", Visible: false, Reason: "missing_agent_link"}},
	}}}}
	out := m.View()
	if !strings.Contains(out, "Build") || !strings.Contains(out, "Claude Code: not linked") {
		t.Fatalf("expected active agent visibility reason, got %q", out)
	}
}

func TestAgentFilterListMarksNonVisibleSkills(t *testing.T) {
	m := appModel{width: 120, height: 32, agent: "claude-code", result: model.ScanResult{Skills: []*model.Skill{
		{
			Name:          "Visible",
			Scope:         model.ScopeProject,
			CanonicalPath: "/tmp/visible",
			Visibility:    []model.SkillVisibility{{Agent: "claude-code", Display: "Claude Code", Visible: true, Reason: "visible_via_symlink"}},
		},
		{
			Name:          "Missing",
			Scope:         model.ScopeProject,
			CanonicalPath: "/tmp/missing",
			Visibility:    []model.SkillVisibility{{Agent: "claude-code", Display: "Claude Code", Visible: false, Reason: "missing_agent_link"}},
		},
	}}}
	out := compat.StripTerminalEscapes(m.listPane(20, 80))
	if !strings.Contains(out, "Visible [P] ✓") || !strings.Contains(out, "Missing [P] ×") {
		t.Fatalf("expected list-level visibility badges, got %q", out)
	}
	if strings.Contains(out, "not linked") || strings.Contains(out, "available") {
		t.Fatalf("expected compact list badges without explanatory text, got %q", out)
	}
}

func TestListRendersIssueRowsWithSeverityBadges(t *testing.T) {
	m := appModel{width: 120, height: 32, selected: 1, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "Healthy", Scope: model.ScopeProject},
		{Name: "Warning", Scope: model.ScopeProject, HealthIssues: []model.HealthIssue{{Type: "missing_global_lock", Severity: "warning", Message: "not tracked"}}},
		{Name: "Error", Scope: model.ScopeProject, HealthIssues: []model.HealthIssue{{Type: "missing_file", Severity: "error", Message: "missing SKILL.md"}}},
	}}}
	out := m.listPane(20, 80)
	if !strings.Contains(out, "Warning [P] ▲1") {
		t.Fatalf("expected warning issue badge, got %q", out)
	}
	if !strings.Contains(out, "Error [P] !1") {
		t.Fatalf("expected error issue badge, got %q", out)
	}
	if strings.Contains(out, "BROKEN") {
		t.Fatalf("issue badge should stay subtle, got %q", out)
	}
}

func TestDetailRendersWarningOnlyIssuesAsWarnings(t *testing.T) {
	m := appModel{selected: 1, result: model.ScanResult{Skills: []*model.Skill{{Name: "Custom", Scope: model.ScopeGlobal, HealthIssues: []model.HealthIssue{{Type: "ghost_agent_skill", Severity: "warning", Message: "agent-specific skill"}}}}}}
	out := m.detailText(80)
	if !strings.Contains(out, "Warnings") || strings.Contains(out, "Health Issues") {
		t.Fatalf("expected warning-only detail section, got %q", out)
	}
}

func TestCommandPreviewToggle(t *testing.T) {
	m := appModel{}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	next := updated.(appModel)
	if !next.commands {
		t.Fatalf("expected command preview mode enabled")
	}
}

func TestActionConfirmationCancelDoesNotExecute(t *testing.T) {
	called := false
	old := runExec
	runExec = func(spec runner.ExecSpec) runner.Result { called = true; return runner.Result{ExitCode: 0} }
	t.Cleanup(func() { runExec = old })

	m := actionTestModel(t.TempDir())
	m.commands = true
	m.action = actionIndex(t, m, "Reinstall/update selected skill")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := updated.(appModel)
	if cmd != nil || !next.confirming {
		t.Fatalf("expected confirmation without command, confirming=%v cmd=%v", next.confirming, cmd)
	}
	updated, cmd = next.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next = updated.(appModel)
	if cmd != nil || next.confirming || called {
		t.Fatalf("expected cancel without execution, confirming=%v called=%v", next.confirming, called)
	}
}

func TestConfirmationAcceptsDefaultYAndYes(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"y", "y"},
		{"yes", "yes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			old := runExec
			runExec = func(spec runner.ExecSpec) runner.Result { called = true; return runner.Result{ExitCode: 0} }
			t.Cleanup(func() { runExec = old })

			m := actionTestModel(t.TempDir())
			m.commands = true
			m.action = actionIndex(t, m, "Reinstall/update selected skill")
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(appModel)
			for _, r := range []rune(tc.input) {
				updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = updated.(appModel)
			}
			updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(appModel)
			if cmd == nil || !m.running {
				t.Fatalf("expected accepted confirmation to run, input=%q running=%v cmd=%v", tc.input, m.running, cmd)
			}
			cmd()
			if !called {
				t.Fatalf("expected command execution")
			}
		})
	}
}

func TestSafeConfirmationAcceptsEmptyInput(t *testing.T) {
	called := false
	old := runExec
	runExec = func(spec runner.ExecSpec) runner.Result { called = true; return runner.Result{ExitCode: 0} }
	t.Cleanup(func() { runExec = old })

	m := actionTestModel(t.TempDir())
	m.commands = true
	m.action = actionIndex(t, m, "Reinstall/update selected skill")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)

	// Safe actions accept empty Enter as the default confirmation.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if cmd == nil || !m.running || m.confirming {
		t.Fatalf("expected empty input to run safe confirmation, running=%v confirming=%v cmd=%v", m.running, m.confirming, cmd)
	}
	cmd()
	if !called {
		t.Fatalf("expected command execution")
	}
}

func TestConfirmationRendersCenteredModal(t *testing.T) {
	m := actionTestModel(t.TempDir())
	m.commands = true
	m.action = actionIndex(t, m, "Reinstall/update selected skill")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	out := m.View()
	if !strings.Contains(out, "Reinstall/update selected skill") || !strings.Contains(out, "Press Enter to continue") || strings.Contains(out, "Bulk actions") {
		t.Fatalf("expected standalone confirmation modal, got %q", out)
	}
}

func TestBundleImportRequiresConfirmationBeforeApply(t *testing.T) {
	cwd := t.TempDir()
	bundlePath := actions.DefaultProjectBundlePath(cwd)
	if err := os.MkdirAll(filepath.Dir(bundlePath), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := model.SkillBundle{Version: 1, Scope: model.ScopeProject, Skills: []model.SkillBundleSkill{{Name: "Lint", Source: "owner/repo", Reference: "v2", SkillPath: "skills/lint/SKILL.md", Scope: model.ScopeProject, LockIdentity: model.SkillBundleLockIdentity{Source: "owner/repo", SourceType: "github", Reference: "v2", SkillPath: "skills/lint/SKILL.md", ComputedHash: "def"}}}}
	if err := actions.WriteProjectSkillBundle(bundlePath, bundle); err != nil {
		t.Fatal(err)
	}
	m := appModel{cwd: cwd, width: 120, height: 32, selected: 0, result: model.ScanResult{Skills: []*model.Skill{{Name: "Deploy", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo", Ref: "main", SkillPath: "skills/deploy/SKILL.md"}}}}}
	m.commands = true
	m.action = actionIndex(t, m, "Import project skill bundle")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := updated.(appModel)
	if cmd != nil || !next.confirming || next.running {
		t.Fatalf("expected import preview to open confirmation only, confirming=%v running=%v cmd=%v", next.confirming, next.running, cmd)
	}
	if next.actionResult != nil {
		t.Fatalf("expected preview phase to avoid mutation/result, got %#v", next.actionResult)
	}
	if !strings.Contains(next.View(), "Install 1 missing skills") || !strings.Contains(next.View(), "Skip 0 matching skills") {
		t.Fatalf("expected preview summary in the modal, got %q", next.View())
	}
}

func TestCommandPickerExposesProjectBundleActionsOnSourceHeader(t *testing.T) {
	m := appModel{
		cwd:     t.TempDir(),
		width:   120,
		height:  32,
		selected: 0,
		result: model.ScanResult{Skills: []*model.Skill{{
			Name:       "Deploy",
			Scope:      model.ScopeProject,
			LocalLock:  &model.LocalLockEntry{Source: "owner/repo", Ref: "main", SkillPath: "skills/deploy/SKILL.md"},
			CanonicalPath: "/tmp/deploy",
		}}},
	}
	m.commands = true
	acts := m.currentActions()
	hasExport := false
	hasImport := false
	hasSourceAction := false
	for _, act := range acts {
		switch act.Title {
		case "Export project skill bundle":
			hasExport = true
		case "Import project skill bundle":
			hasImport = true
		case "Check local source for available skills", "Update installed skills from source", "Remove installed skills from source":
			hasSourceAction = true
		}
	}
	if !hasExport || !hasImport || !hasSourceAction {
		t.Fatalf("expected source header command picker to include bundle and source actions, got %#v", acts)
	}
}

func TestRunningActionRendersProgressModal(t *testing.T) {
	m := actionTestModel(t.TempDir())
	m.running = true
	m.runningTitle = "Reinstall/update selected skill"
	out := m.View()
	if !strings.Contains(out, "Running") || !strings.Contains(out, "Reinstall/update selected skill") || !strings.Contains(out, "Working") {
		t.Fatalf("expected running progress modal, got %q", out)
	}
}

func TestActionExecUsesProjectCwdAndPreventsDuplicateWhileRunning(t *testing.T) {
	cwd := t.TempDir()
	old := runExec
	runExec = func(spec runner.ExecSpec) runner.Result {
		if spec.Cwd != cwd {
			t.Fatalf("expected cwd %q, got %q", cwd, spec.Cwd)
		}
		return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 0, Stdout: "ok"}
	}
	t.Cleanup(func() { runExec = old })

	m := actionTestModel(cwd)
	m.commands = true
	m.action = actionIndex(t, m, "Reinstall/update selected skill")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	for _, r := range []rune("yes") {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(appModel)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if cmd == nil || !m.running {
		t.Fatalf("expected running command")
	}
	updated, dup := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if dup != nil || !updated.(appModel).running {
		t.Fatalf("expected duplicate enter ignored while running")
	}
	msg := cmd()
	updated, rescan := m.Update(msg)
	m = updated.(appModel)
	if m.running || m.actionResult == nil || m.actionResult.Stdout != "ok" || rescan == nil {
		t.Fatalf("expected result and rescan, model=%#v rescan=%v", m.actionResult, rescan)
	}
}

func TestSpaceMarksSkillsAndEscClearsSelection(t *testing.T) {
	m := bulkActionTestModel(t.TempDir())
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(appModel)
	if m.selectedCount() != 1 || !strings.Contains(m.View(), "● One") {
		t.Fatalf("expected one marked skill, count=%d view=%q", m.selectedCount(), m.View())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(appModel)
	if m.selectedCount() != 0 {
		t.Fatalf("expected esc to clear selection, got %d", m.selectedCount())
	}
}

func TestEscClosesActionPickerOverlay(t *testing.T) {
	m := bulkActionTestModel(t.TempDir())
	m.commands = true
	m.agent = "opencode"
	m.selectedKeys = map[string]bool{skillKey(m.result.Skills[0]): true}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next := updated.(appModel)
	if next.commands {
		t.Fatalf("expected esc to close action mode overlay, commands is still true")
	}
	if next.selectedCount() != 1 {
		t.Fatalf("expected selection to be preserved when action mode is closed, got %d", next.selectedCount())
	}
	if next.agent != "opencode" {
		t.Fatalf("expected agent to be preserved when action mode is closed, got %q", next.agent)
	}
}

func TestBulkActionsRenderAndExecuteSequentially(t *testing.T) {
	cwd := t.TempDir()
	calls := 0
	old := runExec
	runExec = func(spec runner.ExecSpec) runner.Result {
		calls++
		return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 0}
	}
	t.Cleanup(func() { runExec = old })

	m := bulkActionTestModel(cwd)
	m.selectedKeys = map[string]bool{}
	for _, skill := range m.result.Skills {
		m.selectedKeys[skillKey(skill)] = true
	}
	m.commands = true
	out := m.View()
	if !strings.Contains(out, "Bulk actions") || !strings.Contains(out, "Reinstall/update 2 selected skills") || strings.Contains(out, "Open selected skill") {
		t.Fatalf("expected constrained bulk actions, got %q", out)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	for _, r := range []rune("update 2 skills") {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(appModel)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if cmd == nil || !m.running {
		t.Fatalf("expected bulk command to run")
	}
	updated, rescan := m.Update(cmd())
	m = updated.(appModel)
	if calls != 2 || m.selectedCount() != 0 || rescan == nil {
		t.Fatalf("expected two commands, cleared selection, and rescan; calls=%d selected=%d rescan=%v", calls, m.selectedCount(), rescan)
	}
}

func TestBulkActionStopsOnFirstFailure(t *testing.T) {
	cwd := t.TempDir()
	calls := 0
	old := runExec
	runExec = func(spec runner.ExecSpec) runner.Result {
		calls++
		if calls == 1 {
			return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 2, Stderr: "nope"}
		}
		return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 0}
	}
	t.Cleanup(func() { runExec = old })
	m := bulkActionTestModel(cwd)
	m.selectedKeys = map[string]bool{}
	for _, skill := range m.result.Skills {
		m.selectedKeys[skillKey(skill)] = true
	}
	result, partialSuccess := m.runBatch(m.currentActions()[0].Exec.Batch)
	if calls != 1 || result.ExitCode != 2 {
		t.Fatalf("expected stop on first failure, calls=%d result=%#v", calls, result)
	}
	if partialSuccess {
		t.Fatalf("first-command failure should not count as partial success")
	}
}

func TestBulkPartialFailureRescansAndKeepsSelection(t *testing.T) {
	cwd := t.TempDir()
	calls := 0
	old := runExec
	runExec = func(spec runner.ExecSpec) runner.Result {
		calls++
		if calls == 2 {
			return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 2, Stderr: "nope"}
		}
		return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 0}
	}
	t.Cleanup(func() { runExec = old })
	m := bulkActionTestModel(cwd)
	m.selectedKeys = map[string]bool{}
	for _, skill := range m.result.Skills {
		m.selectedKeys[skillKey(skill)] = true
	}
	m.commands = true
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	for _, r := range []rune("update 2 skills") {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(appModel)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if cmd == nil {
		t.Fatalf("expected bulk command")
	}
	updated, rescan := m.Update(cmd())
	m = updated.(appModel)
	if calls != 2 || rescan == nil || m.selectedCount() != 2 || m.actionResult == nil || m.actionResult.ExitCode != 2 {
		t.Fatalf("expected partial failure to rescan and keep selection; calls=%d rescan=%v selected=%d result=%#v", calls, rescan, m.selectedCount(), m.actionResult)
	}
}

func TestDirectUpdateHotkeyStartsCurrentOrBulkAction(t *testing.T) {
	m := actionTestModel(t.TempDir())
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	next := updated.(appModel)
	if cmd != nil || next.commands || !next.confirming || next.currentActions()[next.action].ID != "reinstall_update" {
		t.Fatalf("expected u to open current-skill update confirmation without actions, action=%#v confirming=%v commands=%v cmd=%v", next.currentActions()[next.action], next.confirming, next.commands, cmd)
	}
	m = bulkActionTestModel(t.TempDir())
	m.selectedKeys = map[string]bool{skillKey(m.result.Skills[0]): true, skillKey(m.result.Skills[1]): true}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	next = updated.(appModel)
	if cmd != nil || next.commands || !next.confirming || next.currentActions()[next.action].ID != "bulk_reinstall_update" {
		t.Fatalf("expected u to open bulk update confirmation without actions, action=%#v confirming=%v commands=%v cmd=%v", next.currentActions()[next.action], next.confirming, next.commands, cmd)
	}
}

func TestDirectRemoveHotkeyStartsStrictConfirmation(t *testing.T) {
	m := actionTestModel(t.TempDir())
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	next := updated.(appModel)
	if cmd != nil || next.commands || !next.confirming || next.currentActions()[next.action].ID != "remove" || next.currentActions()[next.action].ConfirmValue != "deploy-skill" {
		t.Fatalf("expected x to open single remove confirmation without actions, action=%#v confirming=%v commands=%v cmd=%v", next.currentActions()[next.action], next.confirming, next.commands, cmd)
	}
}

func TestDirectOpenHotkeyUsesCurrentSkillEvenWithBulkSelection(t *testing.T) {
	t.Setenv("EDITOR", "vim")
	m := bulkActionTestModel(t.TempDir())
	m.result.Skills[0].SkillPath = "/tmp/one/SKILL.md"
	m.selectedKeys = map[string]bool{skillKey(m.result.Skills[1]): true}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	next := updated.(appModel)
	if cmd == nil || !next.running || next.commands {
		t.Fatalf("expected o to start interactive open without opening actions, running=%v commands=%v cmd=%v", next.running, next.commands, cmd)
	}
}

func TestDirectOpenHotkeyClosesExistingActionMode(t *testing.T) {
	t.Setenv("EDITOR", "vim")
	m := actionTestModel(t.TempDir())
	m.result.Skills[0].SkillPath = "/tmp/deploy-skill/SKILL.md"
	m.commands = true
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	next := updated.(appModel)
	if cmd == nil || !next.running || next.commands {
		t.Fatalf("expected o to close action mode before opening editor, running=%v commands=%v cmd=%v", next.running, next.commands, cmd)
	}
}

func TestSourceMarkSelectsCurrentSource(t *testing.T) {
	m := appModel{width: 120, height: 32, selected: 1, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "One", Scope: model.ScopeProject, CanonicalPath: "/tmp/one", LocalLock: &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/web/SKILL.md"}},
		{Name: "Two", Scope: model.ScopeProject, CanonicalPath: "/tmp/two", LocalLock: &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/web/SKILL.md"}},
		{Name: "Other", Scope: model.ScopeProject, CanonicalPath: "/tmp/other", LocalLock: &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/data/SKILL.md"}},
		{Name: "Global", Scope: model.ScopeGlobal, CanonicalPath: "/tmp/global", GlobalLock: &model.GlobalLockEntry{Source: "owner/repo"}},
		{Name: "Different", Scope: model.ScopeProject, CanonicalPath: "/tmp/different", LocalLock: &model.LocalLockEntry{Source: "other/repo", SkillPath: "skills/web/SKILL.md"}},
	}}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	next := updated.(appModel)
	if next.selectedCount() != 4 || !next.isSelected(next.result.Skills[0]) || !next.isSelected(next.result.Skills[1]) || !next.isSelected(next.result.Skills[2]) || !next.isSelected(next.result.Skills[3]) || next.isSelected(next.result.Skills[4]) {
		t.Fatalf("expected source mark to select all skills from owner/repo only, selected=%#v", next.selectedKeys)
	}
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	next = updated.(appModel)
	if next.selectedCount() != 0 {
		t.Fatalf("expected second source mark to unselect marked source skills, selected=%#v", next.selectedKeys)
	}
}

func TestSourceMarkOnlySelectsFilteredSkills(t *testing.T) {
	m := appModel{width: 120, height: 32, search: "one", result: model.ScanResult{Skills: []*model.Skill{
		{Name: "One", Scope: model.ScopeProject, CanonicalPath: "/tmp/one", LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
		{Name: "Two", Scope: model.ScopeProject, CanonicalPath: "/tmp/two", LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
	}}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	next := updated.(appModel)
	if next.selectedCount() != 1 || !next.isSelected(next.result.Skills[0]) || next.isSelected(next.result.Skills[1]) {
		t.Fatalf("expected source mark to select only visible filtered skills, selected=%#v", next.selectedKeys)
	}
}

func TestSourceAndFolderDetailsRender(t *testing.T) {
	m := appModel{width: 120, height: 32, selected: 1, result: model.ScanResult{Skills: []*model.Skill{{
		Name: "One", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/web/SKILL.md", Ref: "main"},
	}}}}
	out := m.View()
	if !strings.Contains(out, "Source:") || !strings.Contains(out, "owner/repo") ||
		!strings.Contains(out, "Folder:") || !strings.Contains(out, "skills/web") ||
		!strings.Contains(out, "Ref:") || !strings.Contains(out, "main") {
		t.Fatalf("expected source/folder/ref details, got %q", out)
	}
}

func TestSkillListShowsSourceGroups(t *testing.T) {
	m := appModel{width: 120, height: 32, selected: 1, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "One", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/web/SKILL.md"}},
		{Name: "Two", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/web/SKILL.md"}},
		{Name: "Other", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/data/SKILL.md"}},
	}}}
	out := m.View()
	if !strings.Contains(out, "owner/repo") {
		t.Fatalf("expected source group header, got %q", out)
	}
	if strings.Count(out, "owner/repo") != 2 || strings.Contains(out, "owner/repo / skills/web") || strings.Contains(out, "owner/repo / skills/data") {
		t.Fatalf("expected repo-level group header (one in list, one in Source detail), got %q", out)
	}
}

func TestSkillListSeparatesNoSourceMetadata(t *testing.T) {
	m := appModel{width: 120, height: 32, selected: 1, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "Tracked", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
		{Name: "Manual", Scope: model.ScopeProject},
	}}}
	out := m.View()
	if !strings.Contains(out, "owner/repo") || !strings.Contains(out, "Custom / untracked") {
		t.Fatalf("expected explicit source and no-source groups, got %q", out)
	}
}

func TestBracketJumpSourceGroupsWithoutChangingScope(t *testing.T) {
	m := appModel{width: 120, height: 32, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "One", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/one"}},
		{Name: "Two", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/one"}},
		{Name: "Three", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/two"}},
		{Name: "Manual", Scope: model.ScopeProject},
	}}}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{']'}})
	next := updated.(appModel)
	if next.filter != scopeAll || next.selected != 3 {
		t.Fatalf("expected ] to jump to next source group without changing scope, filter=%d selected=%d", next.filter, next.selected)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	next = updated.(appModel)
	if next.filter != scopeAll || next.selected != 0 {
		t.Fatalf("expected [ to jump to previous source group without changing scope, filter=%d selected=%d", next.filter, next.selected)
	}
}

func TestPaneFocusAndScroll(t *testing.T) {
	m := appModel{width: 120, height: 32, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "One", Scope: model.ScopeProject},
		{Name: "Two", Scope: model.ScopeProject},
	}}}

	if m.detailsFocused {
		t.Fatal("expected detailsFocused to start false")
	}

	// Press 2 to focus metadata/details area.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	next := updated.(appModel)
	if !next.detailsFocused {
		t.Fatal("expected detailsFocused to be true after pressing 2")
	}

	// Press 1 to return to skills list.
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	next = updated.(appModel)
	if next.detailsFocused {
		t.Fatal("expected detailsFocused to be false after pressing 1")
	}
}

func TestScopeFilterKeys(t *testing.T) {
	m := appModel{width: 120, height: 32, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "Project", Scope: model.ScopeProject},
		{Name: "Global", Scope: model.ScopeGlobal},
	}}}

	// Test 'f' cycles scope All -> Project -> Global -> All
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	next := updated.(appModel)
	if next.filter != scopeProject {
		t.Fatalf("expected f to switch to project scope, got %d", next.filter)
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	next = updated.(appModel)
	if next.filter != scopeGlobal {
		t.Fatalf("expected f to switch to global scope, got %d", next.filter)
	}

	// Test 'F' resets scope to All
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'F'}})
	next = updated.(appModel)
	if next.filter != scopeAll {
		t.Fatalf("expected F to reset scope to All, got %d", next.filter)
	}
}

func TestMain(m *testing.M) {
	// Redirect the discovery clone cache to a throwaway dir so tests never
	// touch the real user cache directory.
	dir, err := os.MkdirTemp("", "lazyskills-test-clones-*")
	if err != nil {
		panic(err)
	}
	discoveryCacheRoot = func() (string, error) { return dir, nil }
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func TestCachedSourceCloneReuse(t *testing.T) {
	oldRoot := discoveryCacheRoot
	oldClone := gitClone
	defer func() { discoveryCacheRoot = oldRoot; gitClone = oldClone }()

	tmp := t.TempDir()
	discoveryCacheRoot = func() (string, error) { return tmp, nil }

	clones := 0
	gitClone = func(url, ref, dir string) error {
		clones++
		return os.MkdirAll(filepath.Join(dir, ".git"), 0o755) // simulate a real clone
	}

	if _, _, err := cachedSourceClone("https://github.com/owner/repo", "main", false); err != nil {
		t.Fatalf("first clone failed: %v", err)
	}
	if clones != 1 {
		t.Fatalf("expected 1 clone on cache miss, got %d", clones)
	}
	// Cache hit: a cached repo is reused without cloning.
	if _, _, err := cachedSourceClone("https://github.com/owner/repo", "main", false); err != nil {
		t.Fatalf("reuse failed: %v", err)
	}
	if clones != 1 {
		t.Fatalf("expected cache reuse (no extra clone), got %d clones", clones)
	}
	// Force re-clones.
	if _, _, err := cachedSourceClone("https://github.com/owner/repo", "main", true); err != nil {
		t.Fatalf("forced clone failed: %v", err)
	}
	if clones != 2 {
		t.Fatalf("expected force to re-clone, got %d clones", clones)
	}
}

func TestExecutePruneLockRemovesEntry(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "skills-lock.json")
	if err := os.WriteFile(lockPath, []byte(`{"version":1,"skills":{"ghost":{"source":"o/r"},"keep":{"source":"o/r"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	m := appModel{cwd: dir, width: 120, height: 32}
	action := actions.CommandPreview{
		ID:           "prune_lock",
		ConfirmValue: "ghost",
		Exec:         actions.ExecSpec{Internal: "prune_project_lock"},
	}
	_, cmd := m.executeAction(action)
	if cmd == nil {
		t.Fatal("expected a rescan command after pruning")
	}

	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "ghost") {
		t.Fatalf("expected 'ghost' entry pruned, got %s", data)
	}
	if !strings.Contains(string(data), "keep") {
		t.Fatalf("expected 'keep' entry preserved, got %s", data)
	}
}

func TestExecuteRemovePrunesProjectLockEntry(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "skills-lock.json")
	if err := os.WriteFile(lockPath, []byte(`{"version":1,"skills":{"ghost":{"source":"o/r"},"keep":{"source":"o/r"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	originalRunExec := runExec
	runExec = func(spec runner.ExecSpec) runner.Result {
		return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 0}
	}
	t.Cleanup(func() { runExec = originalRunExec })

	m := appModel{cwd: dir, width: 120, height: 32}
	action := actions.CommandPreview{
		ID:           "remove",
		ConfirmValue: "ghost",
		Mutates:      true,
		Exec:         actions.ExecSpec{Program: "skills", Args: []string{"remove", "ghost", "--yes"}},
	}
	_, cmd := m.executeAction(action)
	if cmd == nil {
		t.Fatal("expected command")
	}
	msg, ok := cmd().(actionResultMsg)
	if !ok {
		t.Fatalf("expected actionResultMsg")
	}
	if msg.result.ExitCode != 0 || msg.result.Err != "" {
		t.Fatalf("expected successful remove result, got %#v", msg.result)
	}

	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "ghost") {
		t.Fatalf("expected 'ghost' entry pruned after remove, got %s", data)
	}
	if !strings.Contains(string(data), "keep") {
		t.Fatalf("expected 'keep' entry preserved, got %s", data)
	}
}

func TestHumanizeSince(t *testing.T) {
	now := time.Now()
	cases := []struct {
		at   time.Time
		want string
	}{
		{now.Add(-2 * time.Second), "just now"},
		{now.Add(-30 * time.Second), "30s ago"},
		{now.Add(-5 * time.Minute), "5m ago"},
		{now.Add(-3 * time.Hour), "3h ago"},
		{now.Add(-50 * time.Hour), "2d ago"},
	}
	for _, c := range cases {
		if got := humanizeSince(c.at); got != c.want {
			t.Errorf("humanizeSince(%v) = %q, want %q", c.at, got, c.want)
		}
	}
}

func TestModalShowsScanFreshness(t *testing.T) {
	m := appModel{width: 120, height: 32, modalSource: "owner/repo", detailModal: true, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "Existing", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
	}}}
	m.discovery = map[string]SourceDiscovery{
		"owner/repo": {Status: DiscoveryReady, ScannedAt: time.Now().Add(-5 * time.Minute)},
	}
	out := strings.Join(m.sourceModalDetailLines(80), "\n")
	if !strings.Contains(out, "scanned 5m ago") {
		t.Fatalf("expected scan freshness in modal, got %q", out)
	}
}

func TestTopBottomJumpKeys(t *testing.T) {
	m := appModel{width: 120, height: 32, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "One", Scope: model.ScopeProject},
		{Name: "Two", Scope: model.ScopeProject},
		{Name: "Three", Scope: model.ScopeProject},
	}}}
	last := len(m.visibleRows()) - 1

	// 'G' jumps to the bottom.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	next := updated.(appModel)
	if next.selected != last {
		t.Fatalf("expected G to select last row %d, got %d", last, next.selected)
	}

	// A lone 'g' arms but does not move selection.
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	armed := updated.(appModel)
	if armed.selected != last || !armed.pendingG {
		t.Fatalf("expected lone g to arm without moving, got selected=%d pendingG=%v", armed.selected, armed.pendingG)
	}

	// The second 'g' jumps to the top and disarms.
	updated, _ = armed.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	top := updated.(appModel)
	if top.selected != 0 || top.pendingG {
		t.Fatalf("expected gg to select first row and disarm, got selected=%d pendingG=%v", top.selected, top.pendingG)
	}

	// A non-g key disarms a pending g.
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	updated, _ = updated.(appModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if updated.(appModel).pendingG {
		t.Fatal("expected a non-g key to disarm pendingG")
	}
}

func TestFocusControls(t *testing.T) {
	m := appModel{width: 120, height: 32}

	if m.detailsFocused {
		t.Fatal("expected detailsFocused to start false")
	}

	// '2' focuses details
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(appModel)
	if !m.detailsFocused {
		t.Fatal("expected 2 to focus details")
	}

	// '1' focuses list
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = updated.(appModel)
	if m.detailsFocused {
		t.Fatal("expected 1 to focus list")
	}

	// 'tab' cycles focus
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(appModel)
	if !m.detailsFocused {
		t.Fatal("expected tab to cycle focus")
	}

	// 'shift+tab' cycles focus back
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = updated.(appModel)
	if m.detailsFocused {
		t.Fatal("expected shift+tab to cycle focus")
	}
}

func TestEnterSkillModal(t *testing.T) {
	m := appModel{width: 120, height: 32, selected: 1, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "One", Scope: model.ScopeProject},
	}}}

	// Press Enter to open modal
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if !m.detailModal {
		t.Fatal("expected enter to open skill detail modal")
	}

	// Render modal view
	out := m.View()
	if !strings.Contains(out, "Skill Detail View") {
		t.Fatalf("expected modal title in View, got %s", out)
	}

	// Scroll down in modal
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(appModel)

	// Press Esc to close modal
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(appModel)
	if m.detailModal {
		t.Fatal("expected esc to close modal")
	}
}

func TestGroupedListKeepsSelectedRowVisible(t *testing.T) {
	skills := []*model.Skill{}
	for i := 0; i < 12; i++ {
		skills = append(skills, &model.Skill{Name: fmt.Sprintf("Skill %02d", i), Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: fmt.Sprintf("owner/repo-%02d", i)}})
	}
	m := appModel{width: 120, height: 10, selected: 23, result: model.ScanResult{Skills: skills}}
	out := m.View()
	if !strings.Contains(out, "Skill 11 [P]") {
		t.Fatalf("expected selected row visible with many group headers, got %q", out)
	}
}

func TestFailedMutationDoesNotTriggerRescan(t *testing.T) {
	cwd := t.TempDir()
	old := runExec
	runExec = func(spec runner.ExecSpec) runner.Result {
		return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 2, Stderr: "nope"}
	}
	t.Cleanup(func() { runExec = old })

	m := actionTestModel(cwd)
	m.commands = true
	m.action = actionIndex(t, m, "Reinstall/update selected skill")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	for _, r := range []rune("yes") {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(appModel)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if cmd == nil {
		t.Fatalf("expected command")
	}
	updated, rescan := m.Update(cmd())
	m = updated.(appModel)
	if rescan != nil || m.actionResult == nil || m.actionResult.ExitCode != 2 {
		t.Fatalf("expected failed result without rescan, result=%#v rescan=%v", m.actionResult, rescan)
	}
}

func TestSingleRemoveRequiresYesConfirmation(t *testing.T) {
	m := actionTestModel(t.TempDir())
	m.result.Skills[0].CanonicalPath = "/tmp/deploy-skill"
	m.commands = true
	m.action = actionIndex(t, m, "Remove selected skill")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	for _, r := range []rune("wrong") {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(appModel)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if cmd != nil || !m.confirming || !strings.Contains(m.confirmError, "Type y or yes") || m.actionResult != nil {
		t.Fatalf("expected inline confirmation error without command, confirming=%v err=%q result=%#v cmd=%v", m.confirming, m.confirmError, m.actionResult, cmd)
	}
}

func TestSingleRemoveAcceptsYesConfirmation(t *testing.T) {
	old := runExec
	runExec = func(spec runner.ExecSpec) runner.Result {
		return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 0}
	}
	t.Cleanup(func() { runExec = old })

	m := actionTestModel(t.TempDir())
	m.commands = true
	m.action = actionIndex(t, m, "Remove selected skill")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	for _, r := range []rune("yes") {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(appModel)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if cmd == nil || !m.running {
		t.Fatalf("expected yes to run single remove, running=%v cmd=%v", m.running, cmd)
	}
}

func TestBulkRemoveRequiresExactPhrase(t *testing.T) {
	m := bulkActionTestModel(t.TempDir())
	m.selectedKeys = map[string]bool{skillKey(m.result.Skills[0]): true, skillKey(m.result.Skills[1]): true}
	m.commands = true
	m.action = actionIndex(t, m, "Remove 2 selected skills")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	for _, r := range []rune("yes") {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(appModel)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if cmd != nil || !m.confirming || !strings.Contains(m.confirmError, `Type "remove 2 skills" exactly`) {
		t.Fatalf("expected bulk remove to require exact phrase, confirming=%v err=%q cmd=%v", m.confirming, m.confirmError, cmd)
	}
}

func TestConfirmationAllowsSkillNamesStartingWithN(t *testing.T) {
	m := actionTestModel(t.TempDir())
	m.result.Skills[0].CanonicalPath = "/tmp/next-auth"
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(appModel)
	if cmd != nil || !m.confirming || m.currentActions()[m.action].ConfirmValue != "next-auth" {
		t.Fatalf("expected remove confirmation for next-auth, confirming=%v action=%#v cmd=%v", m.confirming, m.currentActions()[m.action], cmd)
	}
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = updated.(appModel)
	if cmd != nil || !m.confirming || m.confirmInput != "n" {
		t.Fatalf("expected first n to be typed, not cancel confirmation; confirming=%v input=%q cmd=%v", m.confirming, m.confirmInput, cmd)
	}
}

func TestDirectSafeConfirmationAcceptsEmptyEnter(t *testing.T) {
	old := runExec
	runExec = func(spec runner.ExecSpec) runner.Result {
		return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 0}
	}
	t.Cleanup(func() { runExec = old })

	m := actionTestModel(t.TempDir())
	m.commands = true
	m.action = actionIndex(t, m, "Reinstall/update selected skill")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if cmd == nil || !m.running {
		t.Fatalf("expected empty Enter to run safe action, running=%v cmd=%v", m.running, cmd)
	}
}

func TestConfirmationOverlayCopyMatchesRisk(t *testing.T) {
	m := actionTestModel(t.TempDir())
	m.commands = true
	m.action = actionIndex(t, m, "Reinstall/update selected skill")
	m.confirming = true
	safeOut := m.confirmationOverlay(appLayout{Width: 120, Height: 32})
	if !strings.Contains(safeOut, "Press Enter to continue, or Esc to cancel.") || strings.Contains(safeOut, "Type ") {
		t.Fatalf("expected safe confirmation copy, got %q", safeOut)
	}

	m.action = actionIndex(t, m, "Remove selected skill")
	dangerOut := m.confirmationOverlay(appLayout{Width: 120, Height: 32})
	if !strings.Contains(dangerOut, "Type y or yes to confirm") || strings.Contains(dangerOut, `Type "deploy-skill"`) {
		t.Fatalf("expected single-remove y/yes copy, got %q", dangerOut)
	}

	m = bulkActionTestModel(t.TempDir())
	m.selectedKeys = map[string]bool{skillKey(m.result.Skills[0]): true, skillKey(m.result.Skills[1]): true}
	m.commands = true
	m.action = actionIndex(t, m, "Remove 2 selected skills")
	m.confirming = true
	bulkOut := m.confirmationOverlay(appLayout{Width: 120, Height: 32})
	if !strings.Contains(bulkOut, `remove 2 skills`) || !strings.Contains(bulkOut, `to confirm`) || strings.Contains(bulkOut, "y / yes") {
		t.Fatalf("expected bulk-remove exact-phrase copy, got %q", bulkOut)
	}
}

func actionTestModel(cwd string) appModel {
	return appModel{cwd: cwd, width: 120, height: 32, selected: 1, result: model.ScanResult{Skills: []*model.Skill{{
		Name:          "Deploy Skill",
		Description:   "desc",
		Scope:         model.ScopeProject,
		CanonicalPath: "/tmp/deploy-skill",
		LocalLock:     &model.LocalLockEntry{Source: "owner/repo"},
	}}}}
}

func bulkActionTestModel(cwd string) appModel {
	return appModel{cwd: cwd, width: 120, height: 32, selected: 1, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "One", Description: "desc", Scope: model.ScopeProject, CanonicalPath: "/tmp/one", LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
		{Name: "Two", Description: "desc", Scope: model.ScopeProject, CanonicalPath: "/tmp/two", LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
	}}}
}

func actionIndex(t *testing.T, m appModel, title string) int {
	t.Helper()
	for i, action := range m.currentActions() {
		if action.Title == title {
			return i
		}
	}
	t.Fatalf("action %q not found in %#v", title, m.currentActions())
	return 0
}

func TestActionSelectionDoesNotResetDetailScroll(t *testing.T) {
	m := actionTestModel(t.TempDir())
	for i := 0; i < 40; i++ {
		m.result.Skills[0].HealthIssues = append(m.result.Skills[0].HealthIssues, model.HealthIssue{Type: "warning", Message: fmt.Sprintf("issue %d", i)})
	}
	m.height = 12
	m.commands = true
	m.syncViewport()
	m.viewport.SetYOffset(5)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	next := updated.(appModel)
	if next.viewport.YOffset == 0 {
		t.Fatalf("expected action selection not to reset detail scroll to top")
	}
}

func TestActionResultClearsOnSkillSelectionChange(t *testing.T) {
	m := appModel{width: 120, height: 32, actionResult: &runner.Result{ExitCode: 0}, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "One", Scope: model.ScopeProject},
		{Name: "Two", Scope: model.ScopeProject},
	}}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	next := updated.(appModel)
	if next.actionResult != nil {
		t.Fatalf("expected action result cleared on skill change")
	}
}

func TestSearchEscapeClearsQueryAndBackspaceWorksOutsideSearch(t *testing.T) {
	m := appModel{searching: true, search: "build", result: model.ScanResult{Skills: []*model.Skill{{Name: "Build", Scope: model.ScopeProject}}}}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next := updated.(appModel)
	if next.searching || next.search != "" {
		t.Fatalf("expected esc to clear search, searching=%v search=%q", next.searching, next.search)
	}
	m = appModel{searching: false, search: "abc", result: model.ScanResult{Skills: []*model.Skill{{Name: "Build", Scope: model.ScopeProject}}}}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	next = updated.(appModel)
	if next.search != "ab" {
		t.Fatalf("expected backspace outside search to trim query, got %q", next.search)
	}
}

func TestDetailPaneClipsLongPreview(t *testing.T) {
	preview := strings.Repeat("- line\n", 80)
	m := appModel{width: 100, height: 20, result: model.ScanResult{Skills: []*model.Skill{{Name: "Long", Description: "desc", Scope: model.ScopeProject, Preview: preview}}}}
	m.syncViewport()
	out := m.previewViewport.View()
	lines := strings.Split(out, "\n")
	if len(lines) > m.previewViewport.Height {
		t.Fatalf("preview pane overflowed: got %d lines\n%s", len(lines), out)
	}
}

func TestMarkdownPreviewStripsFrontmatter(t *testing.T) {
	preview := "---\nname: better-icons\ndescription: icons\n---\n# Better Icons\n\nSearch icons."
	lines := renderMarkdownPreview(preview, 80)
	out := strings.Join(lines, "\n")
	if strings.Contains(out, "name: better-icons") || strings.Contains(out, "description: icons") || strings.Contains(out, "--------") {
		t.Fatalf("expected frontmatter to be hidden from markdown preview, got %q", out)
	}
	if !strings.Contains(out, "Better") || !strings.Contains(out, "Icons") || !strings.Contains(out, "Search") || !strings.Contains(out, "icons") {
		t.Fatalf("expected rendered markdown body, got %q", out)
	}
}

func TestMetadataShowsSkillDescription(t *testing.T) {
	m := actionTestModel(t.TempDir())
	lines := m.metadataLines(80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Description:") || !strings.Contains(joined, "desc") {
		t.Fatalf("expected skill description in metadata, got %q", joined)
	}
}

func TestDetailScrollKeysMoveViewport(t *testing.T) {
	preview := strings.Repeat("- line\n", 80)
	m := appModel{width: 100, height: 20, selected: 1, result: model.ScanResult{Skills: []*model.Skill{{Name: "Long", Description: "desc", Scope: model.ScopeProject, Preview: preview}}}}
	m.syncViewport()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	next := updated.(appModel)
	if next.previewViewport.YOffset <= 0 {
		t.Fatalf("expected preview scroll to move down, got %d", next.previewViewport.YOffset)
	}
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyHome})
	next = updated.(appModel)
	if next.previewViewport.YOffset != 0 {
		t.Fatalf("expected home to reset preview scroll, got %d", next.previewViewport.YOffset)
	}
}

func TestThreePaneLayoutFocusAndScroll(t *testing.T) {
	m := appModel{width: 120, height: 32, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "One", Scope: model.ScopeProject, Preview: "preview content", LocalLock: &model.LocalLockEntry{Source: "owner/one"}},
		{Name: "Two", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/two"}},
	}}}
	m.syncViewport()

	// Initial focus is focusSkills (0)
	if m.focus != focusSkills {
		t.Fatalf("expected initial focus to be focusSkills, got %v", m.focus)
	}

	// In Skills focus, h/l or left/right jump source groups instead of changing focus.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(appModel)
	if m.focus != focusSkills || m.selected != 2 {
		t.Fatalf("expected right in skills focus to jump source group, got focus %v selection %d", m.focus, m.selected)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(appModel)
	if m.focus != focusSkills || m.selected != 0 {
		t.Fatalf("expected left in skills focus to jump source group back, got focus %v selection %d", m.focus, m.selected)
	}

	// Press 2 to focus Metadata
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	m = updated.(appModel)
	if m.focus != focusMetadata {
		t.Fatalf("expected 2 to focus metadata, got %v", m.focus)
	}

	// Press 3 to focus Preview
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = updated.(appModel)
	if m.focus != focusPreview {
		t.Fatalf("expected 3 to focus preview, got %v", m.focus)
	}

	// Press tab to cycle: Preview -> Skills
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(appModel)
	if m.focus != focusSkills {
		t.Fatalf("expected tab cycle to Skills, got %v", m.focus)
	}

	// Press shift+tab to cycle backward: Skills -> Preview
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = updated.(appModel)
	if m.focus != focusPreview {
		t.Fatalf("expected shift+tab cycle to Preview, got %v", m.focus)
	}

	// Press left to cycle backward: Preview -> Metadata
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = updated.(appModel)
	if m.focus != focusMetadata {
		t.Fatalf("expected left cycle to Metadata, got %v", m.focus)
	}

	// Press right to cycle forward: Metadata -> Preview
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(appModel)
	if m.focus != focusPreview {
		t.Fatalf("expected right cycle to Preview, got %v", m.focus)
	}

	// h/l should never change focus outside the Skills pane.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(appModel)
	if m.focus != focusPreview {
		t.Fatalf("expected h to keep Preview focus, got %v", m.focus)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(appModel)
	if m.focus != focusPreview {
		t.Fatalf("expected l to keep Preview focus, got %v", m.focus)
	}

	// In Preview focus, scrolling j/k should change previewViewport offset, not skill selection
	m.selected = 0
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updated.(appModel)
	if m.selected != 0 {
		t.Fatalf("expected preview scroll to not change skill selection, got selection %d", m.selected)
	}

	// Check View output has border titles
	out := m.View()
	if !strings.Contains(out, "1 Inventory") || !strings.Contains(out, "2 Metadata") || !strings.Contains(out, "3 Preview") {
		t.Fatalf("expected border titles in View, got:\n%s", out)
	}

	// Enter opens modal
	m.selected = 1
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if !m.detailModal {
		t.Fatal("expected enter to open modal")
	}
}

func TestFullViewFitsTerminalDimensionsWithLongPreview(t *testing.T) {
	preview := strings.Repeat("a very long line that should be clipped and not wrap the entire screen\n", 120)
	m := appModel{width: 100, height: 24, result: model.ScanResult{Skills: []*model.Skill{{Name: "Long", Description: strings.Repeat("description ", 30), Scope: model.ScopeProject, Preview: preview}}}}
	assertViewFits(t, m, 100, 24)
}

func TestSmallTerminalFallbackFitsTinyHeights(t *testing.T) {
	for _, height := range []int{4, 5, 6} {
		t.Run(fmt.Sprintf("height_%d", height), func(t *testing.T) {
			m := appModel{width: 80, height: height, result: model.ScanResult{Skills: []*model.Skill{{Name: "Long", Description: "desc", Scope: model.ScopeProject}}}}
			out := m.View()
			assertRenderedSize(t, out, 80, height)
			if strings.Contains(out, "╭") || strings.Contains(out, "╰") {
				t.Fatalf("tiny terminal should render fallback, not cards: %q", out)
			}
		})
	}
}

func TestResponsiveViewFitsCommonTerminalSizes(t *testing.T) {
	preview := strings.Repeat("a very long line that should reflow into the detail viewport without breaking borders\n", 100)
	sizes := []struct {
		width  int
		height int
	}{
		{40, 7},
		{60, 12},
		{80, 20},
		{100, 24},
		{120, 40},
	}
	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.width, size.height), func(t *testing.T) {
			m := appModel{width: size.width, height: size.height, result: model.ScanResult{Skills: []*model.Skill{{Name: "Long", Description: strings.Repeat("description ", 12), Scope: model.ScopeProject, Preview: preview}}}}
			assertViewFits(t, m, size.width, size.height)
		})
	}
}

func TestNormalViewPreservesBottomBorders(t *testing.T) {
	m := appModel{width: 100, height: 24, result: model.ScanResult{Skills: []*model.Skill{{Name: "Build", Description: "desc", Scope: model.ScopeProject}}}}
	out := m.View()
	assertRenderedSize(t, out, 100, 24)
	if got := strings.Count(out, "╰"); got != 3 {
		t.Fatalf("expected three complete bottom-left borders, got %d\n%s", got, out)
	}
	if got := strings.Count(out, "╯"); got != 3 {
		t.Fatalf("expected three complete bottom-right borders, got %d\n%s", got, out)
	}
}

func assertViewFits(t *testing.T, m appModel, width, height int) {
	t.Helper()
	out := m.View()
	assertRenderedSize(t, out, width, height)
}

func assertRenderedSize(t *testing.T, out string, width, height int) {
	t.Helper()
	if got := lipgloss.Height(out); got > height {
		t.Fatalf("view height overflowed: got %d want <= %d\n%s", got, height, out)
	}
	for i, line := range strings.Split(out, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("line %d width overflowed: got %d want <= %d\n%s", i, got, width, line)
		}
	}
}

func TestVisibilityReasonTranslation(t *testing.T) {
	sk := &model.Skill{
		Name:  "TestSkill",
		Scope: model.ScopeProject,
		Visibility: []model.SkillVisibility{
			{Agent: "claude-code", Display: "Claude Code", Visible: true, Reason: "visible_via_symlink"},
			{Agent: "opencode", Display: "OpenCode", Visible: false, Reason: "missing_agent_link"},
			{Agent: "cursor", Display: "Cursor", Visible: false, Reason: "not_in_universal_canonical_dir"},
		},
	}

	// List badges stay compact; detail pane carries the explanation.
	if badge := compat.StripTerminalEscapes(agentVisibilityBadge(sk, "claude-code")); badge != "✓" {
		t.Errorf("expected compact available badge, got %q", badge)
	}
	if badge := agentVisibilityBadge(sk, "opencode"); badge != "×" {
		t.Errorf("expected compact unavailable badge, got %q", badge)
	}
	if badge := agentVisibilityBadge(sk, "cursor"); badge != "×" {
		t.Errorf("expected compact unavailable badge, got %q", badge)
	}

	// Test detail summary translation
	mAll := appModel{} // agent = ""
	linesAll := mAll.visibilitySummary(display.Skill(sk), 80)
	if len(linesAll) != 1 || !strings.Contains(linesAll[0], "Available to 1/3 agents") {
		t.Errorf("expected all-agents summary, got %v", linesAll)
	}
	mDetected := appModel{result: model.ScanResult{Agents: []model.AgentState{
		{Name: "claude-code", Detected: true},
		{Name: "opencode", Detected: true},
		{Name: "cursor", Detected: false},
	}}}
	linesDetected := mDetected.visibilitySummary(display.Skill(sk), 80)
	if len(linesDetected) != 1 || !strings.Contains(linesDetected[0], "Available to 1/2 detected agents") {
		t.Errorf("expected detected-agent summary, got %v", linesDetected)
	}

	mFiltered := appModel{agent: "claude-code"}
	linesFiltered := mFiltered.visibilitySummary(display.Skill(sk), 80)
	if len(linesFiltered) != 1 || !strings.Contains(linesFiltered[0], "Claude Code: available (symlinked)") {
		t.Errorf("expected filtered agent translation, got %v", linesFiltered)
	}
}

func TestMetadataOmitsHashAndUpdateNotes(t *testing.T) {
	m := appModel{width: 120, height: 32, selected: 1, result: model.ScanResult{Skills: []*model.Skill{{
		Name: "One", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo", ComputedHash: "abcdef123456"},
	}}}}
	out := m.View()
	if !strings.Contains(out, "Source:") || !strings.Contains(out, "owner/repo") {
		t.Fatalf("expected source detail in metadata, got: %s", out)
	}
	if strings.Contains(out, "Hash:") || strings.Contains(out, "abcdef123456") {
		t.Fatalf("hash should no longer appear in metadata, got: %s", out)
	}
	if strings.Contains(out, "Live update status") || strings.Contains(out, "check for updates") {
		t.Fatalf("update-status notes should no longer appear in metadata, got: %s", out)
	}
}

func TestDetailsOnboardingEmptyState(t *testing.T) {
	mEmpty := appModel{
		result: model.ScanResult{
			Skills: []*model.Skill{},
		},
	}
	lines := mEmpty.detailLines(80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Welcome to LazySkills!") || !strings.Contains(joined, "No skills were found") {
		t.Fatalf("expected onboarding instructions in details pane, got %q", joined)
	}

	mMissingDeps := appModel{
		result: model.ScanResult{
			Skills: []*model.Skill{},
			Preflight: &model.Preflight{
				CanRunSkills: false,
				Tools: map[string]model.ToolStatus{
					"skills": {Exists: false},
					"npx":    {Exists: false},
					"node":   {Exists: false},
					"npm":    {Exists: false},
				},
			},
		},
	}
	linesMissing := mMissingDeps.detailLines(80)
	joinedMissing := strings.Join(linesMissing, "\n")
	if !strings.Contains(joinedMissing, "Dependency Issue") || !strings.Contains(joinedMissing, "skills:") || !strings.Contains(joinedMissing, "missing") {
		t.Fatalf("expected dependency error details, got %q", joinedMissing)
	}
}

func TestSkillsFindTriggersRescan(t *testing.T) {
	m := appModel{}
	previews := m.currentActions()
	var findAct *actions.CommandPreview
	for _, p := range previews {
		if p.ID == "skills_find" {
			findAct = &p
			break
		}
	}
	if findAct == nil {
		t.Fatal("expected skills_find action in app-level actions")
	}
	if !findAct.Mutates {
		t.Error("expected skills_find to have Mutates = true to trigger TUI rescan")
	}
}

func TestContextualFooterAndHelpModal(t *testing.T) {
	oldLookPath := actions.LookPath
	actions.LookPath = func(file string) (string, error) {
		if file == "skills" {
			return "/usr/bin/skills", nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { actions.LookPath = oldLookPath })

	m := appModel{
		width:    100,
		height:   30,
		selected: 1,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{Name: "Build", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}, ObservedPaths: []model.ObservedPath{{Path: "/tmp/build", Scope: model.ScopeProject, Status: model.StatusCanonical}}},
			},
		},
	}
	m.syncViewport()

	// 1. Default footer (Skills focused)
	outDefault := m.View()
	if !strings.Contains(outDefault, "enter open · e enable/disable · c actions") {
		t.Fatalf("expected default footer in View, got:\n%s", outDefault)
	}
	if !strings.Contains(outDefault, "u update") || !strings.Contains(outDefault, "x remove") {
		t.Fatalf("expected tracked skill footer to include update/remove, got:\n%s", outDefault)
	}

	// 1b. Source row selected footer
	m.selected = 0
	outHeaderFooter := m.View()
	if !strings.Contains(outHeaderFooter, "enter browse · e enable/disable source") {
		t.Fatalf("expected source group footer in View, got:\n%s", outHeaderFooter)
	}
	m.selected = 1

	// 2. Metadata focus footer
	m.focus = focusMetadata
	outMeta := m.View()
	if !strings.Contains(outMeta, "scroll metadata") {
		t.Fatalf("expected metadata footer in View, got:\n%s", outMeta)
	}

	// 3. Preview focus footer
	m.focus = focusPreview
	outPreview := m.View()
	if !strings.Contains(outPreview, "scroll preview") {
		t.Fatalf("expected preview footer in View, got:\n%s", outPreview)
	}

	// 4. Search active footer
	m.searching = true
	outSearch := m.View()
	if !strings.Contains(outSearch, "type search · enter apply") {
		t.Fatalf("expected search footer in View, got:\n%s", outSearch)
	}
	m.searching = false

	// 5. Help modal open
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = updated.(appModel)
	if !m.helpOpen {
		t.Fatal("expected ? to open help modal")
	}

	outHelp := m.View()
	if !strings.Contains(outHelp, "LazySkills Keyboard Help") || !strings.Contains(outHelp, "Navigation & Focus:") {
		t.Fatalf("expected help modal content in View, got:\n%s", outHelp)
	}

	// 6. Help modal close with 'q'
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = updated.(appModel)
	if m.helpOpen {
		t.Fatal("expected q to close help modal")
	}
}

func TestFooterHidesUnavailableSkillHotkeys(t *testing.T) {
	oldLookPath := actions.LookPath
	actions.LookPath = func(file string) (string, error) {
		if file == "skills" {
			return "/usr/bin/skills", nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { actions.LookPath = oldLookPath })

	m := appModel{
		width:    100,
		height:   30,
		selected: 1,
		result: model.ScanResult{Skills: []*model.Skill{{
			Name:          "Loose Skill",
			Scope:         model.ScopeGlobal,
			CanonicalPath: "/tmp/loose-skill",
		}}},
	}
	m.syncViewport()

	footer := m.footerText(100)
	if strings.Contains(footer, "u update") {
		t.Fatalf("untracked skill footer should hide unavailable update hotkey, got %q", footer)
	}
	if !strings.Contains(footer, "enter open") || !strings.Contains(footer, "c actions") || !strings.Contains(footer, "? help") {
		t.Fatalf("expected neutral actions to remain visible, got %q", footer)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	next := updated.(appModel)
	if cmd != nil || next.confirming || next.running {
		t.Fatalf("unavailable update hotkey should stay idle, confirming=%v running=%v cmd=%v", next.confirming, next.running, cmd)
	}
}

func TestSourceScanHintIgnoresBulkSelection(t *testing.T) {
	m := appModel{
		width:    120,
		height:   32,
		selected: 0,
		result: model.ScanResult{Skills: []*model.Skill{
			{Name: "One", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
			{Name: "Two", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
		}},
	}
	m.selectedKeys = map[string]bool{skillKey(m.result.Skills[0]): true}
	m.syncViewport()

	footer := m.footerText(120)
	if !strings.Contains(footer, "d scan") {
		t.Fatalf("source header should show scan even with bulk selection active, got %q", footer)
	}
}

func TestDetailModalHelpHidesUnavailableOpen(t *testing.T) {
	t.Setenv("EDITOR", "")
	m := appModel{
		width:       120,
		height:      32,
		selected:    1,
		detailModal: true,
		result: model.ScanResult{Skills: []*model.Skill{{
			Name:  "Loose Skill",
			Scope: model.ScopeProject,
		}}},
	}

	help := m.detailModalHelpLine()
	if strings.Contains(help, "o open") || strings.Contains(help, "o open in editor") {
		t.Fatalf("detail modal should hide unavailable open hint, got %q", help)
	}
	if !strings.Contains(help, "c command picker") || !strings.Contains(help, "↑/↓ scroll") {
		t.Fatalf("detail modal should keep neutral hints, got %q", help)
	}
}

func TestDetailModalOpenHintIgnoresBulkSelection(t *testing.T) {
	t.Setenv("EDITOR", "vim")
	m := appModel{
		width:       120,
		height:      32,
		selected:    2,
		detailModal: true,
		result: model.ScanResult{Skills: []*model.Skill{
			{Name: "Selected Elsewhere", Scope: model.ScopeProject, SkillPath: "/tmp/selected/SKILL.md"},
			{Name: "Current Detail", Scope: model.ScopeProject, SkillPath: "/tmp/current/SKILL.md"},
		}},
	}
	m.selectedKeys = map[string]bool{skillKey(m.result.Skills[0]): true}
	m.syncViewport()

	help := m.detailModalHelpLine()
	if !strings.Contains(help, "o open in editor") {
		t.Fatalf("detail modal should show current-row open hint even with bulk selection, got %q", help)
	}
}

func TestSourceModalHelpIsCapabilityAware(t *testing.T) {
	oldLookPath := actions.LookPath
	actions.LookPath = func(file string) (string, error) {
		if file == "skills" {
			return "/usr/bin/skills", nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { actions.LookPath = oldLookPath })
	t.Setenv("EDITOR", "")

	m := appModel{
		width:       120,
		height:      32,
		selected:    0,
		detailModal: true,
		modalSource: "owner/repo",
		result: model.ScanResult{Skills: []*model.Skill{{
			Name:      "Installed",
			Scope:     model.ScopeProject,
			LocalLock: &model.LocalLockEntry{Source: "owner/repo"},
		}}},
	}
	m.syncViewport()

	help := m.detailModalHelpLine()
	if strings.Contains(help, "enter open") || strings.Contains(help, "o open") {
		t.Fatalf("source modal should hide unavailable installed-skill open hints, got %q", help)
	}
	if !strings.Contains(help, "d scan") || !strings.Contains(help, "c more") {
		t.Fatalf("source modal should keep available scan and actions hints, got %q", help)
	}
}

func TestSelectedSkillActionsDoNotIncludeAppActions(t *testing.T) {
	m := appModel{selected: 1, result: model.ScanResult{Skills: []*model.Skill{{Name: "Build", Scope: model.ScopeProject}}}}
	previews := m.currentActions()
	for _, preview := range previews {
		if preview.ID == "skills_init" || preview.ID == "skills_find" || preview.ID == "skills_update" {
			t.Fatalf("selected-skill actions should not include app-level action %q", preview.ID)
		}
	}
}

func TestCollapseExpandSourceGroups(t *testing.T) {
	m := appModel{
		width:           120,
		height:          32,
		focus:           focusSkills,
		collapsedGroups: make(map[string]bool),
		result: model.ScanResult{
			Skills: []*model.Skill{
				{Name: "One", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/one"}},
				{Name: "Two", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/one"}},
				{Name: "Three", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/two"}},
			},
		},
	}
	m.syncViewport()

	// Initially, both are expanded
	out1 := m.View()
	if !strings.Contains(out1, "- owner/one") || !strings.Contains(out1, "One") || !strings.Contains(out1, "Two") {
		t.Fatalf("expected expanded groups (with ASCII minus) and skills, got:\n%s", out1)
	}

	// Selection starts at 0 (header row of owner/one)
	if m.selected != 0 {
		t.Fatalf("expected selection to start at header row 0, got %d", m.selected)
	}

	// Press h to collapse "owner/one"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(appModel)
	if !m.isCollapsed("owner/one") {
		t.Fatal("expected owner/one to be collapsed")
	}

	// Selection should remain on index 0 (collapsed header)
	if m.selected != 0 {
		t.Fatalf("expected selection to remain on collapsed header row 0, got %d", m.selected)
	}

	// View should show + and hide "One" and "Two"
	out2 := m.View()
	if !strings.Contains(out2, "+ owner/one") || strings.Contains(out2, "  One [P]") || strings.Contains(out2, "  Two [P]") {
		t.Fatalf("expected collapsed group A (with ASCII plus) to hide child skills, got:\n%s", out2)
	}

	// Press l to expand "owner/one" again
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	m = updated.(appModel)
	if m.isCollapsed("owner/one") {
		t.Fatal("expected owner/one to be expanded")
	}

	out3 := m.View()
	if !strings.Contains(out3, "- owner/one") || !strings.Contains(out3, "One") {
		t.Fatalf("expected expanded group A to show child skills, got:\n%s", out3)
	}

	// Test j/k navigation skips collapsed rows
	// Collapse "owner/one" again
	m.collapsedGroups["owner/one"] = true
	m.clampSelection() // selection should clamp to 0 (header) or 1 (owner/two) or 2 (Three)

	// Since owner/one is collapsed, visible rows are:
	// index 0: + owner/one
	// index 1: - owner/two
	// index 2: Three
	m.selected = 2 // on Three

	// Pressing up/k from Three (index 2) should go to owner/two (index 1)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updated.(appModel)
	if m.selected != 1 {
		t.Fatalf("expected selection to go to index 1 (owner/two), got %d", m.selected)
	}

	// Test h/l outside focusSkills do not collapse groups and do not change focus
	m.focus = focusMetadata
	m.collapsedGroups = make(map[string]bool)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = updated.(appModel)
	if len(m.collapsedGroups) > 0 {
		t.Fatalf("h outside focusSkills should not collapse groups")
	}
	if m.focus != focusMetadata {
		t.Fatalf("h outside focusSkills should not change focus")
	}

	// Test group header row action details and commands
	m.focus = focusSkills
	m.selected = 0 // select header row owner/one
	m.collapsedGroups["owner/one"] = true
	outHeader := m.View()
	if !strings.Contains(outHeader, "State:") || !strings.Contains(outHeader, "collapsed") {
		t.Fatalf("expected header placeholder/metadata in Metadata pane, got:\n%s", outHeader)
	}

	// Verify that header row selection has no skill-scoped actions
	previewsHeader := m.currentActions()
	for _, p := range previewsHeader {
		if p.ID == "reinstall_update" || p.ID == "remove" {
			t.Fatalf("header row actions should not include skill-scoped action %q", p.ID)
		}
	}

	// Test enter on header row opens source detail modal without toggling collapse.
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if !m.detailModal {
		t.Fatal("expected enter on header to open source detail modal")
	}
	if !m.isCollapsed("owner/one") {
		t.Fatal("expected enter on header not to change collapsed state")
	}
}

func TestRichSourceInventoryMetadataAndActions(t *testing.T) {
	m := appModel{
		width:           120,
		height:          32,
		focus:           focusSkills,
		collapsedGroups: make(map[string]bool),
		result: model.ScanResult{
			Skills: []*model.Skill{
				{Name: "One", Scope: model.ScopeProject, CanonicalPath: "/tmp/one", LocalLock: &model.LocalLockEntry{Source: "owner/one", ComputedHash: "hash1"}},
				{Name: "Two", Scope: model.ScopeGlobal, CanonicalPath: "/tmp/two", LocalLock: &model.LocalLockEntry{Source: "owner/one"}},
			},
		},
	}
	m.syncViewport()

	// Select the header row owner/one (index 0)
	m.selected = 0

	// 1. Assert structured metadata info
	metaLines := m.metadataLines(80)
	metaJoined := strings.Join(metaLines, "\n")
	if !strings.Contains(metaJoined, "Source:      owner/one") {
		t.Errorf("expected source name in metadata, got %q", metaJoined)
	}
	if !strings.Contains(metaJoined, "State:") || !strings.Contains(metaJoined, "expanded") {
		t.Errorf("expected state in metadata, got %q", metaJoined)
	}
	if !strings.Contains(metaJoined, "Skills:      2 visible / 2 total") {
		t.Errorf("expected skills count in metadata, got %q", metaJoined)
	}
	if !strings.Contains(metaJoined, "Scope:") || (!strings.Contains(metaJoined, "mixed") && !strings.Contains(metaJoined, "Mixed")) {
		t.Errorf("expected scope mixed in metadata, got %q", metaJoined)
	}
	if strings.Contains(metaJoined, "Hash:") {
		t.Errorf("hash should no longer appear in metadata, got %q", metaJoined)
	}

	// 2. Assert source preview lists installed skills + available section
	prevLines := m.previewLines(80)
	prevJoined := strings.Join(prevLines, "\n")
	if !strings.Contains(prevJoined, "Installed (2)") {
		t.Errorf("expected installed count header in preview, got %q", prevJoined)
	}
	if !strings.Contains(prevJoined, "• One [P]") || !strings.Contains(prevJoined, "• Two [G]") {
		t.Errorf("expected installed skills listed in preview, got %q", prevJoined)
	}
	if !strings.Contains(prevJoined, "press d to scan this source") {
		t.Errorf("expected available scan hint in preview, got %q", prevJoined)
	}
	if !strings.Contains(prevJoined, "enter to browse · d to scan") {
		t.Errorf("expected action hint in preview, got %q", prevJoined)
	}

	// 3. Assert currentActions for source row
	acts := m.currentActions()
	var hasUpdate, hasRemove, hasAppLevel, hasSkillLevel bool
	for _, act := range acts {
		if act.ID == "bulk_reinstall_update" && act.Title == "Update installed skills from source" {
			hasUpdate = true
		}
		if act.ID == "bulk_remove" && act.Title == "Remove installed skills from source" {
			hasRemove = true
		}
		if act.ID == "skills_init" || act.ID == "skills_find" || act.ID == "skills_update" {
			hasAppLevel = true
		}
		if act.ID == "reinstall_update" || act.ID == "remove" || act.ID == "open_skill" {
			hasSkillLevel = true
		}
	}
	if !hasUpdate || !hasRemove {
		t.Errorf("expected source update/remove actions to be present, got %+v", acts)
	}
	if hasAppLevel {
		t.Errorf("did not expect app-level actions for source row, got %+v", acts)
	}
	if hasSkillLevel {
		t.Errorf("did not expect skill-scoped actions for source row, got %+v", acts)
	}
}

func TestRemoteDiscoverySuccess(t *testing.T) {
	oldGitClone := gitClone
	defer func() { gitClone = oldGitClone }()

	var capturedURL, capturedRef string
	gitClone = func(url, ref, tempDir string) error {
		capturedURL = url
		capturedRef = ref
		skillPath := filepath.Join(tempDir, "SKILL.md")
		skillContent := "---\nname: \"Remote Skill\"\ndescription: \"Remote description\"\n---\nRemote preview content"
		if err := os.WriteFile(skillPath, []byte(skillContent), 0o644); err != nil {
			return err
		}
		return nil
	}

	m := appModel{
		width: 120, height: 32, selected: 0,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{Name: "Existing", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
			},
		},
	}
	m.syncViewport()

	updated, cmd := m.startDiscovery("owner/repo", false)
	if cmd == nil {
		t.Fatal("expected discovery cmd")
	}
	msg := cmd()
	updated, _ = updated.Update(msg)
	next := updated.(appModel)

	if next.discovery["owner/repo"].Status != DiscoveryReady {
		t.Fatalf("expected discovery ready, got status %s, error: %s", next.discovery["owner/repo"].Status, next.discovery["owner/repo"].Error)
	}
	if capturedURL != "https://github.com/owner/repo" {
		t.Fatalf("expected URL to be https://github.com/owner/repo, got %s", capturedURL)
	}
	if capturedRef != "" {
		t.Fatalf("expected empty captured ref, got %s", capturedRef)
	}

	var foundAvailable bool
	for _, ds := range next.discovery["owner/repo"].Skills {
		if ds.Name == "Remote Skill" {
			foundAvailable = true
			break
		}
	}
	if !foundAvailable {
		t.Fatalf("expected 'Remote Skill' among discovered skills, got %#v", next.discovery["owner/repo"].Skills)
	}
	if next.availableCount("owner/repo") < 1 {
		t.Fatalf("expected at least one available (uninstalled) skill, got %d", next.availableCount("owner/repo"))
	}
}

func TestRemoteDiscoveryFailure(t *testing.T) {
	oldGitClone := gitClone
	defer func() { gitClone = oldGitClone }()

	gitClone = func(url, ref, tempDir string) error {
		return fmt.Errorf("network connection error")
	}

	m := appModel{
		width: 120, height: 32, selected: 0,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{Name: "Existing", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
			},
		},
	}
	updated, cmd := m.startDiscovery("owner/repo", false)
	if cmd == nil {
		t.Fatal("expected discovery cmd")
	}
	msg := cmd()
	updated, _ = updated.Update(msg)
	next := updated.(appModel)

	disc := next.discovery["owner/repo"]
	if disc.Status != DiscoveryFailed {
		t.Fatalf("expected status failed, got %s", disc.Status)
	}
	if !strings.Contains(disc.Error, "network connection error") {
		t.Fatalf("expected error to contain network connection error, got %s", disc.Error)
	}
}

func TestRemoteDiscoveryRejectsUnsafe(t *testing.T) {
	cases := []struct {
		source string
		safe   bool
	}{
		{"owner/repo", true},
		{"github:owner/repo", true},
		{"https://github.com/owner/repo", true},
		{"https://github.com/owner/repo.git", true},
		{"owner/repo#ref", true},
		{"owner/repo#ref-name_123.4", true},
		{"owner/repo#ref/with/slash", true},
		{"-owner/repo", false},
		{"owner/-repo", false},
		{"owner/repo#--ref", false},
		{"owner/repo;-somecmd", false},
		{"owner/repo/sub", false},
		{"https://gitlab.com/owner/repo", false},
	}
	for _, tc := range cases {
		_, _, ok := parseRemoteGitHubSource(tc.source)
		if ok != tc.safe {
			t.Errorf("expected %q safe=%v, got %v", tc.source, tc.safe, ok)
		}
	}
}

func TestIsSafeGitHubRef(t *testing.T) {
	cases := []struct {
		ref  string
		safe bool
	}{
		{"main", true},
		{"v1.2.3", true},
		{"feature/branch-name_1", true},
		{"", false},
		{"-branch", false},
		{"/branch", false},
		{"branch/", false},
		{"branch..name", false},
		{"branch@{1}", false},
		{"branch\\name", false},
		{"branch\x00name", false},
		{"branch\x1bname", false},
		{"branch name", false},
	}
	for _, tc := range cases {
		ok := isSafeGitHubRef(tc.ref)
		if ok != tc.safe {
			t.Errorf("expected ref %q safe=%v, got %v", tc.ref, tc.safe, ok)
		}
	}
}

func TestRemoteDiscoverySanitization(t *testing.T) {
	oldGitClone := gitClone
	defer func() { gitClone = oldGitClone }()

	gitClone = func(url, ref, tempDir string) error {
		skillPath := filepath.Join(tempDir, "SKILL.md")
		skillContent := "---\nname: \"Bad\\u001b[31m Skill\"\ndescription: \"Bad\\u001b[31m Description\"\n---\nBad\x1b[31m preview content"
		if err := os.WriteFile(skillPath, []byte(skillContent), 0o644); err != nil {
			return err
		}
		return nil
	}

	m := appModel{
		width: 120, height: 32, selected: 0,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{Name: "Existing", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
			},
		},
	}
	m.syncViewport()

	updated, cmd := m.startDiscovery("owner/repo", false)
	if cmd == nil {
		t.Fatal("expected discovery cmd")
	}
	msg := cmd()
	updated, _ = updated.Update(msg)
	next := updated.(appModel)

	disc := next.discovery["owner/repo"]
	if disc.Status != DiscoveryReady {
		t.Fatalf("expected discovery ready, got status %s, error: %s", disc.Status, disc.Error)
	}

	if len(disc.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(disc.Skills))
	}

	ds := disc.Skills[0]
	if strings.Contains(ds.Name, "\x1b") {
		t.Errorf("expected name to be sanitized, got %q", ds.Name)
	}
	if strings.Contains(ds.Description, "\x1b") {
		t.Errorf("expected description to be sanitized, got %q", ds.Description)
	}
	if strings.Contains(ds.Preview, "\x1b") {
		t.Errorf("expected preview to be sanitized, got %q", ds.Preview)
	}
}

func TestRemoteDiscoveryUnsafeRef(t *testing.T) {
	m := appModel{
		width: 120, height: 32, selected: 0,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{Name: "Existing", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
			},
		},
	}
	// Verify that starting discovery on an unsafe ref fails immediately with a clear message
	updated, cmd := m.startDiscovery("owner/repo#--unsafe-ref", false)
	if cmd != nil {
		t.Fatal("expected discovery cmd to be nil for unsafe ref")
	}
	next := updated.(appModel)
	disc := next.discovery["owner/repo#--unsafe-ref"]
	if disc.Status != DiscoveryFailed {
		t.Fatalf("expected status failed, got %s", disc.Status)
	}
	if !strings.Contains(disc.Error, "ref contains unsafe or invalid characters") {
		t.Fatalf("expected error to mention unsafe ref, got %s", disc.Error)
	}
}

func TestRemoteDiscoveryUnsafeFallbackRef(t *testing.T) {
	oldGitClone := gitClone
	defer func() { gitClone = oldGitClone }()

	calledClone := false
	gitClone = func(url, ref, tempDir string) error {
		calledClone = true
		return nil
	}

	m := appModel{
		width: 120, height: 32, selected: 0,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{
					Name:      "Existing",
					Scope:     model.ScopeProject,
					LocalLock: &model.LocalLockEntry{Source: "owner/repo", Ref: "--unsafe-ref"},
				},
			},
		},
	}
	// Trigger discovery on "owner/repo" which has an unsafe fallback ref
	updated, cmd := m.startDiscovery("owner/repo", false)
	if cmd != nil {
		t.Fatal("expected discovery cmd to be nil for unsafe fallback ref")
	}
	next := updated.(appModel)
	disc := next.discovery["owner/repo"]
	if disc.Status != DiscoveryFailed {
		t.Fatalf("expected status failed, got %s", disc.Status)
	}
	if !strings.Contains(disc.Error, "ref contains unsafe or invalid characters") {
		t.Fatalf("expected error to mention unsafe ref, got %s", disc.Error)
	}
	if calledClone {
		t.Fatal("gitClone was called but it should not have been")
	}
}

func TestSourceDiscoverActionAvailability(t *testing.T) {
	m := appModel{
		width: 120, height: 32, selected: 0,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{Name: "Untracked", Scope: model.ScopeProject}, // Custom / untracked group
			},
		},
	}
	m.syncViewport()
	acts := m.sourceActions("Custom / untracked")
	var discAction *actions.CommandPreview
	for _, act := range acts {
		if act.ID == "source_discover" {
			discAction = &act
			break
		}
	}
	if discAction == nil {
		t.Fatal("expected source_discover action to be present")
	}
	if discAction.Available {
		t.Fatal("expected discovery to be unavailable for untracked/custom group")
	}
	if !strings.Contains(discAction.Reason, "requires a local checkout") {
		t.Fatalf("expected meaningful reason, got %q", discAction.Reason)
	}
	if discAction.Command != "discover Custom / untracked" {
		t.Fatalf("expected Command text 'discover Custom / untracked', got %q", discAction.Command)
	}
}

func TestRemoteDiscoveryRawFallbackRefBypass(t *testing.T) {
	oldGitClone := gitClone
	defer func() { gitClone = oldGitClone }()

	calledClone := false
	gitClone = func(url, ref, tempDir string) error {
		calledClone = true
		return nil
	}

	// Case 1: Ref contains control/escape char \x1b
	m1 := appModel{
		width: 120, height: 32, selected: 0,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{
					Name:      "Existing",
					Scope:     model.ScopeProject,
					LocalLock: &model.LocalLockEntry{Source: "owner/repo", Ref: "main\x1b"},
				},
			},
		},
	}
	updated1, cmd1 := m1.startDiscovery("owner/repo", false)
	if cmd1 != nil {
		t.Fatal("expected discovery cmd to be nil for unsafe fallback ref containing escape char")
	}
	next1 := updated1.(appModel)
	disc1 := next1.discovery["owner/repo"]
	if disc1.Status != DiscoveryFailed {
		t.Fatalf("expected status failed, got %s", disc1.Status)
	}
	if !strings.Contains(disc1.Error, "ref contains unsafe or invalid characters") {
		t.Fatalf("expected error to mention unsafe ref, got %s", disc1.Error)
	}

	// Case 2: Ref contains newline char \n
	m2 := appModel{
		width: 120, height: 32, selected: 0,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{
					Name:      "Existing",
					Scope:     model.ScopeProject,
					LocalLock: &model.LocalLockEntry{Source: "owner/repo", Ref: "main\nnext"},
				},
			},
		},
	}
	updated2, cmd2 := m2.startDiscovery("owner/repo", false)
	if cmd2 != nil {
		t.Fatal("expected discovery cmd to be nil for unsafe fallback ref containing newline")
	}
	next2 := updated2.(appModel)
	disc2 := next2.discovery["owner/repo"]
	if disc2.Status != DiscoveryFailed {
		t.Fatalf("expected status failed, got %s", disc2.Status)
	}
	if !strings.Contains(disc2.Error, "ref contains unsafe or invalid characters") {
		t.Fatalf("expected error to mention unsafe ref, got %s", disc2.Error)
	}

	if calledClone {
		t.Fatal("gitClone was called but it should not have been")
	}
}

func TestInteractiveSourceDetailModal(t *testing.T) {
	t.Setenv("EDITOR", "nano")

	// Mock git clone
	oldGitClone := gitClone
	defer func() { gitClone = oldGitClone }()
	gitClone = func(url, ref, tempDir string) error {
		return nil
	}

	// 1. Set up model with a header/source row and one installed skill
	m := appModel{
		width:           120,
		height:          32,
		focus:           focusSkills,
		collapsedGroups: make(map[string]bool),
		result: model.ScanResult{
			Skills: []*model.Skill{
				{
					Name:          "InstalledSkill",
					Scope:         model.ScopeProject,
					CanonicalPath: "/path/to/InstalledSkill",
					Description:   "This is installed.",
					LocalLock:     &model.LocalLockEntry{Source: "owner/repo", Ref: "main"},
				},
			},
		},
	}
	m.discovery = make(map[string]SourceDiscovery)

	// In visibleRows, index 0 is the header "owner/repo", and index 1 is "InstalledSkill"
	rows := m.visibleRows()
	if len(rows) < 2 || !rows[0].isHeader {
		t.Fatalf("expected first row to be header, got visibleRows: %+v", rows)
	}

	// Select the header row
	m.selected = 0

	// 2. Open source modal (triggers discovery command when not checked)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	if !m.detailModal {
		t.Fatal("expected enter on header to open detail modal")
	}
	if m.modalSource != "owner/repo" {
		t.Fatalf("expected modalSource to be owner/repo, got %q", m.modalSource)
	}
	if cmd == nil {
		t.Fatal("expected opening unchecked source modal to trigger discovery command")
	}

	// 3. Re-opening (from the sidebar) an already-loading source doesn't re-trigger
	//    discovery. Use a modal-closed copy so enter targets the header, leaving the
	//    open modal in m intact for the steps below.
	mLoading := m
	mLoading.detailModal = false
	mLoading.modalSource = ""
	mLoading.discovery = map[string]SourceDiscovery{"owner/repo": {Status: DiscoveryLoading}}
	if _, cmdLoading := mLoading.Update(tea.KeyMsg{Type: tea.KeyEnter}); cmdLoading != nil {
		t.Fatal("expected re-opening a loading source not to trigger discovery again")
	}

	// Let's set it to ready with some available skills
	m.discovery["owner/repo"] = SourceDiscovery{
		Status: DiscoveryReady,
		Skills: []DiscoveredSkill{
			{Name: "InstalledSkill", Description: "This is installed.", Source: "owner/repo"},
			{Name: "AvailableSkill", Description: "This is available.", Source: "owner/repo"},
		},
	}

	// 4. Test j/k inside source modal changes modal child selection
	childRows := m.modalChildRows("owner/repo")
	if len(childRows) != 2 {
		t.Fatalf("expected 2 child rows (1 installed, 1 available), got %d", len(childRows))
	}
	if childRows[0].isAvailable || childRows[0].skill.Name != "InstalledSkill" {
		t.Fatalf("expected first child row to be InstalledSkill, got: %+v", childRows[0])
	}
	if !childRows[1].isAvailable || childRows[1].discoveredSkill.Name != "AvailableSkill" {
		t.Fatalf("expected second child row to be AvailableSkill, got: %+v", childRows[1])
	}

	if m.modalSelected != 0 {
		t.Fatalf("expected initial modalSelected to be 0, got %d", m.modalSelected)
	}

	// Press 'j' to go to next child row
	updatedJ, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = updatedJ.(appModel)
	if m.modalSelected != 1 {
		t.Fatalf("expected modalSelected to be 1 after pressing j, got %d", m.modalSelected)
	}

	// Press 'k' to go back to first child row
	updatedK, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = updatedK.(appModel)
	if m.modalSelected != 0 {
		t.Fatalf("expected modalSelected to be 0 after pressing k, got %d", m.modalSelected)
	}

	// 5. Test 'c' on installed child exposes installed skill actions
	m.modalSelected = 0 // installed child
	updatedC, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	mC := updatedC.(appModel)
	if mC.detailModal {
		t.Fatal("expected 'c' to close detailModal")
	}
	if !mC.commands {
		t.Fatal("expected 'c' to open commands picker")
	}
	acts := mC.currentActions()
	hasOpen := false
	hasRemove := false
	for _, act := range acts {
		if act.ID == "open_skill" {
			hasOpen = true
		}
		if act.ID == "remove" {
			hasRemove = true
		}
	}
	if !hasOpen || !hasRemove {
		t.Fatalf("expected installed child actions to include open and remove, got: %+v", acts)
	}

	// 6. Test 'c' on available child exposes install action
	m.modalSelected = 1  // available child
	m.detailModal = true // reopen modal for test
	m.commands = false
	updatedCAvail, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	mCAvail := updatedCAvail.(appModel)
	if mCAvail.detailModal {
		t.Fatal("expected 'c' to close detailModal")
	}
	if !mCAvail.commands {
		t.Fatal("expected 'c' to open commands picker")
	}
	actsAvail := mCAvail.currentActions()
	hasInstall := false
	for _, act := range actsAvail {
		if act.ID == "install_skill" {
			hasInstall = true
		}
	}
	if !hasInstall {
		t.Fatalf("expected available child actions to include install_skill, got: %+v", actsAvail)
	}

	// 7. Verify Source modal shows installed and available sections after discovery
	m.detailModal = true
	m.modalSelected = 0
	viewOut := m.View()
	if !strings.Contains(viewOut, "Installed Skills:") {
		t.Fatal("expected view to contain Installed Skills: section header")
	}
	if !strings.Contains(viewOut, "InstalledSkill [P]") {
		t.Fatal("expected view to contain InstalledSkill [P]")
	}
	if !strings.Contains(viewOut, "Available Skills:") {
		t.Fatal("expected view to contain Available Skills: section header")
	}
	if !strings.Contains(viewOut, "AvailableSkill [available]") {
		t.Fatal("expected view to contain AvailableSkill [available]")
	}
}

func TestModalEnterInstallsAvailableSkill(t *testing.T) {
	m := appModel{width: 120, height: 32, detailModal: true, modalSource: "owner/repo", modalSelected: 1,
		result: model.ScanResult{Skills: []*model.Skill{
			{Name: "Installed", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
		}}}
	m.discovery = map[string]SourceDiscovery{
		"owner/repo": {Status: DiscoveryReady, Skills: []DiscoveredSkill{
			{Name: "Installed", Source: "owner/repo"},
			{Name: "Available", Source: "owner/repo"},
		}},
	}
	// modalSelected 1 = the available child.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := updated.(appModel)
	if next.detailModal {
		t.Fatal("expected enter on available child to close the modal")
	}
	if !next.confirming || next.pendingAction == nil {
		t.Fatalf("expected enter to arm install confirmation, confirming=%v pending=%v", next.confirming, next.pendingAction)
	}
	if next.pendingAction.ID != "install_skill" {
		t.Fatalf("expected pending install_skill action, got %q", next.pendingAction.ID)
	}
	// Cancel clears the pending action.
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if c := updated.(appModel); c.confirming || c.pendingAction != nil {
		t.Fatalf("expected esc to clear confirm, confirming=%v pending=%v", c.confirming, c.pendingAction)
	}
}

func TestSourceDetailModalSelectionScrollsIntoView(t *testing.T) {
	discovered := make([]DiscoveredSkill, 0, 30)
	for i := 0; i < 30; i++ {
		discovered = append(discovered, DiscoveredSkill{
			Name:        fmt.Sprintf("Available%02d", i),
			Description: "available skill",
			Source:      "owner/repo",
		})
	}
	m := appModel{
		width:           120,
		height:          24,
		focus:           focusSkills,
		selected:        0,
		detailModal:     true,
		modalSource:     "owner/repo",
		modalSelected:   0,
		viewport:        viewport.New(80, 8),
		collapsedGroups: make(map[string]bool),
		discovery: map[string]SourceDiscovery{
			"owner/repo": {Status: DiscoveryReady, Skills: discovered},
		},
		result: model.ScanResult{Skills: []*model.Skill{{
			Name:      "InstalledSkill",
			Scope:     model.ScopeProject,
			LocalLock: &model.LocalLockEntry{Source: "owner/repo"},
		}}},
	}

	for i := 0; i < 12; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
		m = updated.(appModel)
	}
	if m.modalSelected != 12 {
		t.Fatalf("expected modalSelected to follow j navigation, got %d", m.modalSelected)
	}
	if m.viewport.YOffset == 0 {
		t.Fatal("expected source modal viewport to scroll down as selection moves")
	}
}

func TestRawSourceValidationBeforeRemoteDiscovery(t *testing.T) {
	oldGitClone := gitClone
	defer func() { gitClone = oldGitClone }()

	calledClone := false
	gitClone = func(url, ref, tempDir string) error {
		calledClone = true
		return nil
	}

	// Case 1: LocalLock.Source contains controls/escapes
	m1 := appModel{
		width: 120, height: 32, selected: 0,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{
					Name:      "BadLocal",
					Scope:     model.ScopeProject,
					LocalLock: &model.LocalLockEntry{Source: "owner/repo\x1b[31m", Ref: "main"},
				},
			},
		},
	}
	m1.discovery = make(map[string]SourceDiscovery)
	// Open detail modal on header row
	updated1, cmd1 := m1.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m1 = updated1.(appModel)
	if !m1.detailModal {
		t.Fatal("expected detailModal to be open")
	}
	if cmd1 != nil {
		t.Fatal("expected no discovery command to be generated due to raw local source validation failure")
	}
	disc1 := m1.discovery["owner/repo"]
	if disc1.Status != DiscoveryFailed {
		t.Fatalf("expected discovery to fail, got status: %s", disc1.Status)
	}
	if !strings.Contains(disc1.Error, "raw source contains control, newline, or escape characters") {
		t.Fatalf("expected validation error message, got: %s", disc1.Error)
	}

	// Case 2: GlobalLock.SourceURL contains newline
	m2 := appModel{
		width: 120, height: 32, selected: 0,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{
					Name:       "BadGlobal",
					Scope:      model.ScopeGlobal,
					GlobalLock: &model.GlobalLockEntry{SourceURL: "owner/repo\nnext", Ref: "main"},
				},
			},
		},
	}
	m2.discovery = make(map[string]SourceDiscovery)
	updated2, cmd2 := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 = updated2.(appModel)
	if !m2.detailModal {
		t.Fatal("expected detailModal to be open")
	}
	if cmd2 != nil {
		t.Fatal("expected no discovery command to be generated due to raw global source validation failure")
	}
	disc2 := m2.discovery["owner/repo next"]
	if disc2.Status != DiscoveryFailed {
		t.Fatalf("expected discovery to fail, got status: %s", disc2.Status)
	}
	if !strings.Contains(disc2.Error, "raw source contains control, newline, or escape characters") {
		t.Fatalf("expected validation error message, got: %s", disc2.Error)
	}

	// Case 3: LocalLock.Source contains trailing space (sanitization changes it)
	m3 := appModel{
		width: 120, height: 32, selected: 0,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{
					Name:      "BadLocalSpace",
					Scope:     model.ScopeProject,
					LocalLock: &model.LocalLockEntry{Source: "owner/repo ", Ref: "main"},
				},
			},
		},
	}
	m3.discovery = make(map[string]SourceDiscovery)
	updated3, cmd3 := m3.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m3 = updated3.(appModel)
	if !m3.detailModal {
		t.Fatal("expected detailModal to be open")
	}
	if cmd3 != nil {
		t.Fatal("expected no discovery command to be generated due to raw local source sanitization check failure")
	}
	disc3 := m3.discovery["owner/repo"]
	if disc3.Status != DiscoveryFailed {
		t.Fatalf("expected discovery to fail, got status: %s", disc3.Status)
	}
	if !strings.Contains(disc3.Error, "raw source contains unsafe characters or is modified by sanitization") {
		t.Fatalf("expected validation error message, got: %s", disc3.Error)
	}

	if calledClone {
		t.Fatal("gitClone was unexpectedly called")
	}
}

func TestModalCommandPickerUXWithBulkSelection(t *testing.T) {
	t.Setenv("EDITOR", "nano")

	// Set up model with a header/source row, one installed skill, bulk selectedKeys, source modal open, modal selected installed child, commands open.
	m := appModel{
		width:           120,
		height:          32,
		focus:           focusSkills,
		collapsedGroups: make(map[string]bool),
		selectedKeys:    map[string]bool{"InstalledSkill": true},
		detailModal:     false,
		commands:        true,
		modalSource:     "owner/repo",
		modalSelected:   0,
		result: model.ScanResult{
			Skills: []*model.Skill{
				{
					Name:          "InstalledSkill",
					Scope:         model.ScopeProject,
					CanonicalPath: "/path/to/InstalledSkill",
					Description:   "This is installed.",
					LocalLock:     &model.LocalLockEntry{Source: "owner/repo", Ref: "main"},
				},
			},
		},
	}

	// Pressing 'u' should set reinstall_update (single-skill modal child action) instead of bulk
	updatedU, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	mU := updatedU.(appModel)
	if mU.confirming {
		actions := mU.currentActions()
		if len(actions) == 0 || mU.action >= len(actions) || actions[mU.action].ID != "reinstall_update" {
			t.Fatalf("expected reinstall_update action to be selected, got actions: %+v, selected action: %d", actions, mU.action)
		}
	} else {
		t.Fatal("expected update action to trigger confirmation overlay")
	}

	// Pressing 'x' should set remove instead of bulk
	m.confirming = false
	updatedX, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	mX := updatedX.(appModel)
	if mX.confirming {
		actions := mX.currentActions()
		if len(actions) == 0 || mX.action >= len(actions) || actions[mX.action].ID != "remove" {
			t.Fatalf("expected remove action to be selected, got actions: %+v, selected action: %d", actions, mX.action)
		}
	} else {
		t.Fatal("expected remove action to trigger confirmation overlay")
	}
}

func TestTuiEnableDisableActions(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillFile := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillFile, []byte("---\nname: my-skill\n---\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sk := &model.Skill{
		Name:  "my-skill",
		Scope: model.ScopeProject,
		ObservedPaths: []model.ObservedPath{
			{
				Path:   skillDir,
				Scope:  model.ScopeProject,
				Agent:  "aider-desk",
				Status: model.StatusCopy,
			},
		},
	}

	m := appModel{
		cwd:   tempDir,
		agent: "aider-desk",
	}

	acts := m.appendEnableDisableActions(nil, sk)
	if len(acts) != 1 || acts[0].ID != "disable_skill" {
		t.Fatalf("expected 1 disable action, got %+v", acts)
	}
	disableAction := acts[0]

	_, cmd := m.executeAction(disableAction)
	if cmd == nil {
		t.Fatal("expected loadSnapshot cmd to be returned on success")
	}

	disabledPath := filepath.Join(tempDir, ".lazyskills-disabled", "my-skill")
	if _, err := os.Stat(disabledPath); err != nil {
		t.Fatalf("expected disabled folder to exist at %s, got error: %v", disabledPath, err)
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Fatalf("expected active folder at %s to be gone, got error: %v", skillDir, err)
	}

	sk.ObservedPaths[0].Status = model.StatusDisabled
	sk.ObservedPaths[0].Path = disabledPath
	sk.ObservedPaths[0].TargetPath = skillDir

	acts = m.appendEnableDisableActions(nil, sk)
	if len(acts) != 1 || acts[0].ID != "enable_skill" {
		t.Fatalf("expected 1 enable action, got %+v", acts)
	}
	enableAction := acts[0]

	_, cmd = m.executeAction(enableAction)
	if cmd == nil {
		t.Fatal("expected loadSnapshot cmd to be returned on success")
	}

	if _, err := os.Stat(skillDir); err != nil {
		t.Fatalf("expected active folder to exist at %s, got error: %v", skillDir, err)
	}
	if _, err := os.Stat(disabledPath); !os.IsNotExist(err) {
		t.Fatalf("expected disabled folder at %s to be gone, got error: %v", disabledPath, err)
	}
}

func TestTuiEnableDisableEdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	sk := &model.Skill{
		Name:  "my-skill",
		Scope: model.ScopeProject,
		ObservedPaths: []model.ObservedPath{
			{
				Path:   skillDir,
				Scope:  model.ScopeProject,
				Agent:  "aider-desk",
				Status: model.StatusCopy,
			},
			{
				Path:   skillDir,
				Scope:  model.ScopeProject,
				Agent:  "amp",
				Status: model.StatusCopy,
			},
		},
	}

	// 1. Guard check: per-agent disable blocked when path is shared by multiple agents
	m := appModel{
		cwd:   tempDir,
		agent: "aider-desk",
	}
	acts := m.appendEnableDisableActions(nil, sk)
	if len(acts) != 1 || acts[0].ID != "disable_skill" || acts[0].Available {
		t.Fatalf("expected disable action to be unavailable for shared path, got: %+v", acts)
	}
	if !strings.Contains(acts[0].Reason, "shared by multiple agents") {
		t.Fatalf("expected shared reason, got %q", acts[0].Reason)
	}

	// 2. Guard check: per-agent enable blocked when disabled path is shared by multiple agents
	disabledPath := filepath.Join(tempDir, ".lazyskills-disabled", "my-skill")
	skDisabled := &model.Skill{
		Name:  "my-skill",
		Scope: model.ScopeProject,
		ObservedPaths: []model.ObservedPath{
			{
				Path:       disabledPath,
				TargetPath: skillDir,
				Scope:      model.ScopeProject,
				Agent:      "aider-desk",
				Status:     model.StatusDisabled,
			},
			{
				Path:       disabledPath,
				TargetPath: skillDir,
				Scope:      model.ScopeProject,
				Agent:      "amp",
				Status:     model.StatusDisabled,
			},
		},
	}
	acts = m.appendEnableDisableActions(nil, skDisabled)
	if len(acts) != 1 || acts[0].ID != "enable_skill" || acts[0].Available {
		t.Fatalf("expected enable action to be unavailable for shared path, got: %+v", acts)
	}
	if !strings.Contains(acts[0].Reason, "shared by multiple agents") {
		t.Fatalf("expected shared reason, got %q", acts[0].Reason)
	}

	// 3. Deduplication check: scope-level disable has no duplicates in Args
	mNoAgent := appModel{
		cwd: tempDir,
	}
	acts = mNoAgent.appendEnableDisableActions(nil, sk)
	if len(acts) != 1 || acts[0].ID != "disable_skill" {
		t.Fatalf("expected 1 scope disable action, got %+v", acts)
	}
	if len(acts[0].Exec.Args) != 1 {
		t.Fatalf("expected deduplicated args, got %+v", acts[0].Exec.Args)
	}

	// 4. Preflight validate dest already exists
	destDir := filepath.Join(tempDir, ".lazyskills-disabled", "my-skill")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatal(err)
	}
	disableAction := acts[0]
	resModel, cmd := mNoAgent.executeAction(disableAction)
	if cmd != nil {
		t.Fatal("expected action to fail preflight (no cmd returned) when dest exists")
	}
	resAppModel := resModel.(appModel)
	if resAppModel.actionResult == nil || !strings.Contains(resAppModel.actionResult.Err, "destination already exists") {
		t.Fatalf("expected actionResult to carry destination already exists error, got: %+v", resAppModel.actionResult)
	}
}
