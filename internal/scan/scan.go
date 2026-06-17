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
	skills := map[string]*model.Skill{}

	localLock, err := locks.ReadLocal(res.ProjectLock)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		res.HealthIssues = append(res.HealthIssues, model.HealthIssue{Type: "corrupt_project_lock", Severity: "warning", Message: err.Error(), Path: res.ProjectLock})
	}
	globalLock, err := locks.ReadGlobal(res.GlobalLock)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		res.HealthIssues = append(res.HealthIssues, model.HealthIssue{Type: "corrupt_global_lock", Severity: "warning", Message: err.Error(), Path: res.GlobalLock})
	}

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
		}
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
