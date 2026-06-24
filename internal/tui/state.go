package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/model"
)

func (m *appModel) syncViewport() {
	start := time.Now()
	defer func() {
		m.viewportSyncFingerprint = m.currentViewportSyncFingerprint()
		perfLogf("sync selected=%d focus=%d modal=%t source=%q preview_pending=%t duration=%s", m.selected, m.focus, m.detailModal, m.modalSource, m.previewPending, time.Since(start))
	}()
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
		modalWidth, modalHeight := detailModalDimensions(layout)
		m.viewport.Width = modalWidth - 4
		m.viewport.Height = modalHeight - 6
		m.viewport.SetContent(m.detailText(modalWidth - 4))
	} else {
		_, rightWidth, topHeight, bottomHeight := m.getThreePaneLayout()

		// For metadata viewport:
		m.metadataViewport.Width = max(1, rightWidth-4)
		m.metadataViewport.Height = max(1, topHeight-2)
		rows := m.visibleRows()
		m.metadataViewport.SetContent(strings.Join(m.metadataLinesForRows(rows, rightWidth-4), "\n"))

		// For preview viewport:
		m.previewViewport.Width = max(1, rightWidth-4)
		m.previewViewport.Height = max(1, bottomHeight-2)
		m.previewViewport.SetContent(strings.Join(m.previewLinesForRows(rows, rightWidth-4), "\n"))
	}
	m.clampViewportOffset()
}

func (m appModel) currentViewportSyncFingerprint() string {
	selectedKey := m.currentSelectedKey()
	collapsed := false
	if strings.HasPrefix(selectedKey, "group:") {
		collapsed = m.isCollapsed(strings.TrimPrefix(selectedKey, "group:"))
	}
	return fmt.Sprintf("%d\x00%d\x00%d\x00%d\x00%s\x00%s\x00%t\x00%t\x00%t\x00%s\x00%d\x00%t\x00%s\x00%t",
		m.width,
		m.height,
		m.selected,
		m.focus,
		m.agent,
		m.search,
		m.detailModal,
		m.commands,
		m.helpOpen,
		m.modalSource,
		m.modalSelected,
		m.previewPending,
		selectedKey,
		collapsed,
	)
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

// availableCount reports how many uninstalled skills are new since the last
// radar scan for a source group. Zero when not yet scanned.
func (m appModel) availableCount(groupName string) int {
	disc, ok := m.discovery[groupName]
	if !ok || len(disc.Skills) == 0 {
		return 0
	}
	installed := m.installedSkillNames(groupName)
	n := 0
	for _, ds := range disc.Skills {
		if ds.NewSinceLastScan && !isSkillNameInstalled(ds.Name, installed) {
			n++
		}
	}
	return n
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

// jumpListTop / jumpListBottom move to the start/end of whichever pane has
// focus: the inventory selection, or the metadata/preview scroll position.
// clampSelection (called after key handling) fixes the empty-list case.
func (m *appModel) jumpListTop() {
	switch m.focus {
	case focusMetadata:
		m.metadataViewport.GotoTop()
	case focusPreview:
		m.previewViewport.GotoTop()
	default:
		m.selected = 0
		m.actionResult = nil
		m.metadataViewport.GotoTop()
		m.previewViewport.GotoTop()
	}
}

func (m *appModel) jumpListBottom() {
	switch m.focus {
	case focusMetadata:
		m.metadataViewport.GotoBottom()
	case focusPreview:
		m.previewViewport.GotoBottom()
	default:
		m.selected = len(m.visibleRows()) - 1
		m.actionResult = nil
		m.metadataViewport.GotoTop()
		m.previewViewport.GotoTop()
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
	if row.isHeader || row.skill == nil {
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
	var groupSkills []*model.Skill
	items := m.filteredSkills()
	for _, skill := range items {
		if listGroupLabel(skill) == group {
			groupSkills = append(groupSkills, skill)
		}
	}
	if len(groupSkills) == 0 {
		return
	}

	allSelected := true
	for _, skill := range groupSkills {
		if !m.isSelected(skill) {
			allSelected = false
			break
		}
	}
	if allSelected {
		for _, skill := range groupSkills {
			delete(m.selectedKeys, skillKey(skill))
		}
		if len(m.selectedKeys) == 0 {
			m.selectedKeys = nil
		}
		return
	}

	if m.selectedKeys == nil {
		m.selectedKeys = map[string]bool{}
	}
	for _, skill := range groupSkills {
		m.selectedKeys[skillKey(skill)] = true
	}
	if len(m.selectedKeys) == 0 {
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

func (m appModel) sourceGroupSkills(group string) []*model.Skill {
	var out []*model.Skill
	for _, skill := range m.result.Skills {
		if listGroupLabel(skill) == group {
			out = append(out, skill)
		}
	}
	return out
}

func (m appModel) sourceActions(group string) []actions.CommandPreview {
	skills := m.sourceGroupSkills(group)
	previews := actions.ForSkills(skills)
	for i := range previews {
		switch previews[i].ID {
		case "bulk_reinstall_update":
			previews[i].Title = "Update installed skills from source"
			previews[i].Description = "Refresh installed skills from this source."
		case "bulk_remove":
			previews[i].Title = "Remove installed skills from source"
			previews[i].Description = "Remove installed skills from this source."
			previews[i].ConfirmValue = group
		}
	}
	previews = append(m.sourceEnableDisableActions(skills), previews...)

	discoverable, reason := m.isSourceDiscoverable(group)

	_, _, isRemote := parseRemoteGitHubSource(group)
	title := "Check local source for available skills"
	desc := "Scan the local source root for uninstalled skills."
	if isRemote {
		title = "Check remote source for available skills"
		desc = "Scan this source for available skills."
	}
	discPreview := actions.CommandPreview{
		ID:              "source_discover",
		Title:           title,
		Description:     desc,
		Command:         fmt.Sprintf("discover %s", group),
		Exec:            actions.ExecSpec{Internal: "discover", Args: []string{group}},
		RequiresConfirm: false,
		Available:       discoverable,
		Reason:          reason,
		ConfirmValue:    group, // Reused to pass group name
	}
	previews = append(previews, discPreview)

	return previews
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

	// Metadata carries source, description, visibility, and health details, so
	// give it a little more room while keeping preview as the larger pane.
	topHeight = height * 4 / 10
	if topHeight > 16 {
		topHeight = 16
	}
	if topHeight < 5 {
		topHeight = 5
	}
	if topHeight > height-5 {
		topHeight = height - 5
	}
	bottomHeight = height - topHeight
	return
}
