package actions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

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
	return ForAvailableSkillWithResolver(source, name, ResolveSkillsCommand)
}

func ForAvailableSkillWithResolver(source, name string, resolve SkillsResolver) []CommandPreview {
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

	preview := newPreview("install_skill", "Install selected skill", program, args, "Install this skill to project.", true, true, false, "yes")
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

func HasSkillsOrNpx() (bool, string) {
	if _, err := LookPath("skills"); err == nil {
		return true, ""
	}
	if _, err := LookPath("npx"); err == nil {
		return true, ""
	}
	return false, "neither 'skills' nor 'npx' is available in your PATH"
}

func ResolveSkillsCommand() (string, []string) {
	if _, err := LookPath("skills"); err == nil {
		return "skills", nil
	}
	return "npx", []string{"--yes", "skills"}
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

// DefaultProjectBundlePath returns the canonical bundle path used for project
// skill exports/imports.
func DefaultProjectBundlePath(cwd string) string {
	return filepath.Join(cwd, ".lazyskills", "skills.bundle.json")
}

type SkillBundleExportSummary struct {
	Path         string
	Included     int
	Skipped      int
	SkippedNames []string
}

type SkillBundleImportConflict struct {
	Name   string
	Reason string
}

type SkillBundleImportPlan struct {
	Path      string
	Bundle    model.SkillBundle
	Installs  []CommandPreview
	Skipped   []string
	Conflicts []SkillBundleImportConflict
	Summary   string
}

// BuildProjectSkillBundle extracts project-scoped skills into a portable bundle
// payload. Untracked project skills are skipped because they cannot be
// reproducibly reinstalled from lock identity.
func BuildProjectSkillBundle(cwd string, skills []*model.Skill) (model.SkillBundle, SkillBundleExportSummary) {
	path := DefaultProjectBundlePath(cwd)
	bundle := model.SkillBundle{Version: 1, Scope: model.ScopeProject, Skills: []model.SkillBundleSkill{}}
	skippedNames := []string{}
	for _, sk := range skills {
		if sk == nil || sk.Scope != model.ScopeProject || sk.LocalLock == nil {
			if sk != nil && sk.Scope == model.ScopeProject {
				skippedNames = append(skippedNames, compat.SanitizeMetadata(sk.Name))
			}
			continue
		}
		bundle.Skills = append(bundle.Skills, model.SkillBundleSkill{
			Name:      compat.SanitizeMetadata(sk.Name),
			Source:    compat.SanitizeMetadata(sk.LocalLock.Source),
			Reference: compat.SanitizeMetadata(sk.LocalLock.Ref),
			SkillPath: compat.SanitizeMetadata(sk.LocalLock.SkillPath),
			Scope:     model.ScopeProject,
			LockIdentity: model.SkillBundleLockIdentity{
				Source:       compat.SanitizeMetadata(sk.LocalLock.Source),
				SourceType:   compat.SanitizeMetadata(sk.LocalLock.SourceType),
				Reference:    compat.SanitizeMetadata(sk.LocalLock.Ref),
				SkillPath:    compat.SanitizeMetadata(sk.LocalLock.SkillPath),
				ComputedHash: compat.SanitizeMetadata(sk.LocalLock.ComputedHash),
			},
		})
	}
	sort.Slice(bundle.Skills, func(i, j int) bool {
		li := strings.ToLower(bundle.Skills[i].Name)
		lj := strings.ToLower(bundle.Skills[j].Name)
		if li == lj {
			return bundle.Skills[i].Source < bundle.Skills[j].Source
		}
		return li < lj
	})
	sort.Strings(skippedNames)
	return bundle, SkillBundleExportSummary{Path: path, Included: len(bundle.Skills), Skipped: len(skippedNames), SkippedNames: skippedNames}
}

// WriteProjectSkillBundle serializes a bundle to disk, creating parent
// directories as needed.
func WriteProjectSkillBundle(path string, bundle model.SkillBundle) error {
	if path == "" {
		return errors.New("bundle path is empty")
	}
	if bundle.Version == 0 {
		bundle.Version = 1
	}
	if bundle.Scope == "" {
		bundle.Scope = model.ScopeProject
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// ReadProjectSkillBundle loads a bundle file from disk.
func ReadProjectSkillBundle(path string) (model.SkillBundle, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return model.SkillBundle{}, err
	}
	var bundle model.SkillBundle
	if err := json.Unmarshal(b, &bundle); err != nil {
		return model.SkillBundle{}, err
	}
	if bundle.Version < 1 {
		return model.SkillBundle{}, fmt.Errorf("bundle %s has unsupported version %d", path, bundle.Version)
	}
	if bundle.Scope == "" {
		bundle.Scope = model.ScopeProject
	}
	return bundle, nil
}

// BuildProjectBundleImportPlan compares the bundle against the current scan and
// returns the install preview, skipped matches, and conflicts without mutating
// disk.
func BuildProjectBundleImportPlan(bundlePath string, skills []*model.Skill) (SkillBundleImportPlan, error) {
	bundle, err := ReadProjectSkillBundle(bundlePath)
	if err != nil {
		return SkillBundleImportPlan{}, err
	}
	if bundle.Scope == "" {
		bundle.Scope = model.ScopeProject
	}
	plan := SkillBundleImportPlan{Path: bundlePath, Bundle: bundle}
	for _, entry := range bundle.Skills {
		entry := entry
		scope := entry.Scope
		if scope == "" {
			scope = model.ScopeProject
		}
		if scope != model.ScopeProject {
			plan.Conflicts = append(plan.Conflicts, SkillBundleImportConflict{Name: entry.Name, Reason: "bundle scope must be project"})
			continue
		}
		matched := findMatchingProjectSkill(skills, entry)
		if matched != nil {
			plan.Skipped = append(plan.Skipped, entry.Name)
			continue
		}
		if conflictReason := bundleConflictReason(skills, entry); conflictReason != "" {
			plan.Conflicts = append(plan.Conflicts, SkillBundleImportConflict{Name: entry.Name, Reason: conflictReason})
			continue
		}
		source := buildInstallSource(entry.Source, entry.Reference, entry.SkillPath)
		if source == "" {
			plan.Conflicts = append(plan.Conflicts, SkillBundleImportConflict{Name: entry.Name, Reason: "bundle entry is missing a safe install source"})
			continue
		}
		previews := ForAvailableSkillWithResolver(source, entry.Name, ResolveSkillsCommand)
		if len(previews) == 0 || !previews[0].Available {
			reason := "bundle entry could not be converted into an install command"
			if len(previews) > 0 && previews[0].Reason != "" {
				reason = previews[0].Reason
			}
			plan.Conflicts = append(plan.Conflicts, SkillBundleImportConflict{Name: entry.Name, Reason: reason})
			continue
		}
		plan.Installs = append(plan.Installs, previews[0])
	}
	plan.Summary = renderBundleImportSummary(plan)
	return plan, nil
}

// ProjectBundleActions returns the export/import actions for the current
// project bundle workflow.
func ProjectBundleActions(cwd string, skills []*model.Skill) []CommandPreview {
	_, exportSummary := BuildProjectSkillBundle(cwd, skills)
	export := CommandPreview{
		ID:              "bundle_export",
		Title:           "Export project skill bundle",
		Description:     renderBundleExportSummary(exportSummary),
		Command:         fmt.Sprintf("bundle export -> %s", shellQuote(exportSummary.Path)),
		Exec:            ExecSpec{Internal: "bundle_export", Args: []string{exportSummary.Path}},
		Mutates:         true,
		RequiresConfirm: true,
		ConfirmValue:    exportSummary.Path,
		Available:       true,
	}
	if exportSummary.Included == 0 && exportSummary.Skipped == 0 {
		export.Description = "No project skills are currently tracked for export."
	}
	importPath := exportSummary.Path
	importPlan, err := BuildProjectBundleImportPlan(importPath, skills)
	importPreview := CommandPreview{
		ID:              "bundle_import",
		Title:           "Import project skill bundle",
		Command:         fmt.Sprintf("bundle import --bundle %s", shellQuote(importPath)),
		Exec:            ExecSpec{Internal: "bundle_import", Args: []string{importPath}},
		RequiresConfirm: true,
		ConfirmValue:    importPath,
	}
	if err != nil {
		importPreview.Available = false
		importPreview.Reason = compat.SanitizeMetadata(err.Error())
		importPreview.Description = "Preview the bundle onboarding plan after exporting or creating the bundle file."
	} else {
		importPreview.Available = true
		importPreview.Mutates = len(importPlan.Installs) > 0
		importPreview.Description = importPlan.Summary
	}
	return []CommandPreview{export, importPreview}
}

func renderBundleExportSummary(summary SkillBundleExportSummary) string {
	lines := []string{fmt.Sprintf("Write %d project skills to %s.", summary.Included, summary.Path)}
	if summary.Skipped > 0 {
		lines = append(lines, fmt.Sprintf("Skip %d untracked project skills.", summary.Skipped))
		for _, name := range summary.SkippedNames {
			lines = append(lines, fmt.Sprintf("- %s", name))
		}
	}
	return strings.Join(lines, "\n")
}

func renderBundleImportSummary(plan SkillBundleImportPlan) string {
	lines := []string{fmt.Sprintf("Preview import plan from %s.", plan.Path)}
	lines = append(lines, fmt.Sprintf("Install %d missing skills.", len(plan.Installs)))
	lines = append(lines, fmt.Sprintf("Skip %d matching skills.", len(plan.Skipped)))
	if len(plan.Conflicts) > 0 {
		lines = append(lines, fmt.Sprintf("Surface %d conflicts without overwriting them.", len(plan.Conflicts)))
		for _, conflict := range plan.Conflicts {
			lines = append(lines, fmt.Sprintf("- %s: %s", conflict.Name, conflict.Reason))
		}
	}
	if len(plan.Skipped) > 0 {
		for _, name := range plan.Skipped {
			lines = append(lines, fmt.Sprintf("- already installed: %s", name))
		}
	}
	return strings.Join(lines, "\n")
}

func findMatchingProjectSkill(skills []*model.Skill, entry model.SkillBundleSkill) *model.Skill {
	for _, sk := range skills {
		if sk == nil || sk.Scope != model.ScopeProject {
			continue
		}
		if sameBundleIdentity(sk, entry) {
			return sk
		}
	}
	return nil
}

func bundleConflictReason(skills []*model.Skill, entry model.SkillBundleSkill) string {
	norm := compat.NormalizeName(entry.Name)
	for _, sk := range skills {
		if sk == nil {
			continue
		}
		if compat.NormalizeName(sk.Name) != norm {
			continue
		}
		if sk.Scope != model.ScopeProject {
			return fmt.Sprintf("matching %s skill already exists in %s scope", sk.Name, sk.Scope)
		}
		if !sameBundleIdentity(sk, entry) {
			return fmt.Sprintf("existing project skill uses %s", bundleIdentityLabelForSkill(sk))
		}
	}
	return ""
}

func sameBundleIdentity(sk *model.Skill, entry model.SkillBundleSkill) bool {
	scope, source, ref, skillPath := skillBundleIdentityForSkill(sk)
	entryScope, entrySource, entryRef, entrySkillPath := skillBundleIdentityForEntry(entry)
	return scope == entryScope && source == entrySource && ref == entryRef && skillPath == entrySkillPath
}

func skillBundleIdentityForSkill(sk *model.Skill) (model.Scope, string, string, string) {
	if sk == nil {
		return "", "", "", ""
	}
	if sk.Scope == model.ScopeProject && sk.LocalLock != nil {
		return sk.Scope, compat.SanitizeMetadata(sk.LocalLock.Source), compat.SanitizeMetadata(sk.LocalLock.Ref), compat.SanitizeMetadata(sk.LocalLock.SkillPath)
	}
	if sk.Scope == model.ScopeGlobal && sk.GlobalLock != nil {
		return sk.Scope, compat.SanitizeMetadata(globalUpdateSource(*sk.GlobalLock)), compat.SanitizeMetadata(sk.GlobalLock.Ref), compat.SanitizeMetadata(sk.GlobalLock.SkillPath)
	}
	return sk.Scope, "", "", ""
}

func skillBundleIdentityForEntry(entry model.SkillBundleSkill) (model.Scope, string, string, string) {
	scope := entry.Scope
	if scope == "" {
		scope = model.ScopeProject
	}
	return scope, compat.SanitizeMetadata(entry.Source), compat.SanitizeMetadata(entry.Reference), compat.SanitizeMetadata(entry.SkillPath)
}

func bundleIdentityLabel(scope model.Scope, source, ref, skillPath string) string {
	parts := []string{}
	if source != "" {
		parts = append(parts, source)
	}
	if ref != "" {
		parts = append(parts, "#"+ref)
	}
	if skillPath != "" {
		parts = append(parts, skillPath)
	}
	if len(parts) == 0 {
		parts = append(parts, string(scope))
	}
	return strings.Join(parts, " ")
}

func bundleIdentityLabelForSkill(sk *model.Skill) string {
	scope, source, ref, skillPath := skillBundleIdentityForSkill(sk)
	return bundleIdentityLabel(scope, source, ref, skillPath)
}
