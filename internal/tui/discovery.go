package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/frontmatter"
	"github.com/alvinunreal/lazyskills/internal/model"
	tea "github.com/charmbracelet/bubbletea"
)

func (m appModel) resolveGroupSourceRoot(groupName string) string {
	if st, err := os.Stat(groupName); err == nil && st.IsDir() {
		return groupName
	}
	skills := m.sourceGroupSkills(groupName)
	for _, sk := range skills {
		if root := resolveSourceRoot(sk); root != "" {
			if st, err := os.Stat(root); err == nil && st.IsDir() {
				return root
			}
		}
	}
	return ""
}

func resolveSourceRoot(skill *model.Skill) string {
	if skill == nil {
		return ""
	}
	pathOnDisk := skill.CanonicalPath
	if pathOnDisk == "" {
		for _, op := range skill.ObservedPaths {
			if op.Path != "" {
				pathOnDisk = op.Path
				break
			}
		}
	}
	if pathOnDisk == "" {
		return ""
	}

	relPath := ""
	sourceType := ""
	if skill.LocalLock != nil {
		relPath = skill.LocalLock.SkillPath
		sourceType = skill.LocalLock.SourceType
	} else if skill.GlobalLock != nil {
		relPath = skill.GlobalLock.SkillPath
		sourceType = skill.GlobalLock.SourceType
	}

	relPath = strings.TrimSuffix(relPath, "/SKILL.md")
	relPath = strings.TrimSuffix(relPath, "SKILL.md")
	relPath = strings.Trim(relPath, "/")

	if relPath == "" {
		absDisk := filepath.Clean(pathOnDisk)
		if st, err := os.Stat(filepath.Join(absDisk, ".git")); err == nil && st.IsDir() {
			return absDisk
		}
		if sourceType == "local" || sourceType == "directory" {
			return absDisk
		}
		return ""
	}

	absDisk := filepath.Clean(pathOnDisk)
	relClean := filepath.Clean(relPath)

	if strings.HasSuffix(absDisk, relClean) {
		root := strings.TrimSuffix(absDisk, relClean)
		root = filepath.Clean(root)
		if st, err := os.Stat(filepath.Join(root, ".git")); err == nil && st.IsDir() {
			return root
		}
		if sourceType == "local" || sourceType == "directory" {
			return root
		}
	}

	return ""
}

func discoverSourceSkills(sourceRoot string) ([]DiscoveredSkill, error) {
	var discovered []DiscoveredSkill
	err := filepath.WalkDir(sourceRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".agents" || name == ".slim" {
				return filepath.SkipDir
			}
			rel, relErr := filepath.Rel(sourceRoot, path)
			if relErr == nil {
				depth := len(strings.Split(filepath.ToSlash(rel), "/"))
				if depth > 5 {
					return filepath.SkipDir
				}
			}
			return nil
		}

		if d.Name() == "SKILL.md" {
			doc, parseErr := frontmatter.ParseFile(path)
			if parseErr == nil {
				contentBytes, readErr := os.ReadFile(path)
				previewStr := ""
				if readErr == nil {
					previewStr = string(contentBytes)
				}
				discovered = append(discovered, DiscoveredSkill{
					Name:        compat.SanitizeMetadata(doc.Name),
					Description: compat.SanitizeMetadata(doc.Description),
					SkillPath:   compat.SanitizeMetadata(path),
					Preview:     compat.SanitizePreviewContent(previewStr),
				})
			}
		}
		return nil
	})
	return discovered, err
}

func rawSourceRef(skill *model.Skill) string {
	if skill == nil {
		return ""
	}
	if skill.Scope == model.ScopeProject && skill.LocalLock != nil {
		return skill.LocalLock.Ref
	}
	if skill.Scope == model.ScopeGlobal && skill.GlobalLock != nil {
		return skill.GlobalLock.Ref
	}
	if skill.LocalLock != nil {
		return skill.LocalLock.Ref
	}
	if skill.GlobalLock != nil {
		return skill.GlobalLock.Ref
	}
	return ""
}

func validateRawSource(raw string) error {
	if raw == "" {
		return fmt.Errorf("source is empty")
	}
	// Check for escapes, control characters, or newlines
	for _, r := range raw {
		if r < 32 || r == 127 || (r >= 128 && r <= 159) {
			return fmt.Errorf("raw source contains control, newline, or escape characters")
		}
	}
	// Also check if sanitization changes it
	if compat.SanitizeMetadata(raw) != raw {
		return fmt.Errorf("raw source contains unsafe characters or is modified by sanitization")
	}
	return nil
}

func (m appModel) validateRawSourcesForGroup(groupName string) error {
	skills := m.sourceGroupSkills(groupName)
	if len(skills) == 0 {
		return nil
	}
	for _, sk := range skills {
		var raw string
		var hasLock bool
		if sk.Scope == model.ScopeProject && sk.LocalLock != nil {
			raw = sk.LocalLock.Source
			hasLock = true
		} else if sk.Scope == model.ScopeGlobal && sk.GlobalLock != nil {
			hasLock = true
			if sk.GlobalLock.Source != "" {
				raw = sk.GlobalLock.Source
			} else {
				raw = sk.GlobalLock.SourceURL
			}
		} else {
			if sk.LocalLock != nil {
				raw = sk.LocalLock.Source
				hasLock = true
			} else if sk.GlobalLock != nil {
				hasLock = true
				if sk.GlobalLock.Source != "" {
					raw = sk.GlobalLock.Source
				} else {
					raw = sk.GlobalLock.SourceURL
				}
			}
		}
		if !hasLock {
			continue
		}
		if err := validateRawSource(raw); err != nil {
			return err
		}
	}
	return nil
}

func (m appModel) isSourceDiscoverable(group string) (bool, string) {
	if err := m.validateRawSourcesForGroup(group); err != nil {
		return false, err.Error()
	}
	if root := m.resolveGroupSourceRoot(group); root != "" {
		return true, ""
	}

	refToCheck := ""
	if idx := strings.Index(group, "#"); idx != -1 {
		refToCheck = group[idx+1:]
	}
	if refToCheck != "" && !isSafeGitHubRef(refToCheck) {
		return false, "ref contains unsafe or invalid characters"
	}

	_, ref, ok := parseRemoteGitHubSource(group)
	if !ok {
		return false, "requires a local checkout or a remote GitHub source"
	}

	if ref == "" {
		for _, sk := range m.sourceGroupSkills(group) {
			rawRef := rawSourceRef(sk)
			if rawRef != "" {
				if !isSafeGitHubRef(rawRef) {
					return false, "ref contains unsafe or invalid characters"
				}
				ref = rawRef
				break
			}
		}
	}
	if ref != "" && !isSafeGitHubRef(ref) {
		return false, "ref contains unsafe or invalid characters"
	}

	return true, ""
}

func (m appModel) startDiscovery(groupName string, force bool) (tea.Model, tea.Cmd) {
	if m.discovery != nil {
		if disc, exists := m.discovery[groupName]; exists && disc.Status == DiscoveryLoading {
			return m, nil
		}
	}

	if m.discovery == nil {
		m.discovery = make(map[string]SourceDiscovery)
	}
	m.discovery[groupName] = SourceDiscovery{
		Status: DiscoveryLoading,
	}

	discoverable, reason := m.isSourceDiscoverable(groupName)
	if !discoverable {
		m.discovery[groupName] = SourceDiscovery{
			Status: DiscoveryFailed,
			Error:  reason,
		}
		return m, nil
	}

	root := m.resolveGroupSourceRoot(groupName)
	if root != "" {
		return m, func() tea.Msg {
			skills, err := discoverSourceSkills(root)
			for i := range skills {
				skills[i].Source = compat.SanitizeMetadata(groupName)
			}
			return discoveryResultMsg{
				groupName: groupName,
				skills:    skills,
				err:       err,
			}
		}
	}

	url, ref, _ := parseRemoteGitHubSource(groupName)
	if ref == "" {
		for _, sk := range m.sourceGroupSkills(groupName) {
			rawRef := rawSourceRef(sk)
			if rawRef != "" {
				// Only adopt the lock's ref if it is a safe git ref; an unsafe
				// value must never reach `git checkout` (option-injection guard).
				if isSafeGitHubRef(rawRef) {
					ref = rawRef
				}
				break
			}
		}
	}

	cleanRef := compat.SanitizeMetadata(ref)
	return m, func() tea.Msg {
		dir, cleanup, err := cachedSourceClone(url, cleanRef, force)
		if err != nil {
			return discoveryResultMsg{
				groupName: groupName,
				err:       errors.New(compat.SanitizeMetadata(err.Error())),
			}
		}
		defer cleanup()

		skills, err := discoverSourceSkills(dir)
		for i := range skills {
			skills[i].Source = compat.SanitizeMetadata(groupName)
		}
		return discoveryResultMsg{
			groupName: groupName,
			skills:    skills,
			err:       err,
		}
	}
}

// discoveryCacheRoot is the directory where remote source clones are cached
// between scans and across sessions. Overridable in tests.
var discoveryCacheRoot = func() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "lazyskills", "clones"), nil
}

// cloneCacheKey derives a filesystem-safe cache directory name from a GitHub
// URL and ref. owner/repo/ref are already validated to safe charsets before
// reaching here, so only the path separator needs flattening.
func cloneCacheKey(url, ref string) string {
	key := strings.TrimPrefix(url, "https://github.com/")
	key = strings.ReplaceAll(key, "/", "-")
	if ref != "" {
		key += "@" + strings.ReplaceAll(ref, "/", "-")
	}
	return key
}

func isGitRepo(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil && st.IsDir()
}

// cachedSourceClone returns a directory holding a shallow clone of the remote
// source. A cached clone is reused unless force is set; on a cache miss (or
// force) it clones fresh. The returned cleanup removes throwaway directories
// only — a cached clone is kept. If no cache directory is available it falls
// back to a temp clone that cleanup deletes.
func cachedSourceClone(url, ref string, force bool) (dir string, cleanup func(), err error) {
	noop := func() {}

	root, rootErr := discoveryCacheRoot()
	if rootErr != nil {
		tmp, tmpErr := os.MkdirTemp("", "lazyskills-discover-*")
		if tmpErr != nil {
			return "", noop, fmt.Errorf("failed to create temporary directory: %w", tmpErr)
		}
		if cloneErr := gitClone(url, ref, tmp); cloneErr != nil {
			os.RemoveAll(tmp)
			return "", noop, cloneErr
		}
		return tmp, func() { os.RemoveAll(tmp) }, nil
	}

	dir = filepath.Join(root, cloneCacheKey(url, ref))
	if !force && isGitRepo(dir) {
		return dir, noop, nil
	}

	os.RemoveAll(dir)
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return "", noop, mkErr
	}
	if cloneErr := gitClone(url, ref, dir); cloneErr != nil {
		os.RemoveAll(dir)
		return "", noop, cloneErr
	}
	return dir, noop, nil
}

func parseRemoteGitHubSource(source string) (url string, ref string, ok bool) {
	repoPart := source
	if idx := strings.Index(source, "#"); idx != -1 {
		repoPart = source[:idx]
		ref = source[idx+1:]
	}

	if strings.HasPrefix(repoPart, "https://github.com/") {
		repoPart = strings.TrimPrefix(repoPart, "https://github.com/")
	} else if strings.HasPrefix(repoPart, "github:") {
		repoPart = strings.TrimPrefix(repoPart, "github:")
	}

	repoPart = strings.TrimSuffix(repoPart, ".git")

	parts := strings.Split(repoPart, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	owner, repo := parts[0], parts[1]
	if !isSafeGitHubToken(owner) || !isSafeGitHubToken(repo) {
		return "", "", false
	}

	if ref != "" && !isSafeGitHubRef(ref) {
		return "", "", false
	}

	return fmt.Sprintf("https://github.com/%s/%s", owner, repo), ref, true
}

func isSafeGitHubToken(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func isSafeGitHubRef(s string) bool {
	if s == "" || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") {
		return false
	}
	if strings.Contains(s, "..") || strings.Contains(s, "@{") || strings.Contains(s, "\\") {
		return false
	}
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == '/') {
			return false
		}
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func defaultGitClone(url, ref, tempDir string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git executable not found in PATH")
	}

	if ref != "" {
		cmd := exec.Command("git", "clone", "--depth", "1", "--branch", ref, url, tempDir)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	cmd := exec.Command("git", "clone", "--depth", "1", url, tempDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to scan source: %w", err)
	}

	if ref != "" {
		checkoutCmd := exec.Command("git", "checkout", ref)
		checkoutCmd.Dir = tempDir
		if err := checkoutCmd.Run(); err != nil {
			fetchCmd := exec.Command("git", "fetch", "--depth", "1", "origin", ref)
			fetchCmd.Dir = tempDir
			_ = fetchCmd.Run()
			if err := checkoutCmd.Run(); err != nil {
				return fmt.Errorf("failed to checkout ref %q: %w", ref, err)
			}
		}
	}
	return nil
}
