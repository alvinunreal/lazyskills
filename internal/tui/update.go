package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/locks"
	"github.com/alvinunreal/lazyskills/internal/model"
	"github.com/alvinunreal/lazyskills/internal/runner"
	"github.com/alvinunreal/lazyskills/internal/selfupdate"
)

const previewRefreshDelay = 300 * time.Millisecond

type previewRefreshMsg struct {
	generation int
}

// previewRenderedMsg carries the result of an async glamour markdown render.
type previewRenderedMsg struct {
	markdown   string
	width      int
	lines      []string
	generation int
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	updateStart := time.Now()
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		key := keyMsg.String()
		selectedBefore := m.selected
		defer func() {
			perfLogf("update key=%q selected_before=%d selected_after=%d focus=%d modal=%t source=%q preview_pending=%t generation=%d duration=%s", key, selectedBefore, m.selected, m.focus, m.detailModal, m.modalSource, m.previewPending, m.previewGeneration, time.Since(updateStart))
		}()
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width

		m.height = msg.Height
		m.syncViewport()
		if cmd := m.dispatchPreviewRender(); cmd != nil {
			m.markPreviewRendering()
			return m, cmd
		}
	case snapshotMsg:
		m.result = msg.result
		m.previewCache = make(map[previewCacheKey][]string)
		m.previewPending = true // prevent syncViewport from blocking on glamour
		m.previewRendering = false
		m.previewRenderingGeneration = 0
		m.previewGeneration++
		sortSkills(m.result.Skills)
		m.rebuildSkillSearchText()
		m.rebuildSkillViews()
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
		m.actionResult = msg.actionResult
		// Seed with an empty (non-nil) slice so View's footer path doesn't
		// fall back to currentActions (which calls exec.LookPath ≈4s on WSL2).
		// clampAction in a later Update will populate the real actions.
		m.cachedActions = []actions.CommandPreview{}
		m.syncViewport()
		// Immediately start the initial preview render off the main thread
		// so the 2-3s glamour/chroma cost doesn't block the first frame.
		if cmd := m.dispatchPreviewRender(); cmd != nil {
			m.markPreviewRendering()
			return m, cmd
		}
		m.previewPending = false
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
	case previewRefreshMsg:
		refreshStart := time.Now()
		if msg.generation == m.previewGeneration {
			m.previewPending = false
			if cmd := m.dispatchPreviewRender(); cmd != nil {
				m.markPreviewRendering()
				return m, cmd
			}
			m.syncViewport()
		}
		perfLogf("preview_refresh msg_generation=%d current_generation=%d applied=%t duration=%s", msg.generation, m.previewGeneration, msg.generation == m.previewGeneration, time.Since(refreshStart))
	case previewRenderedMsg:
		if m.previewCache == nil {
			m.previewCache = make(map[previewCacheKey][]string)
		}
		key := previewCacheKey{markdown: msg.markdown, width: msg.width}
		m.previewCache[key] = append([]string(nil), msg.lines...)
		if msg.generation == m.previewRenderingGeneration {
			m.previewRendering = false
			m.previewRenderingGeneration = 0
			// Re-dispatch if the current skill's preview width differs from the
			// rendered width (e.g. the terminal was resized between dispatch and
			// completion). This ensures we never block View on a cache miss.
			if cmd := m.dispatchPreviewRender(); cmd != nil {
				m.markPreviewRendering()
				return m, cmd
			}
			m.syncViewport()
		}
	case updatePlanMsg:
		m.updatePlan = msg.plan
		m.updatePlanErr = msg.err
		m.syncViewport()
	case appUpdateResultMsg:
		m.updatingApp = false
		if msg.err != nil {
			m.updateError = msg.err
			m.updateSuccess = false
		} else {
			m.updateSuccess = true
			m.updateError = nil
		}
		m.syncViewport()
	case tea.KeyMsg:
		key := msg.String()
		var postKeyCmd tea.Cmd
		if m.running {
			if key == "ctrl+c" || key == "q" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.appUpdateModal {
			switch key {
			case "esc", "q":
				m.appUpdateModal = false
				m.updateError = nil
				m.updateSuccess = false
				m.syncViewport()
			case "ctrl+c":
				return m, tea.Quit
			case "enter":
				if m.updatePlan != nil && m.updatePlan.CanExecute && !m.updatingApp && !m.updateSuccess {
					m.updatingApp = true
					m.updateError = nil
					m.updateSuccess = false
					m.syncViewport()
					return m, m.applyUpdateCmd()
				}
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
			case "c":
				// Open the full action picker for the selected child.
				m.detailModal = false
				m.commands = true
				m.action = 0
				m.syncViewport()
			case "enter":
				// Primary action on the selected child: install an available
				// skill (with confirm), or open an installed one.
				if m.modalSource != "" {
					if child, ok := m.currentModalSelectedChild(); ok {
						if child.isAvailable {
							for _, a := range actions.ForAvailableSkill(m.modalSource, child.discoveredSkill.Name) {
								if a.ID != "install_skill" || !a.Available {
									continue
								}
								m.detailModal = false
								m.modalSource = ""
								if a.RequiresConfirm {
									armed := a
									m.pendingAction = &armed
									m.confirming = true
									m.confirmInput = ""
									m.confirmError = ""
									m.syncViewport()
									return m, nil
								}
								return m.executeAction(a)
							}
						} else {
							m.detailModal = false
							m.modalSource = ""
							return m.startSkillActionByID(child.skill, "open_skill")
						}
					}
				}
			case "d":
				if m.modalSource != "" {
					modelTmp, cmd := m.startDiscovery(m.modalSource, true)
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
				m.pendingAction = nil
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
			case "e":
				return m.startToggleAction()
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
		if count := repeatedRuneKeyCount(key, 'j'); count > 1 {
			m, postKeyCmd = m.moveSelectionBy(count)
			m.clampSelection()
			m.clampAction()
			m.syncViewport()
			if postKeyCmd != nil {
				return m, postKeyCmd
			}
			return m, nil
		}
		if count := repeatedRuneKeyCount(key, 'k'); count > 1 {
			m, postKeyCmd = m.moveSelectionBy(-count)
			m.clampSelection()
			m.clampAction()
			m.syncViewport()
			if postKeyCmd != nil {
				return m, postKeyCmd
			}
			return m, nil
		}

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
		case "U":
			m.appUpdateModal = true
			m.updatingApp = false
			m.updateSuccess = false
			m.updateError = nil
			m.syncViewport()
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
						modelTmp, cmd = m.startDiscovery(groupName, false)
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
		case "e":
			rows := m.visibleRows()
			if len(rows) > 0 && m.selected < len(rows) {
				return m.startToggleAction()
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
						return m.startDiscovery(row.groupName, true)
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
				m, postKeyCmd = m.moveSelectionBy(1)
			}
		case "up", "k":
			if m.focus == focusMetadata {
				m.metadataViewport.LineUp(1)
				m.clampViewportOffset()
			} else if m.focus == focusPreview {
				m.previewViewport.LineUp(1)
				m.clampViewportOffset()
			} else {
				m, postKeyCmd = m.moveSelectionBy(-1)
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
		if postKeyCmd != nil {
			return m, postKeyCmd
		}
		if cmd := m.dispatchPreviewRender(); cmd != nil {
			m.markPreviewRendering()
			return m, cmd
		}
	}
	return m, nil
}

func repeatedRuneKeyCount(key string, r rune) int {
	if key == "" {
		return 0
	}
	count := 0
	for _, ch := range key {
		if ch != r {
			return 0
		}
		count++
	}
	return count
}

func (m appModel) moveSelectionBy(delta int) (appModel, tea.Cmd) {
	if delta == 0 {
		return m, nil
	}
	rows := m.visibleRows()
	if len(rows) == 0 {
		return m, nil
	}
	previous := m.selected
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(rows) {
		m.selected = len(rows) - 1
	}
	if m.selected == previous {
		return m, nil
	}
	m.actionResult = nil
	m.metadataViewport.GotoTop()
	m.previewViewport.GotoTop()
	m.previewGeneration++
	m.previewPending = true
	return m, schedulePreviewRefresh(m.previewGeneration)
}

func schedulePreviewRefresh(generation int) tea.Cmd {
	return tea.Tick(previewRefreshDelay, func(time.Time) tea.Msg {
		return previewRefreshMsg{generation: generation}
	})
}

func (m *appModel) markPreviewRendering() {
	m.previewRendering = true
	m.previewRenderingGeneration = m.previewGeneration
}

// dispatchPreviewRender checks whether the currently selected skill needs an
// async glamour markdown render. Returns nil when the preview is already cached
// or the skill has no preview content.
func (m appModel) dispatchPreviewRender() tea.Cmd {
	if m.previewRendering {
		return nil
	}
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		return nil
	}
	row := rows[m.selected]
	if row.isHeader || row.skill == nil {
		return nil
	}
	view := m.cachedSkillView(row.skill)
	if view.Preview == "" {
		return nil
	}
	if m.previewCache == nil {
		return nil // cache not initialized (bootstrapping)
	}
	_, rightWidth, _, _ := m.getThreePaneLayout()
	previewWidth := max(1, rightWidth-4)
	key := previewCacheKey{markdown: view.Preview, width: previewWidth}
	if _, ok := m.previewCache[key]; ok {
		return nil // already cached
	}
	// Capture by value — do NOT capture m (model is value-copied each Update).
	markdown := view.Preview
	width := previewWidth
	gen := m.previewGeneration
	return func() tea.Msg {
		lines := renderMarkdownPreview(markdown, width)
		return previewRenderedMsg{markdown: markdown, width: width, lines: lines, generation: gen}
	}
}

func (m *appModel) clampAction() {
	actions := m.currentActions()
	m.cachedActions = actions
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
	return m.currentActionsForRows(nil)
}

// currentActionsForRows is like currentActions but accepts a precomputed rows
// slice so callers can avoid a redundant visibleRows() re-derivation (which
// walks SanitizeMetadata per skill). Pass nil to fall back to m.visibleRows().
func (m appModel) currentActionsForRows(rows []skillsRow) []actions.CommandPreview {
	if m.modalSource != "" {
		child, ok := m.currentModalSelectedChild()
		if ok {
			if child.isAvailable {
				return actions.ForAvailableSkill(m.modalSource, child.discoveredSkill.Name)
			}
			return m.appendEnableDisableActions(actions.ForSkill(child.skill), child.skill)
		}
	}
	selected := m.selectedSkills()
	if len(selected) > 0 {
		return actions.ForSkills(selected)
	}
	if rows == nil {
		rows = m.visibleRows()
	}
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		return actions.AppLevelActions()
	}
	row := rows[m.selected]
	if row.isHeader {
		return m.sourceActions(row.groupName)
	}
	return m.appendEnableDisableActions(actions.ForSkill(row.skill), row.skill)
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

func (m appModel) startToggleAction() (tea.Model, tea.Cmd) {
	for i, action := range m.currentActions() {
		if (action.ID == "enable_skill" || action.ID == "disable_skill") && action.Available {
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
		installed := m.installedSkillNames(groupName)
		for i, ds := range disc.Skills {
			if !isSkillNameInstalled(ds.Name, installed) {
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
	var action actions.CommandPreview
	if m.pendingAction != nil {
		action = *m.pendingAction
	} else {
		acts := m.currentActions()
		if len(acts) == 0 || m.action >= len(acts) {
			return m, nil
		}
		action = acts[m.action]
	}
	if !confirmationAccepted(m.confirmInput, action) {
		m.confirmError = confirmationError(action)
		m.confirmInput = ""
		m.syncViewport()
		return m, nil
	}
	m.pendingAction = nil
	return m.executeAction(action)
}

func confirmationAccepted(input string, action actions.CommandPreview) bool {
	if requiresExactConfirmation(action) {
		return input == action.ConfirmValue && action.ConfirmValue != ""
	}
	value := strings.TrimSpace(strings.ToLower(input))
	if value == "" {
		return !action.Dangerous
	}
	return value == "y" || value == "yes" || input == action.ConfirmValue
}

func confirmationError(action actions.CommandPreview) string {
	if requiresExactConfirmation(action) {
		return fmt.Sprintf("Type %q exactly, or press Esc to cancel.", action.ConfirmValue)
	}
	if action.Dangerous {
		return "Type y or yes to confirm, or press Esc to cancel."
	}
	return "Press Enter to continue, or Esc to cancel."
}

func requiresExactConfirmation(action actions.CommandPreview) bool {
	return action.Dangerous && action.ID == "bulk_remove"
}

func (m appModel) executeAction(action actions.CommandPreview) (tea.Model, tea.Cmd) {
	if action.ID == "source_discover" {
		m.commands = false
		m.actionResult = nil
		return m.startDiscovery(action.ConfirmValue, true)
	}
	if action.Exec.Internal == "enable_skill" || action.Exec.Internal == "disable_skill" {
		m.commands = false
		m.confirming = false
		m.confirmInput = ""
		m.confirmError = ""
		m.actionResult = nil

		type moveItem struct {
			src, dest string
		}
		var plan []moveItem
		seenSrc := map[string]bool{}

		if action.Exec.Internal == "disable_skill" {
			for _, path := range action.Exec.Args {
				src := path
				dest := filepath.Join(filepath.Dir(src), ".lazyskills-disabled", filepath.Base(src))
				if !seenSrc[src] {
					seenSrc[src] = true
					plan = append(plan, moveItem{src: src, dest: dest})
				}
			}
		} else { // enable_skill
			for i := 0; i < len(action.Exec.Args); i += 2 {
				if i+1 >= len(action.Exec.Args) {
					break
				}
				src := action.Exec.Args[i]
				dest := action.Exec.Args[i+1]
				if !seenSrc[src] {
					seenSrc[src] = true
					plan = append(plan, moveItem{src: src, dest: dest})
				}
			}
		}

		// Preflight validation
		var errs []string
		for _, item := range plan {
			// Check if source exists
			_, err := os.Lstat(item.src)
			if err != nil {
				errs = append(errs, fmt.Sprintf("source path does not exist: %s", item.src))
				continue
			}

			// Destination must not exist
			if _, err := os.Lstat(item.dest); err == nil || !os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("destination already exists: %s", item.dest))
				continue
			}

			// Parent directory check
			parent := filepath.Dir(item.dest)
			parentInfo, err := os.Stat(parent)
			if err == nil {
				if !parentInfo.IsDir() {
					errs = append(errs, fmt.Sprintf("parent of destination is not a directory: %s", parent))
				}
			} else if !os.IsNotExist(err) {
				errs = append(errs, fmt.Sprintf("failed to check parent directory %s: %v", parent, err))
			}
		}

		succeededMoves := 0
		if len(errs) == 0 {
			// Execution
			for _, item := range plan {
				parent := filepath.Dir(item.dest)
				if err := os.MkdirAll(parent, 0755); err != nil {
					errs = append(errs, fmt.Sprintf("failed to create directory %s: %v", parent, err))
					break
				}
				if err := os.Rename(item.src, item.dest); err != nil {
					errs = append(errs, fmt.Sprintf("failed to move %s to %s: %v", item.src, item.dest, err))
					break
				}
				succeededMoves++
			}
		}

		if len(errs) > 0 {
			m.actionResult = &runner.Result{
				Program:  action.Exec.Internal,
				Args:     action.Exec.Args,
				ExitCode: -1,
				Err:      compat.SanitizeMetadata(strings.Join(errs, "; ")),
			}
			m.syncViewport()
			if succeededMoves > 0 {
				return m, loadSnapshot(m.cwd)
			}
			return m, nil
		}

		// Success: rescan
		return m, loadSnapshot(m.cwd)
	}
	if action.Exec.Internal == "prune_project_lock" || action.Exec.Internal == "prune_global_lock" {
		m.commands = false
		m.confirming = false
		m.confirmInput = ""
		m.confirmError = ""
		path := locks.ProjectLockPath(m.cwd)
		if action.Exec.Internal == "prune_global_lock" {
			path = locks.GlobalLockPath()
		}
		if err := locks.RemoveEntry(path, action.ConfirmValue); err != nil {
			m.actionResult = &runner.Result{
				Program:  "prune-lock",
				Args:     []string{action.ConfirmValue},
				ExitCode: -1,
				Err:      compat.SanitizeMetadata(err.Error()),
			}
			m.syncViewport()
			return m, nil
		}
		// Success: rescan drops the now-pruned phantom from the list.
		m.actionResult = nil
		return m, loadSnapshot(m.cwd)
	}
	if action.Exec.Internal == "delete_broken_symlink" {
		m.commands = false
		m.confirming = false
		m.confirmInput = ""
		m.confirmError = ""
		if len(action.Exec.Args) < 2 {
			m.actionResult = &runner.Result{Program: "delete-broken-symlink", Args: []string{action.ConfirmValue}, ExitCode: -1, Err: "delete action is missing scoped skill identity"}
			m.syncViewport()
			return m, nil
		}
		targetScope := action.Exec.Args[0]
		targetName := action.Exec.Args[1]
		removed, failed := 0, 0
		firstErr := ""
		for _, sk := range m.result.Skills {
			if sk.Name != targetName || (targetScope != "" && string(sk.Scope) != targetScope) {
				continue
			}
			for _, op := range sk.ObservedPaths {
				if op.Status != model.StatusBrokenSymlink {
					continue // safety: never touch working symlinks or canonical files
				}
				info, err := os.Lstat(op.Path)
				if err != nil {
					if os.IsNotExist(err) {
						continue
					}
					failed++
					if firstErr == "" {
						firstErr = err.Error()
					}
					continue
				}
				if info.Mode()&os.ModeSymlink == 0 {
					continue
				}
				_, statErr := os.Stat(op.Path)
				if statErr == nil {
					continue // target exists again; no longer a broken symlink
				}
				if !os.IsNotExist(statErr) {
					// If the target cannot be checked (for example EACCES while
					// following the symlink), keep the symlink and surface the
					// failure instead of risking deletion of a path that may no
					// longer be broken.
					failed++
					if firstErr == "" {
						firstErr = statErr.Error()
					}
					continue
				}
				if err := os.Remove(op.Path); err != nil {
					failed++
					if firstErr == "" {
						firstErr = err.Error()
					}
					continue
				}
				removed++
			}
			break
		}
		if failed > 0 {
			result := runner.Result{
				Program:  "delete-broken-symlink",
				Args:     []string{action.ConfirmValue},
				ExitCode: -1,
				Err:      fmt.Sprintf("removed %d broken symlink(s), %d failed: %s", removed, failed, compat.SanitizeMetadata(firstErr)),
			}
			m.actionResult = &result
			m.syncViewport()
			return m, loadSnapshotWithActionResult(m.cwd, result)
		}
		if removed == 0 {
			result := runner.Result{
				Program: "delete-broken-symlink",
				Args:    []string{action.ConfirmValue},
				Stdout:  "0 broken symlink(s) found at deletion time",
			}
			m.actionResult = &result
			m.syncViewport()
			return m, loadSnapshotWithActionResult(m.cwd, result)
		}
		m.actionResult = nil
		return m, loadSnapshot(m.cwd)
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
			partialSuccess := cleanupLockAfterRemove(action, m.cwd, &result)
			return actionResultMsg{result: result, mutates: action.Mutates, partialSuccess: partialSuccess}
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
		partialSuccess := cleanupLockAfterRemove(action, m.cwd, &result)
		return actionResultMsg{result: result, mutates: action.Mutates, partialSuccess: partialSuccess}
	}
}

func cleanupLockAfterRemove(action actions.CommandPreview, cwd string, result *runner.Result) bool {
	if action.ID != "remove" || result == nil || result.ExitCode != 0 || result.Err != "" || action.ConfirmValue == "" {
		return false
	}
	path := locks.ProjectLockPath(cwd)
	for _, arg := range action.Exec.Args {
		if arg == "-g" || arg == "--global" {
			path = locks.GlobalLockPath()
			break
		}
	}
	if _, err := locks.RemoveEntryIfExists(path, action.ConfirmValue); err != nil {
		result.ExitCode = -1
		result.Err = "removed skill, but failed to update lock: " + compat.SanitizeMetadata(err.Error())
		return true
	}
	return false
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

func (m appModel) appendEnableDisableActions(previews []actions.CommandPreview, sk *model.Skill) []actions.CommandPreview {
	if sk == nil {
		return previews
	}
	toggleActions := []actions.CommandPreview{}
	if m.agent != "" {
		var activePaths []string
		var disabledPaths []string
		for _, obs := range sk.ObservedPaths {
			if obs.Agent == m.agent {
				if obs.Status == model.StatusDisabled {
					disabledPaths = append(disabledPaths, obs.Path, obs.TargetPath)
				} else {
					activePaths = append(activePaths, obs.Path)
				}
			}
		}

		if len(activePaths) > 0 {
			for _, path := range activePaths {
				sharingAgents := []string{}
				for _, other := range sk.ObservedPaths {
					if other.Path == path {
						sharingAgents = append(sharingAgents, other.Agent)
					}
				}
				if len(sharingAgents) > 1 {
					reason := fmt.Sprintf("This path is shared by multiple agents (%s). Use scope-level disable instead.", strings.Join(sharingAgents, ", "))
					toggleActions = append(toggleActions, actions.CommandPreview{
						ID:          "disable_skill",
						Title:       fmt.Sprintf("Disable skill for agent %s", m.agentLabel()),
						Description: "This skill cannot be disabled for a single agent because the root directory is shared.",
						Available:   false,
						Reason:      reason,
					})
				} else {
					toggleActions = append(toggleActions, actions.CommandPreview{
						ID:          "disable_skill",
						Title:       fmt.Sprintf("Disable skill for agent %s", m.agentLabel()),
						Description: fmt.Sprintf("Disable this skill for %s by moving it to the disabled shelf.", m.agentLabel()),
						Command:     "disable skill for agent " + m.agentLabel(),
						Exec: actions.ExecSpec{
							Internal: "disable_skill",
							Args:     []string{path},
						},
						Mutates:   true,
						Available: true,
					})
				}
			}
		}

		if len(disabledPaths) > 0 {
			for i := 0; i < len(disabledPaths); i += 2 {
				src := disabledPaths[i]
				dest := disabledPaths[i+1]

				sharingAgents := []string{}
				for _, other := range sk.ObservedPaths {
					if other.Path == src {
						sharingAgents = append(sharingAgents, other.Agent)
					}
				}

				if len(sharingAgents) > 1 {
					reason := fmt.Sprintf("This path is shared by multiple agents (%s). Use scope-level enable instead.", strings.Join(sharingAgents, ", "))
					toggleActions = append(toggleActions, actions.CommandPreview{
						ID:          "enable_skill",
						Title:       fmt.Sprintf("Enable skill for agent %s", m.agentLabel()),
						Description: "This skill cannot be enabled for a single agent because the disabled directory is shared.",
						Available:   false,
						Reason:      reason,
					})
				} else {
					toggleActions = append(toggleActions, actions.CommandPreview{
						ID:          "enable_skill",
						Title:       fmt.Sprintf("Enable skill for agent %s", m.agentLabel()),
						Description: fmt.Sprintf("Enable this skill for %s by moving it back from the disabled shelf.", m.agentLabel()),
						Command:     "enable skill for agent " + m.agentLabel(),
						Exec: actions.ExecSpec{
							Internal: "enable_skill",
							Args:     []string{src, dest},
						},
						Mutates:   true,
						Available: true,
					})
				}
			}
		}

	} else {
		var activePaths []string
		seenActive := map[string]bool{}
		var disabledPaths []string
		seenDisabled := map[string]bool{}
		for _, obs := range sk.ObservedPaths {
			if obs.Scope == sk.Scope {
				if obs.Status == model.StatusDisabled {
					if !seenDisabled[obs.Path] {
						seenDisabled[obs.Path] = true
						disabledPaths = append(disabledPaths, obs.Path, obs.TargetPath)
					}
				} else {
					if !seenActive[obs.Path] {
						seenActive[obs.Path] = true
						activePaths = append(activePaths, obs.Path)
					}
				}
			}
		}

		if len(activePaths) > 0 {
			toggleActions = append(toggleActions, actions.CommandPreview{
				ID:          "disable_skill",
				Title:       fmt.Sprintf("Disable skill (scope: %s)", sk.Scope),
				Description: fmt.Sprintf("Disable this skill across all agent roots in the %s scope.", sk.Scope),
				Command:     fmt.Sprintf("disable skill (scope: %s)", sk.Scope),
				Exec: actions.ExecSpec{
					Internal: "disable_skill",
					Args:     activePaths,
				},
				Mutates:   true,
				Available: true,
			})
		}

		if len(disabledPaths) > 0 {
			toggleActions = append(toggleActions, actions.CommandPreview{
				ID:          "enable_skill",
				Title:       fmt.Sprintf("Enable skill (scope: %s)", sk.Scope),
				Description: fmt.Sprintf("Enable this skill across all agent roots in the %s scope.", sk.Scope),
				Command:     fmt.Sprintf("enable skill (scope: %s)", sk.Scope),
				Exec: actions.ExecSpec{
					Internal: "enable_skill",
					Args:     disabledPaths,
				},
				Mutates:   true,
				Available: true,
			})
		}
	}

	return insertToggleActions(previews, toggleActions)
}

func (m appModel) sourceEnableDisableActions(skills []*model.Skill) []actions.CommandPreview {
	if len(skills) == 0 {
		return nil
	}
	var activePaths []string
	seenActive := map[string]bool{}
	var disabledPaths []string
	seenDisabled := map[string]bool{}
	sharedActive, sharedDisabled := map[string][]string{}, map[string][]string{}

	for _, sk := range skills {
		for _, obs := range sk.ObservedPaths {
			if m.agent != "" && obs.Agent != m.agent {
				continue
			}
			if m.agent != "" {
				sharing := sharingAgentsForPath(sk, obs.Path)
				if len(sharing) > 1 {
					if obs.Status == model.StatusDisabled {
						sharedDisabled[obs.Path] = sharing
					} else {
						sharedActive[obs.Path] = sharing
					}
					continue
				}
			}

			if obs.Status == model.StatusDisabled {
				if !seenDisabled[obs.Path] {
					seenDisabled[obs.Path] = true
					disabledPaths = append(disabledPaths, obs.Path, obs.TargetPath)
				}
				continue
			}
			if !seenActive[obs.Path] {
				seenActive[obs.Path] = true
				activePaths = append(activePaths, obs.Path)
			}
		}
	}

	var out []actions.CommandPreview
	if len(activePaths) > 0 {
		title := "Disable source skills"
		desc := "Disable all enabled skills in this source."
		cmd := "disable source skills"
		if m.agent != "" {
			title = fmt.Sprintf("Disable source skills for agent %s", m.agentLabel())
			desc = fmt.Sprintf("Disable enabled skills in this source for %s.", m.agentLabel())
			cmd = "disable source skills for agent " + m.agentLabel()
		}
		out = append(out, actions.CommandPreview{ID: "disable_skill", Title: title, Description: desc, Command: cmd, Exec: actions.ExecSpec{Internal: "disable_skill", Args: activePaths}, Mutates: true, Available: true})
	} else if len(sharedActive) > 0 {
		out = append(out, actions.CommandPreview{ID: "disable_skill", Title: fmt.Sprintf("Disable source skills for agent %s", m.agentLabel()), Description: "This source cannot be disabled for a single agent because one or more directories are shared.", Available: false, Reason: sharedSourceReason(sharedActive, "disable")})
	}

	if len(disabledPaths) > 0 {
		title := "Enable source skills"
		desc := "Enable all disabled skills in this source."
		cmd := "enable source skills"
		if m.agent != "" {
			title = fmt.Sprintf("Enable source skills for agent %s", m.agentLabel())
			desc = fmt.Sprintf("Enable disabled skills in this source for %s.", m.agentLabel())
			cmd = "enable source skills for agent " + m.agentLabel()
		}
		out = append(out, actions.CommandPreview{ID: "enable_skill", Title: title, Description: desc, Command: cmd, Exec: actions.ExecSpec{Internal: "enable_skill", Args: disabledPaths}, Mutates: true, Available: true})
	} else if len(sharedDisabled) > 0 {
		out = append(out, actions.CommandPreview{ID: "enable_skill", Title: fmt.Sprintf("Enable source skills for agent %s", m.agentLabel()), Description: "This source cannot be enabled for a single agent because one or more disabled directories are shared.", Available: false, Reason: sharedSourceReason(sharedDisabled, "enable")})
	}

	return out
}

func sharingAgentsForPath(sk *model.Skill, path string) []string {
	var agents []string
	for _, other := range sk.ObservedPaths {
		if other.Path == path {
			agents = append(agents, other.Agent)
		}
	}
	return agents
}

func sharedSourceReason(shared map[string][]string, verb string) string {
	for _, agents := range shared {
		return fmt.Sprintf("This path is shared by multiple agents (%s). Use scope-level %s instead.", strings.Join(agents, ", "), verb)
	}
	return "One or more paths are shared by multiple agents. Use scope-level " + verb + " instead."
}

func insertToggleActions(previews, toggleActions []actions.CommandPreview) []actions.CommandPreview {
	if len(toggleActions) == 0 {
		return previews
	}
	insertAt := len(previews)
	for i, preview := range previews {
		if preview.ID == "open_skill" {
			insertAt = i + 1
			break
		}
	}
	out := make([]actions.CommandPreview, 0, len(previews)+len(toggleActions))
	out = append(out, previews[:insertAt]...)
	out = append(out, toggleActions...)
	out = append(out, previews[insertAt:]...)
	return out
}

func (m appModel) applyUpdateCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := selfupdate.Apply(ctx, m.updatePlan, nil)
		return appUpdateResultMsg{err: err}
	}
}
