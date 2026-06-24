package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/display"
	"github.com/alvinunreal/lazyskills/internal/model"
)

const updateReviewSkillLimit = 3

type fileChange struct {
	Path   string
	Status string
}

func (m appModel) updateReviewLines(action actions.CommandPreview, width int) []string {
	if !isUpdateReviewAction(action.ID) {
		return nil
	}
	skills := m.updateReviewTargets(action)
	if len(skills) == 0 {
		return nil
	}

	title := "Update review"
	if len(skills) > 1 {
		title = fmt.Sprintf("Update review (%d skills)", len(skills))
	}
	lines := []string{sectionHeaderStyle.Render(title), ""}

	limit := len(skills)
	if limit > updateReviewSkillLimit {
		limit = updateReviewSkillLimit
	}
	for i, skill := range skills[:limit] {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, m.updateReviewLinesForSkill(skill, width)...)
	}
	if len(skills) > limit {
		lines = append(lines, "", dimStyle.Render(fmt.Sprintf("… and %d more skills", len(skills)-limit)))
	}
	return lines
}

func isUpdateReviewAction(id string) bool {
	switch id {
	case "reinstall_update", "bulk_reinstall_update":
		return true
	default:
		return false
	}
}

func (m appModel) updateReviewTargets(action actions.CommandPreview) []*model.Skill {
	if action.ID == "bulk_reinstall_update" {
		if selected := m.selectedSkills(); len(selected) > 0 {
			return selected
		}
	}
	rows := m.visibleRows()
	if len(rows) == 0 || m.selected < 0 || m.selected >= len(rows) {
		if action.ID == "bulk_reinstall_update" {
			return m.selectedSkills()
		}
		return nil
	}
	row := rows[m.selected]
	if action.ID == "bulk_reinstall_update" {
		if row.isHeader {
			return m.sourceGroupSkills(row.groupName)
		}
	}
	if row.isHeader {
		return nil
	}
	return []*model.Skill{row.skill}
}

func (m appModel) updateReviewLinesForSkill(skill *model.Skill, width int) []string {
	if skill == nil {
		return nil
	}
	view := display.Skill(skill)
	lines := []string{formatMetaLine("Skill:", view.Name, width)}
	if view.Scope != "" {
		lines = append(lines, formatMetaLine("Scope:", styledScopeBadge(view.Scope), width))
	}
	if installed := compat.FirstNonEmpty(view.CanonicalPath, view.SkillPath); installed != "" {
		lines = append(lines, formatMetaLine("Installed:", installed, width))
	}
	if reviewSource := updateReviewSourceLines(skill, width); len(reviewSource) > 0 {
		lines = append(lines, reviewSource...)
	}
	if comparison := m.updateComparisonLines(skill, width); len(comparison) > 0 {
		lines = append(lines, "")
		lines = append(lines, comparison...)
	}
	return lines
}

func updateReviewSourceLines(skill *model.Skill, width int) []string {
	info := sourceInfo(skill)
	if info.Source == "" {
		return nil
	}
	lines := []string{formatMetaLine("Source:", info.Source, width)}
	if info.SourceURL != "" && info.SourceURL != info.Source {
		lines = append(lines, formatMetaLine("Source URL:", info.SourceURL, width))
	}
	if info.SourceType != "" {
		lines = append(lines, formatMetaLine("Source type:", info.SourceType, width))
	}
	if info.Folder != "" {
		lines = append(lines, formatMetaLine("Folder:", info.Folder, width))
	}
	if info.Ref != "" {
		lines = append(lines, formatMetaLine("Ref:", info.Ref, width))
	}
	if info.Hash != "" {
		lines = append(lines, formatMetaLine("Hash:", info.Hash, width))
	}
	return lines
}

func (m appModel) updateComparisonLines(skill *model.Skill, width int) []string {
	installedRoot, sourceRoot, reason, ok := m.updateComparisonRoots(skill)
	if !ok {
		return []string{dimStyle.Render("Content comparison unavailable: " + compat.SanitizeMetadata(reason))}
	}

	changes, err := compareSkillTrees(sourceRoot, installedRoot)
	if err != nil {
		return []string{dimStyle.Render("Content comparison unavailable: " + compat.SanitizeMetadata(err.Error()))}
	}
	if len(changes) == 0 {
		return []string{successStyle.Render("No content changes detected")}
	}

	lines := []string{sectionHeaderStyle.Render("Changed files"), ""}
	for _, change := range changes {
		label := change.Status
		if label == "" {
			label = "modified"
		}
		line := fmt.Sprintf("  • %s: %s", label, compat.SanitizeMetadata(change.Path))
		lines = append(lines, wrapText(line, width))
	}
	return lines
}

func (m appModel) updateComparisonRoots(skill *model.Skill) (installedRoot, sourceRoot, reason string, ok bool) {
	if skill == nil {
		return "", "", "skill is unavailable", false
	}
	installedRoot = normalizeSkillRoot(compat.FirstNonEmpty(skill.CanonicalPath, firstObservedSkillPath(skill), skill.SkillPath))
	if installedRoot == "" {
		return "", "", "installed skill path is unavailable", false
	}
	if st, err := os.Stat(installedRoot); err != nil || !st.IsDir() {
		return "", "", "installed skill directory is unavailable", false
	}

	rawSource, rawSourceType, rawSkillPath := skillUpdateRawSource(skill)
	sourceRoot = localComparisonRoot(m.cwd, rawSourceType, rawSource)
	if sourceRoot == "" {
		return "", "", "source path is remote or unavailable in this workspace", false
	}
	sourceRoot = normalizeSkillSourceRoot(sourceRoot, skillFolder(rawSkillPath))
	if sourceRoot == "" {
		return "", "", "source skill path is unavailable", false
	}
	if st, err := os.Stat(sourceRoot); err != nil || !st.IsDir() {
		return "", "", "source skill directory is unavailable", false
	}
	return installedRoot, sourceRoot, "", true
}

func skillUpdateRawSource(skill *model.Skill) (source, sourceType, skillPath string) {
	if skill == nil {
		return "", "", ""
	}
	if skill.Scope == model.ScopeProject && skill.LocalLock != nil {
		return skill.LocalLock.Source, skill.LocalLock.SourceType, skill.LocalLock.SkillPath
	}
	if skill.Scope == model.ScopeGlobal && skill.GlobalLock != nil {
		return compat.FirstNonEmpty(skill.GlobalLock.Source, skill.GlobalLock.SourceURL), skill.GlobalLock.SourceType, skill.GlobalLock.SkillPath
	}
	if skill.LocalLock != nil {
		return skill.LocalLock.Source, skill.LocalLock.SourceType, skill.LocalLock.SkillPath
	}
	if skill.GlobalLock != nil {
		return compat.FirstNonEmpty(skill.GlobalLock.Source, skill.GlobalLock.SourceURL), skill.GlobalLock.SourceType, skill.GlobalLock.SkillPath
	}
	return "", "", ""
}

func firstObservedSkillPath(skill *model.Skill) string {
	if skill == nil {
		return ""
	}
	for _, observed := range skill.ObservedPaths {
		if observed.Path != "" {
			return observed.Path
		}
	}
	return ""
}

func localComparisonRoot(cwd, sourceType string, source string) string {
	if source == "" {
		return ""
	}
	if isRemoteSource(sourceType, source) {
		return ""
	}
	candidates := []string{source}
	if cwd != "" && !filepath.IsAbs(source) {
		candidates = append([]string{filepath.Join(cwd, source)}, candidates...)
	}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return filepath.Clean(candidate)
		}
	}
	return ""
}

func isRemoteSource(sourceType, source string) bool {
	kind := strings.ToLower(strings.TrimSpace(sourceType))
	switch kind {
	case "github", "git", "remote", "url", "https", "http":
		return true
	case "local", "directory", "path", "file":
		return false
	}
	s := strings.ToLower(strings.TrimSpace(source))
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "git@") || strings.HasSuffix(s, ".git")
}

func normalizeSkillRoot(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			if abs, err := filepath.Abs(path); err == nil {
				return abs
			}
			return filepath.Clean(path)
		}
		path = filepath.Dir(path)
	}
	if strings.HasSuffix(strings.ToUpper(path), string(filepath.Separator)+"SKILL.MD") {
		path = filepath.Dir(path)
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

func normalizeSkillSourceRoot(path, skillFolder string) string {
	if path == "" {
		return ""
	}
	path = normalizeSkillRoot(path)
	if skillFolder == "" {
		return path
	}
	if pathHasSkillMarker(path) || strings.HasSuffix(filepath.ToSlash(path), "/"+strings.Trim(skillFolder, "/")) {
		return path
	}
	joined := filepath.Join(path, filepath.FromSlash(skillFolder))
	if st, err := os.Stat(joined); err == nil && st.IsDir() {
		return joined
	}
	return path
}

func pathHasSkillMarker(path string) bool {
	if path == "" {
		return false
	}
	if st, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil && !st.IsDir() {
		return true
	}
	return strings.HasSuffix(strings.ToUpper(filepath.Base(path)), "SKILL.MD")
}

func compareSkillTrees(sourceRoot, installedRoot string) ([]fileChange, error) {
	sourceFiles, err := snapshotFiles(sourceRoot)
	if err != nil {
		return nil, err
	}
	installedFiles, err := snapshotFiles(installedRoot)
	if err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	changes := make([]fileChange, 0)
	for path, sourceHash := range sourceFiles {
		seen[path] = true
		installedHash, ok := installedFiles[path]
		if !ok {
			changes = append(changes, fileChange{Path: path, Status: "added"})
			continue
		}
		if installedHash != sourceHash {
			changes = append(changes, fileChange{Path: path, Status: "modified"})
		}
	}
	for path := range installedFiles {
		if seen[path] {
			continue
		}
		changes = append(changes, fileChange{Path: path, Status: "removed"})
	}

	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Path == changes[j].Path {
			return changes[i].Status < changes[j].Status
		}
		return changes[i].Path < changes[j].Path
	})
	return changes, nil
}

func snapshotFiles(root string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if shouldSkipComparisonDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if shouldSkipComparisonFile(d.Name()) {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(data)
		files[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func shouldSkipComparisonDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".agents", ".slim":
		return true
	default:
		return false
	}
}

func shouldSkipComparisonFile(name string) bool {
	switch name {
	case ".DS_Store":
		return true
	default:
		return false
	}
}
