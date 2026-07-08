package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/muesli/reflow/wordwrap"
)

// humanizeSince renders a coarse, git-style relative age ("just now", "5m ago").
func humanizeSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 5*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
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
	if width <= 1 || xansi.StringWidth(s) <= width {
		return s
	}
	return xansi.Truncate(s, width, "…")
}

func clampLineWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if xansi.StringWidth(s) <= width {
		return s
	}
	return xansi.Truncate(s, width, "")
}

func clampBlockWidth(s string, width int) string {
	if width <= 0 || s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = clampLineWidth(line, width)
	}
	return strings.Join(lines, "\n")
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
		lines[i] = clampLineWidth(line, width)
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

func detailModalDimensions(layout appLayout) (width, height int) {
	width = int(float64(layout.Width) * 0.85)
	if width < 80 {
		width = 80
	}
	if width > 140 {
		width = 140
	}
	if layout.Width < width+4 {
		width = layout.Width - 4
	}
	if width < 20 {
		width = 20
	}

	height = int(float64(layout.Height) * 0.80)
	if height < 24 {
		height = 24
	}
	if height > 45 {
		height = 45
	}
	if layout.Height < height+4 {
		height = layout.Height - 4
	}
	if height < 7 {
		height = 7
	}
	return width, height
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

func (m appModel) installedSkillNames(group string) map[string]bool {
	installed := make(map[string]bool)
	for _, sk := range m.result.Skills {
		if listGroupLabel(sk) == group {
			installed[compat.NormalizeName(sk.Name)] = true
		}
	}
	return installed
}

func isSkillNameInstalled(name string, installed map[string]bool) bool {
	return installed[compat.NormalizeName(name)]
}
