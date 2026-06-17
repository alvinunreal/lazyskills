package tui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"lazyskills/internal/model"
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

func TestViewRendersReadOnlyFooter(t *testing.T) {
	m := appModel{width: 100, height: 30, result: model.ScanResult{Skills: []*model.Skill{{Name: "Build", Description: "Build desc", Scope: model.ScopeProject}}}}
	out := m.View()
	if !strings.Contains(out, "LazySkills is read-only") || !strings.Contains(out, "Build") {
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
	if first != "claude-code" {
		t.Fatalf("expected first supported agent claude-code, got %q", first)
	}
	m.agent = first
	second := m.nextAgentFilter()
	if second != "codex" {
		t.Fatalf("expected second supported agent codex, got %q", second)
	}
	m.agent = "opencode"
	if got := m.nextAgentFilter(); got != "" {
		t.Fatalf("expected cycle back to all, got %q", got)
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
	if !strings.Contains(out, "Command previews") || !strings.Contains(out, "Preview only") || !strings.Contains(out, "npx skills") {
		t.Fatalf("expected command previews in output: %q", out)
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
