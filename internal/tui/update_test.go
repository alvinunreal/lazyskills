package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/model"
	"github.com/alvinunreal/lazyskills/internal/registry"
	"github.com/alvinunreal/lazyskills/internal/runner"
	"github.com/alvinunreal/lazyskills/internal/selfupdate"
	tea "github.com/charmbracelet/bubbletea"
)

func TestTUIFooterUpdateNotice(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := newModel("")
	m.width = 100
	m.height = 30

	// 1. Without update plan, no notice
	footer := m.footerText(100, m.visibleRows(), m.currentActions())
	if strings.Contains(footer, "U update") {
		t.Errorf("unexpected update notice in footer when no update is available: %q", footer)
	}

	// 2. With update available
	m.updatePlan = &selfupdate.UpdatePlan{
		Current: "v1.0.0",
		Latest:  "v1.1.0",
		Status:  selfupdate.StatusAvailable,
	}
	footer = m.footerText(120, m.visibleRows(), m.currentActions())
	if !strings.Contains(footer, "U update") || !strings.Contains(footer, "v1.1.0 available") {
		t.Errorf("expected update notice in footer, got: %q", footer)
	}

	// 3. Notice omitted if width is too narrow
	m.width = 40
	footerNarrow := m.footerText(40, m.visibleRows(), m.currentActions())
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
		Current:        "v1.0.0",
		Latest:         "v1.1.0",
		Status:         selfupdate.StatusAvailable,
		Channel:        "manual",
		CommandPreview: "curl -fsSL https://raw.githubusercontent.com/alvinunreal/lazyskills/main/scripts/install.sh | sh",
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
	if !strings.Contains(viewOut, "LazySkills Update") {
		t.Errorf("expected app update view header, got: %s", viewOut)
	}
	if !strings.Contains(viewOut, "Current Version: v1.0.0") {
		t.Errorf("expected current version in view, got: %s", viewOut)
	}
	if !strings.Contains(viewOut, "Latest Version:  v1.1.0") {
		t.Errorf("expected latest version in view, got: %s", viewOut)
	}

	// 3. Pressing Enter does not trigger any update execution command
	modelTmp, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelTmp.(appModel)
	if cmd != nil {
		t.Errorf("expected no command on Enter, got: %v", cmd)
	}
	if !m.appUpdateModal {
		t.Error("expected modal to remain open on Enter")
	}

	// 4. Esc closes modal
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
		CommandPreview: "brew upgrade lazyskills",
		Reason:         "Homebrew managed install. Please upgrade using Homebrew.",
	}

	// Pressing 'U' opens the modal
	modelTmp, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("U")})
	m = modelTmp.(appModel)
	if !m.appUpdateModal {
		t.Fatal("expected modal to open")
	}

	// Check that we see the guidance command and the manual update text
	viewOut := m.View()
	if !strings.Contains(viewOut, "A newer version of LazySkills is available.") {
		t.Errorf("expected new version available copy, got: %s", viewOut)
	}
	if !strings.Contains(viewOut, "To update manually, run:") {
		t.Errorf("expected manual update run copy, got: %s", viewOut)
	}
	if !strings.Contains(viewOut, "brew upgrade lazyskills") {
		t.Errorf("expected command preview in guidance view, got: %s", viewOut)
	}

	// Pressing Enter does not trigger update
	modelTmp, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelTmp.(appModel)
	if cmd != nil {
		t.Error("should not return a command on Enter")
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
		if arg == "--global" {
			hasG = true
			break
		}
	}
	if !hasG {
		t.Errorf("expected global install command args to contain '--global', got: %v", m.pendingAction.Args)
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

func TestTUINoFalseNoSkillsFoundWhileSearchPending(t *testing.T) {
	m := newModel("")
	m.width = 100
	m.height = 30
	m.registryModal = true
	m.registryQuery = "a"
	m.registryResults = nil
	m.registryFocusList = false

	modelTmp, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = modelTmp.(appModel)
	if !m.registryLoading {
		t.Fatal("expected registryLoading to be true immediately on typing query >= 2 chars")
	}

	viewStr := m.View()
	if strings.Contains(viewStr, "No skills found in registry.") {
		t.Error("should not show 'No skills found' while search is loading/pending")
	}
	if !strings.Contains(viewStr, "Searching registry...") {
		t.Error("expected 'Searching registry...' to be shown while search is loading/pending")
	}
}

func TestTUICancelPreservesRegistryModalState(t *testing.T) {
	m := newModel("")
	m.width = 100
	m.height = 30
	m.registryModal = true
	m.registryQuery = "hello"
	m.registryResults = []registry.Skill{{
		DisplayName: "Hello Skill",
		Slug:        "hello-slug",
		Source:      "owner/hello",
	}}
	m.registrySelected = 0
	m.registryFocusList = true

	// Press Enter to install (this starts confirmation)
	modelTmp, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelTmp.(appModel)
	if m.registryModal {
		t.Fatal("expected registryModal to be false during confirmation")
	}
	if !m.confirming {
		t.Fatal("expected confirmation to be active")
	}

	// Press Esc to cancel
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = modelTmp.(appModel)
	if !m.registryModal {
		t.Fatal("expected return to registryModal after cancelling")
	}
	if m.registryQuery != "hello" {
		t.Errorf("expected registryQuery to be preserved, got %q", m.registryQuery)
	}
	if len(m.registryResults) != 1 || m.registryResults[0].Slug != "hello-slug" {
		t.Errorf("expected registryResults to be preserved, got %#v", m.registryResults)
	}
}

func TestTUIRegistrySpaceTogglesMultiSelect(t *testing.T) {
	m := newModel("")
	m.width = 100
	m.height = 30
	m.registryModal = true
	m.registryQuery = "hello"
	m.registryResults = []registry.Skill{
		{DisplayName: "One", Slug: "one", Source: "owner/one"},
		{DisplayName: "Two", Slug: "two", Source: "owner/two"},
	}
	m.registrySelected = 0
	m.registryFocusList = true

	// Press Space to select "One"
	modelTmp, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = modelTmp.(appModel)

	key1 := "owner/one\x00one"
	if _, ok := m.registrySelectedKeys[key1]; !ok {
		t.Fatal("expected 'One' to be selected")
	}

	// Press Down then Space to select "Two"
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = modelTmp.(appModel)
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = modelTmp.(appModel)

	key2 := "owner/two\x00two"
	if _, ok := m.registrySelectedKeys[key2]; !ok {
		t.Fatal("expected 'Two' to be selected")
	}
	if len(m.registrySelectedKeys) != 2 {
		t.Errorf("expected 2 selected skills, got %d", len(m.registrySelectedKeys))
	}

	// Press Space again on "Two" to deselect it
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = modelTmp.(appModel)
	if _, ok := m.registrySelectedKeys[key2]; ok {
		t.Fatal("expected 'Two' to be deselected")
	}
	if len(m.registrySelectedKeys) != 1 {
		t.Errorf("expected 1 selected skill remaining, got %d", len(m.registrySelectedKeys))
	}
}

func TestTUIRegistryMultiInstallDispatch(t *testing.T) {
	m := newModel("")
	m.width = 100
	m.height = 30
	m.registryModal = true
	m.registryQuery = "hello"
	m.registryResults = []registry.Skill{
		{DisplayName: "One", Slug: "one", Source: "owner/one"},
		{DisplayName: "Two", Slug: "two", Source: "owner/two"},
	}
	m.registrySelected = 0
	m.registryFocusList = true

	// Select both of them
	modelTmp, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = modelTmp.(appModel)
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = modelTmp.(appModel)
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = modelTmp.(appModel)

	// Press Enter to install
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = modelTmp.(appModel)

	if m.registryModal {
		t.Fatal("expected registryModal to be closed")
	}
	if !m.confirming {
		t.Fatal("expected confirming to be active")
	}
	if m.pendingAction == nil {
		t.Fatal("expected pendingAction to be set")
	}
	if m.pendingAction.ID != "bulk_install_skills" {
		t.Errorf("expected bulk_install_skills action, got %s", m.pendingAction.ID)
	}
	if len(m.pendingAction.Exec.Batch) != 2 {
		t.Errorf("expected a batch size of 2, got %d", len(m.pendingAction.Exec.Batch))
	}

	// Check prompt/visual contains bulk info
	viewStr := m.View()
	if !strings.Contains(viewStr, "Confirm action") && !strings.Contains(viewStr, "Install selected skills") {
		t.Errorf("unexpected confirmation view: %s", viewStr)
	}
}
