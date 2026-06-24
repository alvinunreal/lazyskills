package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/display"
	"github.com/alvinunreal/lazyskills/internal/model"
	"github.com/alvinunreal/lazyskills/internal/selfupdate"
	"github.com/charmbracelet/lipgloss"
)

func (m appModel) View() string {
	viewStart := time.Now()
	defer func() {
		perfLogf("view selected=%d focus=%d modal=%t source=%q preview_pending=%t duration=%s", m.selected, m.focus, m.detailModal, m.modalSource, m.previewPending, time.Since(viewStart))
	}()
	if m.err != nil {
		return fitToScreen(errorStyle.Render(fmt.Sprintf("LazySkills error: %s\n\nPress q to quit.", compat.SanitizeMetadata(m.err.Error()))), viewWidth(m.width), viewHeight(m.height))
	}
	layout := newAppLayout(m.width, m.height)
	if layout.Small {
		return smallTerminalView(layout.Width, layout.Height)
	}

	if m.detailModal {
		return m.detailModalOverlay(layout)
	}
	if m.appUpdateModal {
		return m.appUpdateModalOverlay(layout)
	}
	if m.helpOpen {
		return m.helpModalOverlay(layout)
	}
	if m.running {
		return m.runningOverlay(layout)
	}
	if m.confirming {
		return m.confirmationOverlay(layout)
	}
	if m.commands {
		return m.commandsOverlay(layout)
	}

	viewModel := m
	viewModel.width = layout.Width
	viewModel.height = layout.Height

	listWidth, rightWidth, topHeight, bottomHeight := viewModel.getThreePaneLayout()
	needsSync := viewModel.needsViewportSync(rightWidth, topHeight, bottomHeight)
	perfLogf("view_pre selected=%d needs_sync=%t metadata_size=%dx%d preview_size=%dx%d", viewModel.selected, needsSync, viewModel.metadataViewport.Width, viewModel.metadataViewport.Height, viewModel.previewViewport.Width, viewModel.previewViewport.Height)
	if needsSync {
		// Keep View pure for callers: sync a local copy only when render-time
		// fallback sizing is needed. Normal Update paths already call syncViewport;
		// doing it again here makes every navigation frame recompute details and
		// previews a second time.
		viewModel.syncViewport()
	}

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

	footer := viewModel.footerText(layout.Width)
	return view + "\n" + footer
}

func (m appModel) needsViewportSync(rightWidth, topHeight, bottomHeight int) bool {
	return m.metadataViewport.Width != max(1, rightWidth-4) ||
		m.metadataViewport.Height != max(1, topHeight-2) ||
		m.previewViewport.Width != max(1, rightWidth-4) ||
		m.previewViewport.Height != max(1, bottomHeight-2) ||
		m.viewportSyncFingerprint != m.currentViewportSyncFingerprint()
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

func scopeStyle(scope string) lipgloss.Style {
	switch scope {
	case string(model.ScopeProject):
		return scopeProjectStyle
	case string(model.ScopeGlobal):
		return scopeGlobalStyle
	default:
		return dimStyle
	}
}

func styledScopeBadge(scope string) string {
	s := strings.ToLower(scope)
	switch s {
	case "project":
		return scopeProjectStyle.Render("[Project]")
	case "global":
		return scopeGlobalStyle.Render("[Global]")
	case "mixed":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Render("[Mixed]")
	default:
		return dimStyle.Render("[" + strings.Title(s) + "]")
	}
}

func (m appModel) listTitle() string {
	title := "1 Inventory"
	if m.agent != "" {
		title = "1 Inventory (" + m.agentLabel() + ")"
	}
	return title
}

func (m appModel) listPane(height, width int) string {
	vRows := m.visibleRows()
	var lines []string
	if len(vRows) == 0 {
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
	selectedRow := m.selected
	start := 0
	if selectedRow >= visible {
		start = selectedRow - visible + 1
	}
	end := min(len(vRows), start+visible)

	for idx, row := range vRows[start:end] {
		rowIndex := start + idx
		var line string
		if row.isHeader {
			affordance := "- "
			if m.isCollapsed(row.groupName) {
				affordance = "+ "
			}
			headerText := affordance + row.groupName
			hint := ""
			if n := m.availableCount(row.groupName); n > 0 {
				hint = fmt.Sprintf("  +%d available", n)
			}
			if rowIndex == selectedRow {
				line = selectedStyle.Render(truncate(headerText+hint, width))
			} else {
				line = dimStyle.Render(truncate(headerText, width-lipgloss.Width(hint))) + dimStyle.Render(hint)
			}
		} else {
			view := display.Skill(row.skill)
			mark := "  "
			if m.isSelected(row.skill) {
				mark = "● "
			}

			scopeTag := "[" + scopeBadge(view.Scope) + "]"
			agentBadge := ""
			if m.agent != "" {
				agentBadge = " " + agentVisibilityBadge(row.skill, m.agent)
			}

			issueErrors, issueWarnings := healthIssueCounts(view.HealthIssues)
			severity := ""
			if issueErrors > 0 {
				severity = fmt.Sprintf(" !%d", issueErrors)
			} else if issueWarnings > 0 {
				severity = fmt.Sprintf(" ▲%d", issueWarnings)
			}

			isEffectivelyDisabled := row.skill.Disabled
			if m.agent != "" {
				isEffectivelyDisabled = false
				for _, obs := range row.skill.ObservedPaths {
					if obs.Agent == m.agent && obs.Status == model.StatusDisabled {
						isEffectivelyDisabled = true
						break
					}
				}
			}

			tail := " " + scopeTag + agentBadge + severity
			if isEffectivelyDisabled {
				tail += " [disabled]"
			}
			nameCore := truncate(mark+view.Name, max(1, width-lipgloss.Width(tail)))

			switch {
			case rowIndex == selectedRow:
				line = selectedStyle.Render(nameCore + tail)
			case issueErrors > 0:
				line = errorStyle.Render(nameCore + tail)
			case isEffectivelyDisabled:
				line = dimStyle.Render(nameCore+" "+scopeTag) + agentBadge
				if issueWarnings > 0 {
					line += warningStyle.Render(severity)
				}
				line += " " + dimStyle.Render("[disabled]")
			default:
				line = nameCore + " " + scopeStyle(view.Scope).Render(scopeTag) + agentBadge
				if issueWarnings > 0 {
					line += warningStyle.Render(severity)
				}
			}
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
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

func (m appModel) detailText(width int) string {
	return strings.Join(m.detailLines(width), "\n")
}

func (m appModel) metadataLines(width int) []string {
	return m.metadataLinesForRows(m.visibleRows(), width)
}

func (m appModel) metadataLinesForRows(rows []skillsRow, width int) []string {
	if len(rows) == 0 {
		var lines []string

		if len(m.result.HealthIssues) > 0 {
			lines = append(lines, errorStyle.Render("✗ Scan health issues"), "")
			for _, issue := range m.result.HealthIssues {
				lines = append(lines, truncate(fmt.Sprintf("  • %s: %s", compat.SanitizeMetadata(issue.Type), compat.SanitizeMetadata(issue.Message)), width))
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
				statusStyled := errorStyle.Render(status)
				if m.result.Preflight.Tools[tool].Exists {
					status = "available"
					statusStyled = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Render(status)
				}
				lines = append(lines, fmt.Sprintf("  • %-8s %s", tool+":", statusStyled))
			}
		} else if len(m.result.Skills) == 0 {
			lines = append(lines,
				sectionHeaderStyle.Render("Welcome to LazySkills!"),
				"",
				wrapText("No skills were found in your project or global directory.", width),
				"",
				dimStyle.Render("Quick Onboarding:"),
				wrapText("  1. Press 'c' to open actions and choose 'Initialize skills in project' to create a local skills directory.", width),
				wrapText("  2. Choose 'Find new skills (interactive)' to search and install online skills.", width),
				wrapText("  3. Link your existing skills using symlinks.", width),
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
		stateVal := "▼ expanded"
		if m.isCollapsed(row.groupName) {
			stateVal = "▶ collapsed"
		}

		skills := m.sourceGroupSkills(row.groupName)
		var folders, refs []string
		var projectCount, globalCount int
		var skillIssues []display.HealthIssueView

		for _, sk := range skills {
			info := sourceInfo(sk)
			if info.Folder != "" {
				folders = append(folders, info.Folder)
			}
			if info.Ref != "" {
				refs = append(refs, info.Ref)
			}
			if sk.Scope == model.ScopeProject {
				projectCount++
			} else if sk.Scope == model.ScopeGlobal {
				globalCount++
			}

			// Parse health issues
			view := display.Skill(sk)
			skillIssues = append(skillIssues, view.HealthIssues...)
		}

		scopeStr := "mixed"
		if projectCount > 0 && globalCount == 0 {
			scopeStr = "project"
		} else if globalCount > 0 && projectCount == 0 {
			scopeStr = "global"
		} else if projectCount == 0 && globalCount == 0 {
			scopeStr = "unknown"
		}

		scopeVal := styledScopeBadge(scopeStr)

		healthVal := lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Render("✓ healthy")
		if len(skillIssues) > 0 {
			healthVal = errorStyle.Render("✗ issues detected")
		}

		lines := []string{
			formatMetaLine("Source:", row.groupName, width),
			formatMetaLine("State:", stateVal, width),
			formatMetaLine("Skills:", fmt.Sprintf("%d visible / %d total", visible, total), width),
			formatMetaLine("Scope:", scopeVal, width),
		}

		if len(folders) > 0 {
			lines = append(lines, formatMetaLine("Folder:", folders[0], width))
		}
		if len(refs) > 0 {
			lines = append(lines, formatMetaLine("Ref:", refs[0], width))
		}

		lines = append(lines, formatMetaLine("Health:", healthVal, width))

		if len(skillIssues) > 0 {
			hasErrors := false
			for _, issue := range skillIssues {
				if issue.Severity == "error" {
					hasErrors = true
					break
				}
			}
			headerText := "▲ Warnings"
			headerStyle := warningStyle.Bold(true)
			if hasErrors {
				headerText = "✗ Health Issues"
				headerStyle = healthHeaderStyle
			}
			lines = append(lines, "", headerStyle.Render(headerText))
			for _, issue := range skillIssues {
				bullet := "  • "
				style := warningStyle
				if issue.Severity == "error" {
					style = errorStyle
				}
				line := fmt.Sprintf("%s%s: %s", bullet, humanHealthIssueType(issue.Type), humanHealthIssueMessage(issue.Type, issue.Message))
				if issue.Path != "" {
					line += " (" + issue.Path + ")"
				}
				lines = append(lines, style.Render(wrapText(line, width)))
			}
		}
		return lines
	}

	view := display.Skill(row.skill)
	isEffectivelyDisabled := row.skill.Disabled
	if m.agent != "" {
		isEffectivelyDisabled = false
		for _, obs := range row.skill.ObservedPaths {
			if obs.Agent == m.agent && obs.Status == model.StatusDisabled {
				isEffectivelyDisabled = true
				break
			}
		}
	}
	statusVal := "enabled"
	if isEffectivelyDisabled {
		statusVal = errorStyle.Render("disabled")
	} else {
		statusVal = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Render("enabled")
	}
	lines := []string{
		formatMetaLine("Scope:", styledScopeBadge(string(view.Scope)), width),
		formatMetaLine("Status:", statusVal, width),
	}
	if view.Description != "" {
		lines = append(lines, formatMetaLine("Description:", dimStyle.Render(view.Description), width))
	}
	if sourceLines := sourceDetailLines(row.skill, width); len(sourceLines) > 0 {
		lines = append(lines, sourceLines...)
	} else {
		// No source block to show; fall back to the lock/tracking state
		// (covers "not tracked" and path-only locks).
		lockVal := display.LockSummary(view)
		if lockVal == "not tracked" {
			lockVal = warningStyle.Render("not tracked")
		}
		lines = append(lines, formatMetaLine("Lock:", lockVal, width))
	}
	if view.CanonicalPath != "" {
		lines = append(lines, formatMetaLine("Canonical:", view.CanonicalPath, width))
	}
	if m.agent != "" {
		lines = append(lines, formatMetaLine("Agent:", m.agentLabel(), width))
	}
	lines = append(lines, m.visibilitySummary(view, width)...)

	if len(view.HealthIssues) > 0 {
		issueErrors, _ := healthIssueCounts(view.HealthIssues)
		headerStyle := warningStyle.Bold(true)
		header := "▲ Warnings"
		if issueErrors > 0 {
			headerStyle = healthHeaderStyle
			header = "✗ Health Issues"
		}
		lines = append(lines, "", headerStyle.Render(header))
		for _, issue := range view.HealthIssues {
			bullet := "  • "
			style := warningStyle
			if issue.Severity == "error" {
				style = errorStyle
			}
			line := fmt.Sprintf("%s%s: %s", bullet, humanHealthIssueType(issue.Type), humanHealthIssueMessage(issue.Type, issue.Message))
			if issue.Path != "" {
				line += " (" + issue.Path + ")"
			}
			lines = append(lines, style.Render(wrapText(line, width)))
		}
	}

	if len(m.result.HealthIssues) > 0 {
		lines = append(lines, "", errorStyle.Render("✗ Scan Health Issues"))
		for _, issue := range m.result.HealthIssues {
			lines = append(lines, truncate(fmt.Sprintf("  • %s: %s", compat.SanitizeMetadata(issue.Type), compat.SanitizeMetadata(issue.Message)), width))
		}
	}

	return lines
}

func (m appModel) previewLines(width int) []string {
	return m.previewLinesForRows(m.visibleRows(), width)
}

func (m appModel) previewLinesForRows(rows []skillsRow, width int) []string {
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
		// Read-only glance: list installed + available skills. The modal
		// (enter) is the interactive browse/install surface.
		skills := m.sourceGroupSkills(row.groupName)
		lines := []string{sectionHeaderStyle.Render(fmt.Sprintf("Installed (%d)", len(skills)))}
		if len(skills) == 0 {
			lines = append(lines, dimStyle.Render("  none"))
		} else {
			for _, sk := range skills {
				badge := "[P]"
				if sk.Scope == model.ScopeGlobal {
					badge = "[G]"
				}
				lines = append(lines, fmt.Sprintf("  • %s %s", sk.Name, badge))
			}
		}

		lines = append(lines, "")

		disc, discOk := m.discovery[row.groupName]
		_, _, isRemote := parseRemoteGitHubSource(row.groupName)
		switch {
		case !discOk:
			lines = append(lines, sectionHeaderStyle.Render("Available"), dimStyle.Render("  press d to scan this source"))
		case disc.Status == DiscoveryLoading && isRemote:
			lines = append(lines, sectionHeaderStyle.Render("Available"), dimStyle.Render("  scanning…"))
		case disc.Status == DiscoveryLoading:
			lines = append(lines, sectionHeaderStyle.Render("Available"), dimStyle.Render("  scanning…"))
		case disc.Status == DiscoveryFailed:
			lines = append(lines, sectionHeaderStyle.Render("Available"), errorStyle.Render("  couldn't scan: "+disc.Error))
		default: // DiscoveryReady
			var avail []string
			installed := m.installedSkillNames(row.groupName)
			for _, ds := range disc.Skills {
				if !isSkillNameInstalled(ds.Name, installed) {
					avail = append(avail, ds.Name)
				}
			}
			lines = append(lines, sectionHeaderStyle.Render(fmt.Sprintf("Available (%d)", len(avail))))
			if len(avail) == 0 {
				lines = append(lines, dimStyle.Render("  all installed"))
			} else {
				for _, name := range avail {
					lines = append(lines, fmt.Sprintf("  + %s", name))
				}
			}
		}

		lines = append(lines, "", dimStyle.Render("enter to browse · d to scan"))
		var wrapped []string
		for _, line := range lines {
			wrapped = append(wrapped, wrapText(line, width))
		}
		return wrapped
	}

	view := display.Skill(row.skill)
	if view.Preview == "" {
		return []string{dimStyle.Render("No preview available for this skill.")}
	}
	if m.previewPending && !m.hasRenderedMarkdownPreview(view.Preview, width) {
		return []string{dimStyle.Render("Preview updates when navigation pauses.")}
	}

	return m.renderMarkdownPreviewCached(view.Preview, width)
}

type previewCacheKey struct {
	markdown string
	width    int
}

func (m appModel) renderMarkdownPreviewCached(markdown string, width int) []string {
	if m.previewCache == nil {
		return renderMarkdownPreview(markdown, width)
	}
	key := previewCacheKey{markdown: markdown, width: width}
	if lines, ok := m.previewCache[key]; ok {
		return append([]string(nil), lines...)
	}
	lines := renderMarkdownPreview(markdown, width)
	m.previewCache[key] = append([]string(nil), lines...)
	return lines
}

func (m appModel) hasRenderedMarkdownPreview(markdown string, width int) bool {
	if m.previewCache == nil {
		return false
	}
	_, ok := m.previewCache[previewCacheKey{markdown: markdown, width: width}]
	return ok
}

func renderMarkdownPreview(markdown string, width int) []string {
	markdown = stripMarkdownFrontmatter(markdown)
	if strings.TrimSpace(markdown) == "" {
		return []string{dimStyle.Render("No preview available for this skill.")}
	}
	renderWidth := max(20, width-2)
	renderer, err := glamour.NewTermRenderer(
		glamour.WithStyles(compactPreviewMarkdownStyle()),
		glamour.WithWordWrap(renderWidth),
	)
	if err == nil {
		rendered, renderErr := renderer.Render(markdown)
		if renderErr == nil && strings.TrimSpace(rendered) != "" {
			return strings.Split(strings.TrimRight(rendered, "\n"), "\n")
		}
	}
	lines := []string{}
	for _, line := range strings.Split(markdown, "\n") {
		lines = append(lines, wrapText(line, width))
	}
	return lines
}

func stripMarkdownFrontmatter(markdown string) string {
	trimmed := strings.TrimLeft(markdown, "\ufeff\r\n\t ")
	if !strings.HasPrefix(trimmed, "---\n") && !strings.HasPrefix(trimmed, "---\r\n") {
		return markdown
	}
	lines := strings.Split(trimmed, "\n")
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return markdown
}

func compactPreviewMarkdownStyle() ansi.StyleConfig {
	style := styles.DarkStyleConfig
	zero := uint(0)
	headingColor := "39"
	bold := true
	style.Document.Margin = &zero
	style.Document.BlockPrefix = ""
	style.Document.BlockSuffix = ""
	style.Heading.BlockSuffix = ""
	style.H1.Prefix = "# "
	style.H1.Suffix = ""
	style.H1.Color = &headingColor
	style.H1.BackgroundColor = nil
	style.H1.Bold = &bold
	style.HorizontalRule.Format = strings.Repeat("─", 8)
	return style
}

func (m appModel) detailLines(width int) []string {
	if m.modalSource != "" {
		return m.sourceModalDetailLines(width)
	}
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

func (m appModel) sourceModalDetailLines(width int) []string {
	groupName := m.modalSource
	visible, total := m.getGroupCounts(groupName)
	stateVal := "▼ expanded"
	if m.isCollapsed(groupName) {
		stateVal = "▶ collapsed"
	}

	skills := m.sourceGroupSkills(groupName)
	var folders, refs []string
	var projectCount, globalCount int
	var skillIssues []display.HealthIssueView

	for _, sk := range skills {
		info := sourceInfo(sk)
		if info.Folder != "" {
			folders = append(folders, info.Folder)
		}
		if info.Ref != "" {
			refs = append(refs, info.Ref)
		}
		if sk.Scope == model.ScopeProject {
			projectCount++
		} else if sk.Scope == model.ScopeGlobal {
			globalCount++
		}

		view := display.Skill(sk)
		skillIssues = append(skillIssues, view.HealthIssues...)
	}

	scopeStr := "mixed"
	if projectCount > 0 && globalCount == 0 {
		scopeStr = "project"
	} else if globalCount > 0 && projectCount == 0 {
		scopeStr = "global"
	} else if projectCount == 0 && globalCount == 0 {
		scopeStr = "unknown"
	}

	scopeVal := styledScopeBadge(scopeStr)

	healthVal := lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Render("✓ healthy")
	if len(skillIssues) > 0 {
		healthVal = errorStyle.Render("✗ issues detected")
	}

	lines := []string{
		formatMetaLine("Source:", groupName, width),
		formatMetaLine("State:", stateVal, width),
		formatMetaLine("Skills:", fmt.Sprintf("%d visible / %d total", visible, total), width),
		formatMetaLine("Scope:", scopeVal, width),
	}

	if len(folders) > 0 {
		lines = append(lines, formatMetaLine("Folder:", folders[0], width))
	}
	if len(refs) > 0 {
		lines = append(lines, formatMetaLine("Ref:", refs[0], width))
	}

	lines = append(lines, formatMetaLine("Health:", healthVal, width))

	if len(skillIssues) > 0 {
		hasErrors := false
		for _, issue := range skillIssues {
			if issue.Severity == "error" {
				hasErrors = true
				break
			}
		}
		headerText := "▲ Warnings"
		headerStyle := warningStyle.Bold(true)
		if hasErrors {
			headerText = "✗ Health Issues"
			headerStyle = healthHeaderStyle
		}
		lines = append(lines, "", headerStyle.Render(headerText))
		for _, issue := range skillIssues {
			bullet := "  • "
			style := warningStyle
			if issue.Severity == "error" {
				style = errorStyle
			}
			line := fmt.Sprintf("%s%s: %s", bullet, humanHealthIssueType(issue.Type), humanHealthIssueMessage(issue.Type, issue.Message))
			if issue.Path != "" {
				line += " (" + issue.Path + ")"
			}
			lines = append(lines, style.Render(wrapText(line, width)))
		}
	}

	lines = append(lines, "")

	childRows := m.modalChildRows(groupName)

	lines = append(lines, sectionHeaderStyle.Render("Installed Skills:"))
	installedCount := 0
	for idx, cr := range childRows {
		if !cr.isAvailable {
			installedCount++
			if idx == m.modalSelected {
				scopeBadgeStr := "P"
				if cr.skill.Scope == model.ScopeGlobal {
					scopeBadgeStr = "G"
				}
				label := fmt.Sprintf("%s [%s]", cr.skill.Name, scopeBadgeStr)
				lines = append(lines, selectedStyle.Render(fmt.Sprintf("› %s", label)))
			} else {
				var scopeBadgeStyled string
				if cr.skill.Scope == model.ScopeProject {
					scopeBadgeStyled = scopeProjectStyle.Render("[P]")
				} else {
					scopeBadgeStyled = scopeGlobalStyle.Render("[G]")
				}
				label := fmt.Sprintf("%s %s", cr.skill.Name, scopeBadgeStyled)
				lines = append(lines, fmt.Sprintf("  %s", label))
			}
		}
	}
	if installedCount == 0 {
		lines = append(lines, "  No installed skills under this source.")
	}

	lines = append(lines, "")

	availHeader := sectionHeaderStyle.Render("Available Skills:")
	disc, ok := m.discovery[groupName]
	if ok && disc.Status == DiscoveryReady && !disc.ScannedAt.IsZero() {
		availHeader += "  " + dimStyle.Render("scanned "+humanizeSince(disc.ScannedAt))
	}
	lines = append(lines, availHeader)
	_, _, isRemote := parseRemoteGitHubSource(groupName)
	if !ok {
		discoverable, reason := m.isSourceDiscoverable(groupName)
		if !discoverable {
			lines = append(lines, errorStyle.Render("  Couldn't scan: "+reason))
		} else {
			lines = append(lines, dimStyle.Render("  Press d to scan this source."))
		}
	} else {
		switch disc.Status {
		case DiscoveryLoading:
			if isRemote {
				lines = append(lines, dimStyle.Render("  Scanning…"))
			} else {
				lines = append(lines, dimStyle.Render("  Scanning…"))
			}
		case DiscoveryFailed:
			lines = append(lines, errorStyle.Render("  Couldn't scan: "+disc.Error))
		case DiscoveryReady:
			availableCount := 0
			for idx, cr := range childRows {
				if cr.isAvailable {
					availableCount++
					if idx == m.modalSelected {
						label := fmt.Sprintf("%s [available]", cr.discoveredSkill.Name)
						lines = append(lines, selectedStyle.Render(fmt.Sprintf("› %s", label)))
					} else {
						label := fmt.Sprintf("%s %s", cr.discoveredSkill.Name, lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Render("[available]"))
						lines = append(lines, fmt.Sprintf("  %s", label))
					}
				}
			}
			if availableCount == 0 {
				lines = append(lines, "  All skills from this source are installed.")
			}
		}
	}

	if len(childRows) > 0 && m.modalSelected >= 0 && m.modalSelected < len(childRows) {
		selectedChild := childRows[m.modalSelected]
		lines = append(lines, "")
		previewDivider := lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render(strings.Repeat("─", max(1, width)))
		lines = append(lines, previewDivider)

		if selectedChild.isAvailable {
			ds := selectedChild.discoveredSkill
			lines = append(lines,
				titleStyle.Render(ds.Name+" [available]"),
				"",
				formatMetaLine("Status:", lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Render("available"), width),
				formatMetaLine("Source:", ds.Source, width),
			)
			if ds.SkillPath != "" {
				lines = append(lines, formatMetaLine("Path:", ds.SkillPath, width))
			}
			if ds.Description != "" {
				lines = append(lines, "", wrapText(ds.Description, width))
			}
			if ds.Preview != "" {
				lines = append(lines, "", sectionHeaderStyle.Render("Preview"), "")
				previewLines := strings.Split(ds.Preview, "\n")
				for _, line := range previewLines {
					lines = append(lines, wrapText(line, width))
				}
			}
		} else {
			sk := selectedChild.skill
			view := display.Skill(sk)
			lockVal := display.LockSummary(view)
			if lockVal == "not tracked" {
				lockVal = warningStyle.Render("not tracked")
			}
			lines = append(lines,
				titleStyle.Render(view.Name),
				"",
				formatMetaLine("Scope:", styledScopeBadge(string(view.Scope)), width),
				formatMetaLine("Lock:", lockVal, width),
			)
			if sourceLines := sourceDetailLines(sk, width); len(sourceLines) > 0 {
				lines = append(lines, sourceLines...)
			}
			if view.CanonicalPath != "" {
				lines = append(lines, formatMetaLine("Canonical:", view.CanonicalPath, width))
			}
			if view.Preview != "" {
				lines = append(lines, "", sectionHeaderStyle.Render("Preview"), "")
				previewLines := strings.Split(view.Preview, "\n")
				for _, line := range previewLines {
					lines = append(lines, wrapText(line, width))
				}
			}
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
			case "disabled":
				statusText = "disabled"
			case "unsupported_global":
				statusText = "global unsupported"
			case "agent_not_detected":
				statusText = "agent not detected"
			case "not_in_universal_canonical_dir":
				statusText = "not in shared folder"
			case "missing_agent_link":
				statusText = "not linked"
			}
			var statusStyled string
			if visibility.Visible {
				statusStyled = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Render(statusText)
			} else if visibility.Reason == "broken_symlink" || visibility.Reason == "disabled" {
				statusStyled = errorStyle.Render(statusText)
			} else {
				statusStyled = dimStyle.Render(statusText)
			}
			val := fmt.Sprintf("%s: %s", visibility.Display, statusStyled)
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
	var val string
	fraction := fmt.Sprintf("%d/%d", visible, total)
	if visible == total && total > 0 {
		val = fmt.Sprintf("Available to %s %s", lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Render(fraction), label)
	} else if visible > 0 {
		val = fmt.Sprintf("Available to %s %s", warningStyle.Render(fraction), label)
	} else {
		val = fmt.Sprintf("Available to %s %s", errorStyle.Render(fraction), label)
	}
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
	modalWidth := int(float64(layout.Width) * 0.75)
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
				if preview.Description != "" {
					lines = append(lines, "", sectionHeaderStyle.Render("Details:"))
					for _, descLine := range strings.Split(wrapText(compat.SanitizeMetadata(preview.Description), width-4), "\n") {
						lines = append(lines, activeActionSubStyle.Render(padRight("  "+descLine, width)))
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
				if preview.Description != "" {
					for _, descLine := range strings.Split(wrapText(compat.SanitizeMetadata(preview.Description), width-4), "\n") {
						lines = append(lines, normalActionSubStyle.Render("  "+descLine))
					}
				}
			}
			continue
		}
		titleText := compat.SanitizeMetadata(preview.Title)
		if i == m.action {
			// Selected Action Highlight Block (entire block has same purple background)
			titleLine := activeActionTitleStyle.Render(padRight(selector+titleText, width))
			lines = append(lines, "", titleLine)

			cmdText := truncate(compat.SanitizeMetadata(preview.Command), width-4)
			cmdLine := activeActionSubStyle.Render(padRight("  "+cmdText, width))
			lines = append(lines, cmdLine)
			if preview.Description != "" {
				lines = append(lines, "", sectionHeaderStyle.Render("Details:"))
				for _, descLine := range strings.Split(wrapText(compat.SanitizeMetadata(preview.Description), width-4), "\n") {
					lines = append(lines, activeActionSubStyle.Render(padRight("  "+descLine, width)))
				}
			}

		} else {
			// Unselected Action (normal colors, subordinate metadata very dim)
			titleLine := normalActionTitleStyle.Render(selector + titleText)
			lines = append(lines, "", titleLine)

			cmdText := truncate(compat.SanitizeMetadata(preview.Command), width-4)
			cmdLine := normalActionSubStyle.Render("  " + cmdText)
			lines = append(lines, cmdLine)
			if preview.Description != "" {
				for _, descLine := range strings.Split(wrapText(compat.SanitizeMetadata(preview.Description), width-4), "\n") {
					lines = append(lines, normalActionSubStyle.Render("  "+descLine))
				}
			}
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
		modalActions := m.currentActions()
		parts := []string{"↑/↓ scroll"}
		if hasAvailableAction(modalActions, "open_skill") {
			parts = append(parts, "o edit")
		}
		parts = append(parts, "c commands", "esc/q close")
		text = strings.Join(parts, " · ")
	} else if m.commands {
		text = "↑/↓ choose · enter run · e enable/disable · esc close"
	} else if m.helpOpen {
		text = "esc/q/? close help"
	} else if m.focus == focusMetadata {
		footerActions := m.currentActions()
		parts := []string{"↑/↓ scroll metadata", "enter open", "c commands"}
		if hasAvailableToggleAction(footerActions) {
			parts = append(parts, "e enable/disable")
		}
		parts = append(parts, "? help")
		text = strings.Join(parts, " · ")
	} else if m.focus == focusPreview {
		footerActions := m.currentActions()
		parts := []string{"↑/↓ scroll preview", "enter open", "c commands"}
		if hasAvailableToggleAction(footerActions) {
			parts = append(parts, "e enable/disable")
		}
		parts = append(parts, "? help")
		text = strings.Join(parts, " · ")
	} else {
		// focusSkills
		rows := m.visibleRows()
		if len(rows) > 0 && m.selected >= 0 && m.selected < len(rows) {
			row := rows[m.selected]
			footerActions := m.currentActions()
			if row.isHeader {
				parts := []string{"enter browse"}
				if hasAvailableToggleAction(footerActions) {
					parts = append(parts, "e enable/disable source")
				}
				if discoverable, _ := m.isSourceDiscoverable(row.groupName); discoverable {
					parts = append(parts, "d scan")
				}
				parts = append(parts, "c actions", "? help")
				text = strings.Join(parts, " · ")
			} else {
				parts := []string{"enter open"}
				if hasAvailableToggleAction(footerActions) {
					parts = append(parts, "e enable/disable")
				}
				parts = append(parts, "c actions")
				if hasAvailableAction(footerActions, preferredUpdateActionID(m.selectedCount())) {
					parts = append(parts, "u update")
				}
				if hasAvailableAction(footerActions, preferredRemoveActionID(m.selectedCount())) {
					parts = append(parts, "x remove")
				}
				parts = append(parts, "? help")
				text = strings.Join(parts, " · ")
			}
		} else {
			text = "enter open · c actions · ? help"
		}
	}
	isNormalState := !m.running && !m.confirming && !m.searching && !m.detailModal && !m.commands && !m.helpOpen
	if isNormalState && m.updatePlan != nil && m.updatePlan.Status == selfupdate.StatusAvailable {
		latestClean := m.updatePlan.Latest
		if strings.HasPrefix(strings.ToLower(latestClean), "v") {
			latestClean = latestClean[1:]
		}
		notice := fmt.Sprintf(" · U update (v%s available)", latestClean)
		if width >= 80 {
			text += notice
		}
	}
	return dimStyle.Render(truncate(text, width))
}

func hasAvailableAction(previews []actions.CommandPreview, id string) bool {
	if id == "" {
		return false
	}
	for _, action := range previews {
		if action.ID == id && action.Available {
			return true
		}
	}
	return false
}

func hasAvailableToggleAction(previews []actions.CommandPreview) bool {
	return hasAvailableAction(previews, "enable_skill") || hasAvailableAction(previews, "disable_skill")
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
		"  ↑/↓, j/k        Move selection (Inventory focus) or scroll (Metadata/Preview)",
		"  gg / G          Jump to top / bottom (also home / end)",
		"  1 / 2 / 3       Focus Inventory (1), Metadata (2), or Preview (3) pane",
		"  tab / shift-tab Cycle focus forward / backward through panes",
		"  ← / →           Move focus backward / forward outside Inventory; jump groups in Inventory",
		"  h / l           Collapse / expand current source group in Inventory",
		"  [ / ]           Jump to previous / next source group in Inventory",
		"",
		sectionHeaderStyle.Render("Filters:"),
		"  f / F           Cycle scope filter (All/Project/Global) / Clear to All",
		"  a / A           Cycle agent filter / Clear agent filter",
		"  /               Initiate text search",
		"",
		sectionHeaderStyle.Render("Actions & Selection:"),
		"  enter           Open detail modal for the selected source or skill",
		"  space           Mark / unmark selected skill for bulk actions",
		"  s               Mark / unmark skills in the current source group",
		"  o               Open selected skill directly in editor",
		"  e               Enable / disable selected skill or source group",
		"  c               Open command picker menu",
		"  u / x           Quick reinstall-update / remove for selection",
		"  U               Check/run LazySkills application update",
		"  d               Check local or remote source for available skills (Source row)",
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
	modalWidth, modalHeight := detailModalDimensions(layout)

	m.viewport.Width = modalWidth - 4
	m.viewport.Height = modalHeight - 6
	m.viewport.SetContent(m.detailText(modalWidth - 4))

	helpLine := dimStyle.Render(m.detailModalHelpLine())

	content := []string{
		titleStyle.Render(m.detailModalTitle()),
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

func (m appModel) detailModalTitle() string {
	if m.modalSource != "" {
		return "Source Detail View"
	}
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		return "Detail View"
	}
	row := rows[m.selected]
	if row.isHeader {
		return "Source Detail View"
	}
	return "Skill Detail View"
}

func (m appModel) detailModalHelpLine() string {
	modalActions := m.currentActions()
	if m.modalSource != "" {
		parts := []string{"esc/q close", "↑/↓ select"}
		if child, ok := m.currentModalSelectedChild(); ok {
			if child.isAvailable && hasAvailableAction(modalActions, "install_skill") {
				parts = append(parts, "enter install")
			} else if !child.isAvailable && hasAvailableAction(modalActions, "open_skill") {
				parts = append(parts, "enter open", "o open")
			}
		}
		parts = append(parts, "c more")
		if discoverable, _ := m.isSourceDiscoverable(m.modalSource); discoverable {
			parts = append(parts, "d scan")
		}
		return strings.Join(parts, " · ")
	}
	parts := []string{"esc/q close"}
	if hasAvailableAction(m.currentDetailSkillActions(), "open_skill") {
		parts = append(parts, "o open in editor")
	}
	parts = append(parts, "c command picker", "↑/↓ scroll")
	return strings.Join(parts, " · ")
}

func (m appModel) currentDetailSkillActions() []actions.CommandPreview {
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) || rows[m.selected].isHeader {
		return nil
	}
	return actions.ForSkill(rows[m.selected].skill)
}

func (m *appModel) ensureSourceModalSelectionVisible() {
	if m.modalSource == "" {
		return
	}
	// The source modal renders source metadata before the child list. Keep a
	// little context above the selected child rather than treating modalSelected
	// as a raw viewport line number.
	selectedLine := 8 + m.modalSelected
	if selectedLine < 0 {
		selectedLine = 0
	}
	height := m.viewport.Height
	if height <= 0 {
		// View() computes the final modal viewport height. Use the same rough
		// default here so keyboard movement updates the offset before render.
		height = 18
	}
	if selectedLine < m.viewport.YOffset+3 {
		offset := selectedLine - 3
		if offset < 0 {
			offset = 0
		}
		m.viewport.SetYOffset(offset)
		return
	}
	bottom := m.viewport.YOffset + height - 4
	if selectedLine > bottom {
		offset := selectedLine - height + 4
		if offset < 0 {
			offset = 0
		}
		m.viewport.SetYOffset(offset)
	}
}

func (m appModel) confirmationOverlay(layout appLayout) string {
	title := "Confirm action"
	phrase := ""
	command := ""
	dangerous := false
	exact := false
	if m.pendingAction != nil {
		title = compat.SanitizeMetadata(m.pendingAction.Title)
		phrase = compat.SanitizeMetadata(m.pendingAction.ConfirmValue)
		command = compat.SanitizeMetadata(m.pendingAction.Command)
		dangerous = m.pendingAction.Dangerous
		exact = requiresExactConfirmation(*m.pendingAction)
	} else if acts := m.currentActions(); len(acts) > 0 && m.action < len(acts) {
		action := acts[m.action]
		title = compat.SanitizeMetadata(action.Title)
		phrase = compat.SanitizeMetadata(action.ConfirmValue)
		command = compat.SanitizeMetadata(action.Command)
		dangerous = action.Dangerous
		exact = requiresExactConfirmation(action)
	}
	headerStyle := sectionHeaderStyle
	borderColor := actionBorderColor
	placeholder := "Enter"
	instruction := "Press Enter to continue, or Esc to cancel."
	if dangerous {
		headerStyle = errorStyle.Bold(true)
		borderColor = lipgloss.Color("203")
		placeholder = "y / yes"
		instruction = "Type y or yes to confirm, or Esc to cancel."
		if exact {
			placeholder = phrase
			instruction = fmt.Sprintf("Type %q to confirm, or Esc to cancel.", phrase)
		}
	}
	lines := []string{
		headerStyle.Render(title),
		"",
	}
	if command != "" {
		lines = append(lines, sectionHeaderStyle.Render("Command"), dimStyle.Render(wrapText(command, 48)), "")
	}
	if m.pendingAction != nil && m.pendingAction.Description != "" {
		lines = append(lines, sectionHeaderStyle.Render("Details"), wrapText(m.pendingAction.Description, 48), "")
	} else if acts := m.currentActions(); m.pendingAction == nil && len(acts) > 0 && m.action < len(acts) {
		action := acts[m.action]
		if action.Description != "" {
			lines = append(lines, sectionHeaderStyle.Render("Details"), wrapText(action.Description, 48), "")
		}
	}
	lines = append(lines, wrapText(instruction, 48))

	if m.confirmError != "" {
		lines = append(lines, "", errorStyle.Render(m.confirmError))
	}
	input := compat.SanitizeMetadata(m.confirmInput)
	if input == "" {
		input = dimStyle.Render(placeholder)
	}
	lines = append(lines, "", "> "+input+"_")
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderColor).Padding(1, 2).Width(52).Render(strings.Join(lines, "\n"))
	return fitToScreen(lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, box), layout.Width, layout.Height)
}

func (m appModel) runningOverlay(layout appLayout) string {
	title := compat.SanitizeMetadata(compat.FirstNonEmpty(m.runningTitle, "Running action"))
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

func (m appModel) appUpdateModalOverlay(layout appLayout) string {
	modalWidth := 74
	if layout.Width < modalWidth+4 {
		modalWidth = layout.Width - 4
	}
	if modalWidth < 20 {
		modalWidth = 20
	}

	var sections []string
	sections = append(sections, titleStyle.Render(" LazySkills App Update "))
	sections = append(sections, "")

	plan := m.updatePlan
	if m.updatePlanErr != nil {
		sections = append(sections, errorStyle.Render("✗ Update check failed:"))
		sections = append(sections, wrapText(m.updatePlanErr.Error(), modalWidth-4))
		sections = append(sections, "")
		sections = append(sections, dimStyle.Render("esc/q close"))
	} else if plan == nil {
		sections = append(sections, "Checking for updates...")
		sections = append(sections, "")
		sections = append(sections, dimStyle.Render("esc/q close"))
	} else {
		sections = append(sections, fmt.Sprintf("Current Version: %s", plan.Current))
		sections = append(sections, fmt.Sprintf("Latest Version:  %s", plan.Latest))
		sections = append(sections, fmt.Sprintf("Install Channel: %s", plan.Channel))
		sections = append(sections, "")

		if m.updatingApp {
			sections = append(sections, runningStyle.Render("Updating... Please wait."))
			sections = append(sections, "Downloading archive and verifying checksum...")
		} else if m.updateSuccess {
			sections = append(sections, successStyle.Render("✓ Update successful!"))
			sections = append(sections, "Please restart lazyskills to apply the update.")
			sections = append(sections, "")
			sections = append(sections, dimStyle.Render("esc/q close"))
		} else if m.updateError != nil {
			sections = append(sections, errorStyle.Render("✗ Update failed:"))
			sections = append(sections, wrapText(m.updateError.Error(), modalWidth-4))
			sections = append(sections, "")
			sections = append(sections, dimStyle.Render("enter retry · esc/q close"))
		} else {
			if plan.CanExecute {
				sections = append(sections, "A newer version is available. Would you like to update?")
				sections = append(sections, "")
				if plan.ReleaseNotes != "" {
					sections = append(sections, sectionHeaderStyle.Render("Release Notes:"), truncateReleaseNotes(plan.ReleaseNotes, modalWidth-4), "")
				}
				sections = append(sections, "enter start update · esc/q cancel")
			} else {
				if plan.Status == selfupdate.StatusAlreadyLatest {
					sections = append(sections, "You are already running the latest version.")
				} else if plan.Status == selfupdate.StatusUnknown {
					sections = append(sections, "Update check status is unknown.")
					sections = append(sections, plan.Reason)
				} else {
					sections = append(sections, "Auto-update is not supported for this install channel.")
					sections = append(sections, plan.Reason)
					if plan.CommandPreview != "" {
						sections = append(sections, "")
						sections = append(sections, "To upgrade, run:")
						sections = append(sections, lipgloss.NewStyle().Foreground(lipgloss.Color("141")).Render("  "+plan.CommandPreview))
					}
				}
				sections = append(sections, "")
				sections = append(sections, dimStyle.Render("esc/q close"))
			}
		}
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(actionBorderColor).
		Padding(1, 2).
		Width(modalWidth).
		Render(strings.Join(sections, "\n"))

	return fitToScreen(lipgloss.Place(layout.Width, layout.Height, lipgloss.Center, lipgloss.Center, box), layout.Width, layout.Height)
}

func truncateReleaseNotes(notes string, width int) string {
	lines := strings.Split(notes, "\n")
	var out []string
	for i, line := range lines {
		if i >= 10 {
			out = append(out, "... (truncated)")
			break
		}
		out = append(out, wrapText(line, width))
	}
	return strings.Join(out, "\n")
}
