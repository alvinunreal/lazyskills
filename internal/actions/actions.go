package actions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/model"
)

type CommandPreview struct {
	ID              string
	Title           string
	Program         string
	Args            []string
	Exec            ExecSpec
	Command         string
	Description     string
	Mutates         bool
	RequiresConfirm bool
	Dangerous       bool
	ConfirmValue    string
	Available       bool
	Reason          string
}

type ExecSpec struct {
	Program     string
	Args        []string
	Batch       []ExecSpec
	Cwd         string
	Interactive bool
	Internal    string
}

type SkillsResolver func() (program string, baseArgs []string)

func ForSkill(sk *model.Skill) []CommandPreview {
	return ForSkillWithResolver(sk, ResolveSkillsCommand)
}

func ForSkills(skills []*model.Skill) []CommandPreview {
	return ForSkillsWithResolver(skills, ResolveSkillsCommand)
}

func ForSkillsWithResolver(skills []*model.Skill, resolve SkillsResolver) []CommandPreview {
	if len(skills) == 0 {
		return nil
	}
	if resolve == nil {
		resolve = ResolveSkillsCommand
	}
	count := len(skills)
	updateBatch, updateOK, updateReason := bulkBatch(skills, resolve, "reinstall_update")
	removeBatch, removeOK, removeReason := bulkBatch(skills, resolve, "remove")
	previews := []CommandPreview{}
	if updateOK {
		previews = append(previews, newBatchPreview("bulk_reinstall_update", fmt.Sprintf("Reinstall/update %d selected skills", count), updateBatch, "Refresh the selected skills.", fmt.Sprintf("update %d skills", count), false))
	} else {
		previews = append(previews, unavailablePreview(fmt.Sprintf("Reinstall/update %d selected skills", count), updateReason))
	}
	if removeOK {
		previews = append(previews, newBatchPreview("bulk_remove", fmt.Sprintf("Remove %d selected skills", count), removeBatch, "Delete the selected installed skills.", fmt.Sprintf("remove %d skills", count), true))
	} else {
		previews = append(previews, unavailablePreview(fmt.Sprintf("Remove %d selected skills", count), removeReason))
	}
	return previews
}

func AppLevelActions() []CommandPreview {
	return AppLevelActionsWithResolver(ResolveSkillsCommand)
}

func AppLevelActionsWithResolver(resolve SkillsResolver) []CommandPreview {
	if resolve == nil {
		resolve = ResolveSkillsCommand
	}
	program, baseArgs := resolve()

	// Check availability: gate if neither skills nor npx is in PATH
	available, reason := HasSkillsOrNpx()

	previews := []CommandPreview{}

	// skills init
	initArgs := append([]string{}, baseArgs...)
	initArgs = append(initArgs, "init")
	initPreview := newPreview("skills_init", "Initialize skills in project", program, initArgs, "Initialize local skills configuration.", true, true, false, "yes")
	gateAvailability(&initPreview, available, reason)
	previews = append(previews, initPreview)

	// skills find
	findArgs := append([]string{}, baseArgs...)
	findArgs = append(findArgs, "find")
	findPreview := newPreview("skills_find", "Find new skills (interactive)", program, findArgs, "Interactively discover and install skills.", true, false, false, "")
	if !available {
		findPreview.Available = false
		findPreview.Reason = reason
	} else {
		findPreview.Exec.Interactive = true
	}
	previews = append(previews, findPreview)

	// skills update
	updateArgs := append([]string{}, baseArgs...)
	updateArgs = append(updateArgs, "update")
	updatePreview := newPreview("skills_update", "Update project-local skills", program, updateArgs, "Check for and install updates for project-local skills.", true, true, false, "yes")
	gateAvailability(&updatePreview, available, reason)
	previews = append(previews, updatePreview)

	return previews
}

func bulkBatch(skills []*model.Skill, resolve SkillsResolver, actionID string) ([]ExecSpec, bool, string) {
	batch := make([]ExecSpec, 0, len(skills))
	for _, skill := range skills {
		found := false
		for _, preview := range ForSkillWithResolver(skill, resolve) {
			if preview.ID != actionID {
				continue
			}
			if !preview.Available {
				return nil, false, fmt.Sprintf("%s: %s", compat.SanitizeMetadata(skill.Name), preview.Reason)
			}
			batch = append(batch, preview.Exec)
			found = true
			break
		}
		if !found {
			return nil, false, fmt.Sprintf("%s: action unavailable", compat.SanitizeMetadata(skill.Name))
		}
	}
	return batch, true, ""
}

func ForAvailableSkill(source, name string) []CommandPreview {
	return ForAvailableSkillWithResolver(source, name, false, ResolveSkillsCommand)
}

func ForAvailableSkillWithResolver(source, name string, global bool, resolve SkillsResolver) []CommandPreview {
	if !safeExecValue(source) || strings.HasPrefix(source, "-") {
		return []CommandPreview{unavailablePreview("Install selected skill", "source is empty, option-like, or contains unsafe characters")}
	}
	if !safeExecValue(name) || strings.HasPrefix(name, "-") {
		return []CommandPreview{unavailablePreview("Install selected skill", "skill name is empty, option-like, or contains unsafe characters")}
	}

	if resolve == nil {
		resolve = ResolveSkillsCommand
	}
	available, reason := HasSkillsOrNpx()
	program, baseArgs := resolve()

	args := append([]string{}, baseArgs...)
	args = append(args, "add", source, "--skill", name, "--yes")
	if global {
		args = append(args, "--global")
	}

	description := "Install this skill to project."
	if global {
		description = "Install this skill globally."
	}
	preview := newPreview("install_skill", "Install selected skill", program, args, description, true, true, false, "yes")
	gateAvailability(&preview, available, reason)
	return []CommandPreview{preview}
}

func ForSkillWithResolver(sk *model.Skill, resolve SkillsResolver) []CommandPreview {
	if sk == nil {
		return nil
	}
	if resolve == nil {
		resolve = ResolveSkillsCommand
	}
	available, reason := HasSkillsOrNpx()
	previews := []CommandPreview{}
	if open, ok, reason := openEditorAction(sk); ok {
		previews = append(previews, open)
	} else {
		previews = append(previews, unavailablePreview("Open selected skill", reason))
	}

	if addSource, skillFilter, ok, reasonAdd := addIdentity(sk); ok {
		program, baseArgs := resolve()
		args := append([]string{}, baseArgs...)
		args = append(args, "add", addSource, "--skill", skillFilter, "--yes")
		if sk.Scope == model.ScopeGlobal {
			args = append(args, "-g")
		}
		preview := newPreview("reinstall_update", "Reinstall/update selected skill", program, args, "Refresh this skill from its source.", true, true, false, "yes")
		gateAvailability(&preview, available, reason)
		previews = append(previews, preview)
	} else {
		previews = append(previews, unavailablePreview("Reinstall/update selected skill", reasonAdd))
	}

	if target, ok, reasonRemove := removeIdentity(sk); ok {
		program, baseArgs := resolve()
		args := append([]string{}, baseArgs...)
		args = append(args, "remove", target, "--yes")
		if sk.Scope == model.ScopeGlobal {
			args = append(args, "-g")
		}
		preview := newPreview("remove", "Remove selected skill", program, args, "Delete this installed skill.", true, true, true, target)
		gateAvailability(&preview, available, reason)
		previews = append(previews, preview)
	} else {
		previews = append(previews, unavailablePreview("Remove selected skill", reasonRemove))
	}
	if hasOrphanedLock(sk) {
		previews = append(previews, pruneLockPreview(sk))
	}
	if hasBrokenSymlink(sk) {
		previews = append(previews, deleteBrokenSymlinkPreview(sk))
	}
	return previews
}

// hasOrphanedLock reports whether the skill is a lock entry whose files are
// gone from disk (the scan flags this as lock_without_files).
func hasOrphanedLock(sk *model.Skill) bool {
	for _, issue := range sk.HealthIssues {
		if issue.Type == "lock_without_files" {
			return true
		}
	}
	return false
}

// pruneLockPreview builds the internal action that removes an orphaned entry
// from the project or global lock file. It is handled inside the TUI, which
// owns the lock paths and triggers a rescan afterwards.
func pruneLockPreview(sk *model.Skill) CommandPreview {
	internal := "prune_project_lock"
	if sk.Scope == model.ScopeGlobal {
		internal = "prune_global_lock"
	}
	return CommandPreview{
		ID:              "prune_lock",
		Title:           "Prune stale lock entry",
		Description:     "Remove this orphaned entry from the lock file (its skill files are already gone).",
		Command:         "prune lock entry " + compat.SanitizeMetadata(sk.Name),
		Exec:            ExecSpec{Internal: internal},
		Mutates:         true,
		RequiresConfirm: true,
		ConfirmValue:    sk.Name,
		Available:       true,
	}
}

func hasBrokenSymlink(sk *model.Skill) bool {
	if sk == nil {
		return false
	}
	for _, op := range sk.ObservedPaths {
		if op.Status == model.StatusBrokenSymlink {
			return true
		}
	}
	return false
}

// deleteBrokenSymlinkPreview builds the internal action that deletes the
// broken/dangling symlink files for a skill. The TUI owns the filesystem
// mutation and triggers a rescan afterwards. ONLY broken symlinks are ever
// deleted — never working symlinks or canonical skill files.
func deleteBrokenSymlinkPreview(sk *model.Skill) CommandPreview {
	return CommandPreview{
		ID:              "delete_broken_symlink",
		Title:           "Delete broken symlink(s)",
		Description:     "Delete the broken/dangling symlink file(s) for this skill.",
		Command:         "delete broken symlinks for " + compat.SanitizeMetadata(sk.Name),
		Exec:            ExecSpec{Internal: "delete_broken_symlink", Args: []string{string(sk.Scope), sk.Name}},
		Mutates:         true,
		RequiresConfirm: true,
		Dangerous:       true,
		ConfirmValue:    sk.Name,
		Available:       true,
	}
}

func openEditorAction(sk *model.Skill) (CommandPreview, bool, string) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		return CommandPreview{}, false, "$EDITOR is not set"
	}
	parts := strings.Fields(editor)
	if len(parts) == 0 || !safeEditorToken(parts[0]) {
		return CommandPreview{}, false, "$EDITOR is empty or unsafe"
	}
	for _, arg := range parts[1:] {
		if !safeEditorArg(arg) {
			return CommandPreview{}, false, "$EDITOR arguments are unsafe"
		}
	}
	target := compat.FirstNonEmpty(sk.SkillPath, sk.CanonicalPath)
	if target == "" {
		return CommandPreview{}, false, "skill path is unavailable"
	}
	if !safeExecValue(target) {
		return CommandPreview{}, false, "skill path contains unsafe characters"
	}
	args := append([]string{}, parts[1:]...)
	args = append(args, target)
	preview := newPreview("open_skill", "Open selected skill", parts[0], args, "Open in $EDITOR.", false, false, false, "")
	if !preview.Available {
		return CommandPreview{}, false, preview.Reason
	}
	preview.Exec.Interactive = true
	return preview, true, ""
}

var LookPath = exec.LookPath

// memoised results for HasSkillsOrNpx and ResolveSkillsCommand.
// These depend only on PATH, which never changes during a session.
var (
	hasSkillsMu     sync.Mutex
	hasSkillsOK     bool
	hasSkillsAvail  bool
	hasSkillsReason string

	resolveMu   sync.Mutex
	resolveOK   bool
	resolveProg string
	resolveArgs []string
)

// ResetActionCaches clears the process-lifetime caches for HasSkillsOrNpx
// and ResolveSkillsCommand. Tests that replace LookPath must call this
// before and after the swap so the memo picks up the new value.
func ResetActionCaches() {
	hasSkillsMu.Lock()
	hasSkillsOK = false
	hasSkillsMu.Unlock()
	resolveMu.Lock()
	resolveOK = false
	resolveMu.Unlock()
}

func HasSkillsOrNpx() (bool, string) {
	hasSkillsMu.Lock()
	if !hasSkillsOK {
		ok, reason := true, ""
		if _, err := LookPath("skills"); err == nil {
			// ok
		} else if _, err := LookPath("npx"); err == nil {
			// ok
		} else {
			ok, reason = false, "neither 'skills' nor 'npx' is available in your PATH"
		}
		hasSkillsAvail, hasSkillsReason, hasSkillsOK = ok, reason, true
	}
	avail, reason := hasSkillsAvail, hasSkillsReason
	hasSkillsMu.Unlock()
	return avail, reason
}

func ResolveSkillsCommand() (string, []string) {
	resolveMu.Lock()
	if !resolveOK {
		prog, args := "npx", []string{"--yes", "skills"}
		if _, err := LookPath("skills"); err == nil {
			prog, args = "skills", nil
		}
		resolveProg, resolveArgs, resolveOK = prog, args, true
	}
	prog, args := resolveProg, resolveArgs
	resolveMu.Unlock()
	var argsCopy []string
	if args != nil {
		argsCopy = append([]string(nil), args...)
	}
	return prog, argsCopy
}

func newPreview(id, title, program string, args []string, description string, mutates, confirm, dangerous bool, confirmValue string) CommandPreview {
	if !safeExecValue(program) {
		return unavailablePreview(title, "program is empty or unsafe")
	}
	execArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if !safeExecValue(arg) {
			return unavailablePreview(title, "command argument is empty, option-like, or contains control characters")
		}
		execArgs = append(execArgs, arg)
	}
	preview := CommandPreview{ID: id, Title: title, Program: program, Args: execArgs, Exec: ExecSpec{Program: program, Args: execArgs}, Description: description, Mutates: mutates, RequiresConfirm: confirm, Dangerous: dangerous, ConfirmValue: confirmValue, Available: true}
	preview.Command = renderCommand(program, execArgs)
	return preview
}

func newBatchPreview(id, title string, batch []ExecSpec, description, confirmValue string, dangerous bool) CommandPreview {
	if len(batch) == 0 {
		return unavailablePreview(title, "no executable commands")
	}
	commands := make([]string, 0, len(batch))
	for _, spec := range batch {
		if spec.Program == "" || len(spec.Args) == 0 || spec.Interactive || spec.Internal != "" {
			return unavailablePreview(title, "bulk action contains unsupported command")
		}
		commands = append(commands, renderCommand(spec.Program, spec.Args))
	}
	return CommandPreview{ID: id, Title: title, Exec: ExecSpec{Batch: batch}, Command: strings.Join(commands, " && "), Description: description, Mutates: true, RequiresConfirm: true, Dangerous: dangerous, ConfirmValue: confirmValue, Available: true}
}

func unavailablePreview(title, reason string) CommandPreview {
	return CommandPreview{Title: title, Available: false, Reason: compat.SanitizeMetadata(compat.FirstNonEmpty(reason, "not enough safe identity data to build this command"))}
}

// gateAvailability marks a preview unavailable with the given reason when the
// underlying tooling is missing. Already-unavailable previews are left untouched.
func gateAvailability(p *CommandPreview, available bool, reason string) {
	if !available {
		p.Available = false
		p.Reason = reason
	}
}

func addIdentity(sk *model.Skill) (source string, skillFilter string, ok bool, reason string) {
	source, ref, skillPath := sourceRefPath(sk)
	source = buildInstallSource(source, ref, skillPath)
	if !safeExecValue(source) || strings.HasPrefix(source, "-") {
		return "", "", false, "source is empty or option-like"
	}
	filter := sk.Name
	if !safeExecValue(filter) || strings.HasPrefix(filter, "-") {
		return "", "", false, "skill name is empty or option-like"
	}
	return source, filter, true, ""
}

func removeIdentity(sk *model.Skill) (target string, ok bool, reason string) {
	for _, path := range candidateInstallPaths(sk) {
		base := filepath.Base(path)
		if safeExecValue(base) && !strings.HasPrefix(base, "-") {
			return base, true, ""
		}
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
		return compat.FirstNonEmpty(entry.SourceURL, entry.Source)
	}
	return compat.FirstNonEmpty(entry.Source, entry.SourceURL)
}

func buildInstallSource(source, ref, skillPath string) string {
	if source == "" {
		return ""
	}
	if !safeExecValue(source) || strings.HasPrefix(source, "-") {
		return ""
	}
	if ref != "" && (!safeExecValue(ref) || strings.HasPrefix(ref, "-")) {
		return ""
	}
	if skillPath != "" && (!safeExecValue(skillPath) || strings.HasPrefix(skillPath, "-")) {
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

func safeExecValue(value string) bool {
	if value == "" {
		return false
	}
	return compat.SanitizeMetadata(value) == value && !strings.ContainsAny(value, "\x00\x1b\r\n")
}

func safeEditorToken(value string) bool {
	return safeExecValue(value) && !strings.HasPrefix(value, "-") && !strings.ContainsAny(value, "'\"$`\\!*?[]{}()&;<>|")
}

func safeEditorArg(value string) bool {
	return safeExecValue(value) && !strings.ContainsAny(value, "'\"$`\\!*?[]{}()&;<>|")
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
