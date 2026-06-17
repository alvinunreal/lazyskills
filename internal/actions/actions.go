package actions

import (
	"path/filepath"
	"strings"

	"lazyskills/internal/compat"
	"lazyskills/internal/model"
)

type CommandPreview struct {
	Title       string
	Program     string
	Args        []string
	Command     string
	Description string
	Mutates     bool
	Available   bool
	Reason      string
}

func ForSkill(sk *model.Skill) []CommandPreview {
	if sk == nil {
		return nil
	}
	previews := []CommandPreview{
		newPreview("Refresh LazySkills", "lazyskills", nil, "Refresh is handled inside the TUI with r; this command reopens the dashboard.", false),
	}

	if addSource, skillFilter, ok, reason := addIdentity(sk); ok {
		args := []string{"skills", "add", addSource, "--skill", skillFilter}
		if sk.Scope == model.ScopeGlobal {
			args = append(args, "-g")
		}
		preview := newPreview("Reinstall/update selected skill", "npx", args, "Mutating preview only. Reinstall/update this skill via the official skills CLI.", true)
		previews = append(previews, preview)
	} else {
		previews = append(previews, unavailablePreview("Reinstall/update selected skill", reason))
	}

	if target, ok, reason := removeIdentity(sk); ok {
		args := []string{"skills", "remove", target}
		if sk.Scope == model.ScopeGlobal {
			args = append(args, "-g")
		}
		previews = append(previews, newPreview("Remove selected skill", "npx", args, "Mutating preview only. Remove this installed skill via the official skills CLI after confirmation outside LazySkills.", true))
	} else {
		previews = append(previews, unavailablePreview("Remove selected skill", reason))
	}
	return previews
}

func newPreview(title, program string, args []string, description string, mutates bool) CommandPreview {
	program = safeToken(program)
	safeArgs := make([]string, 0, len(args))
	for _, arg := range args {
		safeArgs = append(safeArgs, compat.SanitizeMetadata(arg))
	}
	preview := CommandPreview{Title: title, Program: program, Args: safeArgs, Description: description, Mutates: mutates, Available: true}
	preview.Command = renderCommand(program, safeArgs)
	return preview
}

func unavailablePreview(title, reason string) CommandPreview {
	return CommandPreview{Title: title, Available: false, Reason: compat.SanitizeMetadata(firstNonEmpty(reason, "not enough safe identity data to build this command"))}
}

func addIdentity(sk *model.Skill) (source string, skillFilter string, ok bool, reason string) {
	source, ref, skillPath := sourceRefPath(sk)
	source = buildInstallSource(source, ref, skillPath)
	if !safeCLIValue(source) {
		return "", "", false, "source is empty or option-like"
	}
	filter := compat.SanitizeMetadata(sk.Name)
	if !safeCLIValue(filter) {
		return "", "", false, "skill name is empty or option-like"
	}
	return source, filter, true, ""
}

func removeIdentity(sk *model.Skill) (target string, ok bool, reason string) {
	for _, path := range candidateInstallPaths(sk) {
		base := compat.SanitizeName(filepath.Base(path))
		if safeCLIValue(base) {
			return base, true, ""
		}
	}
	fallback := compat.SanitizeName(sk.Name)
	if safeCLIValue(sk.Name) && safeCLIValue(fallback) {
		return fallback, true, ""
	}
	return "", false, "installed directory identity is empty or option-like"
}

func sourceRefPath(sk *model.Skill) (source, ref, skillPath string) {
	if sk.Scope == model.ScopeProject && sk.LocalLock != nil {
		return sk.LocalLock.Source, sk.LocalLock.Ref, sk.LocalLock.SkillPath
	}
	if sk.Scope == model.ScopeGlobal && sk.GlobalLock != nil {
		return globalUpdateSource(*sk.GlobalLock), sk.GlobalLock.Ref, sk.GlobalLock.SkillPath
	}
	if sk.LocalLock != nil {
		return sk.LocalLock.Source, sk.LocalLock.Ref, sk.LocalLock.SkillPath
	}
	if sk.GlobalLock != nil {
		return globalUpdateSource(*sk.GlobalLock), sk.GlobalLock.Ref, sk.GlobalLock.SkillPath
	}
	return "", "", ""
}

func globalUpdateSource(entry model.GlobalLockEntry) string {
	if entry.SkillPath == "" {
		return firstNonEmpty(entry.SourceURL, entry.Source)
	}
	return firstNonEmpty(entry.Source, entry.SourceURL)
}

func buildInstallSource(source, ref, skillPath string) string {
	source = compat.SanitizeMetadata(source)
	ref = compat.SanitizeMetadata(ref)
	skillPath = compat.SanitizeMetadata(skillPath)
	if source == "" {
		return ""
	}
	if skillPath != "" && supportsAppendedSubpath(source) {
		folder := deriveSkillFolder(skillPath)
		if folder != "" {
			source = strings.TrimRight(source, "/") + "/" + folder
		}
	}
	if ref != "" {
		source += "#" + ref
	}
	return source
}

func deriveSkillFolder(skillPath string) string {
	folder := skillPath
	if strings.HasSuffix(folder, "/SKILL.md") {
		folder = strings.TrimSuffix(folder, "/SKILL.md")
	} else if strings.HasSuffix(folder, "SKILL.md") {
		folder = strings.TrimSuffix(folder, "SKILL.md")
	}
	return strings.Trim(folder, "/")
}

func supportsAppendedSubpath(source string) bool {
	if strings.HasPrefix(source, "git@") || strings.HasSuffix(source, ".git") {
		return false
	}
	if strings.HasPrefix(source, "https://github.com/") || strings.HasPrefix(source, "https://gitlab.com/") || !strings.Contains(source, "://") {
		return true
	}
	return false
}

func candidateInstallPaths(sk *model.Skill) []string {
	paths := []string{}
	if sk.CanonicalPath != "" {
		paths = append(paths, sk.CanonicalPath)
	}
	for _, observed := range sk.ObservedPaths {
		if observed.Path != "" {
			paths = append(paths, observed.Path)
		}
	}
	return paths
}

func safeCLIValue(value string) bool {
	value = compat.SanitizeMetadata(value)
	return value != "" && !strings.HasPrefix(value, "-")
}

func safeToken(value string) string {
	value = compat.SanitizeMetadata(value)
	if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, " \t\n'\"$`\\!*?[]{}()&;<>|") {
		return ""
	}
	return value
}

func renderCommand(program string, args []string) string {
	parts := []string{shellQuote(program)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	value = compat.SanitizeMetadata(value)
	if value == "" {
		return "''"
	}
	if strings.ContainsAny(value, " \t\n'\"$`\\!*?[]{}()&;<>|#") {
		return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return compat.SanitizeMetadata(value)
		}
	}
	return ""
}
