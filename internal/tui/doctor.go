package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/display"
	"github.com/alvinunreal/lazyskills/internal/model"
	"github.com/charmbracelet/lipgloss"
)

const (
	doctorCopyActionID   = "doctor_copy_markdown"
	doctorExportActionID = "doctor_export_markdown"
)

type doctorIssueRow struct {
	Section     string
	SectionRank int
	Subject     string
	Summary     string
	Message     string
	Path        string
	Skill       *model.Skill
	Actions     []actions.CommandPreview
}

func (m appModel) doctorRows() []doctorIssueRow {
	rows := make([]doctorIssueRow, 0, len(m.result.HealthIssues))
	seen := map[string]bool{}
	appendRow := func(subject string, skill *model.Skill, issue model.HealthIssue) {
		section, rank := doctorSectionForIssue(issue)
		summary := humanHealthIssueType(issue.Type)
		message := humanHealthIssueMessage(issue.Type, issue.Message)
		path := compat.SanitizeMetadata(issue.Path)
		key := strings.Join([]string{section, subject, summary, message, path}, "\x00")
		if seen[key] {
			return
		}
		seen[key] = true
		row := doctorIssueRow{
			Section:     section,
			SectionRank: rank,
			Subject:     compat.SanitizeMetadata(subject),
			Summary:     compat.SanitizeMetadata(summary),
			Message:     compat.SanitizeMetadata(message),
			Path:        path,
			Skill:       skill,
		}
		if skill != nil {
			row.Actions = doctorSafeActions(skill)
		}
		rows = append(rows, row)
	}

	for _, issue := range m.result.HealthIssues {
		appendRow(doctorWorkspaceLabel(m.result), nil, issue)
	}
	for _, skill := range m.result.Skills {
		if skill == nil {
			continue
		}
		view := display.Skill(skill)
		subject := compat.FirstNonEmpty(view.Name, "Unnamed skill")
		for _, issue := range skill.HealthIssues {
			appendRow(subject, skill, issue)
		}
	}

	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SectionRank != rows[j].SectionRank {
			return rows[i].SectionRank < rows[j].SectionRank
		}
		if rows[i].Subject != rows[j].Subject {
			return rows[i].Subject < rows[j].Subject
		}
		if rows[i].Summary != rows[j].Summary {
			return rows[i].Summary < rows[j].Summary
		}
		if rows[i].Path != rows[j].Path {
			return rows[i].Path < rows[j].Path
		}
		return rows[i].Message < rows[j].Message
	})

	return rows
}

func doctorWorkspaceLabel(result model.ScanResult) string {
	return "Workspace"
}

func doctorSectionForIssue(issue model.HealthIssue) (string, int) {
	switch strings.ToLower(compat.SanitizeMetadata(issue.Severity)) {
	case "error", "critical":
		return "Critical", 0
	case "warning", "warn":
		return "Warnings", 1
	default:
		return "Info", 2
	}
}

func doctorSafeActions(sk *model.Skill) []actions.CommandPreview {
	if sk == nil {
		return nil
	}
	previews := actions.ForSkill(sk)
	safe := make([]actions.CommandPreview, 0, len(previews))
	for _, preview := range previews {
		if !preview.Available || preview.Dangerous {
			continue
		}
		safe = append(safe, preview)
	}
	sort.SliceStable(safe, func(i, j int) bool {
		if doctorActionPriority(safe[i].ID) != doctorActionPriority(safe[j].ID) {
			return doctorActionPriority(safe[i].ID) < doctorActionPriority(safe[j].ID)
		}
		return safe[i].Title < safe[j].Title
	})
	return safe
}

func doctorActionPriority(id string) int {
	switch id {
	case "prune_lock":
		return 0
	case "reinstall_update":
		return 1
	case "enable_skill", "disable_skill":
		return 2
	case "open_skill":
		return 3
	default:
		return 4
	}
}

func (m appModel) doctorCurrentActions() []actions.CommandPreview {
	previews := []actions.CommandPreview{}
	if row, ok := m.doctorSelectedRow(); ok {
		previews = append(previews, row.Actions...)
	}
	previews = append(previews,
		actions.CommandPreview{ID: doctorCopyActionID, Title: "Copy doctor report as Markdown", Description: "Copy the prioritized repair report to the clipboard.", Command: "copy doctor report", Exec: actions.ExecSpec{Internal: doctorCopyActionID}, Available: true},
		actions.CommandPreview{ID: doctorExportActionID, Title: "Export doctor report as Markdown", Description: "Write the prioritized repair report to a Markdown file.", Command: "export doctor report", Exec: actions.ExecSpec{Internal: doctorExportActionID}, Available: true},
	)
	return previews
}

func (m appModel) doctorSelectedRow() (doctorIssueRow, bool) {
	rows := m.doctorRows()
	if len(rows) == 0 || m.doctorSelected < 0 || m.doctorSelected >= len(rows) {
		return doctorIssueRow{}, false
	}
	return rows[m.doctorSelected], true
}

func (m appModel) doctorPrimaryAction() (actions.CommandPreview, bool) {
	row, ok := m.doctorSelectedRow()
	if !ok || len(row.Actions) == 0 {
		return actions.CommandPreview{}, false
	}
	return row.Actions[0], true
}

func (m *appModel) clampDoctorSelection() {
	rows := m.doctorRows()
	if len(rows) == 0 {
		m.doctorSelected = 0
		return
	}
	if m.doctorSelected < 0 {
		m.doctorSelected = 0
	}
	if m.doctorSelected >= len(rows) {
		m.doctorSelected = len(rows) - 1
	}
}

func (m appModel) doctorCounts() (critical, warnings, info int) {
	for _, row := range m.doctorRows() {
		switch row.Section {
		case "Critical":
			critical++
		case "Warnings":
			warnings++
		default:
			info++
		}
	}
	return critical, warnings, info
}

func (m appModel) doctorMarkdownReport() string {
	rows := m.doctorRows()
	critical, warnings, info := m.doctorCounts()
	var out strings.Builder
	out.WriteString("# LazySkills Doctor Mode Repair Report\n\n")
	if m.result.Cwd != "" {
		fmt.Fprintf(&out, "- Workspace: %s\n", compat.SanitizeMetadata(m.result.Cwd))
	}
	fmt.Fprintf(&out, "- Critical: %d\n- Warnings: %d\n- Info: %d\n\n", critical, warnings, info)
	for _, section := range []string{"Critical", "Warnings", "Info"} {
		fmt.Fprintf(&out, "## %s\n\n", section)
		sectionRows := doctorRowsForSection(rows, section)
		if len(sectionRows) == 0 {
			out.WriteString("- None\n\n")
			continue
		}
		for _, row := range sectionRows {
			fmt.Fprintf(&out, "- %s — %s\n", row.Subject, row.Summary)
			if row.Message != "" {
				fmt.Fprintf(&out, "  - Message: %s\n", row.Message)
			}
			if row.Path != "" {
				fmt.Fprintf(&out, "  - Path: %s\n", row.Path)
			}
			if len(row.Actions) > 0 {
				fmt.Fprintf(&out, "  - Safe actions: %s\n", doctorActionTitles(row.Actions))
			}
		}
		out.WriteString("\n")
	}
	return out.String()
}

func doctorRowsForSection(rows []doctorIssueRow, section string) []doctorIssueRow {
	out := make([]doctorIssueRow, 0, len(rows))
	for _, row := range rows {
		if row.Section == section {
			out = append(out, row)
		}
	}
	return out
}

func doctorActionTitles(actions []actions.CommandPreview) string {
	titles := make([]string, 0, len(actions))
	for _, action := range actions {
		titles = append(titles, compat.SanitizeMetadata(action.Title))
	}
	return strings.Join(titles, ", ")
}

func (m appModel) doctorText(width int) string {
	rows := m.doctorRows()
	critical, warnings, info := m.doctorCounts()
	lines := []string{
		sectionHeaderStyle.Render(" Doctor Mode Repair Report "),
		fmt.Sprintf("%s Critical · %s Warnings · %s Info", errorStyle.Render(fmt.Sprintf("%d", critical)), warningStyle.Render(fmt.Sprintf("%d", warnings)), dimStyle.Render(fmt.Sprintf("%d", info))),
	}
	if m.doctorStatus != "" {
		lines = append(lines, "", dimStyle.Render(truncate(m.doctorStatus, width)))
	}
	if len(rows) == 0 {
		lines = append(lines, "", dimStyle.Render("No health issues found."))
		return strings.Join(lines, "\n")
	}
	currentSection := ""
	for idx, row := range rows {
		if row.Section != currentSection {
			if currentSection != "" {
				lines = append(lines, "")
			}
			lines = append(lines, doctorSectionHeading(row.Section, doctorSectionCount(rows, row.Section)))
			currentSection = row.Section
		}
		lines = append(lines, m.doctorRowLines(row, idx == m.doctorSelected, width)...)
	}
	return strings.Join(lines, "\n")
}

func doctorSectionHeading(section string, count int) string {
	label := fmt.Sprintf("%s (%d)", section, count)
	switch section {
	case "Critical":
		return errorStyle.Bold(true).Render(label)
	case "Warnings":
		return warningStyle.Bold(true).Render(label)
	default:
		return dimStyle.Bold(true).Render(label)
	}
}

func doctorSectionCount(rows []doctorIssueRow, section string) int {
	count := 0
	for _, row := range rows {
		if row.Section == section {
			count++
		}
	}
	return count
}

func (m appModel) doctorRowLines(row doctorIssueRow, selected bool, width int) []string {
	marker := "  • "
	if selected {
		marker = "› "
	}
	sectionTag := doctorSectionTag(row.Section)
	header := fmt.Sprintf("%s%s — %s", marker, row.Subject, row.Summary)
	if selected {
		header = selectedStyle.Render(truncate(sectionTag+" "+header, width))
	} else {
		header = truncate(sectionTag+" "+header, width)
	}
	lines := []string{header}
	if row.Message != "" {
		lines = append(lines, dimStyle.Render(wrapText("  "+row.Message, width)))
	}
	if row.Path != "" {
		lines = append(lines, dimStyle.Render(truncate("  path: "+row.Path, width)))
	}
	if len(row.Actions) > 0 {
		lines = append(lines, dimStyle.Render(truncate("  safe actions: "+doctorActionTitles(row.Actions), width)))
	}
	return lines
}

func doctorSectionTag(section string) string {
	switch section {
	case "Critical":
		return errorStyle.Render("[Critical]")
	case "Warnings":
		return warningStyle.Render("[Warnings]")
	default:
		return dimStyle.Render("[Info]")
	}
}

func doctorClipboardSequence(markdown string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(markdown)) + "\a"
}

func doctorCopyMarkdown(markdown string) error {
	_, err := os.Stdout.WriteString(doctorClipboardSequence(markdown))
	return err
}

func doctorExportMarkdown(cwd, markdown string) (string, error) {
	if cwd == "" {
		cwd = "."
	}
	path := filepath.Join(cwd, "lazyskills-doctor-report.md")
	if _, err := os.Stat(path); err == nil {
		for i := 1; ; i++ {
			candidate := filepath.Join(cwd, fmt.Sprintf("lazyskills-doctor-report-%d.md", i))
			if _, err := os.Stat(candidate); os.IsNotExist(err) {
				path = candidate
				break
			}
		}
	}
	if err := os.WriteFile(path, []byte(markdown), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func (m appModel) doctorOverlay(layout appLayout) string {
	modalWidth := int(float64(layout.Width) * 0.82)
	if modalWidth < 70 {
		modalWidth = 70
	}
	if modalWidth > 110 {
		modalWidth = 110
	}
	if layout.Width < modalWidth+4 {
		modalWidth = layout.Width - 4
	}
	if modalWidth < 20 {
		modalWidth = 20
	}

	m.viewport.Width = modalWidth - 4
	m.viewport.Height = max(1, layout.Height-8)
	m.viewport.SetContent(m.doctorText(modalWidth - 4))

	lines := []string{
		titleStyle.Render(" Doctor Mode Repair Report "),
		"",
		m.viewport.View(),
		"",
		dimStyle.Render(m.doctorHelpLine()),
	}
	if m.doctorStatus != "" {
		lines = append(lines, dimStyle.Render(truncate(m.doctorStatus, modalWidth-4)))
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(actionBorderColor).
		Padding(1, 2).
		Width(modalWidth).
		Height(max(8, layout.Height-4)).
		Render(strings.Join(lines, "\n"))

	return fitToScreen(lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, box), layout.Width, layout.Height)
}

func (m appModel) doctorHelpLine() string {
	parts := []string{"esc/q/D close", "↑/↓ select", "enter primary action", "c more actions", "C copy markdown", "E export markdown", "r refresh"}
	if row, ok := m.doctorSelectedRow(); ok && len(row.Actions) == 0 {
		parts[2] = "enter no-op"
	}
	return strings.Join(parts, " · ")
}
