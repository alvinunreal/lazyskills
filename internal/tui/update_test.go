package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvinunreal/lazyskills/internal/selfupdate"
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
		Channel: "manual",
	}

	footer = m.footerText(120, m.visibleRows(), m.currentActions())
	if !strings.Contains(footer, "U update") || !strings.Contains(footer, "v1.1.0 available") {
		t.Errorf("expected update notice in footer, got: %q", footer)
	}

	// 3. Notice omitted if width is too narrow
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
