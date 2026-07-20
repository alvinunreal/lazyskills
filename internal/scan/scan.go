package scan

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alvinunreal/lazyskills/internal/agents"
	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/frontmatter"
	"github.com/alvinunreal/lazyskills/internal/locks"
	"github.com/alvinunreal/lazyskills/internal/model"
)

var LookPath = exec.LookPath

func checkPreflight() *model.Preflight {
	tools := []string{"skills", "npx", "node", "npm"}
	toolStates := make(map[string]model.ToolStatus)
	for _, t := range tools {
		p, err := LookPath(t)
		toolStates[t] = model.ToolStatus{
			Exists: err == nil,
			Path:   p,
		}
	}

	canRunSkills := false
	if toolStates["skills"].Exists {
		canRunSkills = true
	} else {
		canRunSkills = toolStates["npx"].Exists && toolStates["node"].Exists && toolStates["npm"].Exists
	}

	return &model.Preflight{
		CanRunSkills: canRunSkills,
		Tools:        toolStates,
	}
}

func Run(cwd string) (model.ScanResult, error) {
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return model.ScanResult{}, err
	}
	if st, err := os.Stat(absCwd); err != nil {
		return model.ScanResult{}, err
	} else if !st.IsDir() {
		return model.ScanResult{}, fmt.Errorf("cwd is not a directory: %s", absCwd)
	}

	res := model.ScanResult{
		Cwd:         absCwd,
		ProjectLock: locks.ProjectLockPath(absCwd),
		GlobalLock:  locks.GlobalLockPath(),
		Preflight:   checkPreflight(),
	}
	res.Agents = agentStates(absCwd)
	skills := map[string]*model.Skill{}

	localLock, err := locks.ReadLocal(res.ProjectLock)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		res.HealthIssues = append(res.HealthIssues, model.HealthIssue{Type: "corrupt_project_lock", Severity: "warning", Message: err.Error(), Path: res.ProjectLock})
	}
	globalLock, err := locks.ReadGlobal(res.GlobalLock)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		res.HealthIssues = append(res.HealthIssues, model.HealthIssue{Type: "corrupt_global_lock", Severity: "warning", Message: err.Error(), Path: res.GlobalLock})
	}

	activeScanCache := map[locationScanCacheKey][]scannedLocationRecord{}
	for _, loc := range agents.Locations(absCwd) {
		scanLocationCached(loc, skills, activeScanCache)
		scanDisabledLocation(loc, skills)
	}

	index := newLockMatchIndex(skills)
	correlateLocksIndexed(skills, localLock.Skills, globalLock.Skills)
	for key, entry := range localLock.Skills {
		if !index.hasMatch(model.ScopeProject, key) {
			sk := ensureSkill(skills, model.ScopeProject, key, key, "")
			e := entry
			sk.LocalLock = &e
			sk.AddHealthIssue(model.HealthIssue{Type: "lock_without_files", Severity: "warning", Message: "project lock entry has no matching skill on disk"})
		}
	}
	for key, entry := range globalLock.Skills {
		if !index.hasMatch(model.ScopeGlobal, key) {
			sk := ensureSkill(skills, model.ScopeGlobal, key, key, "")
			e := entry
			sk.GlobalLock = &e
			sk.AddHealthIssue(model.HealthIssue{Type: "lock_without_files", Severity: "warning", Message: "global lock entry has no matching skill on disk"})
		}
	}

	addDuplicateAndShadowingIssues(skills)
	for _, sk := range skills {
		if len(sk.ObservedPaths) > 0 {
			if sk.Scope == model.ScopeProject && sk.LocalLock == nil {
				sk.AddHealthIssue(model.HealthIssue{Type: "missing_project_lock", Severity: "warning", Message: "project skill is present on disk but not found in project lock"})
			}
			if sk.Scope == model.ScopeGlobal && sk.GlobalLock == nil {
				sk.AddHealthIssue(model.HealthIssue{Type: "missing_global_lock", Severity: "warning", Message: "global skill is present on disk but not found in global lock"})
			}
			if sk.CanonicalPath == "" && hasNonCanonicalObservation(sk) {
				sk.AddHealthIssue(model.HealthIssue{Type: "ghost_agent_skill", Severity: "warning", Message: "skill exists in an agent-specific directory without a canonical .agents/skills copy"})
			}
		}
		sk.Visibility = skillVisibility(sk, res.Agents)

		hasActive := false
		hasDisabled := false
		for _, obs := range sk.ObservedPaths {
			if obs.Status == model.StatusDisabled {
				hasDisabled = true
			} else {
				hasActive = true
			}
		}
		sk.Disabled = hasDisabled && !hasActive

		res.Skills = append(res.Skills, sk)
	}
	sort.Slice(res.Skills, func(i, j int) bool {
		left, right := strings.ToLower(res.Skills[i].Name), strings.ToLower(res.Skills[j].Name)
		if left == right {
			return res.Skills[i].Scope < res.Skills[j].Scope
		}
		return left < right
	})
	return res, nil
}

func hasNonCanonicalObservation(sk *model.Skill) bool {
	for _, observed := range sk.ObservedPaths {
		if observed.Status != model.StatusCanonical && observed.Status != model.StatusDisabled {
			return true
		}
	}
	return false
}

func agentStates(cwd string) []model.AgentState {
	registry := agents.RegistryWithEnv(agents.DefaultEnv(), cwd)
	states := make([]model.AgentState, 0, len(registry))
	for _, agent := range registry {
		projectDir := filepath.Join(cwd, filepath.FromSlash(agent.ProjectDir))
		state := model.AgentState{
			Name:             agent.Name,
			Display:          compat.SanitizeMetadata(agent.Display),
			Supported:        true,
			Detected:         agent.Detected,
			Universal:        agent.Universal,
			SupportsGlobal:   agent.SupportsGlobal,
			ProjectDir:       projectDir,
			GlobalDir:        agent.GlobalDir,
			ProjectDirExists: pathExists(projectDir),
			GlobalDirExists:  agent.SupportsGlobal && pathExists(agent.GlobalDir),
		}
		states = append(states, state)
	}
	return states
}

func pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func skillVisibility(sk *model.Skill, states []model.AgentState) []model.SkillVisibility {
	visibility := make([]model.SkillVisibility, 0, len(states))
	for _, state := range states {
		item := model.SkillVisibility{Agent: state.Name, Display: state.Display}
		if observed, ok := observedForAgent(sk, state.Name); ok {
			item.Visible = observed.Status != model.StatusBrokenSymlink && observed.Status != model.StatusDisabled
			item.Path = observed.Path
			item.Status = observed.Status
			switch observed.Status {
			case model.StatusCanonical:
				if state.Universal {
					item.Reason = "visible_via_universal_canonical"
				} else {
					item.Reason = "visible_via_canonical"
				}
			case model.StatusSymlink:
				item.Reason = "visible_via_symlink"
			case model.StatusCopy:
				item.Reason = "visible_via_copy"
			case model.StatusBrokenSymlink:
				item.Reason = "broken_symlink"
			case model.StatusDisabled:
				item.Reason = "disabled"
			default:
				item.Reason = "visible"
			}
			visibility = append(visibility, item)
			continue
		}

		item.Visible = false
		switch {
		case sk.Scope == model.ScopeGlobal && !state.SupportsGlobal:
			item.Reason = "unsupported_global"
		case !state.Detected:
			item.Reason = "agent_not_detected"
		case state.Universal:
			item.Reason = "not_in_universal_canonical_dir"
		default:
			item.Reason = "missing_agent_link"
		}
		visibility = append(visibility, item)
	}
	return visibility
}

func observedForAgent(sk *model.Skill, agent string) (model.ObservedPath, bool) {
	for _, observed := range sk.ObservedPaths {
		if observed.Agent == agent {
			return observed, true
		}
	}
	return model.ObservedPath{}, false
}

func scanDisabledLocation(loc agents.Location, skills map[string]*model.Skill) {
	disabledRoot := filepath.Join(loc.Root, ".lazyskills-disabled")
	entries, err := os.ReadDir(disabledRoot)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(disabledRoot, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		observed := model.ObservedPath{
			Path:       path,
			Scope:      loc.Scope,
			Agent:      loc.AgentName,
			Status:     model.StatusDisabled,
			TargetPath: filepath.Join(loc.Root, entry.Name()),
			SharedRoot: loc.SharedRoot,
		}
		parseDir := path
		if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(path)
			if !filepath.IsAbs(target) {
				target = filepath.Join(loc.Root, target)
			}
			if st, err := os.Stat(target); err != nil || !st.IsDir() {
				sk := ensureSkill(skills, loc.Scope, entry.Name(), entry.Name(), "")
				addObservedPath(sk, observed)
				continue
			}
			parseDir = target
		} else if !info.IsDir() {
			continue
		}

		skillFile := filepath.Join(parseDir, "SKILL.md")
		doc, err := frontmatter.ParseFile(skillFile)
		if err != nil {
			sk := ensureSkill(skills, loc.Scope, entry.Name(), entry.Name(), "")
			addObservedPath(sk, observed)
			issueType := "invalid_frontmatter"
			if errors.Is(err, os.ErrNotExist) {
				issueType = "missing_skill_md"
			}
			sk.AddHealthIssue(model.HealthIssue{Type: issueType, Severity: "error", Message: fmt.Sprintf("invalid SKILL.md in disabled shelf: %v", err), Path: skillFile})
			continue
		}

		sk := ensureSkill(skills, loc.Scope, entry.Name(), doc.Name, doc.Description)
		if sk.SkillPath == "" {
			sk.SkillPath = skillFile
		}
		if sk.Preview == "" {
			sk.Preview = doc.Raw
		}
		addObservedPath(sk, observed)
	}
}

type locationScanCacheKey struct {
	root      string
	scope     model.Scope
	canonical bool
}

type scannedLocationRecord struct {
	keyHint       string
	name          string
	description   string
	skillPath     string
	preview       string
	canonicalPath string
	observed      model.ObservedPath
	healthIssues  []model.HealthIssue
}

func scanLocationCached(loc agents.Location, skills map[string]*model.Skill, cache map[locationScanCacheKey][]scannedLocationRecord) {
	key := locationScanCacheKey{root: loc.Root, scope: loc.Scope, canonical: loc.Canonical}
	records, ok := cache[key]
	if !ok {
		records = scanLocationRecords(loc)
		cache[key] = records
	}
	applyScannedLocationRecords(loc, skills, records)
}

func scanLocationRecords(loc agents.Location) []scannedLocationRecord {
	entries, err := os.ReadDir(loc.Root)
	if err != nil {
		return nil
	}
	records := make([]scannedLocationRecord, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(loc.Root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		status := model.StatusCopy
		if loc.Canonical {
			status = model.StatusCanonical
		}
		observed := model.ObservedPath{Path: path, Scope: loc.Scope, Agent: loc.AgentName, Status: status}
		parseDir := path
		if info.Mode()&os.ModeSymlink != 0 {
			target, _ := os.Readlink(path)
			if !filepath.IsAbs(target) {
				target = filepath.Join(filepath.Dir(path), target)
			}
			observed.TargetPath = filepath.Clean(target)
			if st, err := os.Stat(path); err != nil || !st.IsDir() {
				observed.Status = model.StatusBrokenSymlink
				records = append(records, scannedLocationRecord{
					keyHint: entry.Name(),
					name:    entry.Name(),
					observed: model.ObservedPath{
						Path:       observed.Path,
						Scope:      observed.Scope,
						Status:     observed.Status,
						TargetPath: observed.TargetPath,
					},
					healthIssues: []model.HealthIssue{{Type: "broken_symlink", Severity: "error", Message: "skill path is a broken symlink", Path: path}},
				})
				continue
			}
			observed.Status = model.StatusSymlink
			parseDir = path
		} else if !info.IsDir() {
			continue
		}

		skillFile := filepath.Join(parseDir, "SKILL.md")
		doc, err := frontmatter.ParseFile(skillFile)
		if err != nil {
			issueType := "invalid_frontmatter"
			if errors.Is(err, os.ErrNotExist) {
				issueType = "missing_skill_md"
			}
			records = append(records, scannedLocationRecord{
				keyHint: entry.Name(),
				name:    entry.Name(),
				observed: model.ObservedPath{
					Path:       observed.Path,
					Scope:      observed.Scope,
					Status:     observed.Status,
					TargetPath: observed.TargetPath,
				},
				healthIssues: []model.HealthIssue{{Type: issueType, Severity: "error", Message: fmt.Sprintf("invalid SKILL.md: %v", err), Path: skillFile}},
			})
			continue
		}

		canonicalPath := ""
		if loc.Canonical {
			canonicalPath = path
		}
		records = append(records, scannedLocationRecord{
			keyHint:       entry.Name(),
			name:          doc.Name,
			description:   doc.Description,
			skillPath:     skillFile,
			preview:       doc.Raw,
			canonicalPath: canonicalPath,
			observed: model.ObservedPath{
				Path:       observed.Path,
				Scope:      observed.Scope,
				Status:     observed.Status,
				TargetPath: observed.TargetPath,
			},
		})
	}
	// Discover skills nested deeper than the immediate children above. A
	// vendored bundle directory (e.g. ~/.agents/skills/medsci/skills/<name>)
	// ships its sub-skills below the one-level scan horizon; without this pass
	// they are invisible to the inventory even though they are in scope on
	// disk. Nested SKILL.md files at depth > 1 inherit the location's scope
	// (so a bundle inside the global canonical root registers its sub-skills as
	// global). A directory that bundles sub-skills is itself a bundle, not a
	// skill, so its own record (whether a real root SKILL.md or a
	// missing_skill_md placeholder) is dropped. Symlinks in the root are
	// resolved so a symlinked skills directory is still walked. Skip patterns +
	// a depth cap keep the walk bounded.
	records = scanNestedSkills(loc, records)
	return records
}

// scanNestedSkills appends records for SKILL.md files under loc.Root that are
// not immediate children (depth > 1). Immediate children are already handled by
// the one-level loop in scanLocationRecords; this only adds deeper finds so
// they are not double-counted. The root is symlink-resolved before walking, but
// records retain paths rooted at loc.Root. An immediate-child directory that
// contains nested SKILL.md files is a bundle (not a skill itself); its record is
// dropped from the returned slice.
func scanNestedSkills(loc agents.Location, records []scannedLocationRecord) []scannedLocationRecord {
	root, err := filepath.EvalSymlinks(loc.Root)
	if err != nil {
		return records
	}
	// bundleDirs collects immediate-child directories of loc.Root that contain
	// nested SKILL.md files. Keyed by logical paths so it matches the one-level
	// records' observed.Path.
	bundleDirs := map[string]bool{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		logicalPath := filepath.Join(loc.Root, filepath.FromSlash(rel))

		// WalkDir does not follow symlinks. A symlinked directory can still be a
		// nested skill, but it must not become a traversal entry point. Symlinked
		// files fall through to ordinary SKILL.md handling below.
		if d.Type()&os.ModeSymlink != 0 {
			st, statErr := os.Stat(path)
			if statErr == nil && st.IsDir() {
				name := d.Name()
				if len(parts) <= 1 || name == ".git" || name == "node_modules" || name == "vendor" || name == ".agents" || name == ".slim" || strings.HasPrefix(name, ".") || len(parts) > 5 {
					return nil
				}
				skillFile := filepath.Join(path, "SKILL.md")
				logicalSkillFile := filepath.Join(logicalPath, "SKILL.md")
				if _, skillErr := os.Stat(skillFile); errors.Is(skillErr, os.ErrNotExist) {
					return nil
				}
				bundleDirs[filepath.Clean(filepath.Join(loc.Root, parts[0]))] = true
				target, _ := os.Readlink(path)
				if !filepath.IsAbs(target) {
					target = filepath.Join(filepath.Dir(logicalPath), target)
				}
				observed := model.ObservedPath{
					Path:       logicalPath,
					Scope:      loc.Scope,
					Agent:      loc.AgentName,
					Status:     model.StatusSymlink,
					TargetPath: filepath.Clean(target),
				}
				doc, parseErr := frontmatter.ParseFile(skillFile)
				if parseErr != nil {
					issueType := "invalid_frontmatter"
					if errors.Is(parseErr, os.ErrNotExist) {
						issueType = "missing_skill_md"
					}
					records = append(records, scannedLocationRecord{
						keyHint:      filepath.Base(logicalPath),
						name:         filepath.Base(logicalPath),
						observed:     observed,
						healthIssues: []model.HealthIssue{{Type: issueType, Severity: "error", Message: fmt.Sprintf("invalid SKILL.md: %v", parseErr), Path: logicalSkillFile}},
					})
					return nil
				}
				canonicalPath := ""
				if loc.Canonical {
					canonicalPath = logicalPath
				}
				records = append(records, scannedLocationRecord{
					keyHint:       filepath.Base(logicalPath),
					name:          doc.Name,
					description:   doc.Description,
					skillPath:     logicalSkillFile,
					preview:       doc.Raw,
					canonicalPath: canonicalPath,
					observed:      observed,
				})
				return nil
			}
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".agents" || name == ".slim" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			depth := len(parts)
			if depth > 5 {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}
		// A SKILL.md is "nested" (not an immediate child) only when its skill
		// directory sits below root's first level. The file's component count is
		// one more than its dir depth (the trailing "SKILL.md"), so a skill dir
		// at depth > 1 means len(parts) > 2. len(parts) == 2 is an immediate
		// child (root/<name>/SKILL.md) already handled by the one-level scan.
		if len(parts) <= 2 {
			return nil
		}
		// This nested SKILL.md lives under an immediate-child directory of
		// loc.Root; that child is a bundle, not a skill.
		bundleDirs[filepath.Clean(filepath.Join(loc.Root, parts[0]))] = true
		logicalSkillDir := filepath.Dir(logicalPath)
		status := model.StatusCopy
		if loc.Canonical {
			status = model.StatusCanonical
		}
		doc, perr := frontmatter.ParseFile(path)
		if perr != nil {
			issueType := "invalid_frontmatter"
			if errors.Is(perr, os.ErrNotExist) {
				issueType = "missing_skill_md"
			}
			records = append(records, scannedLocationRecord{
				keyHint:      filepath.Base(logicalSkillDir),
				name:         filepath.Base(logicalSkillDir),
				observed:     model.ObservedPath{Path: logicalSkillDir, Scope: loc.Scope, Agent: loc.AgentName, Status: status},
				healthIssues: []model.HealthIssue{{Type: issueType, Severity: "error", Message: fmt.Sprintf("invalid SKILL.md: %v", perr), Path: logicalPath}},
			})
			return nil
		}
		canonicalPath := ""
		if loc.Canonical {
			canonicalPath = logicalSkillDir
		}
		records = append(records, scannedLocationRecord{
			keyHint:       filepath.Base(logicalSkillDir),
			name:          doc.Name,
			description:   doc.Description,
			skillPath:     logicalPath,
			preview:       doc.Raw,
			canonicalPath: canonicalPath,
			observed: model.ObservedPath{
				Path:   logicalSkillDir,
				Scope:  loc.Scope,
				Agent:  loc.AgentName,
				Status: status,
			},
		})
		return nil
	})
	// Drop records for bundle directories: a directory that bundles sub-skills
	// is not itself a skill, whether or not it has a root SKILL.md.
	if len(bundleDirs) > 0 {
		filtered := make([]scannedLocationRecord, 0, len(records))
		for _, r := range records {
			if !bundleDirs[filepath.Clean(r.observed.Path)] {
				filtered = append(filtered, r)
			}
		}
		records = filtered
	}
	return records
}

func applyScannedLocationRecords(loc agents.Location, skills map[string]*model.Skill, records []scannedLocationRecord) {
	for _, record := range records {
		sk := ensureSkill(skills, loc.Scope, record.keyHint, record.name, record.description)
		if record.canonicalPath != "" && sk.CanonicalPath == "" {
			sk.CanonicalPath = record.canonicalPath
		}
		if record.skillPath != "" && sk.SkillPath == "" {
			sk.SkillPath = record.skillPath
		}
		if record.preview != "" && sk.Preview == "" {
			sk.Preview = record.preview
		}
		observed := record.observed
		observed.Agent = loc.AgentName
		observed.SharedRoot = loc.SharedRoot
		addObservedPath(sk, observed)
		for _, issue := range record.healthIssues {
			sk.AddHealthIssue(issue)
		}
		if loc.SharedRoot {
			sk.AddHealthIssue(model.HealthIssue{
				Type:     "shared_scope_root",
				Severity: "warning",
				Message:  sharedScopeRootMessage(loc),
				Path:     observed.Path,
			})
		}
	}
}

// sharedScopeRootMessage explains, for the TUI and JSON output, why a skill
// reached through a symlinked scope root has its write actions disabled.
func sharedScopeRootMessage(loc agents.Location) string {
	return fmt.Sprintf(
		"skills root is reached through symlink %s -> %s; files here are shared with other locations and write actions are disabled",
		compat.SanitizeMetadata(loc.SharedRootLink), compat.SanitizeMetadata(loc.SharedRootTarget),
	)
}

func ensureSkill(skills map[string]*model.Skill, scope model.Scope, keyHint, name, desc string) *model.Skill {
	keyName := name
	if compat.NormalizeName(keyName) == "" {
		keyName = keyHint
	}
	key := skillKey(scope, keyName)
	if sk, ok := skills[key]; ok {
		if sk.Description == "" && desc != "" {
			sk.Description = desc
		}
		return sk
	}
	sk := &model.Skill{Name: keyName, Description: desc, Scope: scope}
	skills[key] = sk
	return sk
}

func skillKey(scope model.Scope, name string) string {
	return string(scope) + "\x00" + compat.NormalizeName(name)
}

func addObservedPath(sk *model.Skill, observed model.ObservedPath) {
	for _, existing := range sk.ObservedPaths {
		if existing.Path == observed.Path && existing.Agent == observed.Agent && existing.Scope == observed.Scope {
			return
		}
	}
	sk.ObservedPaths = append(sk.ObservedPaths, observed)
}

func correlateLocksIndexed(skills map[string]*model.Skill, local map[string]model.LocalLockEntry, global map[string]model.GlobalLockEntry) {
	localIndex := newLocalLockLookup(local)
	globalIndex := newGlobalLockLookup(global)
	for _, sk := range skills {
		if sk.Scope == model.ScopeProject {
			if e, ok := localIndex.find(sk); ok {
				entry := e
				sk.LocalLock = &entry
			}
		}
		if sk.Scope == model.ScopeGlobal {
			if e, ok := globalIndex.find(sk); ok {
				entry := e
				sk.GlobalLock = &entry
			}
		}
	}
}

type localLockLookup struct {
	byKey        map[string]model.LocalLockEntry
	byNormalized map[string]model.LocalLockEntry
}

func newLocalLockLookup(lock map[string]model.LocalLockEntry) localLockLookup {
	lookup := localLockLookup{byKey: map[string]model.LocalLockEntry{}, byNormalized: map[string]model.LocalLockEntry{}}
	for key, entry := range lock {
		lookup.byKey[key] = entry
		normalized := compat.NormalizeName(key)
		if _, exists := lookup.byNormalized[normalized]; !exists {
			lookup.byNormalized[normalized] = entry
		}
	}
	return lookup
}

func (lookup localLockLookup) find(sk *model.Skill) (model.LocalLockEntry, bool) {
	for _, candidate := range candidateLockKeys(sk) {
		if entry, ok := lookup.byKey[candidate]; ok {
			return entry, true
		}
		if entry, ok := lookup.byKey[compat.SanitizeName(candidate)]; ok {
			return entry, true
		}
		if entry, ok := lookup.byNormalized[compat.NormalizeName(candidate)]; ok {
			return entry, true
		}
	}
	return model.LocalLockEntry{}, false
}

type globalLockLookup struct {
	byKey        map[string]model.GlobalLockEntry
	byNormalized map[string]model.GlobalLockEntry
}

func newGlobalLockLookup(lock map[string]model.GlobalLockEntry) globalLockLookup {
	lookup := globalLockLookup{byKey: map[string]model.GlobalLockEntry{}, byNormalized: map[string]model.GlobalLockEntry{}}
	for key, entry := range lock {
		lookup.byKey[key] = entry
		normalized := compat.NormalizeName(key)
		if _, exists := lookup.byNormalized[normalized]; !exists {
			lookup.byNormalized[normalized] = entry
		}
	}
	return lookup
}

func (lookup globalLockLookup) find(sk *model.Skill) (model.GlobalLockEntry, bool) {
	for _, candidate := range candidateLockKeys(sk) {
		if entry, ok := lookup.byKey[candidate]; ok {
			return entry, true
		}
		if entry, ok := lookup.byKey[compat.SanitizeName(candidate)]; ok {
			return entry, true
		}
		if entry, ok := lookup.byNormalized[compat.NormalizeName(candidate)]; ok {
			return entry, true
		}
	}
	return model.GlobalLockEntry{}, false
}

type lockMatchIndex struct {
	project map[string]bool
	global  map[string]bool
}

func newLockMatchIndex(skills map[string]*model.Skill) lockMatchIndex {
	index := lockMatchIndex{project: map[string]bool{}, global: map[string]bool{}}
	for _, sk := range skills {
		target := index.project
		if sk.Scope == model.ScopeGlobal {
			target = index.global
		}
		for _, candidate := range candidateLockKeys(sk) {
			target[candidate] = true
			target[compat.SanitizeName(candidate)] = true
			target[compat.NormalizeName(candidate)] = true
		}
	}
	return index
}

func (index lockMatchIndex) hasMatch(scope model.Scope, key string) bool {
	target := index.project
	if scope == model.ScopeGlobal {
		target = index.global
	}
	return target[key] || target[compat.NormalizeName(key)]
}

func candidateLockKeys(sk *model.Skill) []string {
	keys := []string{sk.Name, compat.SanitizeName(sk.Name)}
	for _, p := range sk.ObservedPaths {
		base := filepath.Base(p.Path)
		keys = append(keys, base, compat.SanitizeName(base))
	}
	return keys
}

func addDuplicateAndShadowingIssues(skills map[string]*model.Skill) {
	byName := map[string][]*model.Skill{}
	for _, sk := range skills {
		byName[compat.NormalizeName(sk.Name)] = append(byName[compat.NormalizeName(sk.Name)], sk)
	}
	for _, group := range byName {
		if len(group) < 2 {
			continue
		}
		sawProject, sawGlobal := false, false
		for _, sk := range group {
			sawProject = sawProject || sk.Scope == model.ScopeProject
			sawGlobal = sawGlobal || sk.Scope == model.ScopeGlobal
		}
		issueType := "duplicate_name"
		message := "multiple skills share this name"
		if sawProject && sawGlobal {
			issueType = "project_global_shadowing"
			message = "project and global skills share this name"
		}
		for _, sk := range group {
			sk.AddHealthIssue(model.HealthIssue{Type: issueType, Severity: "warning", Message: message})
		}
	}
}

func Snapshot(cwd string) (model.ScanResult, error) {
	return Run(cwd)
}
