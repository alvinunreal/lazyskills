package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/model"
	"github.com/alvinunreal/lazyskills/internal/runner"
)

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

		m.height = msg.Height
		m.syncViewport()
	case snapshotMsg:
		m.result = msg.result
		sortSkills(m.result.Skills)
		m.err = msg.err
		if m.agent != "" {
			detected := false
			for _, filter := range m.agentFilters() {
				if filter == m.agent {
					detected = true
					break
				}
			}
			if !detected {
				m.agent = ""
			}
		}
		m.clampSelection()
		m.pruneSelected()
		m.actionResult = nil
		m.syncViewport()
	case discoveryResultMsg:
		disc := SourceDiscovery{
			Status:    DiscoveryReady,
			Skills:    msg.skills,
			ScannedAt: time.Now(),
		}
		if msg.err != nil {
			disc.Status = DiscoveryFailed
			disc.Error = msg.err.Error()
		}
		if m.discovery == nil {
			m.discovery = make(map[string]SourceDiscovery)
		}
		m.discovery[msg.groupName] = disc
		m.clampSelection()
		m.syncViewport()
	case actionResultMsg:
		m.running = false
		m.runningTitle = ""
		m.confirming = false
		m.confirmInput = ""
		m.modalSource = ""
		m.actionResult = &msg.result
		succeeded := msg.result.ExitCode == 0 && msg.result.Err == ""
		if msg.mutates && succeeded {
			m.selectedKeys = nil
		}
		m.syncViewport()
		if msg.mutates && (succeeded || msg.partialSuccess) {
			return m, loadSnapshot(m.cwd)
		}
	case tea.KeyMsg:
		key := msg.String()
		if m.running {
			if key == "ctrl+c" || key == "q" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.detailModal {
			switch key {
			case "esc", "q":
				m.detailModal = false
				m.modalSource = ""
				m.syncViewport()
			case "ctrl+c":
				return m, tea.Quit
			case "o":
				if m.modalSource != "" {
					child, ok := m.currentModalSelectedChild()
					if ok && !child.isAvailable {
						m.detailModal = false
						m.modalSource = ""
						return m.startSkillActionByID(child.skill, "open_skill")
					}
				} else {
					m.detailModal = false
					return m.startCurrentSkillActionByID("open_skill")
				}
			case "u":
				if m.modalSource != "" {
					child, ok := m.currentModalSelectedChild()
					if ok && !child.isAvailable {
						m.detailModal = false
						m.modalSource = ""
						return m.startSkillActionByID(child.skill, "reinstall_update")
					}
				}
			case "x":
				if m.modalSource != "" {
					child, ok := m.currentModalSelectedChild()
					if ok && !child.isAvailable {
						m.detailModal = false
						m.modalSource = ""
						return m.startSkillActionByID(child.skill, "remove")
					}
				}
			case "c", "enter":
				// Act on the selected child: open the action picker (install for
				// an available skill, open/update/remove for an installed one).
				m.detailModal = false
				m.commands = true
				m.action = 0
				m.syncViewport()
			case "d":
				if m.modalSource != "" {
					modelTmp, cmd := m.startDiscovery(m.modalSource)
					m = modelTmp.(appModel)
					m.syncViewport()
					return m, cmd
				}
			case "down", "j":
				if m.modalSource != "" {
					childRows := m.modalChildRows(m.modalSource)
					if len(childRows) > 0 {
						m.modalSelected++
						if m.modalSelected >= len(childRows) {
							m.modalSelected = len(childRows) - 1
						}
					}
					m.ensureSourceModalSelectionVisible()
					m.syncViewport()
				} else {
					m.viewport.LineDown(1)
					m.clampViewportOffset()
				}
			case "up", "k":
				if m.modalSource != "" {
					childRows := m.modalChildRows(m.modalSource)
					if len(childRows) > 0 {
						m.modalSelected--
						if m.modalSelected < 0 {
							m.modalSelected = 0
						}
					}
					m.ensureSourceModalSelectionVisible()
					m.syncViewport()
				} else {
					m.viewport.LineUp(1)
					m.clampViewportOffset()
				}
			case "pgdown", "ctrl+d":
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			case "pgup", "ctrl+u":
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			case "home":
				if m.modalSource != "" {
					m.modalSelected = 0
					m.viewport.GotoTop()
					m.syncViewport()
				} else {
					m.viewport.GotoTop()
				}
			}
			return m, nil
		}
		if m.helpOpen {
			switch key {
			case "esc", "q", "?":
				m.helpOpen = false
			case "ctrl+c":
				return m, tea.Quit
			}
			return m, nil
		}
		if m.confirming {
			switch key {
			case "esc":
				m.confirming = false
				m.confirmInput = ""
				m.confirmError = ""
			case "n":
				if m.confirmInput == "" {
					m.confirming = false
					m.confirmInput = ""
					m.confirmError = ""
				}
			case "pgdown", "ctrl+d", "pgup", "ctrl+u":
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			case "enter":
				return m.confirmAction()
			case "backspace", "ctrl+h":
				if len(m.confirmInput) > 0 {
					m.confirmInput = m.confirmInput[:len(m.confirmInput)-1]
					m.confirmError = ""
				}
			default:
				if len(key) == 1 {
					m.confirmInput += key
					m.confirmError = ""
				}
			}
			m.syncViewport()
			return m, nil
		}
		if m.searching {
			switch key {
			case "esc":
				m.search = ""
				m.selected = 0
				m.searching = false
			case "enter":
				m.searching = false
			case "backspace", "ctrl+h":
				if len(m.search) > 0 {
					m.search = m.search[:len(m.search)-1]
					m.selected = 0
				}
			default:
				if len(key) == 1 {
					m.search += key
					m.selected = 0
				}
			}
			m.clampSelection()
			m.syncViewport()
			return m, nil
		}
		if !m.searching && (key == "backspace" || key == "ctrl+h") && len(m.search) > 0 {
			m.search = m.search[:len(m.search)-1]
			m.selected = 0
			m.clampSelection()
			m.actionResult = nil
			m.syncViewport()
			return m, nil
		}

		if m.commands {
			switch key {
			case "esc", "c":
				m.commands = false
				m.modalSource = ""
			case "q", "ctrl+c":
				return m, tea.Quit
			case "up", "k":
				m.action--
				m.clampAction()
			case "down", "j":
				m.action++
				m.clampAction()
			case "enter":
				return m.startAction()
			case "o":
				return m.startActionByID("open_skill")
			case "u":
				actionID := preferredUpdateActionID(m.selectedCount())
				if m.modalSource != "" {
					actionID = "reinstall_update"
				}
				return m.startActionByID(actionID)
			case "x":
				actionID := preferredRemoveActionID(m.selectedCount())
				if m.modalSource != "" {
					actionID = "remove"
				}
				return m.startActionByID(actionID)
			}
			m.syncViewport()
			return m, nil
		}

		// "gg" jumps to top: a lone g arms the flag, any other key disarms it.
		gPending := m.pendingG
		m.pendingG = false

		switch key {
		case "esc":
			if m.selectedCount() > 0 {
				m.selectedKeys = nil
				m.action = 0
				m.actionResult = nil
			} else if m.agent != "" {
				selectedKey := m.currentSelectedKey()
				previousSelected := m.selected
				m.agent = ""
				m.restoreSelection(selectedKey, previousSelected)
				m.action = 0
				m.actionResult = nil
				m.viewport.GotoTop()
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.helpOpen = true
		case "c":
			m.commands = !m.commands
			m.action = 0
		case " ":
			m.toggleSelectedSkill()
			m.action = 0
			m.actionResult = nil
		case "s":
			m.selectCurrentSourceGroup()
			m.action = 0
			m.actionResult = nil
		case "enter":
			rows := m.visibleRows()
			if len(rows) > 0 && m.selected < len(rows) {
				row := rows[m.selected]
				if row.isHeader {
					m.detailModal = true
					m.modalSource = row.groupName
					m.modalSelected = 0
					m.detailsFocused = true
					m.viewport.GotoTop()
					m.syncViewport()

					groupName := row.groupName
					disc, exists := m.discovery[groupName]
					if !exists || (disc.Status != DiscoveryLoading && disc.Status != DiscoveryReady && disc.Status != DiscoveryFailed) {
						var cmd tea.Cmd
						var modelTmp tea.Model
						modelTmp, cmd = m.startDiscovery(groupName)
						m = modelTmp.(appModel)
						m.viewport.GotoTop()
						m.syncViewport()
						return m, cmd
					}
				} else {
					m.detailModal = true
					m.modalSource = ""
					m.modalSelected = 0
					m.detailsFocused = true
					m.viewport.GotoTop()
					m.syncViewport()
				}
			}
		case "o":
			rows := m.visibleRows()
			if len(rows) > 0 && m.selected < len(rows) && !rows[m.selected].isHeader {
				return m.startCurrentSkillActionByID("open_skill")
			}
		case "u":
			rows := m.visibleRows()
			if len(rows) > 0 && m.selected < len(rows) && !rows[m.selected].isHeader {
				return m.startActionByID(preferredUpdateActionID(m.selectedCount()))
			}
		case "x":
			rows := m.visibleRows()
			if len(rows) > 0 && m.selected < len(rows) && !rows[m.selected].isHeader {
				return m.startActionByID(preferredRemoveActionID(m.selectedCount()))
			}
		case "d":
			if m.focus == focusSkills {
				rows := m.visibleRows()
				if len(rows) > 0 && m.selected >= 0 && m.selected < len(rows) {
					row := rows[m.selected]
					if row.isHeader {
						return m.startDiscovery(row.groupName)
					}
				}
			}
		case "/":
			m.searching = true
		case "r":
			m.viewport.GotoTop()
			return m, loadSnapshot(m.cwd)
		case "a":
			selectedKey := m.currentSelectedKey()
			previousSelected := m.selected
			m.agent = m.nextAgentFilter()
			m.restoreSelection(selectedKey, previousSelected)
			m.action = 0
			m.actionResult = nil
			m.viewport.GotoTop()
		case "A":
			selectedKey := m.currentSelectedKey()
			previousSelected := m.selected
			m.agent = ""
			m.restoreSelection(selectedKey, previousSelected)
			m.action = 0
			m.actionResult = nil
			m.viewport.GotoTop()
		case "tab", "shift+tab":
			if key == "tab" {
				m.focus = (m.focus + 1) % 3
			} else {
				m.focus = (m.focus + 2) % 3
			}
			m.detailsFocused = (m.focus != focusSkills)
		case "1":
			m.focus = focusSkills
			m.detailsFocused = false
		case "2":
			m.focus = focusMetadata
			m.detailsFocused = true
		case "3":
			m.focus = focusPreview
			m.detailsFocused = true
		case "f":
			m.filter = (m.filter + 1) % 3
			m.selected = 0
			m.actionResult = nil
			m.viewport.GotoTop()
		case "F":
			m.filter = scopeAll
			m.selected = 0
			m.actionResult = nil
			m.viewport.GotoTop()
		case "[":
			m.jumpSourceGroup(-1)
		case "]":
			m.jumpSourceGroup(1)
		case "l", "+":
			if m.focus == focusSkills {
				rows := m.visibleRows()
				if len(rows) > 0 && m.selected < len(rows) {
					row := rows[m.selected]
					m.expandGroup(row.groupName)
				}
			}
		case "h", "-":
			if m.focus == focusSkills {
				rows := m.visibleRows()
				if len(rows) > 0 && m.selected < len(rows) {
					row := rows[m.selected]
					m.collapseGroup(row.groupName)
				}
			}
		case "right":
			if m.focus == focusSkills {
				m.jumpSourceGroup(1)
			} else {
				m.focus = (m.focus + 1) % 3
				m.detailsFocused = (m.focus != focusSkills)
			}
		case "left":
			if m.focus == focusSkills {
				m.jumpSourceGroup(-1)
			} else {
				m.focus = (m.focus + 2) % 3
				m.detailsFocused = (m.focus != focusSkills)
			}
		case "down", "j":
			if m.focus == focusMetadata {
				m.metadataViewport.LineDown(1)
				m.clampViewportOffset()
			} else if m.focus == focusPreview {
				m.previewViewport.LineDown(1)
				m.clampViewportOffset()
			} else {
				rows := m.visibleRows()
				if m.selected < len(rows)-1 {
					m.selected++
					m.actionResult = nil
					m.metadataViewport.GotoTop()
					m.previewViewport.GotoTop()
				}
			}
		case "up", "k":
			if m.focus == focusMetadata {
				m.metadataViewport.LineUp(1)
				m.clampViewportOffset()
			} else if m.focus == focusPreview {
				m.previewViewport.LineUp(1)
				m.clampViewportOffset()
			} else {
				if m.selected > 0 {
					m.selected--
					m.actionResult = nil
					m.metadataViewport.GotoTop()
					m.previewViewport.GotoTop()
				}
			}
		case "pgdown", "ctrl+d":
			var cmd tea.Cmd
			if m.focus == focusMetadata {
				m.metadataViewport, cmd = m.metadataViewport.Update(msg)
			} else if m.focus == focusPreview {
				m.previewViewport, cmd = m.previewViewport.Update(msg)
			} else {
				m.previewViewport, cmd = m.previewViewport.Update(msg)
			}
			return m, cmd
		case "pgup", "ctrl+u":
			var cmd tea.Cmd
			if m.focus == focusMetadata {
				m.metadataViewport, cmd = m.metadataViewport.Update(msg)
			} else if m.focus == focusPreview {
				m.previewViewport, cmd = m.previewViewport.Update(msg)
			} else {
				m.previewViewport, cmd = m.previewViewport.Update(msg)
			}
			return m, cmd
		case "home":
			m.jumpListTop()
		case "end":
			m.jumpListBottom()
		case "g":
			if gPending {
				m.jumpListTop()
			} else {
				m.pendingG = true
			}
		case "G":
			m.jumpListBottom()
		}
		m.clampSelection()
		m.clampAction()
		m.syncViewport()
	}
	return m, nil
}

func (m *appModel) clampAction() {
	actions := m.currentActions()
	if len(actions) == 0 {
		m.action = 0
		return
	}
	if m.action < 0 {
		m.action = 0
	}
	if m.action >= len(actions) {
		m.action = len(actions) - 1
	}
}

func (m appModel) currentActions() []actions.CommandPreview {
	if m.modalSource != "" {
		child, ok := m.currentModalSelectedChild()
		if ok {
			if child.isAvailable {
				return actions.ForAvailableSkill(m.modalSource, child.discoveredSkill.Name)
			}
			return actions.ForSkill(child.skill)
		}
	}
	selected := m.selectedSkills()
	if len(selected) > 0 {
		return actions.ForSkills(selected)
	}
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		return actions.AppLevelActions()
	}
	row := rows[m.selected]
	if row.isHeader {
		return m.sourceActions(row.groupName)
	}
	return actions.ForSkill(row.skill)
}

func (m appModel) startAction() (tea.Model, tea.Cmd) {
	actions := m.currentActions()
	if len(actions) == 0 || m.action >= len(actions) {
		return m, nil
	}
	action := actions[m.action]
	if !action.Available {
		return m, nil
	}
	if action.RequiresConfirm {
		m.confirming = true
		m.confirmInput = ""
		m.confirmError = ""
		m.actionResult = nil
		m.syncViewport()
		return m, nil
	}
	return m.executeAction(action)
}

func (m appModel) startActionByID(id string) (tea.Model, tea.Cmd) {
	if id == "" {
		return m, nil
	}
	for i, action := range m.currentActions() {
		if action.ID == id {
			m.action = i
			m.commands = false
			return m.startAction()
		}
	}
	return m, nil
}

func (m appModel) startSkillActionByID(sk *model.Skill, id string) (tea.Model, tea.Cmd) {
	for _, action := range actions.ForSkill(sk) {
		if action.ID == id {
			if !action.Available {
				return m, nil
			}
			m.commands = false
			return m.executeAction(action)
		}
	}
	return m, nil
}

func (m appModel) startCurrentSkillActionByID(id string) (tea.Model, tea.Cmd) {
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		return m, nil
	}
	row := rows[m.selected]
	if row.isHeader {
		return m, nil
	}
	return m.startSkillActionByID(row.skill, id)
}

type modalChildRow struct {
	isAvailable     bool
	skill           *model.Skill
	discoveredSkill *DiscoveredSkill
}

func (m appModel) modalChildRows(groupName string) []modalChildRow {
	var rows []modalChildRow
	// 1. Installed skills
	for _, sk := range m.sourceGroupSkills(groupName) {
		rows = append(rows, modalChildRow{
			isAvailable: false,
			skill:       sk,
		})
	}
	// 2. Available skills
	disc, ok := m.discovery[groupName]
	if ok && disc.Status == DiscoveryReady {
		for i, ds := range disc.Skills {
			if !m.isSkillInstalled(ds.Name, groupName) {
				rows = append(rows, modalChildRow{
					isAvailable:     true,
					discoveredSkill: &disc.Skills[i],
				})
			}
		}
	}
	return rows
}

func (m appModel) currentModalSelectedChild() (modalChildRow, bool) {
	if m.modalSource == "" {
		return modalChildRow{}, false
	}
	childRows := m.modalChildRows(m.modalSource)
	if len(childRows) == 0 || m.modalSelected < 0 || m.modalSelected >= len(childRows) {
		return modalChildRow{}, false
	}
	return childRows[m.modalSelected], true
}

func preferredUpdateActionID(selectedCount int) string {
	if selectedCount > 0 {
		return "bulk_reinstall_update"
	}
	return "reinstall_update"
}

func preferredRemoveActionID(selectedCount int) string {
	if selectedCount > 0 {
		return "bulk_remove"
	}
	return "remove"
}

func (m appModel) confirmAction() (tea.Model, tea.Cmd) {
	actions := m.currentActions()
	if len(actions) == 0 || m.action >= len(actions) {
		return m, nil
	}
	action := actions[m.action]
	if !confirmationAccepted(m.confirmInput, action.ConfirmValue) {
		m.confirmError = "Type yes, y, or the displayed phrase. Press Esc to cancel."
		m.confirmInput = ""
		m.syncViewport()
		return m, nil
	}
	return m.executeAction(action)
}

func confirmationAccepted(input, confirmValue string) bool {
	value := strings.TrimSpace(strings.ToLower(input))
	if value == "" {
		return false
	}
	return value == "y" || value == "yes" || input == confirmValue
}

func (m appModel) executeAction(action actions.CommandPreview) (tea.Model, tea.Cmd) {
	if action.ID == "source_discover" {
		m.commands = false
		m.actionResult = nil
		return m.startDiscovery(action.ConfirmValue)
	}
	if action.Exec.Internal == "refresh" {
		m.actionResult = nil
		return m, loadSnapshot(m.cwd)
	}
	if action.Exec.Interactive {
		cmd := exec.Command(action.Exec.Program, action.Exec.Args...)
		cmd.Dir = m.cwd
		m.running = true
		m.runningTitle = action.Title
		m.actionResult = nil
		m.confirming = false
		m.confirmInput = ""
		m.confirmError = ""
		m.syncViewport()
		return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
			result := runner.Result{Program: action.Exec.Program, Args: action.Exec.Args, Cwd: m.cwd, ExitCode: 0}
			if err != nil {
				result.ExitCode = -1
				result.Err = err.Error()
			}
			return actionResultMsg{result: result, mutates: action.Mutates}
		})
	}
	if len(action.Exec.Batch) > 0 {
		m.running = true
		m.runningTitle = action.Title
		m.actionResult = nil
		m.confirming = false
		m.confirmInput = ""
		m.confirmError = ""
		m.syncViewport()
		return m, func() tea.Msg {
			result, partialSuccess := m.runBatch(action.Exec.Batch)
			return actionResultMsg{result: result, mutates: action.Mutates, partialSuccess: partialSuccess}
		}
	}
	spec := runner.ExecSpec{Program: action.Exec.Program, Args: action.Exec.Args, Cwd: m.cwd}
	m.running = true
	m.runningTitle = action.Title
	m.actionResult = nil
	m.confirming = false
	m.confirmInput = ""
	m.confirmError = ""
	m.syncViewport()
	return m, func() tea.Msg {
		result := runExec(spec)
		return actionResultMsg{result: result, mutates: action.Mutates}
	}
}

func (m appModel) runBatch(batch []actions.ExecSpec) (runner.Result, bool) {
	lines := []string{}
	succeeded := 0
	for i, spec := range batch {
		runSpec := runner.ExecSpec{Program: spec.Program, Args: spec.Args, Cwd: m.cwd}
		result := runExec(runSpec)
		prefix := fmt.Sprintf("%d/%d %s", i+1, len(batch), compat.SanitizeMetadata(spec.Program))
		if result.ExitCode != 0 || result.Err != "" {
			result.Stdout = strings.Join(append(lines, prefix+" failed"), "\n")
			return result, succeeded > 0
		}
		succeeded++
		lines = append(lines, prefix+" ok")
	}
	return runner.Result{Program: "bulk", Cwd: m.cwd, ExitCode: 0, Stdout: strings.Join(lines, "\n")}, false
}
