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
	"lazyskills/internal/visibility"
)

type scopeFilter int

const (
	scopeAll scopeFilter = iota
	scopeProject
	scopeGlobal
)

type appModel struct {
	cwd          string
	result       model.ScanResult
	err          error
	selected     int
	filter       scopeFilter
	agent        string
	search       string
	searching    bool
	commands     bool
	selectedKeys map[string]bool
	help         bool
	action       int
	confirming   bool
	confirmInput string
	confirmError string
	running      bool
	runningTitle string
	actionResult *runner.Result
	width        int
	height       int
	viewport     viewport.Model
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
	runExec       = runner.OSRunner{}.Run

	// Action Mode UI Polish Styles
	actionTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	activeActionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	actionNormalStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	actionBorderColor = lipgloss.Color("62")
	runningStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
	successStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)

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
	return appModel{cwd: cwd, help: true, viewport: viewport.New(0, 0)}
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

		switch key {
		case "esc":
			if m.selectedCount() > 0 {
				m.selectedKeys = nil
				m.action = 0
				m.actionResult = nil
			} else if m.commands {
				m.commands = false
			} else if m.agent != "" {
				m.agent = ""
				m.selected = 0
				m.action = 0
				m.actionResult = nil
				m.viewport.GotoTop()
			}
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.help = !m.help
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
		case "o":
			return m.startCurrentSkillActionByID("open_skill")
		case "u":
			return m.startActionByID(preferredUpdateActionID(m.selectedCount()))
		case "x":
			return m.startActionByID(preferredRemoveActionID(m.selectedCount()))
		case "/":
			m.searching = true
		case "enter":
			if m.commands {
				return m.startAction()
			}
		case "r":
			m.viewport.GotoTop()
			return m, loadSnapshot(m.cwd)
		case "a":
			m.agent = m.nextAgentFilter()
			m.selected = 0
			m.action = 0
			m.actionResult = nil
			m.viewport.GotoTop()
		case "A":
			m.agent = ""
			m.selected = 0
			m.action = 0
			m.actionResult = nil
			m.viewport.GotoTop()
		case "tab":
			m.filter = (m.filter + 1) % 3
			m.selected = 0
			m.actionResult = nil
			m.viewport.GotoTop()
		case "shift+tab":
			m.filter = (m.filter + 2) % 3
			m.selected = 0
			m.actionResult = nil
			m.viewport.GotoTop()
		case "right", "l":
			m.jumpSourceGroup(1)
		case "left", "h":
			m.jumpSourceGroup(-1)
		case "down", "j":
			if m.commands {
				m.action++
			} else {
				m.selected++
				m.actionResult = nil
				m.viewport.GotoTop()
			}
		case "up", "k":
			if m.commands {
				m.action--
			} else {
				m.selected--
				m.actionResult = nil
				m.viewport.GotoTop()
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
			m.viewport.GotoTop()
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
	items := m.filteredSkills()
	if len(items) == 0 || m.selected >= len(items) {
		return nil
	}
	return actions.ForSkillWithContext(items[m.selected], m.actionContext(items[m.selected]), nil)
}

func (m appModel) actionContext(sk *model.Skill) actions.ActionContext {
	ctx := actions.ActionContext{Cwd: m.cwd, ActiveAgent: m.agent}
	if sk == nil || m.agent == "" {
		return ctx
	}
	statePath, err := visibility.StatePath(m.cwd, sk.Scope)
	if err != nil {
		return ctx
	}
	state, err := visibility.ReadState(statePath)
	if err == nil {
		ctx.DisabledState = state
	}
	return ctx
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
	items := m.filteredSkills()
	if len(items) == 0 || m.selected >= len(items) {
		return m, nil
	}
	for _, action := range actions.ForSkill(items[m.selected]) {
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
	return value == "" || value == "y" || value == "yes" || input == confirmValue
}

func (m appModel) executeAction(action actions.CommandPreview) (tea.Model, tea.Cmd) {
	if action.Exec.Internal == "refresh" {
		m.actionResult = nil
		return m, loadSnapshot(m.cwd)
	}
	if action.Exec.Internal == "disable_skill" || action.Exec.Internal == "enable_skill" {
		items := m.filteredSkills()
		if len(items) == 0 || m.selected >= len(items) {
			return m, nil
		}
		skill := items[m.selected]
		agent := m.agent
		internal := action.Exec.Internal
		m.running = true
		m.runningTitle = action.Title
		m.actionResult = nil
		m.confirming = false
		m.confirmInput = ""
		m.confirmError = ""
		m.syncViewport()
		return m, func() tea.Msg {
			result := runner.Result{Program: "lazyskills", Args: []string{strings.TrimSuffix(internal, "_skill"), "--agent", agent, skill.Name}, Cwd: m.cwd, ExitCode: 0}
			var err error
			if internal == "disable_skill" {
				err = visibility.Disable(m.cwd, skill, agent, skill.Scope)
			} else {
				err = visibility.Enable(m.cwd, skill, agent, skill.Scope)
			}
			if err != nil {
				result.ExitCode = -1
				result.Err = err.Error()
			}
			return actionResultMsg{result: result, mutates: action.Mutates}
		}
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
		return
	}
	m.viewport.Width = layout.Detail.ContentWidth
	m.viewport.Height = layout.Detail.ContentHeight
	m.viewport.SetContent(m.detailText(layout.Detail.ContentWidth))
	m.clampViewportOffset()
}

func (m *appModel) clampSelection() {
	items := m.filteredSkills()
	if len(items) == 0 {
		m.selected = 0
		return
	}
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(items) {
		m.selected = len(items) - 1
	}
}

func (m *appModel) toggleSelectedSkill() {
	items := m.filteredSkills()
	if len(items) == 0 || m.selected >= len(items) {
		return
	}
	if m.selectedKeys == nil {
		m.selectedKeys = map[string]bool{}
	}
	key := skillKey(items[m.selected])
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
	items := m.filteredSkills()
	if len(items) == 0 || m.selected >= len(items) {
		return
	}
	currentGroup := sourceGroupKey(items[m.selected])
	if currentGroup == "" {
		return
	}
	if m.selectedKeys == nil {
		m.selectedKeys = map[string]bool{}
	}
	changed := false
	for _, skill := range items {
		if sourceGroupKey(skill) == currentGroup {
			m.selectedKeys[skillKey(skill)] = true
			changed = true
		}
	}
	if !changed && len(m.selectedKeys) == 0 {
		m.selectedKeys = nil
	}
}

func (m *appModel) jumpSourceGroup(direction int) {
	items := m.filteredSkills()
	if len(items) == 0 || direction == 0 {
		return
	}
	m.clampSelection()
	starts := sourceGroupStartIndexes(items)
	if len(starts) <= 1 {
		return
	}
	currentGroup := 0
	for i, start := range starts {
		if start <= m.selected {
			currentGroup = i
		}
	}
	if direction > 0 {
		currentGroup = (currentGroup + 1) % len(starts)
	} else {
		currentGroup = (currentGroup + len(starts) - 1) % len(starts)
	}
	m.selected = starts[currentGroup]
	m.actionResult = nil
	m.viewport.GotoTop()
}

func sourceGroupStartIndexes(items []*model.Skill) []int {
	starts := []int{}
	previousGroup := ""
	for i, skill := range items {
		group := listGroupLabel(skill)
		if i == 0 || group != previousGroup {
			starts = append(starts, i)
			previousGroup = group
		}
	}
	return starts
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
	return "No source metadata"
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

	leftStyle := paneStyle(layout.Left)
	listStyle := paneStyle(layout.List)
	detailStyle := paneStyle(layout.Detail)
	if viewModel.commands {
		detailStyle = detailStyle.BorderForeground(actionBorderColor)
	}
	left := leftStyle.Render(fitLines(viewModel.filterPane(layout.Left.ContentWidth), layout.Left.ContentHeight))
	list := listStyle.Render(fitLines(viewModel.listPane(layout.List.ContentHeight, layout.List.ContentWidth), layout.List.ContentHeight))
	detail := detailStyle.Render(viewModel.detailPane())
	footer := fitToScreen(viewModel.footer(), layout.Width, 1)
	view := lipgloss.JoinHorizontal(lipgloss.Top, left, list, detail) + "\n" + footer
	if viewModel.running {
		return viewModel.runningOverlay(layout)
	}
	if viewModel.confirming {
		return viewModel.confirmationOverlay(layout)
	}
	return view
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
		filterLine(fmt.Sprintf("Project (%d)", counts[model.ScopeProject]), m.filter == scopeProject),
		filterLine(fmt.Sprintf("Global (%d)", counts[model.ScopeGlobal]), m.filter == scopeGlobal),
		"",
		fmt.Sprintf("Skills: %d", len(m.result.Skills)),
		fmt.Sprintf("Issues: %d", issues),
		fmt.Sprintf("Agent: %s", m.agentLabel()),
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
		lines = append(lines, "", "Keys", "↑/↓ j/k select", "space mark", "s source mark", "o open", "u update", "x remove", "tab scope", "a agent", "A all agents", "c actions", "/ search", "r refresh", "? help", "q quit")
	}
	return strings.Join(lines, "\n")
}

func filterLine(label string, active bool) string {
	if active {
		return selectedStyle.Render("› " + label)
	}
	return "  " + label
}

func (m appModel) listPane(height, width int) string {
	items := m.filteredSkills()
	lines := []string{titleStyle.Render("Skills")}
	if len(items) == 0 {
		detail := "No skills match."
		if m.agent != "" {
			detail += fmt.Sprintf(" %s has no visible skills for this view.", m.agentLabel())
		}
		if m.search != "" {
			detail += " Clear search with backspace."
		}
		return strings.Join(append(lines, "", dimStyle.Render(detail)), "\n")
	}
	visible := max(1, height-3)
	rows := []listRow{}
	previousGroup := ""
	selectedRow := 0
	for i := 0; i < len(items); i++ {
		skill := items[i]
		view := display.Skill(skill)
		if group := listGroupLabel(skill); group != previousGroup {
			rows = append(rows, listRow{line: dimStyle.Render(truncate("─ "+group, width)), skillIndex: -1})
			previousGroup = group
		}
		mark := "  "
		if m.isSelected(skill) {
			mark = "● "
		}
		label := fmt.Sprintf("%s%s [%s]", mark, view.Name, view.Scope)
		if m.agent != "" {
			label += " " + agentVisibilityBadge(items[i], m.agent)
		}
		if len(view.HealthIssues) > 0 {
			label += fmt.Sprintf(" ⚠ %d", len(view.HealthIssues))
		}
		var line string
		if i == m.selected {
			line = selectedStyle.Render(truncate(label, width))
		} else if len(view.HealthIssues) > 0 {
			line = errorStyle.Render(truncate(label, width))
		} else {
			line = truncate(label, width)
		}
		if i == m.selected {
			selectedRow = len(rows)
		}
		rows = append(rows, listRow{line: line, skillIndex: i})
	}
	start := 0
	if selectedRow >= visible {
		start = selectedRow - visible + 1
	}
	end := min(len(rows), start+visible)
	for _, row := range rows[start:end] {
		lines = append(lines, row.line)
	}
	return strings.Join(lines, "\n")
}

type listRow struct {
	line       string
	skillIndex int
}

func (m appModel) detailPane() string {
	return m.viewport.View()
}

func (m appModel) detailText(width int) string {
	return strings.Join(m.detailLines(width), "\n")
}

func (m appModel) detailLines(width int) []string {
	items := m.filteredSkills()
	if len(items) == 0 {
		lines := []string{titleStyle.Render("Details"), "", dimStyle.Render("Select a skill to inspect it.")}
		if m.commands && m.selectedCount() > 0 {
			lines = append(lines, "")
			divider := lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render(strings.Repeat("─", max(1, width)))
			lines = append(lines, divider)
			lines = append(lines, m.commandPreview(nil, max(1, width-4))...)
		}
		return lines
	}
	view := display.Skill(items[m.selected])
	lines := []string{
		titleStyle.Render(view.Name),
		wrapText(view.Description, width),
		"",
		sectionHeaderStyle.Render("Metadata"),
		formatMetaLine("Scope:", string(view.Scope), width),
		formatMetaLine("Lock:", display.LockSummary(view), width),
	}
	if sourceLines := sourceDetailLines(items[m.selected], width); len(sourceLines) > 0 {
		lines = append(lines, sourceLines...)
	}
	if view.CanonicalPath != "" {
		lines = append(lines, formatMetaLine("Canonical:", view.CanonicalPath, width))
	}
	if m.agent != "" {
		lines = append(lines, formatMetaLine("Agent:", m.agentLabel(), width))
	}
	lines = append(lines, m.visibilitySummary(view, width)...)

	if len(view.Observed) > 0 {
		lines = append(lines, "", sectionHeaderStyle.Render("Observed Paths"))
		for _, p := range view.Observed {
			line := fmt.Sprintf("- %s %s %s", p.Agent, p.Scope, p.Status)
			if p.TargetPath != "" {
				line += " → " + p.TargetPath
			}
			lines = append(lines, wrapText(line, width))
		}
	}

	if len(view.HealthIssues) > 0 {
		lines = append(lines, "", healthHeaderStyle.Render("Health Issues"))
		for _, issue := range view.HealthIssues {
			line := fmt.Sprintf("- %s: %s", issue.Type, issue.Message)
			if issue.Path != "" {
				line += " (" + issue.Path + ")"
			}
			lines = append(lines, wrapText(line, width))
		}
	}

	if m.commands {
		lines = append(lines, "")
		divider := lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Render(strings.Repeat("─", max(1, width)))
		lines = append(lines, divider)
		lines = append(lines, m.commandPreview(items[m.selected], max(1, width-4))...)
		return lines
	}
	if view.Preview != "" {
		lines = append(lines, "")
		previewDivider := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(strings.Repeat("─", max(1, width)))
		lines = append(lines, previewDivider)
		lines = append(lines, sectionHeaderStyle.Render("Preview"), "")
		previewLines := strings.Split(view.Preview, "\n")
		for _, line := range previewLines {
			lines = append(lines, wrapText(line, width))
		}
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
			prefix := "cannot see"
			if visibility.Visible {
				prefix = "can see"
			}
			val := fmt.Sprintf("%s %s (%s)", visibility.Display, prefix, visibility.Reason)
			if visibility.Path != "" {
				val += " at " + visibility.Path
			}
			return []string{formatMetaLine("Visibility:", val, width)}
		}
		return []string{formatMetaLine("Visibility:", "no compatibility data for "+m.agentLabel(), width)}
	}
	visible := 0
	for _, visibility := range view.Visibility {
		if visibility.Visible {
			visible++
		}
	}
	val := fmt.Sprintf("%d/%d supported agents", visible, len(view.Visibility))
	return []string{formatMetaLine("Visibility:", val, width)}
}

func (m appModel) commandPreview(sk *model.Skill, width int) []string {
	title := " Actions "
	if count := m.selectedCount(); count > 0 {
		title = fmt.Sprintf(" Bulk actions · %d selected ", count)
	}
	lines := []string{actionTitleStyle.Render(title)}
	lines = append(lines, dimStyle.Render("  j/k choose, enter run, o/u/x shortcuts, c hide"))
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
				lines = append(lines, "", activeActionStyle.Render(selector+titleText))
			} else {
				lines = append(lines, "", dimStyle.Render(selector+titleText))
			}
			if preview.Reason != "" {
				lines = append(lines, indent(wrap(compat.SanitizeMetadata(preview.Reason), width-2), "  "))
			}
			continue
		}
		marker := "read-only"
		if preview.Mutates {
			marker = "mutates"
		}
		if preview.Dangerous {
			marker += ", dangerous"
		}

		titleText := fmt.Sprintf("%s (%s)", compat.SanitizeMetadata(preview.Title), marker)
		if i == m.action {
			lines = append(lines, "", activeActionStyle.Render(selector+titleText))
			lines = append(lines, "  "+actionNormalStyle.Render(truncate(compat.SanitizeMetadata(preview.Command), width-2)))
			if preview.Description != "" {
				lines = append(lines, indent(wrap(compat.SanitizeMetadata(preview.Description), width-2), "  "))
			}
		} else {
			lines = append(lines, "", selector+titleText)
			lines = append(lines, dimStyle.Render("  "+truncate(compat.SanitizeMetadata(preview.Command), width-2)))
			if preview.Description != "" {
				lines = append(lines, indent(dimStyle.Render(wrap(compat.SanitizeMetadata(preview.Description), width-2)), "  "))
			}
		}
	}
	return lines
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

func (m appModel) confirmationOverlay(layout appLayout) string {
	actions := m.currentActions()
	title := "Confirm action"
	phrase := ""
	if len(actions) > 0 && m.action < len(actions) {
		action := actions[m.action]
		title = compat.SanitizeMetadata(action.Title)
		phrase = compat.SanitizeMetadata(action.ConfirmValue)
	}
	lines := []string{
		errorStyle.Bold(true).Render("Confirm"),
		wrapText(title, 44),
		"",
		"Press Enter or y to confirm.",
		"Type n or Esc to cancel.",
	}
	if phrase != "" {
		lines = append(lines, dimStyle.Render("Also accepted: "+phrase))
	}
	if m.confirmError != "" {
		lines = append(lines, "", errorStyle.Render(m.confirmError))
	}
	input := compat.SanitizeMetadata(m.confirmInput)
	if input == "" {
		input = dimStyle.Render("yes")
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

func (m appModel) footer() string {
	mode := ""
	selection := ""
	if count := m.selectedCount(); count > 0 {
		selection = fmt.Sprintf(" %s", lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true).Render(fmt.Sprintf("%d selected", count))) + " : space toggle, esc clear"
	}
	if m.searching {
		mode = " " + lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true).Render("SEARCH MODE") + " : type to filter, esc/enter to leave"
	} else if m.commands {
		mode = " " + lipgloss.NewStyle().Foreground(lipgloss.Color("117")).Bold(true).Render("ACTION MODE") + " : j/k choose, enter run, c hide"
	}
	return dimStyle.Render("LazySkills "+appVersion) + selection + mode
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
	observed := map[string]bool{}
	for _, skill := range m.result.Skills {
		for _, path := range skill.ObservedPaths {
			observed[path.Agent] = true
		}
	}
	if len(m.result.Agents) == 0 {
		ids := make([]string, 0, len(observed))
		for agent := range observed {
			if agent != "" && agent != "universal" {
				ids = append(ids, agent)
			}
		}
		sort.Strings(ids)
		if len(ids) > 0 {
			return ids
		}
		return supportedAgentIDs()
	}
	detected, rest := []string{}, []string{}
	for _, agent := range m.result.Agents {
		if agent.Name == "universal" {
			continue
		}
		if agent.Detected || observed[agent.Name] {
			detected = append(detected, agent.Name)
		} else {
			rest = append(rest, agent.Name)
		}
	}
	sort.Strings(detected)
	sort.Strings(rest)
	if len(detected) > 0 {
		return detected
	}
	return append(detected, rest...)
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
			return "✓ visible"
		}
		return "× " + compat.SanitizeMetadata(visibility.Reason)
	}
	if skillObservedByAgent(sk, agent) {
		return "✓ observed"
	}
	return "× no data"
}

func (m appModel) agentLabel() string {
	if m.agent == "" {
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
	paneHeight := height - 1 // reserve one row for the footer
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

func paneStyle(p paneLayout) lipgloss.Style {
	return borderStyle.
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
