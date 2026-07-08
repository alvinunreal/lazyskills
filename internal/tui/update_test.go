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
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
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
	m.registryResults = []registry.Skill{{
		DisplayName: "Old Result",
		Slug:        "old",
		Source:      "owner/old",
	}}
	m.registrySelected = 0
	m.registryFocusList = false

	modelTmp, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'b'}})
	m = modelTmp.(appModel)
	if !m.registryLoading {
		t.Fatal("expected registryLoading to be true immediately on typing query >= 2 chars")
	}

	viewStr := m.View()
	if strings.Contains(viewStr, "Old Result") {
		t.Error("should hide old results while a new search is pending/loading")
	}
	if strings.Contains(viewStr, "No skills found in registry.") {
		t.Error("should not show 'No skills found' while search is loading/pending")
	}
	if !strings.Contains(viewStr, "Searching registry...") {
		t.Error("expected 'Searching registry...' to be shown while search is loading/pending")
	}

	// Verify navigation is blocked when loading
	m.registryFocusList = true
	m.registryResults = []registry.Skill{{
		DisplayName: "Old",
		Slug:        "old",
	}}
	// Pressing 'j' should be a no-op when loading
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = modelTmp.(appModel)
	if m.registrySelected != 0 {
		t.Errorf("expected navigation to be blocked during search loading, got selection=%d", m.registrySelected)
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

func TestTUIRegistryListRenderingWithContextAndFocus(t *testing.T) {
	m := newModel("")
	m.width = 120
	m.height = 30
	m.registryModal = true
	m.registryQuery = "xyz"
	m.registryResults = []registry.Skill{
		{DisplayName: "My Display Name", Slug: "my-display-name", Source: "https://github.com/my-org/my-repo/skills/my-folder"},
	}
	m.registrySelected = 0

	// Mock local discovery map to test description lookup matching
	appDisc := SourceDiscovery{
		Status: DiscoveryReady,
		Skills: []DiscoveredSkill{
			{Name: "My Display Name", Description: "This is a great skill description!"},
		},
	}
	m.discovery = map[string]SourceDiscovery{
		"https://github.com/my-org/my-repo/skills/my-folder": appDisc,
	}

	// Test 1: Search focused (list unfocused)
	m.registryFocusList = false
	viewStr1 := m.View()
	if !strings.Contains(viewStr1, "My Display Name") || !strings.Contains(viewStr1, "my-org/my-repo") {
		t.Fatal("expected view to contain display name and source context")
	}
	// Prefix reserves one marker cell for selection: focused/unselected is "> Name".
	if !strings.Contains(viewStr1, "> My Display Name") {
		t.Errorf("expected prefix '> ' for highlighted unselected row, got view:\n%s", viewStr1)
	}
	m.registrySelectedKeys = map[string]registry.Skill{
		"https://github.com/my-org/my-repo/skills/my-folder\x00my-display-name": m.registryResults[0],
	}
	viewSelected := m.View()
	if !strings.Contains(viewSelected, ">●My Display Name") {
		t.Errorf("expected selected marker to replace reserved blank as '>●', got view:\n%s", viewSelected)
	}
	m.registrySelectedKeys = nil
	// Verify parsed Repository/Folder and matched Description in Right Pane
	if !strings.Contains(viewStr1, "Source:      my-org/my-repo") {
		t.Errorf("expected parsed Source to be displayed, got view:\n%s", viewStr1)
	}
	if !strings.Contains(viewStr1, "Path:        skills/my-folder") {
		t.Errorf("expected parsed Path to be displayed, got view:\n%s", viewStr1)
	}
	if !strings.Contains(viewStr1, "This is a great skill description") {
		t.Errorf("expected matched Description to be displayed, got view:\n%s", viewStr1)
	}

	// Test 2: List focused -> selectedStyle (active selected background)
	m.registryFocusList = true
	// Trigger project install to show confirmation overlay
	modelTmp, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mClone := modelTmp.(appModel)

	if !mClone.confirming {
		t.Fatal("expected confirmation overlay to be active")
	}

	// Confirmation screen should explicitly show target agents
	viewStr2 := mClone.View()
	if !strings.Contains(viewStr2, "Target agents") || !strings.Contains(viewStr2, "all detected") {
		t.Errorf("expected confirmation view to explicitly state target agents, got:\n%s", viewStr2)
	}
}

func TestTUIRegistryPreviewScroll(t *testing.T) {
	m := newModel("")
	m.width = 120
	m.height = 30
	m.registryModal = true
	m.registryQuery = "xyz"
	m.registryResults = []registry.Skill{
		{DisplayName: "Long Preview", Slug: "long-preview", Source: "https://github.com/my-org/my-repo/skills/long-preview"},
	}
	m.registrySelected = 0
	m.registryFocusList = true
	key := "https://github.com/my-org/my-repo/skills/long-preview" + "\x00" + "long-preview"
	longPreview := []string{"# Long Preview"}
	for i := 1; i <= 40; i++ {
		longPreview = append(longPreview, fmt.Sprintf("- line %02d with enough words to stay visible as its own preview row", i))
	}
	m.registryPreviews[key] = strings.Join(longPreview, "\n")

	viewTop := m.View()
	if !strings.Contains(viewTop, "ctrl-u/d scroll") {
		t.Fatalf("expected scroll indicator for long preview, got:\n%s", viewTop)
	}
	if strings.Contains(viewTop, "line 40") {
		t.Fatalf("expected long preview not to show final line before scrolling")
	}

	modelTmp, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = modelTmp.(appModel)
	if m.registryPreviewOffset == 0 {
		t.Fatal("expected ctrl-d to scroll registry preview")
	}
	viewScrolled := m.View()
	if !strings.Contains(viewScrolled, "line 06") && !strings.Contains(viewScrolled, "line 07") {
		t.Fatalf("expected scrolled preview content, got:\n%s", viewScrolled)
	}
}

func TestTUIRegistryResultsScrollWithSelection(t *testing.T) {
	m := newModel("")
	m.width = 120
	m.height = 30
	m.registryModal = true
	m.registryQuery = "skill"
	m.registryFocusList = true
	m.registryPreviews = map[string]string{}
	for i := 1; i <= 30; i++ {
		m.registryResults = append(m.registryResults, registry.Skill{
			DisplayName: fmt.Sprintf("Skill Result %02d", i),
			Slug:        fmt.Sprintf("skill-result-%02d", i),
			Source:      "https://github.com/example/skills",
		})
	}
	m.registrySelected = 29

	view := m.View()
	if !strings.Contains(view, "> Skill Result 30") {
		t.Fatalf("expected selected result to stay visible near bottom of long list, got:\n%s", view)
	}
	if !strings.Contains(view, "↑ ") {
		t.Fatalf("expected registry result scroll indicator, got:\n%s", view)
	}
	if strings.Contains(view, "Skill Result 01") {
		t.Fatalf("expected top result to be scrolled out of the visible list, got:\n%s", view)
	}
}

func TestTUIRegistryModalDoesNotSoftWrapRows(t *testing.T) {
	m := newModel("")
	m.width = 110
	m.height = 32
	m.registryModal = true
	m.registryQuery = "cloudf"
	m.registryFocusList = true
	m.registryPreviews = map[string]string{}
	for i := 1; i <= 24; i++ {
		m.registryResults = append(m.registryResults, registry.Skill{
			DisplayName: fmt.Sprintf("cloudflare-email-service-with-a-very-long-display-name-%02d", i),
			Slug:        fmt.Sprintf("cloudflare-email-service-with-a-very-long-slug-%02d", i),
			Source:      "https://github.com/cloudflare/skills/tree/main/packages/skills/cloudflare-email-service-with-a-very-long-path",
			Installs:    13274,
		})
	}
	m.registrySelected = 18
	key := m.registryResults[m.registrySelected].Source + "\x00" + m.registryResults[m.registrySelected].Slug
	m.registryPreviews[key] = "# Cloudflare Skills\n" + strings.Repeat("https://example.com/this/is/a/very/long/url/that/must/not/soft/wrap/the/registry/modal\n", 8)

	view := m.View()
	for i, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > m.width {
			t.Fatalf("registry modal line %d exceeds terminal width: got %d want <= %d\n%s", i+1, got, m.width, view)
		}
	}
}

func TestTUIANSIClampKeepsStyledLinesValid(t *testing.T) {
	styled := selectedStyle.Render(strings.Repeat("cloudflare-email-service ", 12))
	clamped := clampLineWidth(styled, 32)
	if got := xansi.StringWidth(clamped); got > 32 {
		t.Fatalf("clamped styled line exceeds target width: got %d", got)
	}
	if strings.ContainsRune(clamped, '\uFFFD') {
		t.Fatalf("clamped styled line contains replacement characters: %q", clamped)
	}
	if strings.Contains(clamped, "\x1b[") && !strings.Contains(clamped, "\x1b[0m") {
		t.Fatalf("clamped styled line appears to have lost ANSI reset: %q", clamped)
	}
}

func TestTUIRegistryModeKeySeparation(t *testing.T) {
	m := newModel("")
	m.width = 100
	m.height = 30
	m.registryModal = true
	m.registryQuery = "ab"
	m.registryResults = []registry.Skill{
		{DisplayName: "One", Slug: "one", Source: "owner/one"},
		{DisplayName: "Two", Slug: "two", Source: "owner/two"},
		{DisplayName: "Three", Slug: "three", Source: "owner/three"},
	}
	m.registrySelected = 0

	// Test 1: Search focused (list unfocused)
	m.registryFocusList = false

	// Typing 'j' should append to query, not change selection
	modelTmp, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = modelTmp.(appModel)
	if m.registryQuery != "abj" {
		t.Errorf("expected 'j' to append to query, got query=%q", m.registryQuery)
	}
	if m.registrySelected != 0 {
		t.Errorf("expected selection to remain 0, got %d", m.registrySelected)
	}

	// Typing 'k' should append to query
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = modelTmp.(appModel)
	if m.registryQuery != "abjk" {
		t.Errorf("expected 'k' to append to query, got query=%q", m.registryQuery)
	}
	if m.registrySelected != 0 {
		t.Errorf("expected selection to remain 0, got %d", m.registrySelected)
	}

	// Test 2: List focused
	m.registryFocusList = true
	m.registryLoading = false

	// Pressing 'j' should navigate list down
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = modelTmp.(appModel)
	if m.registrySelected != 1 {
		t.Errorf("expected selection to move to 1, got %d", m.registrySelected)
	}
	if m.registryQuery != "abjk" {
		t.Errorf("expected query to remain unchanged, got query=%q", m.registryQuery)
	}

	// Pressing 'k' should navigate list up
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	m = modelTmp.(appModel)
	if m.registrySelected != 0 {
		t.Errorf("expected selection to move back to 0, got %d", m.registrySelected)
	}
	if m.registryQuery != "abjk" {
		t.Errorf("expected query to remain unchanged, got query=%q", m.registryQuery)
	}

	// Pressing any other printable key (e.g. 'x') when list is focused should be ignored
	modelTmp, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = modelTmp.(appModel)
	if m.registryQuery != "abjk" {
		t.Errorf("expected query to remain unchanged on list-focused alpha input, got query=%q", m.registryQuery)
	}
}

func TestTUIRegistryPreviewAndNoInstallCommands(t *testing.T) {
	m := newModel("")
	m.width = 120
	m.height = 30
	m.registryModal = true
	m.registryQuery = "xyz"
	m.registryResults = []registry.Skill{
		{DisplayName: "Test Skill", Slug: "test-skill", Source: "https://github.com/my-org/my-repo/skills/test-folder"},
	}
	m.registrySelected = 0

	key := "https://github.com/my-org/my-repo/skills/test-folder" + "\x00" + "test-skill"

	// 1. Before async preview fetch returns, show an explicit loading state.
	viewStr1 := m.View()
	if strings.Contains(viewStr1, "No preview available") {
		t.Error("must not show 'No preview available'")
	}
	if !strings.Contains(viewStr1, "Loading preview") {
		t.Error("expected loading preview indicator before async fetch result")
	}

	// 2. Install Commands section must not be present in the right pane
	if strings.Contains(viewStr1, "Install Commands:") || strings.Contains(viewStr1, "Install:") || strings.Contains(viewStr1, "Local:") {
		t.Error("Install section must be completely removed from the right pane")
	}

	// 3. Empty fetch result should fall back to useful metadata context.
	m.registryPreviews[key] = ""
	viewStr2 := m.View()
	if !strings.Contains(viewStr2, "A lazyskills skill named") {
		t.Error("expected derived preview context after empty fetch result")
	}

	// 4. Fetched preview is rendered as markdown-ish text, with HTML tags stripped.
	m.registryPreviews[key] = "# Heading\n<p>This is a fetched <strong>SKILL.md</strong> markdown preview!</p>"
	viewStr3 := m.View()
	if !strings.Contains(viewStr3, "This is a fetched") {
		t.Error("expected the fetched HTTP preview to be displayed")
	}
	if strings.Contains(viewStr3, "<p>") || strings.Contains(viewStr3, "<strong>") {
		t.Error("expected HTML tags to be stripped from fetched preview")
	}
}
