package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/model"
	"github.com/alvinunreal/lazyskills/internal/runner"
)

type visibilityRepairRow struct {
	Agent       string
	Display     string
	Visible     bool
	Reason      string
	Source      string
	Destination string
	Strategy    string
	Detected    bool
	Supported   bool
	Fixable     bool
	Selected    bool
	Note        string
}

type visibilityRepairExec struct {
	Agent       string
	Source      string
	Destination string
	Strategy    string
}

func (m appModel) currentActionSkill() (*model.Skill, bool) {
	if m.modalSource != "" {
		if child, ok := m.currentModalSelectedChild(); ok && !child.isAvailable {
			return child.skill, true
		}
	}
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		return nil, false
	}
	row := rows[m.selected]
	if row.isHeader {
		return nil, false
	}
	return row.skill, true
}

func (m appModel) visibilityRepairSkill() *model.Skill {
	if m.visibilityRepairSkillKey == "" {
		return nil
	}
	for _, skill := range m.result.Skills {
		if skillKey(skill) == m.visibilityRepairSkillKey {
			return skill
		}
	}
	return nil
}

func (m appModel) appendVisibilityRepairActions(previews []actions.CommandPreview, sk *model.Skill) []actions.CommandPreview {
	if sk == nil {
		return previews
	}
	repair := m.visibilityRepairActionPreview(sk)
	insertAt := len(previews)
	for i, preview := range previews {
		if preview.ID == "reinstall_update" || preview.ID == "remove" || preview.ID == "prune_lock" {
			insertAt = i
			break
		}
	}
	out := make([]actions.CommandPreview, 0, len(previews)+1)
	out = append(out, previews[:insertAt]...)
	out = append(out, repair)
	out = append(out, previews[insertAt:]...)
	return out
}

func (m appModel) visibilityRepairActionPreview(sk *model.Skill) actions.CommandPreview {
	rows := m.visibilityRepairRows(sk)
	fixable := 0
	for _, row := range rows {
		if row.Fixable {
			fixable++
		}
	}
	preview := actions.CommandPreview{
		ID:          "repair_visibility_wizard",
		Title:       "Fix visibility…",
		Description: "Open the visibility repair wizard.",
		Command:     "open visibility repair wizard",
		Exec:        actions.ExecSpec{Internal: "open_visibility_repair"},
		Available:   fixable > 0,
	}
	if fixable == 0 {
		preview.Reason = m.visibilityRepairUnavailableReason(rows)
		preview.Command = "no visibility repair available"
		return preview
	}
	preview.Description = fmt.Sprintf("Open the visibility repair wizard (%d fixable target%s).", fixable, pluralize(fixable))
	return preview
}

func (m appModel) visibilityRepairUnavailableReason(rows []visibilityRepairRow) string {
	if len(rows) == 0 {
		return "No visibility data available for this skill."
	}
	parts := []string{}
	for _, row := range rows {
		if row.Fixable {
			return ""
		}
		if row.Note != "" {
			parts = append(parts, row.Display+": "+row.Note)
		}
	}
	if len(parts) == 0 {
		return "No actionable visibility gaps can be repaired safely."
	}
	return strings.Join(parts, "; ")
}

func (m appModel) visibilityRepairRows(skill *model.Skill) []visibilityRepairRow {
	if skill == nil || skill.CanonicalPath == "" {
		return nil
	}
	agentStates := map[string]model.AgentState{}
	for _, state := range m.result.Agents {
		agentStates[state.Name] = state
	}
	strategy := visibilityRepairPreferredStrategy(skill)
	base := filepath.Base(skill.CanonicalPath)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return nil
	}
	rows := make([]visibilityRepairRow, 0, len(skill.Visibility))
	for _, vis := range skill.Visibility {
		row := visibilityRepairRow{
			Agent:     vis.Agent,
			Display:   compat.SanitizeMetadata(compat.FirstNonEmpty(vis.Display, vis.Agent)),
			Visible:   vis.Visible,
			Reason:    vis.Reason,
			Source:    skill.CanonicalPath,
			Strategy:  strategy,
			Selected:  false,
			Fixable:   false,
			Detected:  false,
			Supported: false,
		}
		state, ok := agentStates[vis.Agent]
		if ok {
			row.Detected = state.Detected
			row.Supported = state.Supported
			root := visibilityRepairTargetRoot(skill, state)
			if root != "" {
				row.Destination = filepath.Join(root, base)
			}
		}
		switch vis.Reason {
		case "visible_via_universal_canonical", "visible_via_canonical", "visible_via_symlink", "visible_via_copy", "visible":
			row.Note = "already visible"
		case "missing_agent_link":
			row.Note = "create " + row.Strategy + " link"
			row.Fixable = row.Destination != "" && row.Source != "" && row.Destination != row.Source && row.Detected && row.Supported
		case "not_in_universal_canonical_dir":
			row.Note = "place skill in the universal canonical directory"
			row.Fixable = row.Destination != "" && row.Source != "" && row.Destination != row.Source && row.Detected
		case "unsupported_global":
			row.Note = "global unsupported"
		case "agent_not_detected":
			row.Note = "agent not detected"
		case "broken_symlink":
			row.Note = "broken symlink requires manual cleanup"
		default:
			row.Note = row.Reason
		}
		if row.Fixable && visibilityPathExists(row.Destination) {
			row.Fixable = false
			row.Note = "destination already exists"
		}
		rows = append(rows, row)
	}
	return rows
}

func (m appModel) visibilityRepairSelectedRows(skill *model.Skill) []visibilityRepairRow {
	rows := m.visibilityRepairRows(skill)
	if len(rows) == 0 || len(m.visibilityRepairTargets) == 0 {
		return nil
	}
	out := make([]visibilityRepairRow, 0, len(rows))
	for _, row := range rows {
		if row.Fixable && m.visibilityRepairTargets[row.Agent] {
			row.Selected = true
			out = append(out, row)
		}
	}
	return out
}

func (m appModel) visibilityRepairSelectionPreview() (actions.CommandPreview, bool) {
	skill := m.visibilityRepairSkill()
	if skill == nil {
		return actions.CommandPreview{}, false
	}
	selected := m.visibilityRepairSelectedRows(skill)
	if len(selected) == 0 {
		return actions.CommandPreview{}, false
	}
	return visibilityRepairApplyPreview(skill, selected), true
}

func visibilityRepairApplyPreview(skill *model.Skill, rows []visibilityRepairRow) actions.CommandPreview {
	if skill == nil || len(rows) == 0 {
		return actions.CommandPreview{}
	}
	args := make([]string, 0, len(rows)*4)
	commands := make([]string, 0, len(rows))
	details := []string{fmt.Sprintf("Repair visibility for %s", compat.SanitizeMetadata(skill.Name))}
	for _, row := range rows {
		args = append(args, row.Agent, row.Source, row.Destination, row.Strategy)
		commands = append(commands, visibilityRepairCommandLine(row))
		details = append(details, fmt.Sprintf("%s: %s", row.Display, visibilityRepairCommandSummary(row)))
	}
	return actions.CommandPreview{
		ID:              "repair_visibility",
		Title:           fmt.Sprintf("Repair visibility for %d agent%s", len(rows), pluralize(len(rows))),
		Description:     strings.Join(details, "\n"),
		Command:         strings.Join(commands, " && "),
		Exec:            actions.ExecSpec{Internal: "repair_visibility", Args: args},
		Mutates:         true,
		RequiresConfirm: true,
		Dangerous:       true,
		ConfirmValue:    "yes",
		Available:       true,
	}
}

func pluralize(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func visibilityRepairCommandSummary(row visibilityRepairRow) string {
	if row.Strategy == "copy" {
		return fmt.Sprintf("copy %s -> %s", row.Source, row.Destination)
	}
	return fmt.Sprintf("symlink %s -> %s", row.Source, row.Destination)
}

func visibilityRepairCommandLine(row visibilityRepairRow) string {
	if row.Strategy == "copy" {
		return fmt.Sprintf("cp -R %s %s", shellQuoteVisibility(row.Source), shellQuoteVisibility(row.Destination))
	}
	return fmt.Sprintf("ln -s %s %s", shellQuoteVisibility(row.Source), shellQuoteVisibility(row.Destination))
}

func shellQuoteVisibility(value string) string {
	value = compat.SanitizeMetadata(value)
	if value == "" {
		return "''"
	}
	if strings.ContainsAny(value, " \t\n'\"$`\\!*?[]{}()&;<>|#") {
		return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	return value
}

func (m appModel) beginVisibilityRepair(skill *model.Skill) (tea.Model, tea.Cmd) {
	rows := m.visibilityRepairRows(skill)
	if len(rows) == 0 {
		return m, nil
	}
	selected := map[string]bool{}
	selectedIndex := 0
	for i, row := range rows {
		if row.Fixable {
			selected[row.Agent] = true
			if selectedIndex == 0 {
				selectedIndex = i
			}
		}
	}
	if len(selected) == 0 {
		return m, nil
	}
	m.commands = false
	m.modalSource = ""
	m.detailModal = false
	m.confirming = false
	m.confirmInput = ""
	m.confirmError = ""
	m.pendingAction = nil
	m.actionResult = nil
	m.visibilityRepairModal = true
	m.visibilityRepairSkillKey = skillKey(skill)
	m.visibilityRepairTargets = selected
	m.visibilityRepairSelected = selectedIndex
	m.syncViewport()
	return m, nil
}

func (m appModel) closeVisibilityRepair() appModel {
	m.visibilityRepairModal = false
	m.visibilityRepairSkillKey = ""
	m.visibilityRepairTargets = nil
	m.visibilityRepairSelected = 0
	return m
}

func (m appModel) applyVisibilityRepairPreview() (tea.Model, tea.Cmd) {
	preview, ok := m.visibilityRepairSelectionPreview()
	if !ok || !preview.Available {
		return m, nil
	}
	m.pendingAction = &preview
	m.confirming = true
	m.confirmInput = ""
	m.confirmError = ""
	m.syncViewport()
	return m, nil
}

func (m appModel) toggleVisibilityRepairSelection() appModel {
	skill := m.visibilityRepairSkill()
	if skill == nil {
		return m
	}
	rows := m.visibilityRepairRows(skill)
	if len(rows) == 0 || m.visibilityRepairSelected < 0 || m.visibilityRepairSelected >= len(rows) {
		return m
	}
	row := rows[m.visibilityRepairSelected]
	if !row.Fixable {
		return m
	}
	if m.visibilityRepairTargets == nil {
		m.visibilityRepairTargets = map[string]bool{}
	}
	m.visibilityRepairTargets[row.Agent] = !m.visibilityRepairTargets[row.Agent]
	return m
}

func (m appModel) moveVisibilityRepairSelection(delta int) appModel {
	skill := m.visibilityRepairSkill()
	if skill == nil {
		return m
	}
	rows := m.visibilityRepairRows(skill)
	if len(rows) == 0 {
		return m
	}
	m.visibilityRepairSelected += delta
	if m.visibilityRepairSelected < 0 {
		m.visibilityRepairSelected = 0
	}
	if m.visibilityRepairSelected >= len(rows) {
		m.visibilityRepairSelected = len(rows) - 1
	}
	return m
}

func (m appModel) visibilityRepairModalHelpLine() string {
	skill := m.visibilityRepairSkill()
	rows := m.visibilityRepairRows(skill)
	if len(rows) == 0 {
		return "esc/q close"
	}
	selected := m.visibilityRepairSelected
	if selected < 0 || selected >= len(rows) {
		selected = 0
	}
	parts := []string{"esc/q close", "↑/↓ choose"}
	if row := rows[selected]; row.Fixable {
		parts = append(parts, "space toggle")
	}
	if len(m.visibilityRepairSelectedRows(skill)) > 0 {
		parts = append(parts, "enter apply selected")
	} else {
		parts = append(parts, "enter no-op")
	}
	return strings.Join(parts, " · ")
}

func (m appModel) visibilityRepairModalOverlay(layout appLayout) string {
	modalWidth, modalHeight := detailModalDimensions(layout)
	innerWidth := modalWidth - 4
	innerHeight := modalHeight - 6
	leftWidth := innerWidth * 46 / 100
	rightWidth := innerWidth - leftWidth - 3
	skill := m.visibilityRepairSkill()
	rows := m.visibilityRepairRows(skill)

	leftLines := []string{titleStyle.Render(" Visibility Repair Wizard "), ""}
	if skill == nil || len(rows) == 0 {
		leftLines = append(leftLines, errorStyle.Render("  No visibility repair targets are available."))
	} else {
		for i, row := range rows {
			selector := "  "
			if i == m.visibilityRepairSelected {
				selector = "› "
			}
			box := "[-]"
			if row.Fixable {
				if m.visibilityRepairTargets[row.Agent] {
					box = "[x]"
				} else {
					box = "[ ]"
				}
			}
			status := visibilityRepairRowStatus(row)
			line := fmt.Sprintf("%s %s %s %s", selector, box, compat.SanitizeMetadata(row.Display), status)
			if i == m.visibilityRepairSelected {
				leftLines = append(leftLines, selectedStyle.Render(padRight(line, leftWidth)))
			} else {
				leftLines = append(leftLines, line)
			}
		}
	}
	leftPane := fitLines(strings.Join(leftLines, "\n"), innerHeight)

	rightLines := []string{titleStyle.Render(" Repair Preview "), ""}
	if skill == nil {
		rightLines = append(rightLines, dimStyle.Render("Select a skill row to repair visibility."))
	} else {
		rightLines = append(rightLines,
			formatMetaLine("Skill:", compat.SanitizeMetadata(skill.Name), rightWidth),
			formatMetaLine("Source:", compat.SanitizeMetadata(skill.CanonicalPath), rightWidth),
		)
		selectedRows := m.visibilityRepairSelectedRows(skill)
		if len(rows) > 0 && m.visibilityRepairSelected >= 0 && m.visibilityRepairSelected < len(rows) {
			row := rows[m.visibilityRepairSelected]
			rightLines = append(rightLines, formatMetaLine("Agent:", compat.SanitizeMetadata(row.Display), rightWidth))
			rightLines = append(rightLines, formatMetaLine("State:", visibilityRepairRowState(row), rightWidth))
			if row.Destination != "" {
				rightLines = append(rightLines, formatMetaLine("Destination:", compat.SanitizeMetadata(row.Destination), rightWidth))
			}
			if row.Strategy != "" && row.Fixable {
				rightLines = append(rightLines, formatMetaLine("Strategy:", row.Strategy, rightWidth))
			}
			if row.Note != "" {
				rightLines = append(rightLines, "", wrapText(row.Note, rightWidth))
			}
			if row.Fixable {
				rightLines = append(rightLines, "", sectionHeaderStyle.Render("Selected operation"), "", visibilityRepairCommandSummaryLine(row))
			} else if row.Note != "" {
				rightLines = append(rightLines, "", dimStyle.Render("This row is not selectable."))
			}
		}
		if len(selectedRows) > 0 {
			rightLines = append(rightLines, "", sectionHeaderStyle.Render("Apply Preview"), "")
			for _, row := range selectedRows {
				rightLines = append(rightLines, "  "+compat.SanitizeMetadata(row.Display), "    "+visibilityRepairCommandLine(row))
			}
		} else {
			rightLines = append(rightLines, "", dimStyle.Render("Select one or more fixable agents to preview operations."))
		}
	}
	rightPane := fitLines(strings.Join(rightLines, "\n"), innerHeight)

	dividerLines := make([]string, 0, innerHeight)
	for i := 0; i < innerHeight; i++ {
		dividerLines = append(dividerLines, dimStyle.Render("│"))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(leftWidth).Render(leftPane),
		" ",
		strings.Join(dividerLines, "\n"),
		" ",
		lipgloss.NewStyle().Width(rightWidth).Render(rightPane),
	)

	content := []string{
		body,
		"",
		dimStyle.Render(m.visibilityRepairModalHelpLine()),
	}

	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(actionBorderColor).Padding(1, 2).Width(modalWidth).Height(modalHeight).Render(strings.Join(content, "\n"))
	return fitToScreen(lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, box), layout.Width, layout.Height)
}

func visibilityRepairRowStatus(row visibilityRepairRow) string {
	if row.Note == "destination already exists" {
		return errorStyle.Render("collision")
	}
	if row.Note == "agent metadata unavailable" {
		return dimStyle.Render("unavailable")
	}
	if row.Visible {
		return successStyle.Render("visible")
	}
	switch row.Reason {
	case "missing_agent_link":
		return warningStyle.Render("missing")
	case "not_in_universal_canonical_dir":
		return warningStyle.Render("universal missing")
	case "unsupported_global":
		return dimStyle.Render("unsupported")
	case "agent_not_detected":
		return dimStyle.Render("not detected")
	case "broken_symlink":
		return errorStyle.Render("broken")
	default:
		if row.Note != "" {
			return dimStyle.Render(row.Note)
		}
		return dimStyle.Render(compat.SanitizeMetadata(row.Reason))
	}
}

func visibilityRepairRowState(row visibilityRepairRow) string {
	if row.Visible {
		return "visible"
	}
	if row.Fixable {
		return "fixable"
	}
	return compat.SanitizeMetadata(compat.FirstNonEmpty(row.Note, row.Reason))
}

func visibilityRepairCommandSummaryLine(row visibilityRepairRow) string {
	style := dimStyle
	if row.Strategy == "copy" {
		style = warningStyle
	}
	return style.Render(visibilityRepairCommandSummary(row))
}

func visibilityRepairPreferredStrategy(skill *model.Skill) string {
	for _, observed := range skill.ObservedPaths {
		if observed.Status == model.StatusCopy {
			return "copy"
		}
	}
	return "symlink"
}

func visibilityRepairTargetRoot(skill *model.Skill, state model.AgentState) string {
	if state.Universal {
		return state.ProjectDir
	}
	if skill.Scope == model.ScopeGlobal {
		if state.SupportsGlobal {
			return state.GlobalDir
		}
		return ""
	}
	return state.ProjectDir
}

func visibilityPathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Lstat(path)
	return err == nil
}

func (m appModel) repairVisibilityExecutionPreview(action actions.CommandPreview) (runner.Result, bool) {
	plan := decodeVisibilityRepairExec(action.Exec.Args)
	return m.runVisibilityRepair(plan)
}

func decodeVisibilityRepairExec(args []string) []visibilityRepairExec {
	if len(args) == 0 || len(args)%4 != 0 {
		return nil
	}
	plan := make([]visibilityRepairExec, 0, len(args)/4)
	for i := 0; i < len(args); i += 4 {
		plan = append(plan, visibilityRepairExec{
			Agent:       args[i],
			Source:      args[i+1],
			Destination: args[i+2],
			Strategy:    args[i+3],
		})
	}
	return plan
}

func (m appModel) runVisibilityRepair(plan []visibilityRepairExec) (runner.Result, bool) {
	if len(plan) == 0 {
		return runner.Result{Program: "repair_visibility", Cwd: m.cwd, ExitCode: -1, Err: "no visibility repair operations were selected"}, false
	}
	lines := make([]string, 0, len(plan))
	errs := []string{}
	succeeded := 0
	for i, item := range plan {
		prefix := fmt.Sprintf("%d/%d %s", i+1, len(plan), compat.SanitizeMetadata(item.Agent))
		if !visibilityPathExists(item.Source) {
			errs = append(errs, fmt.Sprintf("%s source path does not exist: %s", prefix, item.Source))
			lines = append(lines, prefix+" failed")
			continue
		}
		if _, err := os.Lstat(item.Destination); err == nil {
			errs = append(errs, fmt.Sprintf("%s destination already exists: %s", prefix, item.Destination))
			lines = append(lines, prefix+" failed")
			continue
		} else if !os.IsNotExist(err) {
			errs = append(errs, fmt.Sprintf("%s could not inspect destination: %v", prefix, err))
			lines = append(lines, prefix+" failed")
			continue
		}
		if err := os.MkdirAll(filepath.Dir(item.Destination), 0o755); err != nil {
			errs = append(errs, fmt.Sprintf("%s failed to create destination directory: %v", prefix, err))
			lines = append(lines, prefix+" failed")
			continue
		}
		if item.Strategy == "copy" {
			if err := copyVisibilityTree(item.Source, item.Destination); err != nil {
				errs = append(errs, fmt.Sprintf("%s failed to copy: %v", prefix, err))
				lines = append(lines, prefix+" failed")
				continue
			}
		} else {
			target := item.Source
			if rel, err := filepath.Rel(filepath.Dir(item.Destination), item.Source); err == nil {
				target = rel
			}
			if err := os.Symlink(target, item.Destination); err != nil {
				errs = append(errs, fmt.Sprintf("%s failed to create symlink: %v", prefix, err))
				lines = append(lines, prefix+" failed")
				continue
			}
		}
		succeeded++
		lines = append(lines, prefix+" ok")
	}
	result := runner.Result{Program: "repair_visibility", Args: flattenVisibilityRepairPlan(plan), Cwd: m.cwd, ExitCode: 0, Stdout: strings.Join(lines, "\n")}
	if len(errs) > 0 {
		result.ExitCode = -1
		result.Err = strings.Join(errs, "; ")
	}
	return result, succeeded > 0 && len(errs) > 0
}

func flattenVisibilityRepairPlan(plan []visibilityRepairExec) []string {
	args := make([]string, 0, len(plan)*4)
	for _, item := range plan {
		args = append(args, item.Agent, item.Source, item.Destination, item.Strategy)
	}
	return args
}

func copyVisibilityTree(src, dest string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(src)
		if err != nil {
			return err
		}
		src = resolved
		info, err = os.Stat(src)
		if err != nil {
			return err
		}
	}
	if !info.IsDir() {
		return copyVisibilityFile(src, dest, info.Mode())
	}
	return copyVisibilityDir(src, dest, info.Mode())
}

func copyVisibilityDir(src, dest string, mode os.FileMode) error {
	if err := os.MkdirAll(dest, mode.Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcChild := filepath.Join(src, entry.Name())
		destChild := filepath.Join(dest, entry.Name())
		info, err := os.Lstat(srcChild)
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(srcChild)
			if err != nil {
				return err
			}
			if err := os.Symlink(target, destChild); err != nil {
				return err
			}
		case info.IsDir():
			if err := copyVisibilityDir(srcChild, destChild, info.Mode()); err != nil {
				return err
			}
		default:
			if err := copyVisibilityFile(srcChild, destChild, info.Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyVisibilityFile(src, dest string, mode os.FileMode) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, mode.Perm())
}
