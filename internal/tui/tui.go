package tui

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"

	"lazyskills/internal/actions"
	"lazyskills/internal/agents"
	"lazyskills/internal/compat"
	"lazyskills/internal/display"
	"lazyskills/internal/model"
	"lazyskills/internal/runner"
	"lazyskills/internal/scan"
)

type scopeFilter int

const (
	scopeAll scopeFilter = iota
	scopeProject
	scopeGlobal
)

type focusState int

const (
	focusSkills focusState = iota
	focusMetadata
	focusPreview
)

type appModel struct {
	cwd              string
	result           model.ScanResult
	err              error
	selected         int
	filter           scopeFilter
	agent            string
	search           string
	searching        bool
	commands         bool
	selectedKeys     map[string]bool
	help             bool
	action           int
	confirming       bool
	confirmInput     string
	confirmError     string
	running          bool
	runningTitle     string
	actionResult     *runner.Result
	width            int
	height           int
	viewport         viewport.Model
	metadataViewport viewport.Model
	previewViewport  viewport.Model
	detailsFocused   bool
	detailModal      bool
	helpOpen         bool
	focus            focusState
	collapsedGroups  map[string]bool
}

type paneLayout struct {
	OuterWidth    int
	OuterHeight   int
	StyleWidth    int
	StyleHeight   int
	ContentWidth  int
	ContentHeight int
}

type appLayout struct {
	Small  bool
	Width  int
	Height int
	Left   paneLayout
	List   paneLayout
	Detail paneLayout
}

const (
	minLayoutWidth  = 40
	minLayoutHeight = 7
	appVersion      = "v1"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	borderStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	warningStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	runExec       = runner.OSRunner{}.Run

	// Action Mode UI Polish Styles
	actionTitleStyle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	activeActionStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	activeActionTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	activeActionSubStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Background(lipgloss.Color("62"))
	normalActionTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	normalActionSubStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	actionNormalStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	actionBorderColor      = lipgloss.Color("62")
	runningStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	successStyle           = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)

	// Metadata / Details styling
	metaKeyStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	sectionHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	healthHeaderStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
)

type snapshotMsg struct {
	result model.ScanResult
	err    error
}

type actionResultMsg struct {
	result         runner.Result
	mutates        bool
	partialSuccess bool
}

func Run(cwd string) error {
	program := tea.NewProgram(newModel(cwd), tea.WithAltScreen())
	_, err := program.Run()
	return err
}

func newModel(cwd string) appModel {
	return appModel{
		cwd:              cwd,
		help:             true,
		viewport:         viewport.New(0, 0),
		metadataViewport: viewport.New(0, 0),
		previewViewport:  viewport.New(0, 0),
		collapsedGroups:  make(map[string]bool),
	}
}

func (m appModel) Init() tea.Cmd {
	return loadSnapshot(m.cwd)
}

func loadSnapshot(cwd string) tea.Cmd {
	return func() tea.Msg {
		result, err := scan.Snapshot(cwd)
		return snapshotMsg{result: result, err: err}
	}
}

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
	case actionResultMsg:
		m.running = false
		m.runningTitle = ""
		m.confirming = false
		m.confirmInput = ""
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
				m.syncViewport()
			case "ctrl+c":
				return m, tea.Quit
			case "o":
				m.detailModal = false
				return m.startCurrentSkillActionByID("open_skill")
			case "c":
				m.detailModal = false
				m.commands = true
				m.action = 0
				m.syncViewport()
			case "down", "j":
				m.viewport.LineDown(1)
				m.clampViewportOffset()
			case "up", "k":
				m.viewport.LineUp(1)
				m.clampViewportOffset()
			case "pgdown", "ctrl+d":
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			case "pgup", "ctrl+u":
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			case "home":
				m.viewport.GotoTop()
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
				return m.startCurrentSkillActionByID("open_skill")
			case "u":
				return m.startActionByID(preferredUpdateActionID(m.selectedCount()))
			case "x":
				return m.startActionByID(preferredRemoveActionID(m.selectedCount()))
			}
			m.syncViewport()
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
					if m.isCollapsed(row.groupName) {
						m.expandGroup(row.groupName)
					} else {
						m.collapseGroup(row.groupName)
					}
				} else {
					m.detailModal = true
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
		case "P":
			m.filter = scopeProject
			m.selected = 0
			m.actionResult = nil
			m.viewport.GotoTop()
		case "G":
			m.filter = scopeGlobal
			m.selected = 0
			m.actionResult = nil
			m.viewport.GotoTop()
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
			if m.focus == focusMetadata {
				m.metadataViewport.GotoTop()
			} else if m.focus == focusPreview {
				m.previewViewport.GotoTop()
			} else {
				m.previewViewport.GotoTop()
			}
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
		return actions.AppLevelActions()
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

func (m appModel) startCurrentSkillActionByID(id string) (tea.Model, tea.Cmd) {
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		return m, nil
	}
	row := rows[m.selected]
	if row.isHeader {
		return m, nil
	}
	for _, action := range actions.ForSkill(row.skill) {
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

func (m *appModel) syncViewport() {
	layout := newAppLayout(m.width, m.height)
	if layout.Small {
		m.viewport.Width = 0
		m.viewport.Height = 0
		m.viewport.SetContent("")
		m.viewport.SetYOffset(0)

		m.metadataViewport.Width = 0
		m.metadataViewport.Height = 0
		m.metadataViewport.SetContent("")
		m.metadataViewport.SetYOffset(0)

		m.previewViewport.Width = 0
		m.previewViewport.Height = 0
		m.previewViewport.SetContent("")
		m.previewViewport.SetYOffset(0)
		return
	}
	if m.detailModal || m.commands {
		modalWidth := 80
		if layout.Width < modalWidth+4 {
			modalWidth = layout.Width - 4
		}
		if modalWidth < 20 {
			modalWidth = 20
		}
		modalHeight := 24
		if layout.Height < modalHeight+4 {
			modalHeight = layout.Height - 4
		}
		if modalHeight < 7 {
			modalHeight = 7
		}
		m.viewport.Width = modalWidth - 4
		m.viewport.Height = modalHeight - 6
		m.viewport.SetContent(m.detailText(modalWidth - 4))
	} else {
		_, rightWidth, topHeight, bottomHeight := m.getThreePaneLayout()

		// For metadata viewport:
		m.metadataViewport.Width = max(1, rightWidth-4)
		m.metadataViewport.Height = max(1, topHeight-2)
		m.metadataViewport.SetContent(strings.Join(m.metadataLines(rightWidth-4), "\n"))

		// For preview viewport:
		m.previewViewport.Width = max(1, rightWidth-4)
		m.previewViewport.Height = max(1, bottomHeight-2)
		m.previewViewport.SetContent(strings.Join(m.previewLines(rightWidth-4), "\n"))
	}
	m.clampViewportOffset()
}

type skillsRow struct {
	isHeader   bool
	groupName  string
	skill      *model.Skill
	skillIndex int
}

func (m appModel) visibleRows() []skillsRow {
	items := m.filteredSkills()
	var rows []skillsRow
	previousGroup := ""
	for i, skill := range items {
		group := listGroupLabel(skill)
		if group != previousGroup {
			rows = append(rows, skillsRow{
				isHeader:   true,
				groupName:  group,
				skillIndex: -1,
			})
			previousGroup = group
		}
		if m.isCollapsed(group) {
			continue
		}
		rows = append(rows, skillsRow{
			isHeader:   false,
			groupName:  group,
			skill:      skill,
			skillIndex: i,
		})
	}
	return rows
}

func (m appModel) getGroupCounts(group string) (visible int, total int) {
	items := m.filteredSkills()
	for _, skill := range items {
		if listGroupLabel(skill) == group {
			total++
			if !m.isCollapsed(group) {
				visible++
			}
		}
	}
	return
}

func (m *appModel) collapseGroup(group string) {
	if m.collapsedGroups == nil {
		m.collapsedGroups = make(map[string]bool)
	}
	m.collapsedGroups[group] = true

	rows := m.visibleRows()
	for idx, r := range rows {
		if r.isHeader && r.groupName == group {
			m.selected = idx
			break
		}
	}
	m.clampSelection()
}

func (m *appModel) expandGroup(group string) {
	if m.collapsedGroups == nil {
		m.collapsedGroups = make(map[string]bool)
	}
	delete(m.collapsedGroups, group)

	rows := m.visibleRows()
	for idx, r := range rows {
		if r.isHeader && r.groupName == group {
			m.selected = idx
			break
		}
	}
	m.clampSelection()
}

func (m *appModel) clampSelection() {
	rows := m.visibleRows()
	if len(rows) == 0 {
		m.selected = 0
		return
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(rows) {
		m.selected = len(rows) - 1
	}
}

func (m appModel) isCollapsed(group string) bool {
	if m.collapsedGroups == nil {
		return false
	}
	return m.collapsedGroups[group]
}

func (m *appModel) toggleSelectedSkill() {
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		return
	}
	row := rows[m.selected]
	if row.isHeader {
		return
	}
	if m.selectedKeys == nil {
		m.selectedKeys = map[string]bool{}
	}
	key := skillKey(row.skill)
	if m.selectedKeys[key] {
		delete(m.selectedKeys, key)
	} else {
		m.selectedKeys[key] = true
	}
	if len(m.selectedKeys) == 0 {
		m.selectedKeys = nil
	}
}

func (m *appModel) selectCurrentSourceGroup() {
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		return
	}
	row := rows[m.selected]
	group := row.groupName
	if group == "" {
		return
	}
	if m.selectedKeys == nil {
		m.selectedKeys = map[string]bool{}
	}
	changed := false
	items := m.filteredSkills()
	for _, skill := range items {
		if listGroupLabel(skill) == group {
			m.selectedKeys[skillKey(skill)] = true
			changed = true
		}
	}
	if !changed && len(m.selectedKeys) == 0 {
		m.selectedKeys = nil
	}
}

func (m *appModel) jumpSourceGroup(direction int) {
	rows := m.visibleRows()
	if len(rows) == 0 {
		return
	}
	m.clampSelection()

	var headerIndices []int
	for idx, r := range rows {
		if r.isHeader {
			headerIndices = append(headerIndices, idx)
		}
	}
	if len(headerIndices) <= 1 {
		if len(headerIndices) == 1 {
			m.selected = headerIndices[0]
		}
		return
	}

	currentHeaderIdx := 0
	for i, idx := range headerIndices {
		if idx <= m.selected {
			currentHeaderIdx = i
		}
	}

	if direction > 0 {
		currentHeaderIdx = (currentHeaderIdx + 1) % len(headerIndices)
	} else {
		currentHeaderIdx = (currentHeaderIdx + len(headerIndices) - 1) % len(headerIndices)
	}

	m.selected = headerIndices[currentHeaderIdx]
	m.actionResult = nil
	m.viewport.GotoTop()
}

func (m appModel) isSelected(skill *model.Skill) bool {
	return len(m.selectedKeys) > 0 && m.selectedKeys[skillKey(skill)]
}

func (m appModel) selectedCount() int {
	return len(m.selectedKeys)
}

func (m appModel) selectedSkills() []*model.Skill {
	if len(m.selectedKeys) == 0 {
		return nil
	}
	selected := make([]*model.Skill, 0, len(m.selectedKeys))
	for _, skill := range m.result.Skills {
		if m.isSelected(skill) {
			selected = append(selected, skill)
		}
	}
	return selected
}

func skillKey(skill *model.Skill) string {
	if skill == nil {
		return ""
	}
	return strings.Join([]string{string(skill.Scope), skill.Name, skill.CanonicalPath, skill.SkillPath}, "\x00")
}

func (m appModel) currentSelectedKey() string {
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		return ""
	}
	row := rows[m.selected]
	if row.isHeader {
		return "group:" + row.groupName
	}
	return "skill:" + skillKey(row.skill)
}

func (m *appModel) restoreSelection(selectedKey string, fallback int) {
	rows := m.visibleRows()
	if selectedKey != "" {
		for i, r := range rows {
			key := ""
			if r.isHeader {
				key = "group:" + r.groupName
			} else {
				key = "skill:" + skillKey(r.skill)
			}
			if key == selectedKey {
				m.selected = i
				return
			}
		}
	}
	m.selected = fallback
	m.clampSelection()
}

func sourceGroupKey(skill *model.Skill) string {
	info := sourceInfo(skill)
	if info.Source == "" {
		return ""
	}
	return info.Source
}

func sourceGroupLabel(skill *model.Skill) string {
	info := sourceInfo(skill)
	if info.Source == "" {
		return ""
	}
	return info.Source
}

func listGroupLabel(skill *model.Skill) string {
	if label := sourceGroupLabel(skill); label != "" {
		return label
	}
	return "Custom / untracked"
}

type skillSourceInfo struct {
	Source string
	Folder string
	Ref    string
}

func sourceInfo(skill *model.Skill) skillSourceInfo {
	if skill == nil {
		return skillSourceInfo{}
	}
	if skill.Scope == model.ScopeProject && skill.LocalLock != nil {
		return localSourceInfo(skill.LocalLock)
	}
	if skill.Scope == model.ScopeGlobal && skill.GlobalLock != nil {
		return globalSourceInfo(skill.GlobalLock)
	}
	if skill.LocalLock != nil {
		return localSourceInfo(skill.LocalLock)
	}
	if skill.GlobalLock != nil {
		return globalSourceInfo(skill.GlobalLock)
	}
	return skillSourceInfo{}
}

func localSourceInfo(lock *model.LocalLockEntry) skillSourceInfo {
	if lock == nil {
		return skillSourceInfo{}
	}
	return skillSourceInfo{Source: compat.SanitizeMetadata(lock.Source), Folder: skillFolder(lock.SkillPath), Ref: compat.SanitizeMetadata(lock.Ref)}
}

func globalSourceInfo(lock *model.GlobalLockEntry) skillSourceInfo {
	if lock == nil {
		return skillSourceInfo{}
	}
	source := lock.Source
	if source == "" {
		source = lock.SourceURL
	}
	return skillSourceInfo{Source: compat.SanitizeMetadata(source), Folder: skillFolder(lock.SkillPath), Ref: compat.SanitizeMetadata(lock.Ref)}
}

func skillFolder(skillPath string) string {
	folder := compat.SanitizeMetadata(skillPath)
	folder = strings.TrimSuffix(folder, "/SKILL.md")
	folder = strings.TrimSuffix(folder, "SKILL.md")
	return strings.Trim(folder, "/")
}

func sourceDetailLines(skill *model.Skill, width int) []string {
	info := sourceInfo(skill)
	if info.Source == "" {
		return nil
	}
	lines := []string{formatMetaLine("Source:", info.Source, width)}
	if info.Folder != "" {
		lines = append(lines, formatMetaLine("Folder:", info.Folder, width))
	}
	if info.Ref != "" {
		lines = append(lines, formatMetaLine("Ref:", info.Ref, width))
	}
	if skill.LocalLock != nil && skill.LocalLock.ComputedHash != "" {
		lines = append(lines, formatMetaLine("Hash:", skill.LocalLock.ComputedHash, width))
	} else if skill.GlobalLock != nil && skill.GlobalLock.SkillFolderHash != "" {
		lines = append(lines, formatMetaLine("Hash:", skill.GlobalLock.SkillFolderHash, width))
	}
	lines = append(lines, "", dimStyle.Render("Note: Live update status is not checked here."))
	lines = append(lines, dimStyle.Render("Use update actions ('u' or 'c' menu) to check for updates."))
	return lines
}

func (m *appModel) pruneSelected() {
	if len(m.selectedKeys) == 0 {
		return
	}
	valid := map[string]bool{}
	for _, skill := range m.result.Skills {
		valid[skillKey(skill)] = true
	}
	for key := range m.selectedKeys {
		if !valid[key] {
			delete(m.selectedKeys, key)
		}
	}
	if len(m.selectedKeys) == 0 {
		m.selectedKeys = nil
	}
}

func (m appModel) getThreePaneLayout() (listWidth, rightWidth, topHeight, bottomHeight int) {
	width := viewWidth(m.width)
	height := viewHeight(m.height) - 1 // Deduct 1 for persistent footer
	listWidth = width * 4 / 10
	if listWidth < 25 {
		listWidth = 25
	}
	if listWidth > width-30 {
		listWidth = width - 30
	}
	if listWidth < 10 {
		listWidth = 10
	}
	rightWidth = width - listWidth

	topHeight = height * 4 / 10
	if topHeight < 5 {
		topHeight = 5
	}
	if topHeight > height-5 {
		topHeight = height - 5
	}
	bottomHeight = height - topHeight
	return
}

func (m appModel) View() string {
	if m.err != nil {
		return fitToScreen(errorStyle.Render(fmt.Sprintf("LazySkills error: %s\n\nPress q to quit.", compat.SanitizeMetadata(m.err.Error()))), viewWidth(m.width), viewHeight(m.height))
	}
	layout := newAppLayout(m.width, m.height)
	if layout.Small {
		return smallTerminalView(layout.Width, layout.Height)
	}

	// Keep View pure for callers: sync a local copy so render-time fallback
	// sizing does not mutate the model stored by Bubble Tea.
	viewModel := m
	viewModel.width = layout.Width
	viewModel.height = layout.Height
	viewModel.syncViewport()

	listWidth, rightWidth, topHeight, bottomHeight := viewModel.getThreePaneLayout()

	listLayout := newPaneLayout(listWidth, viewModel.height-1)
	metadataLayout := newPaneLayout(rightWidth, topHeight)
	previewLayout := newPaneLayout(rightWidth, bottomHeight)

	listStyle := paneStyle(listLayout, viewModel.focus == focusSkills)
	metadataStyle := paneStyle(metadataLayout, viewModel.focus == focusMetadata)
	previewStyle := paneStyle(previewLayout, viewModel.focus == focusPreview)

	listContent := fitLines(viewModel.listPane(listLayout.ContentHeight, listLayout.ContentWidth), listLayout.ContentHeight)
	list := decoratePane(listStyle.Render(listContent), listLayout, viewModel.focus == focusSkills, viewModel.listTitle())

	metadataContent := fitLines(viewModel.metadataViewport.View(), metadataLayout.ContentHeight)
	metadata := decoratePane(metadataStyle.Render(metadataContent), metadataLayout, viewModel.focus == focusMetadata, "2 Metadata")

	previewContent := fitLines(viewModel.previewViewport.View(), previewLayout.ContentHeight)
	preview := decoratePane(previewStyle.Render(previewContent), previewLayout, viewModel.focus == focusPreview, "3 Preview")

	rightSide := lipgloss.JoinVertical(lipgloss.Left, metadata, preview)
	view := lipgloss.JoinHorizontal(lipgloss.Top, list, rightSide)

	if viewModel.detailModal {
		return viewModel.detailModalOverlay(layout)
	}
	if viewModel.helpOpen {
		return viewModel.helpModalOverlay(layout)
	}
	if viewModel.running {
		return viewModel.runningOverlay(layout)
	}
	if viewModel.confirming {
		return viewModel.confirmationOverlay(layout)
	}
	if viewModel.commands {
		return viewModel.commandsOverlay(layout)
	}
	footer := viewModel.footerText(layout.Width)
	return view + "\n" + footer
}

func (m appModel) filterPane(width int) string {
	counts := map[model.Scope]int{}
	issues := 0
	for _, sk := range m.result.Skills {
		counts[sk.Scope]++
		issues += len(sk.HealthIssues)
	}
	issues += len(m.result.HealthIssues)
	lines := []string{
		titleStyle.Render("LazySkills"),
		dimStyle.Render(compat.SanitizeMetadata(m.result.Cwd)),
		"",
		filterLine("All", m.filter == scopeAll),
		filterLine(fmt.Sprintf("[P]roject (%d)", counts[model.ScopeProject]), m.filter == scopeProject),
		filterLine(fmt.Sprintf("[G]lobal (%d)", counts[model.ScopeGlobal]), m.filter == scopeGlobal),
		"",
		fmt.Sprintf("Skills: %d", len(m.result.Skills)),
		fmt.Sprintf("Issues: %d", issues),
		fmt.Sprintf("Agent: %s", m.agentLabel()),
	}
	if m.result.Preflight != nil {
		lines = append(lines, "", titleStyle.Render("Dependencies"))
		if m.result.Preflight.CanRunSkills {
			lines = append(lines, successStyle.Render("  ✓ Ready"))
		} else {
			lines = append(lines, errorStyle.Render("  ✗ Missing"))
			for _, tool := range []string{"skills", "npx", "node", "npm"} {
				status := "✓"
				style := successStyle
				if !m.result.Preflight.Tools[tool].Exists {
					status = "✗"
					style = errorStyle
				}
				lines = append(lines, style.Render(fmt.Sprintf("    %s %s", status, tool)))
			}
		}
	}
	if len(m.result.HealthIssues) > 0 {
		lines = append(lines, "", errorStyle.Render("Scan health"))
		for _, issue := range m.result.HealthIssues {
			lines = append(lines, truncate(fmt.Sprintf("- %s: %s", compat.SanitizeMetadata(issue.Type), compat.SanitizeMetadata(issue.Message)), width))
		}
	}
	if m.search != "" || m.searching {
		prompt := "/" + compat.SanitizeMetadata(m.search)
		if m.searching {
			prompt += "_"
		}
		lines = append(lines, "", "Search", prompt)
	}
	if m.help {
		lines = append(lines, "", "Keys",
			"↑/↓ j/k select/scroll",
			"1 / 2 focus pane",
			"tab cycle focus",
			"→/l focus det",
			"←/h focus list",
			"enter details modal",
			"P project scope",
			"G global scope",
			"f cycle scope",
			"F clear scope",
			"[ / ] grp jump",
			"space mark",
			"s source mark",
			"o open",
			"u update",
			"x remove",
			"a agent",
			"A all agents",
			"c actions",
			"/ search",
			"r refresh",
			"? help",
			"q quit",
		)
	}
	return strings.Join(lines, "\n")
}

func filterLine(label string, active bool) string {
	if active {
		return selectedStyle.Render("› " + label)
	}
	return "  " + label
}

func scopeBadge(scope string) string {
	switch scope {
	case string(model.ScopeProject):
		return "P"
	case string(model.ScopeGlobal):
		return "G"
	default:
		return compat.SanitizeMetadata(scope)
	}
}

func (m appModel) listTitle() string {
	title := "1 Skills"
	if m.agent != "" {
		title = "1 Skills (" + m.agentLabel() + ")"
	}
	return title
}

func (m appModel) listPane(height, width int) string {
	items := m.filteredSkills()
	var lines []string
	if len(items) == 0 {
		var detail []string
		if m.result.Preflight != nil && !m.result.Preflight.CanRunSkills {
			detail = append(detail,
				errorStyle.Render("Missing Dependencies:"),
				"LazySkills cannot execute commands because required",
				"tools are missing.",
				"",
				"Please install Node.js & npm (which provides npx),",
				"or install the 'skills' CLI directly.",
			)
		} else if len(m.result.Skills) == 0 {
			detail = append(detail,
				"No skills found on your machine or project.",
				"",
				"To get started:",
				"1. Press 'c' to open actions and select 'skills init'.",
				"2. Or run 'skills find' to discover online skills.",
				"3. Or check documentation to manually link skills.",
			)
		} else {
			// Filters are active
			detail = append(detail, "No skills matched active filters.")
			if m.search != "" {
				detail = append(detail, "", fmt.Sprintf("• Search: '%s' (press Backspace to clear)", m.search))
			}
			if m.agent != "" {
				detail = append(detail, "", fmt.Sprintf("• Agent: '%s' has no compatible/visible skills in this view.", m.agentLabel()))
			}
			if m.filter == scopeProject {
				detail = append(detail, "", "• Scope: Project (press Tab to switch to Global)")
			} else if m.filter == scopeGlobal {
				detail = append(detail, "", "• Scope: Global (press Tab to switch to Project)")
			}
		}

		wrappedLines := []string{""}
		for _, line := range detail {
			if line == "" {
				wrappedLines = append(wrappedLines, "")
			} else {
				wrappedLines = append(wrappedLines, dimStyle.Render(truncate(line, width)))
			}
		}
		return strings.Join(wrappedLines, "\n")
	}

	// Active status headers (clean, non-verbose)
	if m.search != "" {
		lines = append(lines, dimStyle.Render("Search: /"+m.search))
	}
	if m.agent != "" {
		lines = append(lines, dimStyle.Render("Agent:  "+m.agentLabel()))
	}
	if m.result.Preflight != nil && !m.result.Preflight.CanRunSkills {
		lines = append(lines, errorStyle.Render("✗ Missing dependencies (press ? for help)"))
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}

	visible := max(1, height-len(lines))
	vRows := m.visibleRows()
	var renderedRows []string
	selectedRow := m.selected

	for idx, row := range vRows {
		var line string
		if row.isHeader {
			affordance := "- "
			if m.isCollapsed(row.groupName) {
				affordance = "+ "
			}
			headerText := affordance + row.groupName
			if idx == selectedRow {
				line = selectedStyle.Render(truncate(headerText, width))
			} else {
				line = dimStyle.Render(truncate(headerText, width))
			}
		} else {
			view := display.Skill(row.skill)
			mark := "  "
			if m.isSelected(row.skill) {
				mark = "● "
			}
			coreLabel := fmt.Sprintf("%s%s [%s]", mark, view.Name, scopeBadge(view.Scope))
			if m.agent != "" {
				coreLabel += " " + agentVisibilityBadge(row.skill, m.agent)
			}
			issueErrors, issueWarnings := healthIssueCounts(view.HealthIssues)
			badgeLen := 0
			if issueErrors > 0 {
				badgeLen = len(fmt.Sprintf(" !%d", issueErrors))
			} else if issueWarnings > 0 {
				badgeLen = len(fmt.Sprintf(" ⚠ %d", issueWarnings))
			}
			truncatedCore := truncate(coreLabel, width-badgeLen)
			if idx == selectedRow {
				badge := ""
				if issueErrors > 0 {
					badge = fmt.Sprintf(" !%d", issueErrors)
				} else if issueWarnings > 0 {
					badge = fmt.Sprintf(" ⚠ %d", issueWarnings)
				}
				line = selectedStyle.Render(truncatedCore + badge)
			} else if issueErrors > 0 {
				badge := errorStyle.Render(fmt.Sprintf(" !%d", issueErrors))
				line = errorStyle.Render(truncatedCore) + badge
			} else if issueWarnings > 0 {
				badge := warningStyle.Render(fmt.Sprintf(" ⚠ %d", issueWarnings))
				line = truncatedCore + badge
			} else {
				line = truncatedCore
			}
		}
		renderedRows = append(renderedRows, line)
	}

	start := 0
	if selectedRow >= visible {
		start = selectedRow - visible + 1
	}
	end := min(len(renderedRows), start+visible)
	for _, line := range renderedRows[start:end] {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

type listRow struct {
	line       string
	skillIndex int
}

func healthIssueCounts(issues []display.HealthIssueView) (errors int, warnings int) {
	for _, issue := range issues {
		if issue.Severity == "error" {
			errors++
		} else {
			warnings++
		}
	}
	return errors, warnings
}

func humanHealthIssueType(issueType string) string {
	switch issueType {
	case "missing_skill_md":
		return "Missing SKILL.md"
	case "invalid_frontmatter":
		return "Invalid Frontmatter"
	case "broken_symlink":
		return "Broken Symlink"
	case "missing_project_lock":
		return "Not Tracked in Project"
	case "missing_global_lock":
		return "Not Tracked in Global"
	case "ghost_agent_skill":
		return "Agent-specific skill"
	case "duplicate_name":
		return "Duplicate Name"
	case "project_global_shadowing":
		return "Name Conflict"
	case "lock_without_files":
		return "Lock Entry Missing Files"
	default:
		return strings.ReplaceAll(issueType, "_", " ")
	}
}

func humanHealthIssueMessage(issueType, message string) string {
	switch issueType {
	case "ghost_agent_skill":
		return "This skill is custom/untracked and only installed for specific agents."
	case "missing_project_lock":
		return "This skill is not tracked by the project lock."
	case "missing_global_lock":
		return "This skill is not tracked by the global lock."
	default:
		return message
	}
}

func (m appModel) detailPane() string {
	return m.viewport.View()
}

func (m appModel) detailText(width int) string {
	return strings.Join(m.detailLines(width), "\n")
}

func (m appModel) detailTitle() string {
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		return "2 Details"
	}
	row := rows[m.selected]
	if row.isHeader {
		return "2 Details: " + row.groupName
	}
	view := display.Skill(row.skill)
	return "2 Details: " + view.Name
}

func (m appModel) metadataLines(width int) []string {
	rows := m.visibleRows()
	if len(rows) == 0 {
		var lines []string

		if len(m.result.HealthIssues) > 0 {
			lines = append(lines, errorStyle.Render("Scan health:"), "")
			for _, issue := range m.result.HealthIssues {
				lines = append(lines, truncate(fmt.Sprintf("- %s: %s", compat.SanitizeMetadata(issue.Type), compat.SanitizeMetadata(issue.Message)), width))
			}
			lines = append(lines, "")
		}

		if m.result.Preflight != nil && !m.result.Preflight.CanRunSkills {
			lines = append(lines,
				errorStyle.Render("Dependency Issue"),
				wrapText("LazySkills requires the 'skills' CLI or Node.js/npm (npx) to be installed and available in your PATH.", width),
				"",
				dimStyle.Render("Status:"),
			)
			for _, tool := range []string{"skills", "npx", "node", "npm"} {
				status := "missing"
				style := errorStyle
				if m.result.Preflight.Tools[tool].Exists {
					status = "available"
					style = successStyle
				}
				lines = append(lines, style.Render(fmt.Sprintf("  • %s: %s", tool, status)))
			}
		} else if len(m.result.Skills) == 0 {
			lines = append(lines,
				sectionHeaderStyle.Render("Welcome to LazySkills!"),
				"",
				wrapText("No skills were found in your project or global directory.", width),
				"",
				dimStyle.Render("Quick Onboarding:"),
				wrapText("1. Press 'c' to open actions and choose 'Initialize skills in project' to create a local skills directory.", width),
				wrapText("2. Choose 'Find new skills (interactive)' to search and install online skills.", width),
				wrapText("3. Link your existing skills using symlinks.", width),
			)
		} else {
			lines = append(lines,
				dimStyle.Render("Select a skill to inspect it."),
			)
			if m.search != "" {
				lines = append(lines, "", dimStyle.Render(fmt.Sprintf("Active search: '%s'", m.search)))
			}
			if m.agent != "" {
				lines = append(lines, "", dimStyle.Render(fmt.Sprintf("Active agent filter: '%s'", m.agentLabel())))
			}
		}

		return lines
	}

	if m.selected < 0 || m.selected >= len(rows) {
		return []string{dimStyle.Render("Select a skill to inspect it.")}
	}

	row := rows[m.selected]
	if row.isHeader {
		visible, total := m.getGroupCounts(row.groupName)
		stateStr := "expanded"
		if m.isCollapsed(row.groupName) {
			stateStr = "collapsed"
		}
		lines := []string{
			formatMetaLine("Source:", row.groupName, width),
			formatMetaLine("State:", stateStr, width),
			formatMetaLine("Skills:", fmt.Sprintf("%d visible / %d total", visible, total), width),
			"",
			dimStyle.Render("Source-level actions coming later."),
		}
		if len(m.result.HealthIssues) > 0 {
			lines = append(lines, "", errorStyle.Render("Scan health"))
			for _, issue := range m.result.HealthIssues {
				lines = append(lines, truncate(fmt.Sprintf("- %s: %s", compat.SanitizeMetadata(issue.Type), compat.SanitizeMetadata(issue.Message)), width))
			}
		}
		return lines
	}

	view := display.Skill(row.skill)
	lines := []string{
		formatMetaLine("Scope:", string(view.Scope), width),
		formatMetaLine("Lock:", display.LockSummary(view), width),
	}
	if sourceLines := sourceDetailLines(row.skill, width); len(sourceLines) > 0 {
		lines = append(lines, sourceLines...)
	}
	if view.CanonicalPath != "" {
		lines = append(lines, formatMetaLine("Canonical:", view.CanonicalPath, width))
	}
	if m.agent != "" {
		lines = append(lines, formatMetaLine("Agent:", m.agentLabel(), width))
	}
	lines = append(lines, m.visibilitySummary(view, width)...)
	if len(view.Observed) > 0 && m.agent == "" {
		agentsSet := map[string]bool{}
		observedAgents := []string{}
		for _, p := range view.Observed {
			if p.Agent != "" && !agentsSet[p.Agent] {
				agentsSet[p.Agent] = true
				observedAgents = append(observedAgents, p.Agent)
			}
		}
		if len(observedAgents) > 0 {
			lines = append(lines, formatMetaLine("Observed:", strings.Join(observedAgents, ", "), width))
		}
	}

	if len(view.Observed) > 0 && m.agent != "" {
		showObservedSection := false
		for _, p := range view.Observed {
			if p.Agent == m.agent {
				if !showObservedSection {
					lines = append(lines, "", sectionHeaderStyle.Render("Observed Paths"))
					showObservedSection = true
				}
				line := fmt.Sprintf("- %s %s %s", p.Agent, p.Scope, p.Status)
				if p.TargetPath != "" {
					line += " → " + p.TargetPath
				}
				lines = append(lines, wrapText(line, width))
			}
		}
	}

	if len(view.HealthIssues) > 0 {
		issueErrors, _ := healthIssueCounts(view.HealthIssues)
		headerStyle := warningStyle.Bold(true)
		header := "Warnings"
		if issueErrors > 0 {
			headerStyle = healthHeaderStyle
			header = "Health Issues"
		}
		lines = append(lines, "", headerStyle.Render(header))
		for _, issue := range view.HealthIssues {
			line := fmt.Sprintf("- %s: %s", humanHealthIssueType(issue.Type), humanHealthIssueMessage(issue.Type, issue.Message))
			if issue.Path != "" {
				line += " (" + issue.Path + ")"
			}
			style := warningStyle
			if issue.Severity == "error" {
				style = errorStyle
			}
			lines = append(lines, style.Render(wrapText(line, width)))
		}
	}

	if len(m.result.HealthIssues) > 0 {
		lines = append(lines, "", errorStyle.Render("Scan health"))
		for _, issue := range m.result.HealthIssues {
			lines = append(lines, truncate(fmt.Sprintf("- %s: %s", compat.SanitizeMetadata(issue.Type), compat.SanitizeMetadata(issue.Message)), width))
		}
	}

	return lines
}

func (m appModel) previewLines(width int) []string {
	rows := m.visibleRows()
	if len(rows) == 0 {
		if m.result.Preflight != nil && !m.result.Preflight.CanRunSkills {
			return []string{
				errorStyle.Render("Preview Unavailable"),
				"Dependencies are missing.",
			}
		}
		return []string{
			dimStyle.Render("No skill selected for preview."),
		}
	}

	if m.selected < 0 || m.selected >= len(rows) {
		return []string{dimStyle.Render("No skill selected for preview.")}
	}

	row := rows[m.selected]
	if row.isHeader {
		return []string{
			dimStyle.Render("Preview not applicable for source groups."),
		}
	}

	view := display.Skill(row.skill)
	if view.Preview == "" {
		return []string{dimStyle.Render("No preview available for this skill.")}
	}

	lines := []string{}
	previewLines := strings.Split(view.Preview, "\n")
	for _, line := range previewLines {
		lines = append(lines, wrapText(line, width))
	}
	return lines
}

func (m appModel) detailLines(width int) []string {
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		return m.metadataLines(width)
	}
	row := rows[m.selected]
	if row.isHeader {
		return m.metadataLines(width)
	}
	view := display.Skill(row.skill)
	lines := []string{
		titleStyle.Render(view.Name),
		"",
	}
	lines = append(lines, m.metadataLines(width)...)
	if view.Preview != "" {
		lines = append(lines, "")
		previewDivider := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(strings.Repeat("─", max(1, width)))
		lines = append(lines, previewDivider)
		lines = append(lines, sectionHeaderStyle.Render("Preview"), "")
		lines = append(lines, m.previewLines(width)...)
	}
	return lines
}

func (m appModel) visibilitySummary(view display.SkillView, width int) []string {
	if len(view.Visibility) == 0 {
		return nil
	}
	if m.agent != "" {
		for _, visibility := range view.Visibility {
			if visibility.Agent != m.agent {
				continue
			}
			statusText := "not linked"
			if visibility.Visible {
				statusText = "available"
			}
			switch visibility.Reason {
			case "visible_via_universal_canonical", "visible_via_canonical":
				statusText = "available (canonical)"
			case "visible_via_symlink":
				statusText = "available (symlinked)"
			case "visible_via_copy":
				statusText = "available (copied)"
			case "broken_symlink":
				statusText = "broken link"
			case "unsupported_global":
				statusText = "global unsupported"
			case "agent_not_detected":
				statusText = "agent not detected"
			case "not_in_universal_canonical_dir":
				statusText = "not in shared folder"
			case "missing_agent_link":
				statusText = "not linked"
			}
			val := fmt.Sprintf("%s: %s", visibility.Display, statusText)
			if visibility.Path != "" {
				val += " at " + visibility.Path
			}
			return []string{formatMetaLine("Visibility:", val, width)}
		}
		return []string{formatMetaLine("Visibility:", "no compatibility data for "+m.agentLabel(), width)}
	}
	if view.CanonicalPath == "" {
		observedAgents := []string{}
		for _, p := range view.Observed {
			if p.Agent != "" {
				displayName := p.Agent
				for _, state := range m.result.Agents {
					if state.Name == p.Agent {
						displayName = state.Display
						break
					}
				}
				observedAgents = append(observedAgents, displayName)
			}
		}
		if len(observedAgents) > 0 {
			val := "Agent-specific: " + strings.Join(observedAgents, ", ")
			return []string{formatMetaLine("Visibility:", val, width)}
		}
	}
	detected := m.detectedAgentSet()
	visible := 0
	total := 0
	label := "agents"
	if len(detected) > 0 {
		label = "detected agents"
	}
	for _, visibility := range view.Visibility {
		if len(detected) > 0 && !detected[visibility.Agent] {
			continue
		}
		total++
		if visibility.Visible {
			visible++
		}
	}
	if total == 0 {
		label = "agents"
		total = len(view.Visibility)
		for _, visibility := range view.Visibility {
			if visibility.Visible {
				visible++
			}
		}
	}
	val := fmt.Sprintf("Available to %d/%d %s", visible, total, label)
	return []string{formatMetaLine("Visibility:", val, width)}
}

func (m appModel) detectedAgentSet() map[string]bool {
	out := map[string]bool{}
	for _, agent := range m.result.Agents {
		if agent.Detected {
			out[agent.Name] = true
		}
	}
	return out
}

func (m appModel) commandsOverlay(layout appLayout) string {
	modalWidth := 70
	if layout.Width < modalWidth+4 {
		modalWidth = layout.Width - 4
	}
	if modalWidth < 20 {
		modalWidth = 20
	}

	lines := m.commandPreview(nil, modalWidth-4)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(actionBorderColor).
		Padding(1, 2).
		Width(modalWidth).
		Render(strings.Join(lines, "\n"))

	return fitToScreen(lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, box), layout.Width, layout.Height)
}

func (m appModel) commandPreview(sk *model.Skill, width int) []string {
	title := " Actions "
	if count := m.selectedCount(); count > 0 {
		title = fmt.Sprintf(" Bulk actions · %d selected ", count)
	}
	lines := []string{actionTitleStyle.Render(title)}
	lines = append(lines, dimStyle.Render("  ↑/↓ choose · enter run · c/esc close"))
	if m.running {
		lines = append(lines, "", "  "+runningStyle.Render("Running action..."))
	}
	if m.confirming {
		lines = append(lines, "", "  "+errorStyle.Render("Confirmation pending"))
	}
	if m.actionResult != nil {
		lines = append(lines, "")
		lines = append(lines, m.renderActionResult(width)...)
	}
	for i, preview := range m.currentActions() {
		selector := "  "
		if i == m.action {
			selector = "› "
		}
		if !preview.Available {
			titleText := fmt.Sprintf("%s (unavailable)", compat.SanitizeMetadata(preview.Title))
			if i == m.action {
				titleLine := activeActionTitleStyle.Render(padRight(selector+titleText, width))
				lines = append(lines, "", titleLine)
				if preview.Reason != "" {
					reasonText := wrap(compat.SanitizeMetadata(preview.Reason), width-4)
					for _, reasonLine := range strings.Split(reasonText, "\n") {
						lines = append(lines, activeActionSubStyle.Render(padRight("  "+reasonLine, width)))
					}
				}
			} else {
				titleLine := normalActionSubStyle.Render(selector + titleText)
				lines = append(lines, "", titleLine)
				if preview.Reason != "" {
					reasonText := wrap(compat.SanitizeMetadata(preview.Reason), width-4)
					for _, reasonLine := range strings.Split(reasonText, "\n") {
						lines = append(lines, normalActionSubStyle.Render("  "+reasonLine))
					}
				}
			}
			continue
		}
		titleText := compat.SanitizeMetadata(preview.Title)
		if preview.Dangerous {
			titleText += " — removes skills"
		} else if preview.Mutates {
			titleText += " — changes skills"
		}
		if i == m.action {
			// Selected Action Highlight Block (entire block has same purple background)
			titleLine := activeActionTitleStyle.Render(padRight(selector+titleText, width))
			lines = append(lines, "", titleLine)

			cmdText := truncate(compat.SanitizeMetadata(preview.Command), width-4)
			cmdLine := activeActionSubStyle.Render(padRight("  "+cmdText, width))
			lines = append(lines, cmdLine)

		} else {
			// Unselected Action (normal colors, subordinate metadata very dim)
			titleLine := normalActionTitleStyle.Render(selector + titleText)
			lines = append(lines, "", titleLine)

			cmdText := truncate(compat.SanitizeMetadata(preview.Command), width-4)
			cmdLine := normalActionSubStyle.Render("  " + cmdText)
			lines = append(lines, cmdLine)
		}
	}
	return lines
}

func padRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func (m appModel) renderActionResult(width int) []string {
	if m.actionResult == nil {
		return nil
	}
	result := m.actionResult
	status := successStyle.Render("success")
	if result.ExitCode != 0 || result.Err != "" {
		status = errorStyle.Render("failed")
	}
	lines := []string{fmt.Sprintf("  Result: %s (exit %d)", status, result.ExitCode)}
	if result.Err != "" {
		lines = append(lines, indent(wrap(compat.SanitizeMetadata(result.Err), width-2), "  "))
	}
	if result.Stdout != "" {
		lines = append(lines, "  stdout:", fitLines(indent(wrapText(result.Stdout, width-4), "    "), 8))
	}
	if result.Stderr != "" {
		lines = append(lines, "  stderr:", fitLines(indent(wrapText(result.Stderr, width-4), "    "), 8))
	}
	if result.Truncated {
		lines = append(lines, dimStyle.Render("  output truncated"))
	}
	return lines
}

func (m appModel) footerText(width int) string {
	var text string
	if m.running {
		text = "Working…"
	} else if m.confirming {
		text = "type y/yes/phrase · enter confirm · esc cancel"
	} else if m.searching {
		text = "type search · enter apply · esc cancel · backspace edit"
	} else if m.detailModal {
		text = "↑/↓ scroll · o edit · c commands · esc/q close"
	} else if m.commands {
		text = "↑/↓ choose · enter run · esc close"
	} else if m.helpOpen {
		text = "esc/q/? close help"
	} else if m.focus == focusMetadata {
		text = "↑/↓ scroll metadata · enter open · c commands · o edit · ? help"
	} else if m.focus == focusPreview {
		text = "↑/↓ scroll preview · enter open · c commands · o edit · ? help"
	} else {
		// focusSkills
		text = "enter open · c commands · / search · f scope · a agent · r reload · ? help"
	}
	return dimStyle.Render(truncate(text, width))
}

func (m appModel) helpModalOverlay(layout appLayout) string {
	modalWidth := 74
	if layout.Width < modalWidth+4 {
		modalWidth = layout.Width - 4
	}
	if modalWidth < 20 {
		modalWidth = 20
	}

	sections := []string{
		titleStyle.Render(" LazySkills Keyboard Help "),
		"",
		sectionHeaderStyle.Render("Navigation & Focus:"),
		"  ↑/↓, j/k        Move selection (Skills focus) or scroll (Metadata/Preview)",
		"  1 / 2 / 3       Focus Skills (1), Metadata (2), or Preview (3) pane",
		"  tab / shift-tab Cycle focus forward / backward through panes",
		"  ← / →           Move focus backward / forward outside Skills; jump groups in Skills",
		"  h / l           Collapse / expand current source group in Skills",
		"  [ / ]           Jump to previous / next source group in Skills",
		"",
		sectionHeaderStyle.Render("Filters:"),
		"  P / G           Filter Project-only / Global-only scope",
		"  f / F           Cycle scope filter / Clear scope filter (All)",
		"  a / A           Cycle agent filter / Clear agent filter",
		"  /               Initiate text search",
		"",
		sectionHeaderStyle.Render("Actions & Selection:"),
		"  enter           Open scrollable read-only skill detail modal",
		"  space           Mark / unmark selected skill for bulk actions",
		"  s               Mark all skills in the current source group",
		"  o               Open selected skill directly in editor",
		"  c               Open command picker menu",
		"  u / x           Quick reinstall-update / remove for selection",
		"  r               Refresh scan snapshot",
		"",
		sectionHeaderStyle.Render("Safety & Modals:"),
		"  esc             Cancel action confirmation, search, picker, or modals",
		"  q               Close help modal, detail modal, or quit the app",
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(actionBorderColor).
		Padding(1, 2).
		Width(modalWidth).
		Render(strings.Join(sections, "\n"))

	return fitToScreen(lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, box), layout.Width, layout.Height)
}

func (m appModel) detailModalOverlay(layout appLayout) string {
	modalWidth := 80
	if layout.Width < modalWidth+4 {
		modalWidth = layout.Width - 4
	}
	if modalWidth < 20 {
		modalWidth = 20
	}
	modalHeight := 24
	if layout.Height < modalHeight+4 {
		modalHeight = layout.Height - 4
	}
	if modalHeight < 7 {
		modalHeight = 7
	}

	m.viewport.Width = modalWidth - 4
	m.viewport.Height = modalHeight - 6
	m.viewport.SetContent(m.detailText(modalWidth - 4))

	helpLine := dimStyle.Render("esc/q close · o open in editor · c command picker · ↑/↓ scroll")

	content := []string{
		titleStyle.Render("Skill Detail View"),
		"",
		m.viewport.View(),
		"",
		helpLine,
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(actionBorderColor).
		Padding(1, 2).
		Width(modalWidth).
		Height(modalHeight).
		Render(strings.Join(content, "\n"))

	return fitToScreen(lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, box), layout.Width, layout.Height)
}

func (m appModel) confirmationOverlay(layout appLayout) string {
	actions := m.currentActions()
	title := "Confirm action"
	phrase := ""
	command := ""
	if len(actions) > 0 && m.action < len(actions) {
		action := actions[m.action]
		title = compat.SanitizeMetadata(action.Title)
		phrase = compat.SanitizeMetadata(action.ConfirmValue)
		command = compat.SanitizeMetadata(action.Command)
	}
	lines := []string{
		errorStyle.Bold(true).Render("Confirm Action"),
		"",
		sectionHeaderStyle.Render("Action:"),
		wrapText(title, 48),
		"",
		sectionHeaderStyle.Render("Command:"),
		dimStyle.Render(wrapText(command, 48)),
		"",
	}
	if phrase == "yes" || phrase == "y" {
		lines = append(lines, "Type 'y' or 'yes' and press Enter to confirm.")
	} else if phrase != "" {
		lines = append(lines, "Type 'y', 'yes', or '"+phrase+"' and press Enter to confirm.")
	} else {
		lines = append(lines, "Type 'y' or 'yes' and press Enter to confirm.")
	}
	lines = append(lines, "Press Esc to cancel.")

	if m.confirmError != "" {
		lines = append(lines, "", errorStyle.Render(m.confirmError))
	}
	input := compat.SanitizeMetadata(m.confirmInput)
	if input == "" {
		placeholder := "y / yes"
		if phrase != "yes" && phrase != "y" && phrase != "" {
			placeholder = fmt.Sprintf("y / yes / %s", phrase)
		}
		input = dimStyle.Render(placeholder)
	}
	lines = append(lines, "", "> "+input+"_")
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(actionBorderColor).Padding(1, 2).Width(52).Render(strings.Join(lines, "\n"))
	return fitToScreen(lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, box), layout.Width, layout.Height)
}

func (m appModel) runningOverlay(layout appLayout) string {
	title := compat.SanitizeMetadata(firstNonEmpty(m.runningTitle, "Running action"))
	lines := []string{
		runningStyle.Render("Running"),
		wrapText(title, 44),
		"",
		"Working…",
		dimStyle.Render("Press q or Ctrl+C to quit LazySkills."),
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(actionBorderColor).Padding(1, 2).Width(52).Render(strings.Join(lines, "\n"))
	return fitToScreen(lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, box), layout.Width, layout.Height)
}

func (m appModel) filteredSkills() []*model.Skill {
	query := strings.ToLower(m.search)
	out := make([]*model.Skill, 0, len(m.result.Skills))
	for _, sk := range m.result.Skills {
		if m.filter == scopeProject && sk.Scope != model.ScopeProject {
			continue
		}
		if m.filter == scopeGlobal && sk.Scope != model.ScopeGlobal {
			continue
		}
		if m.agent != "" && !skillRelevantToAgent(sk, m.agent) {
			continue
		}
		if query != "" {
			view := display.Skill(sk)
			haystack := strings.ToLower(view.Name + " " + view.Description)
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		out = append(out, sk)
	}
	return out
}

func sortSkills(skills []*model.Skill) {
	sort.SliceStable(skills, func(i, j int) bool {
		leftGroup := listGroupLabel(skills[i])
		rightGroup := listGroupLabel(skills[j])
		if leftGroup != rightGroup {
			return leftGroup < rightGroup
		}
		left := strings.ToLower(display.Skill(skills[i]).Name)
		right := strings.ToLower(display.Skill(skills[j]).Name)
		if left != right {
			return left < right
		}
		return string(skills[i].Scope) < string(skills[j].Scope)
	})
}

func (m appModel) agentFilters() []string {
	var detected []string
	if len(m.result.Agents) == 0 {
		for _, agent := range agents.DetectInstalled(m.cwd) {
			if agent.Name == "universal" {
				continue
			}
			detected = append(detected, agent.Name)
		}
	} else {
		for _, agent := range m.result.Agents {
			if agent.Name == "universal" {
				continue
			}
			if agent.Detected {
				detected = append(detected, agent.Name)
			}
		}
	}
	sort.Strings(detected)
	return detected
}

func supportedAgentIDs() []string {
	ids := []string{}
	for _, agent := range agents.InitialAgents() {
		if agent.Name == "universal" {
			continue
		}
		ids = append(ids, agent.Name)
	}
	sort.Strings(ids)
	return ids
}

func (m appModel) nextAgentFilter() string {
	agents := m.agentFilters()
	if len(agents) == 0 {
		return ""
	}
	if m.agent == "" {
		return agents[0]
	}
	for i, agent := range agents {
		if agent == m.agent {
			if i == len(agents)-1 {
				return ""
			}
			return agents[i+1]
		}
	}
	return ""
}

func skillObservedByAgent(sk *model.Skill, agent string) bool {
	for _, observed := range sk.ObservedPaths {
		if compat.SanitizeMetadata(observed.Agent) == agent {
			return true
		}
	}
	return false
}

func skillRelevantToAgent(sk *model.Skill, agent string) bool {
	if skillObservedByAgent(sk, agent) {
		return true
	}
	if sk.CanonicalPath == "" {
		return false
	}
	for _, visibility := range sk.Visibility {
		if visibility.Agent == agent {
			return true
		}
	}
	return false
}

func agentVisibilityBadge(sk *model.Skill, agent string) string {
	for _, visibility := range sk.Visibility {
		if visibility.Agent != agent {
			continue
		}
		if visibility.Visible {
			return successStyle.Render("✓")
		}
		return "×"
	}
	if skillObservedByAgent(sk, agent) {
		return successStyle.Render("✓")
	}
	return "×"
}

func (m appModel) agentLabel() string {
	if m.agent == "" {
		if len(m.agentFilters()) == 0 {
			return "all (none detected)"
		}
		return "all"
	}
	for _, agent := range agents.InitialAgents() {
		if agent.Name == m.agent {
			return compat.SanitizeMetadata(agent.Display)
		}
	}
	return compat.SanitizeMetadata(m.agent)
}

func wrap(s string, width int) string {
	if width <= 8 || len(s) <= width {
		return s
	}
	words := strings.Fields(s)
	lines := []string{}
	current := ""
	for _, word := range words {
		if len(current)+len(word)+1 > width {
			lines = append(lines, current)
			current = word
		} else if current == "" {
			current = word
		} else {
			current += " " + word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

func wrapText(s string, width int) string {
	if width <= 1 {
		return ""
	}
	s = strings.ReplaceAll(s, "\t", "    ")
	return wordwrap.String(s, width)
}

func indent(s string, prefix string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func formatMetaLine(key, val string, width int) string {
	paddedKey := fmt.Sprintf("%-12s", key)
	wrappedVal := wrapText(val, max(1, width-13))
	indentedVal := indent(wrappedVal, strings.Repeat(" ", 13))
	indentedVal = strings.TrimPrefix(indentedVal, strings.Repeat(" ", 13))
	return metaKeyStyle.Render(paddedKey) + " " + indentedVal
}

func truncate(s string, width int) string {
	runes := []rune(s)
	if width <= 1 || len(runes) <= width {
		return s
	}
	return string(runes[:width-1]) + "…"
}

func fitLines(s string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= height {
		return s
	}
	return strings.Join(lines[:height], "\n")
}

func fitToScreen(s string, width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		for lipgloss.Width(line) > width {
			runes := []rune(line)
			if len(runes) == 0 {
				break
			}
			line = string(runes[:len(runes)-1])
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func viewWidth(width int) int {
	if width > 0 {
		return width
	}
	return 100
}

func viewHeight(height int) int {
	if height > 0 {
		return height
	}
	return 32
}

func newAppLayout(width, height int) appLayout {
	width = viewWidth(width)
	height = viewHeight(height)
	layout := appLayout{Width: width, Height: height}
	if width < minLayoutWidth || height < minLayoutHeight {
		layout.Small = true
		return layout
	}

	leftOuter, listOuter, detailOuter := paneOuterWidths(width)
	paneHeight := height
	layout.Left = newPaneLayout(leftOuter, paneHeight)
	layout.List = newPaneLayout(listOuter, paneHeight)
	layout.Detail = newPaneLayout(detailOuter, paneHeight)
	return layout
}

func newPaneLayout(outerWidth, outerHeight int) paneLayout {
	contentWidth := max(1, outerWidth-borderStyle.GetHorizontalFrameSize())
	contentHeight := max(1, outerHeight-borderStyle.GetVerticalFrameSize())
	return paneLayout{
		OuterWidth:    outerWidth,
		OuterHeight:   outerHeight,
		StyleWidth:    contentWidth + borderStyle.GetHorizontalPadding(),
		StyleHeight:   contentHeight + borderStyle.GetVerticalPadding(),
		ContentWidth:  contentWidth,
		ContentHeight: contentHeight,
	}
}

func paneStyle(p paneLayout, focused bool) lipgloss.Style {
	borderColor := lipgloss.Color("241")
	if focused {
		borderColor = actionBorderColor
	}
	return borderStyle.Copy().
		BorderForeground(borderColor).
		Width(p.StyleWidth).
		Height(p.StyleHeight).
		MaxWidth(p.OuterWidth).
		MaxHeight(p.OuterHeight)
}

func smallTerminalView(width, height int) string {
	message := "Terminal too small. Please resize."
	if height >= 2 && width >= 22 {
		message = "Terminal too small.\nPlease resize."
	}
	return fitToScreen(message, width, height)
}

func (m *appModel) clampViewportOffset() {
	maxOffset := max(0, m.viewport.TotalLineCount()-m.viewport.Height)
	if m.viewport.YOffset > maxOffset {
		m.viewport.SetYOffset(maxOffset)
	}
	if m.viewport.YOffset < 0 {
		m.viewport.SetYOffset(0)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func paneOuterWidths(total int) (left, list, detail int) {
	if total < 60 {
		left = max(16, total/4)
		list = max(20, total/3)
	} else {
		left = max(24, total/4)
		list = max(28, total/3)
	}
	if left+list > total-20 {
		left = max(12, total/5)
		list = max(18, total/3)
	}
	detail = total - left - list
	if detail < 20 {
		detail = 20
		list = max(12, total-left-detail)
	}
	if left+list+detail > total {
		detail = max(1, total-left-list)
	}
	return left, list, detail
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func decoratePane(rendered string, p paneLayout, focused bool, title string) string {
	if title == "" {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return rendered
	}

	borderColor := lipgloss.Color("241")
	if focused {
		borderColor = actionBorderColor
	}
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	formattedTitle := " " + title + " "
	titleWidth := lipgloss.Width(formattedTitle)

	leftCorner := "╭"
	leftLine := "─"
	rightCorner := "╮"

	totalWidth := p.OuterWidth

	rightLinesLen := totalWidth - 3 - titleWidth
	if rightLinesLen < 0 {
		// Truncate title
		maxTitleWidth := totalWidth - 5
		if maxTitleWidth > 0 {
			formattedTitle = " " + truncateTitle(title, maxTitleWidth) + " "
			titleWidth = lipgloss.Width(formattedTitle)
			rightLinesLen = totalWidth - 3 - titleWidth
		} else {
			formattedTitle = ""
			titleWidth = 0
			rightLinesLen = totalWidth - 2
		}
	}

	var styledTitle string
	if focused {
		styledTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Render(formattedTitle)
	} else {
		styledTitle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(formattedTitle)
	}

	var topLine string
	if rightLinesLen > 0 {
		topLine = borderStyle.Render(leftCorner) +
			borderStyle.Render(leftLine) +
			styledTitle +
			borderStyle.Render(strings.Repeat("─", rightLinesLen)) +
			borderStyle.Render(rightCorner)
	} else {
		topLine = borderStyle.Render(leftCorner) +
			borderStyle.Render(leftLine) +
			styledTitle +
			borderStyle.Render(rightCorner)
	}

	lines[0] = topLine
	return strings.Join(lines, "\n")
}

func truncateTitle(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
}
