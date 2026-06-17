package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lazyskills/internal/model"
	"lazyskills/internal/runner"
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

func TestViewRendersQuietVersionFooter(t *testing.T) {
	m := appModel{width: 100, height: 30, result: model.ScanResult{Skills: []*model.Skill{{Name: "Build", Description: "Build desc", Scope: model.ScopeProject}}}}
	out := m.View()
	if !strings.Contains(out, "LazySkills v1") || strings.Contains(out, "actions are guarded") || !strings.Contains(out, "Build") {
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

func TestNextAgentFilterCyclesThroughObservedAgents(t *testing.T) {
	m := appModel{result: model.ScanResult{Skills: []*model.Skill{
		{Name: "A", ObservedPaths: []model.ObservedPath{{Agent: "opencode"}, {Agent: "cursor"}}},
	}}}
	first := m.nextAgentFilter()
	if first != "cursor" {
		t.Fatalf("expected first observed agent cursor, got %q", first)
	}
	m.agent = first
	second := m.nextAgentFilter()
	if second != "opencode" {
		t.Fatalf("expected second observed agent opencode, got %q", second)
	}
	m.agent = second
	if got := m.nextAgentFilter(); got != "" {
		t.Fatalf("expected cycle back to all, got %q", got)
	}
}

func TestAgentFilterCyclesDetectedAgentsBeforeSupportedFallback(t *testing.T) {
	m := appModel{result: model.ScanResult{Agents: []model.AgentState{
		{Name: "opencode", Display: "OpenCode", Detected: true},
		{Name: "cursor", Display: "Cursor"},
		{Name: "claude-code", Display: "Claude Code", Detected: true},
	}}}
	if got := m.agentFilters(); strings.Join(got, ",") != "claude-code,opencode" {
		t.Fatalf("expected only detected agents in rotation, got %#v", got)
	}
	m = appModel{result: model.ScanResult{Agents: []model.AgentState{{Name: "opencode", Display: "OpenCode"}}}}
	if got := m.agentFilters(); len(got) != 1 || got[0] != "opencode" {
		t.Fatalf("expected fallback to supported agents when none detected, got %#v", got)
	}
}

func TestAgentFilterCanSelectSupportedAgentWithNoSkills(t *testing.T) {
	m := appModel{width: 100, height: 30, agent: "claude-code", result: model.ScanResult{Skills: []*model.Skill{
		{Name: "OpenCode Skill", Description: "desc", Scope: model.ScopeProject, ObservedPaths: []model.ObservedPath{{Agent: "opencode"}}},
	}}}
	items := m.filteredSkills()
	if len(items) != 0 {
		t.Fatalf("expected no skills for claude-code, got %#v", items)
	}
	out := m.View()
	if !strings.Contains(out, "Claude Code") || !strings.Contains(out, "has no visible skills") {
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
	m := appModel{width: 120, height: 40, commands: true, result: model.ScanResult{Skills: []*model.Skill{{
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
	m := appModel{width: 120, height: 32, agent: "claude-code", result: model.ScanResult{Skills: []*model.Skill{{
		Name:        "Build",
		Description: "desc",
		Scope:       model.ScopeProject,
		Visibility:  []model.SkillVisibility{{Agent: "claude-code", Display: "Claude Code", Visible: false, Reason: "missing_agent_link"}},
	}}}}
	out := m.View()
	if !strings.Contains(out, "Build") || !strings.Contains(out, "Claude Code cannot see") || !strings.Contains(out, "missing_agent_link") {
		t.Fatalf("expected active agent visibility reason, got %q", out)
	}
}

func TestAgentFilterListMarksNonVisibleSkills(t *testing.T) {
	m := appModel{width: 120, height: 32, agent: "claude-code", result: model.ScanResult{Skills: []*model.Skill{
		{
			Name:       "Visible",
			Scope:      model.ScopeProject,
			Visibility: []model.SkillVisibility{{Agent: "claude-code", Display: "Claude Code", Visible: true, Reason: "visible_via_symlink"}},
		},
		{
			Name:       "Missing",
			Scope:      model.ScopeProject,
			Visibility: []model.SkillVisibility{{Agent: "claude-code", Display: "Claude Code", Visible: false, Reason: "missing_agent_link"}},
		},
	}}}
	out := m.View()
	if !strings.Contains(out, "Visible [project] ✓ visible") || !strings.Contains(out, "Missing [project] × missing_agent") {
		t.Fatalf("expected list-level visibility badges, got %q", out)
	}
}

func TestListRendersIssueRowsInRedWithSubtleBadge(t *testing.T) {
	m := appModel{width: 120, height: 32, selected: 1, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "Healthy", Scope: model.ScopeProject},
		{Name: "Problem", Scope: model.ScopeProject, HealthIssues: []model.HealthIssue{{Type: "missing_file", Message: "missing SKILL.md"}}},
	}}}
	out := m.View()
	if !strings.Contains(out, "Problem [project] ⚠ 1") {
		t.Fatalf("expected subtle issue badge, got %q", out)
	}
	if strings.Contains(out, "BROKEN") {
		t.Fatalf("issue badge should stay subtle, got %q", out)
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
		{"default_enter", ""},
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

func TestConfirmationRendersCenteredModal(t *testing.T) {
	m := actionTestModel(t.TempDir())
	m.commands = true
	m.action = actionIndex(t, m, "Reinstall/update selected skill")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(appModel)
	out := m.View()
	if !strings.Contains(out, "Confirm") || !strings.Contains(out, "Press Enter or y to confirm") || strings.Contains(out, "Bulk actions") {
		t.Fatalf("expected standalone confirmation modal, got %q", out)
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
	if m.selectedCount() != 1 || !strings.Contains(m.View(), "1 selected") || !strings.Contains(m.View(), "● One") {
		t.Fatalf("expected one marked skill, count=%d view=%q", m.selectedCount(), m.View())
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(appModel)
	if m.selectedCount() != 0 {
		t.Fatalf("expected esc to clear selection, got %d", m.selectedCount())
	}
}

func TestEscClearsSelectionBeforeLeavingActionModeOrAgentFilter(t *testing.T) {
	m := bulkActionTestModel(t.TempDir())
	m.commands = true
	m.agent = "opencode"
	m.selectedKeys = map[string]bool{skillKey(m.result.Skills[0]): true}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next := updated.(appModel)
	if next.selectedCount() != 0 || !next.commands || next.agent != "opencode" {
		t.Fatalf("expected esc to clear selection first, selected=%d commands=%v agent=%q", next.selectedCount(), next.commands, next.agent)
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
	m := appModel{width: 120, height: 32, result: model.ScanResult{Skills: []*model.Skill{{
		Name: "One", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/web/SKILL.md", Ref: "main"},
	}}}}
	out := m.View()
	if !strings.Contains(out, "Source: owner/repo") || !strings.Contains(out, "Folder: skills/web") || !strings.Contains(out, "Ref: main") {
		t.Fatalf("expected source/folder/ref details, got %q", out)
	}
}

func TestSkillListShowsSourceGroups(t *testing.T) {
	m := appModel{width: 120, height: 32, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "One", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/web/SKILL.md"}},
		{Name: "Two", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/web/SKILL.md"}},
		{Name: "Other", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo", SkillPath: "skills/data/SKILL.md"}},
	}}}
	out := m.View()
	if !strings.Contains(out, "owner/repo") {
		t.Fatalf("expected source group header, got %q", out)
	}
	if strings.Count(out, "─ owner/repo") != 1 || strings.Contains(out, "owner/repo / skills/web") || strings.Contains(out, "owner/repo / skills/data") {
		t.Fatalf("expected one repo-level group header, got %q", out)
	}
}

func TestSkillListSeparatesNoSourceMetadata(t *testing.T) {
	m := appModel{width: 120, height: 32, result: model.ScanResult{Skills: []*model.Skill{
		{Name: "Tracked", Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: "owner/repo"}},
		{Name: "Manual", Scope: model.ScopeProject},
	}}}
	out := m.View()
	if !strings.Contains(out, "─ owner/repo") || !strings.Contains(out, "─ No source metadata") {
		t.Fatalf("expected explicit source and no-source groups, got %q", out)
	}
}

func TestGroupedListKeepsSelectedRowVisible(t *testing.T) {
	skills := []*model.Skill{}
	for i := 0; i < 12; i++ {
		skills = append(skills, &model.Skill{Name: fmt.Sprintf("Skill %02d", i), Scope: model.ScopeProject, LocalLock: &model.LocalLockEntry{Source: fmt.Sprintf("owner/repo-%02d", i)}})
	}
	m := appModel{width: 120, height: 10, selected: 11, result: model.ScanResult{Skills: skills}}
	out := m.View()
	if !strings.Contains(out, "Skill 11 [project]") {
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

func TestRemoveRequiresExactTypedIdentity(t *testing.T) {
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
	if cmd != nil || !m.confirming || !strings.Contains(m.confirmError, "Type yes") || m.actionResult != nil {
		t.Fatalf("expected inline confirmation error without command, confirming=%v err=%q result=%#v cmd=%v", m.confirming, m.confirmError, m.actionResult, cmd)
	}
}

func actionTestModel(cwd string) appModel {
	return appModel{cwd: cwd, width: 120, height: 32, result: model.ScanResult{Skills: []*model.Skill{{
		Name:          "Deploy Skill",
		Description:   "desc",
		Scope:         model.ScopeProject,
		CanonicalPath: "/tmp/deploy-skill",
		LocalLock:     &model.LocalLockEntry{Source: "owner/repo"},
	}}}}
}

func bulkActionTestModel(cwd string) appModel {
	return appModel{cwd: cwd, width: 120, height: 32, result: model.ScanResult{Skills: []*model.Skill{
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
	preview := strings.Repeat("line\n", 80)
	m := appModel{width: 100, height: 20, result: model.ScanResult{Skills: []*model.Skill{{Name: "Long", Description: "desc", Scope: model.ScopeProject, Preview: preview}}}}
	m.syncViewport()
	out := m.detailPane()
	lines := strings.Split(out, "\n")
	if len(lines) > m.viewport.Height {
		t.Fatalf("detail pane overflowed: got %d lines\n%s", len(lines), out)
	}
}

func TestDetailScrollKeysMoveViewport(t *testing.T) {
	preview := strings.Repeat("line\n", 80)
	m := appModel{width: 100, height: 20, result: model.ScanResult{Skills: []*model.Skill{{Name: "Long", Description: "desc", Scope: model.ScopeProject, Preview: preview}}}}
	m.syncViewport()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	next := updated.(appModel)
	if next.viewport.YOffset <= 0 {
		t.Fatalf("expected detail scroll to move down, got %d", next.viewport.YOffset)
	}
	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyHome})
	next = updated.(appModel)
	if next.viewport.YOffset != 0 {
		t.Fatalf("expected home to reset detail scroll, got %d", next.viewport.YOffset)
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
