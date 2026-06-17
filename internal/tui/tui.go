package tui

import (
	"fmt"
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
	"lazyskills/internal/scan"
)

type scopeFilter int

const (
	scopeAll scopeFilter = iota
	scopeProject
	scopeGlobal
)

type appModel struct {
	cwd       string
	result    model.ScanResult
	err       error
	selected  int
	filter    scopeFilter
	agent     string
	search    string
	searching bool
	commands  bool
	help      bool
	width     int
	height    int
	viewport  viewport.Model
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	borderStyle   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

type snapshotMsg struct {
	result model.ScanResult
	err    error
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
		m.err = msg.err
		m.clampSelection()
		m.syncViewport()
	case tea.KeyMsg:
		key := msg.String()
		if m.searching {
			switch key {
			case "esc", "enter":
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

		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "?":
			m.help = !m.help
		case "c":
			m.commands = !m.commands
		case "/":
			m.searching = true
		case "r":
			m.viewport.GotoTop()
			return m, loadSnapshot(m.cwd)
		case "a":
			m.agent = m.nextAgentFilter()
			m.selected = 0
			m.viewport.GotoTop()
		case "tab", "right", "l":
			m.filter = (m.filter + 1) % 3
			m.selected = 0
			m.viewport.GotoTop()
		case "shift+tab", "left", "h":
			m.filter = (m.filter + 2) % 3
			m.selected = 0
			m.viewport.GotoTop()
		case "down", "j":
			m.selected++
			m.viewport.GotoTop()
		case "up", "k":
			m.selected--
			m.viewport.GotoTop()
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
		m.syncViewport()
	}
	return m, nil
}

func (m *appModel) syncViewport() {
	_, _, detailOuter := paneOuterWidths(max(1, m.width))
	detailInner := max(1, detailOuter-4)
	innerHeight := max(1, max(8, m.height-2)-2)
	m.viewport.Width = detailInner
	m.viewport.Height = innerHeight
	m.viewport.SetContent(m.detailText(detailInner))
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

func (m appModel) View() string {
	if m.err != nil {
		return errorStyle.Render(fmt.Sprintf("LazySkills error: %s\n\nPress q to quit.", compat.SanitizeMetadata(m.err.Error())))
	}
	if m.width == 0 {
		m.width = 100
	}
	if m.height == 0 {
		m.height = 32
	}

	outerHeight := max(8, m.height-2)
	innerHeight := max(1, outerHeight-2)
	leftOuter, listOuter, detailOuter := paneOuterWidths(m.width)
	leftInner := max(1, leftOuter-4)
	listInner := max(1, listOuter-4)
	detailInner := max(1, detailOuter-4)

	leftStyle := borderStyle.Width(leftInner).MaxWidth(leftInner).Height(innerHeight).MaxHeight(innerHeight)
	listStyle := borderStyle.Width(listInner).MaxWidth(listInner).Height(innerHeight).MaxHeight(innerHeight)
	detailStyle := borderStyle.Width(detailInner).MaxWidth(detailInner).Height(innerHeight).MaxHeight(innerHeight)
	left := leftStyle.Render(fitLines(m.filterPane(), innerHeight))
	list := listStyle.Render(fitLines(m.listPane(innerHeight), innerHeight))
	detail := detailStyle.Render(m.detailPane(innerHeight, detailInner))
	footer := m.footer()
	return lipgloss.JoinHorizontal(lipgloss.Top, left, list, detail) + "\n" + footer
}

func (m appModel) filterPane() string {
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
			lines = append(lines, truncate(fmt.Sprintf("- %s: %s", compat.SanitizeMetadata(issue.Type), compat.SanitizeMetadata(issue.Message)), 36))
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
		lines = append(lines, "", "Keys", "↑/↓ j/k select", "tab scope", "a agent", "c commands", "/ search", "r refresh", "? help", "q quit")
	}
	return strings.Join(lines, "\n")
}

func filterLine(label string, active bool) string {
	if active {
		return selectedStyle.Render("› " + label)
	}
	return "  " + label
}

func (m appModel) listPane(height int) string {
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
	start := 0
	if m.selected >= visible {
		start = m.selected - visible + 1
	}
	end := min(len(items), start+visible)
	for i := start; i < end; i++ {
		view := display.Skill(items[i])
		label := fmt.Sprintf("%s [%s]", view.Name, view.Scope)
		if len(view.HealthIssues) > 0 {
			label += fmt.Sprintf(" !%d", len(view.HealthIssues))
		}
		if i == m.selected {
			lines = append(lines, selectedStyle.Render(truncate(label, 48)))
		} else {
			lines = append(lines, truncate(label, 48))
		}
	}
	return strings.Join(lines, "\n")
}

func (m appModel) detailPane(height, width int) string {
	if m.viewport.Width != width || m.viewport.Height != height || m.viewport.View() == "" {
		m.viewport.Width = width
		m.viewport.Height = height
		m.viewport.SetContent(m.detailText(width))
	}
	return m.viewport.View()
}

func (m appModel) detailText(width int) string {
	return strings.Join(m.detailLines(width), "\n")
}

func (m appModel) detailLines(width int) []string {
	items := m.filteredSkills()
	if len(items) == 0 {
		return []string{titleStyle.Render("Details"), "", dimStyle.Render("Select a skill to inspect it.")}
	}
	view := display.Skill(items[m.selected])
	lines := []string{
		titleStyle.Render(view.Name),
		wrapText(view.Description, width),
		"",
		fmt.Sprintf("Scope: %s", view.Scope),
		wrapText(fmt.Sprintf("Lock: %s", display.LockSummary(view)), width),
	}
	if view.CanonicalPath != "" {
		lines = append(lines, wrapText("Canonical: "+view.CanonicalPath, width))
	}
	if m.agent != "" {
		lines = append(lines, "Agent filter: "+m.agentLabel())
	}
	lines = append(lines, "", "Observed")
	for _, p := range view.Observed {
		line := fmt.Sprintf("- %s %s %s", p.Agent, p.Scope, p.Status)
		if p.TargetPath != "" {
			line += " → " + p.TargetPath
		}
		lines = append(lines, wrapText(line, width))
	}
	if len(view.HealthIssues) == 0 {
		// no-op
	} else {
		lines = append(lines, "", errorStyle.Render("Health"))
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
		lines = append(lines, m.commandPreview(items[m.selected], width-4)...)
		return lines
	}
	if view.Preview != "" {
		lines = append(lines, "", "Preview")
		previewLines := strings.Split(view.Preview, "\n")
		for _, line := range previewLines {
			lines = append(lines, wrapText(line, width))
		}
	}
	return lines
}

func (m appModel) commandPreview(sk *model.Skill, width int) []string {
	lines := []string{titleStyle.Render("Command previews")}
	lines = append(lines, dimStyle.Render("Preview only. LazySkills will not run these commands yet."))
	for _, preview := range actions.ForSkill(sk) {
		if !preview.Available {
			lines = append(lines, "", fmt.Sprintf("%s (unavailable)", compat.SanitizeMetadata(preview.Title)))
			lines = append(lines, wrap(compat.SanitizeMetadata(preview.Reason), width))
			continue
		}
		marker := "read-only"
		if preview.Mutates {
			marker = "mutates"
		}
		lines = append(lines, "", fmt.Sprintf("%s (%s)", compat.SanitizeMetadata(preview.Title), marker))
		lines = append(lines, truncate(compat.SanitizeMetadata(preview.Command), width))
		if preview.Description != "" {
			lines = append(lines, wrap(compat.SanitizeMetadata(preview.Description), width))
		}
	}
	return lines
}

func (m appModel) footer() string {
	mode := ""
	if m.searching {
		mode = " search mode: type to filter, esc/enter to leave"
	} else if m.commands {
		mode = " command previews: c to hide"
	}
	return dimStyle.Render("LazySkills is read-only." + mode)
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
		if m.agent != "" && !skillObservedByAgent(sk, m.agent) {
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

func (m appModel) agentFilters() []string {
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
