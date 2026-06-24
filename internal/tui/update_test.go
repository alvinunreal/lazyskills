package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/model"
	"github.com/alvinunreal/lazyskills/internal/registry"
	"github.com/alvinunreal/lazyskills/internal/runner"
	"github.com/alvinunreal/lazyskills/internal/selfupdate"
)

func TestTUIFooterUpdateNotice(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := newModel("")
	m.width = 100
	m.height = 30

	// 1. Without update plan, no notice
	footer := m.footerText(100)
	if strings.Contains(footer, "U update") {
		t.Errorf("unexpected update notice in footer when no update is available: %q", footer)
	}

	// 2. With update available
	m.updatePlan = &selfupdate.UpdatePlan{
		Current: "v1.0.0",
		Latest:  "v1.1.0",
		Status:  selfupdate.StatusAvailable,
		Channel: "manual",
	}

	footer = m.footerText(120)
	if !strings.Contains(footer, "U update") || !strings.Contains(footer, "v1.1.0 available") {
		t.Errorf("expected update notice in footer, got: %q", footer)
	}

	// 3. Notice omitted if width is too narrow
	footerNarrow := m.footerText(40)
	if strings.Contains(footerNarrow, "U update") {
		t.Errorf("expected update notice to be omitted in narrow viewport, got: %q", footerNarrow)
	}
}

func TestTUIAppUpdateModalTransitions(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := newModel("")
	m.width = 100
	m.height = 30
	m.updatePlan = &selfupdate.UpdatePlan{
		Current:    "v1.0.0",
		Latest:     "v1.1.0",
		Status:     selfupdate.StatusAvailable,
		Channel:    "manual",
		CanExecute: true,
	}

	// 1. Pressing 'U' opens the modal
	modelTmp, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("U")})
	m = modelTmp.(appModel)
	if !m.appUpdateModal {
		t.Error("expected appUpdateModal to be true after pressing 'U'")
	}
	if cmd != nil {
		t.Error("expected no immediate command on key 'U'")
	}

	// 2. View of modal
	viewOut := m.View()
	if !strings.Contains(viewOut, "LazySkills App Update") {
		t.Errorf("expected app update view header, got: %s", viewOut)
	}
	if !strings.Contains(viewOut, "Current Version: v1.0.0") {
		t.Errorf("expected current version in view, got: %s", viewOut)
	}
	if !strings.Contains(viewOut, "Latest Version:  v1.1.0") {
		t.Errorf("expected latest version in view, got: %s", viewOut)
	}

	// 3. Pressing Enter starts update execution
	modelTmp, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelTmp.(appModel)
	if !m.updatingApp {
		t.Error("expected updatingApp to be true after pressing Enter")
	}
	if cmd == nil {
		t.Fatal("expected update command to be scheduled, got nil")
	}

	// 4. Update result message handler success
	modelTmp, cmd = m.Update(appUpdateResultMsg{err: nil})
	m = modelTmp.(appModel)
	if m.updatingApp {
		t.Error("expected updatingApp to be false after completion")
	}
	if !m.updateSuccess {
		t.Error("expected updateSuccess to be true after nil err result")
	}
	if m.updateError != nil {
		t.Errorf("expected no updateError, got: %v", m.updateError)
	}

	viewSuccess := m.View()
	if !strings.Contains(viewSuccess, "Update successful") {
		t.Errorf("expected success message in view, got: %s", viewSuccess)
	}

	// 5. Esc closes modal
	modelTmp, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = modelTmp.(appModel)
	if m.appUpdateModal {
		t.Error("expected appUpdateModal to be false after pressing Esc")
	}
}

func TestTUIAppUpdateModalNonActionable(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := newModel("")
	m.width = 100
	m.height = 30
	m.updatePlan = &selfupdate.UpdatePlan{
		Current:        "v1.0.0",
		Latest:         "v1.1.0",
		Status:         selfupdate.StatusAvailable,
		Channel:        "brew",
		Confidence:     "high",
		CanExecute:     false,
		CommandPreview: "brew upgrade lazyskills",
		Reason:         "Homebrew managed install. Please upgrade using Homebrew.",
	}

	// Pressing 'U' opens the modal
	modelTmp, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("U")})
	m = modelTmp.(appModel)
	if !m.appUpdateModal {
		t.Fatal("expected modal to open")
	}

	// Check that we see the guidance command instead of an update trigger
	viewOut := m.View()
	if !strings.Contains(viewOut, "Auto-update is not supported for this install channel.") {
		t.Errorf("expected unsupported guidance, got: %s", viewOut)
	}
	if !strings.Contains(viewOut, "brew upgrade lazyskills") {
		t.Errorf("expected command preview in guidance view, got: %s", viewOut)
	}

	// Pressing Enter does not trigger update since CanExecute is false
	modelTmp, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelTmp.(appModel)
	if m.updatingApp {
		t.Error("should not set updatingApp to true when CanExecute is false")
	}
	if cmd != nil {
		t.Error("should not return a command when CanExecute is false")
	}
}

func TestTUIAppUpdateModalStates(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := newModel("")
	m.width = 100
	m.height = 30

	// 1. Pending check state: updatePlan is nil
	modelTmp, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("U")})
	m = modelTmp.(appModel)
	if !m.appUpdateModal {
		t.Fatal("expected modal to open")
	}
	viewOut := m.View()
	if !strings.Contains(viewOut, "Checking for updates...") {
		t.Errorf("expected Checking for updates message, got: %s", viewOut)
	}

	// 2. Error state: updatePlanErr is set
	m.appUpdateModal = false
	m.updatePlanErr = fmt.Errorf("sample query error")
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("U")})
	m = modelTmp.(appModel)
	if !m.appUpdateModal {
		t.Fatal("expected modal to open")
	}
	viewOut = m.View()
	if !strings.Contains(viewOut, "Update check failed") || !strings.Contains(viewOut, "sample query error") {
		t.Errorf("expected error message to be surfaced, got: %s", viewOut)
	}
}

func TestTUIRegistrySearchModalFlow(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := newModel("")
	m.width = 100
	m.height = 30
	m.result = model.ScanResult{
		Skills: []*model.Skill{
			{
				Name:      "already",
				Scope:     model.ScopeProject,
				LocalLock: &model.LocalLockEntry{Source: "github.com/org/already"},
			},
			{
				Name:      "similar-matched",
				Scope:     model.ScopeProject,
				LocalLock: &model.LocalLockEntry{Source: "github.com/org/another"},
			},
		},
	}

	// 1. Modal opens from app actions
	previews := actions.AppLevelActions()
	var regPreview *actions.CommandPreview
	for i := range previews {
		if previews[i].ID == "find_new_skills" {
			regPreview = &previews[i]
		}
	}
	if regPreview == nil {
		t.Fatal("expected find_new_skills action in app-level actions")
	}

	modelTmp, cmd := m.Update(actionResultMsg{result: runner.Result{ExitCode: 0}}) // dummy trigger/sync check
	m = modelTmp.(appModel)
	modelTmp, cmd = m.executeAction(*regPreview)
	m = modelTmp.(appModel)
	if !m.registryModal {
		t.Fatal("expected registryModal to be true after executing find_new_skills action")
	}
	if cmd != nil {
		t.Fatal("expected no immediate command after opening registry search")
	}

	// Direct keyboard entry should also open the modal from normal inventory.
	m.registryModal = false
	modelTmp, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	m = modelTmp.(appModel)
	if !m.registryModal {
		t.Fatal("expected n to open registry search modal")
	}
	if cmd != nil {
		t.Fatal("expected no immediate command after n opens registry search")
	}

	// 2. Query <2 chars does not search
	modelTmp, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = modelTmp.(appModel)
	if m.registryQuery != "a" {
		t.Errorf("expected query to be 'a', got %q", m.registryQuery)
	}
	if cmd != nil {
		t.Errorf("expected no search command for query < 2 chars, got: %T", cmd)
	}

	// 3. Query >= 2 chars schedules debounce search
	modelTmp, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = modelTmp.(appModel)
	if m.registryQuery != "ab" {
		t.Fatalf("expected query to be 'ab', got %q", m.registryQuery)
	}
	if cmd == nil {
		t.Fatal("expected search command to be scheduled for query >= 2 chars")
	}

	// 4. Debounced/latest-query result handling
	// Simulate debounce msg firing
	modelTmp, cmd = m.Update(registryDebounceMsg{generation: m.registryGeneration, query: "ab"})
	m = modelTmp.(appModel)
	if !m.registryLoading {
		t.Error("expected registryLoading to be true after debounce triggers search")
	}
	if cmd == nil {
		t.Fatal("expected searchRegistryCmd to be returned by debounce handler")
	}

	// Stale/old generation debounce msg should be ignored
	modelTmp2, cmd2 := m.Update(registryDebounceMsg{generation: m.registryGeneration - 1, query: "ab"})
	if modelTmp2.(appModel).registryResults != nil || cmd2 != nil {
		t.Error("expected stale debounce msg to be ignored")
	}

	// Make sure the view renders without crash in loading state
	loadingView := m.View()
	if !strings.Contains(loadingView, "Searching registry...") {
		t.Errorf("expected view to indicate loading, got: %s", loadingView)
	}

	// Simulate search results payload
	registryResults := []registry.Skill{
		{
			DisplayName: "Already Installed Skill",
			Slug:        "already",
			Source:      "github.com/org/already",
			Installs:    42,
		},
		{
			DisplayName: "Similar Installed Skill",
			Slug:        "similar-matched",
			Source:      "github.com/org/different-source",
			Installs:    10,
		},
		{
			DisplayName: "Brand New Skill",
			Slug:        "brand-new",
			Source:      "github.com/org/brand-new",
			Installs:    999,
		},
	}

	modelTmp, cmd = m.Update(registrySearchMsg{
		generation: m.registryGeneration,
		results:    registryResults,
		err:        nil,
	})
	m = modelTmp.(appModel)
	if m.registryLoading {
		t.Error("expected registryLoading to be false after search result arrives")
	}
	if len(m.registryResults) != 3 {
		t.Fatalf("expected 3 results, got %d", len(m.registryResults))
	}

	// View of results
	resultsView := m.View()
	if !strings.Contains(resultsView, "Already Installed Skill") {
		t.Errorf("expected view to contain Displays Names, got: %s", resultsView)
	}
	if !strings.Contains(resultsView, "Similar Installed") {
		t.Errorf("expected view to contain similarity check name, got: %s", resultsView)
	}

	// 5. Exact installed disables install; similar installed warns/allows.
	// First selection is index 0: "Already Installed Skill" (exact installed)
	m.registrySelected = 0
	status0, _ := m.checkRegistrySkillStatus(m.registryResults[0])
	if status0 != StatusInstalled {
		t.Errorf("expected StatusInstalled, got %d", status0)
	}
	// Try to install project (enter) on exact installed: should do nothing (not close modal, not go to confirming)
	m.registryFocusList = true
	modelTmp, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelTmp.(appModel)
	if !m.registryModal || m.confirming {
		t.Error("expected install request on exact installed to be ignored (modal remains open, no confirmation)")
	}

	// Second selection is index 1: "Similar Installed Skill" (similar installed)
	m.registrySelected = 1
	status1, warnMsg1 := m.checkRegistrySkillStatus(m.registryResults[1])
	if status1 != StatusSimilarInstalled {
		t.Errorf("expected StatusSimilarInstalled, got %d", status1)
	}
	if warnMsg1 == "" {
		t.Error("expected similar warning message, got empty")
	}
	// Try to install project on similar installed: should warn but allow (confirming = true, registryModal = false)
	modelTmp, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelTmp.(appModel)
	if m.registryModal || !m.confirming {
		t.Error("expected install request on similar/name-only match to close registry search and schedule confirmation")
	}
	if m.pendingAction == nil {
		t.Fatal("expected pending action to be armed")
	}
	if !strings.Contains(m.pendingAction.Description, "Warning: A similar skill") {
		t.Errorf("expected confirmation descriptions to warn about similar installed skill, got: %s", m.pendingAction.Description)
	}
	usesSlug := false
	for _, arg := range m.pendingAction.Args {
		if arg == "similar-matched" {
			usesSlug = true
		}
		if arg == "Similar Installed Skill" {
			t.Fatalf("install command must use registry slug, not display name: %v", m.pendingAction.Args)
		}
	}
	if !usesSlug {
		t.Fatalf("expected project install command to use slug similar-matched, got %v", m.pendingAction.Args)
	}

	// 6. Global install appends -g
	m.registryModal = true
	m.confirming = false
	m.pendingAction = nil
	m.registrySelected = 2 // "Brand New Skill"
	status2, _ := m.checkRegistrySkillStatus(m.registryResults[2])
	if status2 != StatusInstallable {
		t.Errorf("expected StatusInstallable, got %d", status2)
	}
	// Try to install globally ('g'): should arm confirmation with -g
	modelTmp, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
	m = modelTmp.(appModel)
	if m.registryModal || !m.confirming {
		t.Error("expected global install ('g') to close registry search and schedule confirmation")
	}
	if m.pendingAction == nil {
		t.Fatal("expected armed global confirmation action")
	}
	hasG := false
	for _, arg := range m.pendingAction.Args {
		if arg == "-g" {
			hasG = true
			break
		}
	}
	if !hasG {
		t.Errorf("expected global install command args to contain '-g', got: %v", m.pendingAction.Args)
	}
}

func TestTUIRegistrySearchErrorStates(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := newModel("")
	m.width = 100
	m.height = 30
	m.registryModal = true
	m.registryQuery = "xyz"

	// Simulate error payload
	modelTmp, _ := m.Update(registrySearchMsg{
		generation: m.registryGeneration,
		results:    nil,
		err:        fmt.Errorf("API timeout/offline error"),
	})
	m = modelTmp.(appModel)

	if m.registryError == nil {
		t.Fatal("expected registryError to be recorded")
	}

	viewOut := m.View()
	if !strings.Contains(viewOut, "API timeout/offline error") {
		t.Errorf("expected error message in view, got: %s", viewOut)
	}
	if m.registryQuery != "xyz" {
		t.Errorf("expected query text to be preserved on error, got %q", m.registryQuery)
	}

	// Simulate editing query to change it/retry
	modelTmp, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	m = modelTmp.(appModel)
	if m.registryQuery != "xyzz" {
		t.Errorf("expected query to be editable after error, got %q", m.registryQuery)
	}
	if cmd == nil {
		t.Error("expected editing to trigger another debounced search")
	}

	// Enter in error state should retry the same query immediately.
	m.registryError = fmt.Errorf("still offline")
	modelTmp, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelTmp.(appModel)
	if !m.registryLoading {
		t.Error("expected enter to retry and set loading state")
	}
	if cmd == nil {
		t.Error("expected enter retry to start registry search command")
	}
}

func TestTUIRegistryInvalidResultDisablesInstall(t *testing.T) {
	m := newModel("")
	m.width = 100
	m.height = 30
	m.registryModal = true
	m.registryQuery = "bad"
	m.registryFocusList = true
	m.registryResults = []registry.Skill{{
		DisplayName: "Unsafe Skill",
		Slug:        "unsafe",
		Source:      "owner/repo",
		Invalid:     true,
		Reason:      "registry skill slug contains unsafe characters",
	}}

	help := m.registryModalHelpLine()
	if strings.Contains(help, "enter install") || strings.Contains(help, "g install") {
		t.Fatalf("invalid registry result should not advertise install keys, got %q", help)
	}
	if !strings.Contains(help, "install unavailable") {
		t.Fatalf("expected unavailable help copy, got %q", help)
	}

	modelTmp, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelTmp.(appModel)
	if cmd != nil || m.confirming || !m.registryModal {
		t.Fatalf("invalid registry install should no-op in modal, confirming=%v modal=%v cmd=%v", m.confirming, m.registryModal, cmd)
	}
}

func TestSourceModalSearchFiltersChildRows(t *testing.T) {
	m := appModel{
		width:       100,
		height:      30,
		detailModal: true,
		modalSource: "owner/repo",
		modalSelected: 5,
		result: model.ScanResult{Skills: []*model.Skill{{
			Name:      "Installed Alpha",
			Scope:     model.ScopeProject,
			LocalLock: &model.LocalLockEntry{Source: "owner/repo"},
		}}},
		discovery: map[string]SourceDiscovery{
			"owner/repo": {
				Status: DiscoveryReady,
				Skills: []DiscoveredSkill{
					{Name: "Alpha Available", Description: "first", Source: "owner/repo"},
					{Name: "Beta Available", Description: "second", Source: "owner/repo"},
					{Name: "Gamma Available", Description: "third", Source: "owner/repo"},
				},
			},
		},
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = updated.(appModel)
	if !m.modalSearching {
		t.Fatal("expected / to enter source modal search mode")
	}

	for _, r := range []rune{'b', 'e'} {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(appModel)
	}

	if m.modalSearch != "be" {
		t.Fatalf("expected search query to update, got %q", m.modalSearch)
	}
	rows := m.filteredModalChildRows("owner/repo")
	if len(rows) != 1 || !rows[0].isAvailable || rows[0].discoveredSkill == nil || rows[0].discoveredSkill.Name != "Beta Available" {
		t.Fatalf("expected filtered modal rows to keep only Beta Available, got %#v", rows)
	}
	if m.modalSelected != 0 {
		t.Fatalf("expected modal selection to clamp to first filtered row, got %d", m.modalSelected)
	}

	help := m.detailModalHelpLine()
	if !strings.Contains(help, "type to filter") {
		t.Fatalf("expected modal help to mention search mode, got %q", help)
	}

	out := m.View()
	if !strings.Contains(out, "Beta Available") || strings.Contains(out, "Alpha Available") || strings.Contains(out, "Gamma Available") {
		t.Fatalf("expected filtered view to show only matching child rows, got %q", out)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(appModel)
	if m.modalSearching {
		t.Fatal("expected esc to exit modal search mode without closing the modal")
	}
	if !m.detailModal || m.modalSource != "owner/repo" {
		t.Fatalf("expected source modal to remain open after leaving search mode, got detail=%v source=%q", m.detailModal, m.modalSource)
	}
}
