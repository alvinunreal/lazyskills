package scan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"lazyskills/internal/agents"
	"lazyskills/internal/compat"
	"lazyskills/internal/frontmatter"
	"lazyskills/internal/locks"
	"lazyskills/internal/model"
	"lazyskills/internal/visibility"
)

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

	res := model.ScanResult{Cwd: absCwd, ProjectLock: locks.ProjectLockPath(absCwd), GlobalLock: locks.GlobalLockPath()}
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
	projectVisibility, globalVisibility := readVisibilityStates(absCwd, &res)

	for _, loc := range agents.Locations(absCwd) {
		scanLocation(loc, skills)
	}

	correlateLocks(skills, localLock.Skills, globalLock.Skills)
	for key, entry := range localLock.Skills {
		if !hasLockMatch(skills, model.ScopeProject, key) {
			sk := ensureSkill(skills, model.ScopeProject, key, key, "")
			e := entry
			sk.LocalLock = &e
			sk.AddHealthIssue(model.HealthIssue{Type: "lock_without_files", Severity: "warning", Message: "project lock entry has no matching skill on disk"})
		}
	}
	for key, entry := range globalLock.Skills {
		if !hasLockMatch(skills, model.ScopeGlobal, key) {
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
		sk.Visibility = skillVisibility(absCwd, sk, res.Agents, projectVisibility, globalVisibility, &res)
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

func readVisibilityStates(cwd string, res *model.ScanResult) (*visibility.VisibilityState, *visibility.VisibilityState) {
	projectState, err := visibility.ReadState(visibility.ProjectStatePath(cwd))
	if err != nil {
		res.HealthIssues = append(res.HealthIssues, model.HealthIssue{Type: "corrupt_visibility_state", Severity: "warning", Message: err.Error(), Path: visibility.ProjectStatePath(cwd)})
	}
	globalPath, pathErr := visibility.GlobalStatePath()
	if pathErr != nil {
		res.HealthIssues = append(res.HealthIssues, model.HealthIssue{Type: "visibility_state_unavailable", Severity: "warning", Message: pathErr.Error()})
		return projectState, visibility.NewVisibilityState()
	}
	globalState, err := visibility.ReadState(globalPath)
	if err != nil {
		res.HealthIssues = append(res.HealthIssues, model.HealthIssue{Type: "corrupt_visibility_state", Severity: "warning", Message: err.Error(), Path: globalPath})
	}
	return projectState, globalState
}

func hasNonCanonicalObservation(sk *model.Skill) bool {
	for _, observed := range sk.ObservedPaths {
		if observed.Status != model.StatusCanonical {
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

func skillVisibility(cwd string, sk *model.Skill, states []model.AgentState, projectVisibility, globalVisibility *visibility.VisibilityState, res *model.ScanResult) []model.SkillVisibility {
	visibility := make([]model.SkillVisibility, 0, len(states))
	for _, state := range states {
		item := model.SkillVisibility{Agent: state.Name, Display: state.Display}
		if observed, ok := observedForAgent(sk, state.Name); ok {
			item.Visible = observed.Status != model.StatusBrokenSymlink
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
			default:
				item.Reason = "visible"
			}
			if disabledEntry, _, disabled := disabledEntryForSkill(sk, state.Name, projectVisibility, globalVisibility); disabled {
				if reason, ok := reconcileDisabledEntry(cwd, sk, state.Name, disabledEntry, res); ok && reason != "" {
					item.Reason = reason
					item.Visible = true
					item.Path = firstNonEmpty(item.Path, disabledEntry.MutatedPath)
				}
			}
			visibility = append(visibility, item)
			continue
		}

		item.Visible = false
		if disabledEntry, _, disabled := disabledEntryForSkill(sk, state.Name, projectVisibility, globalVisibility); disabled {
			if reason, ok := reconcileDisabledEntry(cwd, sk, state.Name, disabledEntry, res); ok && (reason == "disabled_by_lazyskills" || reason == "disabled_pending_finalize") {
				item.Reason = "disabled_by_lazyskills"
				item.Path = disabledEntry.MutatedPath
				item.Status = disabledEntry.ObservedStatus
			} else {
				applyDefaultVisibilityReason(sk, state, &item)
			}
		} else {
			applyDefaultVisibilityReason(sk, state, &item)
		}
		visibility = append(visibility, item)
	}
	return visibility
}

func applyDefaultVisibilityReason(sk *model.Skill, state model.AgentState, item *model.SkillVisibility) {
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
}

func reconcileDisabledEntry(cwd string, sk *model.Skill, agent string, entry visibility.DisabledEntry, res *model.ScanResult) (string, bool) {
	if entry.ObservedStatus != model.StatusSymlink {
		addVisibilityHealth(res, "invalid_visibility_state", "disabled entry is not symlink-backed", entry.MutatedPath)
		return "", false
	}
	if err := visibility.ValidateEntryPath(cwd, entry); err != nil {
		addVisibilityHealth(res, "invalid_visibility_state", err.Error(), entry.MutatedPath)
		return "", false
	}
	exists, err := visibility.EntryPathExists(entry)
	if err != nil {
		addVisibilityHealth(res, "visibility_state_conflict", err.Error(), entry.MutatedPath)
		return "", false
	}
	switch entry.State {
	case visibility.StateActive:
		if exists {
			addVisibilityHealth(res, "stale_visibility_state", fmt.Sprintf("disabled entry for %s/%s exists but the skill path is present", agent, sk.Name), entry.MutatedPath)
			return "disabled_state_conflict", true
		}
		return "disabled_by_lazyskills", true
	case visibility.StatePendingDisable:
		if exists {
			addVisibilityHealth(res, "pending_visibility_state", fmt.Sprintf("pending disable for %s/%s did not remove the skill path", agent, sk.Name), entry.MutatedPath)
			return "pending_disable", true
		}
		addVisibilityHealth(res, "pending_visibility_state", fmt.Sprintf("pending disable for %s/%s removed the path but was not finalized", agent, sk.Name), entry.MutatedPath)
		return "disabled_pending_finalize", true
	case visibility.StatePendingEnable, visibility.StateConflict:
		addVisibilityHealth(res, "visibility_state_conflict", fmt.Sprintf("disabled entry for %s/%s has state %s", agent, sk.Name, entry.State), entry.MutatedPath)
		return "", false
	default:
		addVisibilityHealth(res, "invalid_visibility_state", fmt.Sprintf("disabled entry for %s/%s has unknown state %s", agent, sk.Name, entry.State), entry.MutatedPath)
		return "", false
	}
}

func addVisibilityHealth(res *model.ScanResult, issueType, message, path string) {
	if res == nil {
		return
	}
	for _, existing := range res.HealthIssues {
		if existing.Type == issueType && existing.Message == message && existing.Path == path {
			return
		}
	}
	res.HealthIssues = append(res.HealthIssues, model.HealthIssue{Type: issueType, Severity: "warning", Message: message, Path: path})
}

func disabledEntryForSkill(sk *model.Skill, agent string, projectVisibility, globalVisibility *visibility.VisibilityState) (visibility.DisabledEntry, string, bool) {
	state := projectVisibility
	if sk.Scope == model.ScopeGlobal {
		state = globalVisibility
	}
	return visibility.FindEntryForSkill(state, sk, agent, sk.Scope)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func observedForAgent(sk *model.Skill, agent string) (model.ObservedPath, bool) {
	for _, observed := range sk.ObservedPaths {
		if observed.Agent == agent {
			return observed, true
		}
	}
	return model.ObservedPath{}, false
}

func scanLocation(loc agents.Location, skills map[string]*model.Skill) {
	entries, err := os.ReadDir(loc.Root)
	if err != nil {
		return
	}
	for _, entry := range entries {
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
				sk := ensureSkill(skills, loc.Scope, entry.Name(), entry.Name(), "")
				addObservedPath(sk, observed)
				sk.AddHealthIssue(model.HealthIssue{Type: "broken_symlink", Severity: "error", Message: "skill path is a broken symlink", Path: path})
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
			sk := ensureSkill(skills, loc.Scope, entry.Name(), entry.Name(), "")
			addObservedPath(sk, observed)
			issueType := "invalid_frontmatter"
			if errors.Is(err, os.ErrNotExist) {
				issueType = "missing_skill_md"
			}
			sk.AddHealthIssue(model.HealthIssue{Type: issueType, Severity: "error", Message: fmt.Sprintf("invalid SKILL.md: %v", err), Path: skillFile})
			continue
		}

		sk := ensureSkill(skills, loc.Scope, entry.Name(), doc.Name, doc.Description)
		if loc.Canonical && sk.CanonicalPath == "" {
			sk.CanonicalPath = path
		}
		if sk.SkillPath == "" {
			sk.SkillPath = skillFile
		}
		if sk.Preview == "" {
			sk.Preview = doc.Raw
		}
		addObservedPath(sk, observed)
	}
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

func correlateLocks(skills map[string]*model.Skill, local map[string]model.LocalLockEntry, global map[string]model.GlobalLockEntry) {
	for _, sk := range skills {
		if sk.Scope == model.ScopeProject {
			if e, ok := findLocalLock(local, sk); ok {
				entry := e
				sk.LocalLock = &entry
			}
		}
		if sk.Scope == model.ScopeGlobal {
			if e, ok := findGlobalLock(global, sk); ok {
				entry := e
				sk.GlobalLock = &entry
			}
		}
	}
}

func candidateLockKeys(sk *model.Skill) []string {
	keys := []string{sk.Name, compat.SanitizeName(sk.Name)}
	for _, p := range sk.ObservedPaths {
		base := filepath.Base(p.Path)
		keys = append(keys, base, compat.SanitizeName(base))
	}
	return keys
}

func findLocalLock(lock map[string]model.LocalLockEntry, sk *model.Skill) (model.LocalLockEntry, bool) {
	for key, entry := range lock {
		if lockKeyMatches(key, sk) {
			return entry, true
		}
	}
	return model.LocalLockEntry{}, false
}

func findGlobalLock(lock map[string]model.GlobalLockEntry, sk *model.Skill) (model.GlobalLockEntry, bool) {
	for key, entry := range lock {
		if lockKeyMatches(key, sk) {
			return entry, true
		}
	}
	return model.GlobalLockEntry{}, false
}

func lockKeyMatches(key string, sk *model.Skill) bool {
	for _, candidate := range candidateLockKeys(sk) {
		if key == candidate || key == compat.SanitizeName(candidate) || compat.NormalizeName(key) == compat.NormalizeName(candidate) {
			return true
		}
	}
	return false
}

func hasLockMatch(skills map[string]*model.Skill, scope model.Scope, key string) bool {
	for _, sk := range skills {
		if sk.Scope == scope && lockKeyMatches(key, sk) {
			return true
		}
	}
	return false
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

type Scanner struct {
	Cwd string
}

func New(cwd string) Scanner {
	return Scanner{Cwd: cwd}
}

func (s Scanner) Snapshot() (model.ScanResult, error) {
	return Run(s.Cwd)
}

func Snapshot(cwd string) (model.ScanResult, error) {
	return New(cwd).Snapshot()
}
