package visibility

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"lazyskills/internal/agents"
	"lazyskills/internal/compat"
	"lazyskills/internal/model"
)

type State string

const (
	StatePendingDisable State = "pending_disable"
	StateActive         State = "active"
	StatePendingEnable  State = "pending_enable"
	StateConflict       State = "conflict"
)

type LockIdentity struct {
	Source          string `json:"source,omitempty"`
	Ref             string `json:"ref,omitempty"`
	SourceType      string `json:"sourceType,omitempty"`
	SkillPath       string `json:"skillPath,omitempty"`
	ComputedHash    string `json:"computedHash,omitempty"`
	SourceURL       string `json:"sourceUrl,omitempty"`
	SkillFolderHash string `json:"skillFolderHash,omitempty"`
	InstalledAt     string `json:"installedAt,omitempty"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
	PluginName      string `json:"pluginName,omitempty"`
}

type DisabledEntry struct {
	Version             int          `json:"version"`
	Scope               model.Scope  `json:"scope"`
	Agent               string       `json:"agent"`
	SkillDisplayName    string       `json:"skill_display_name"`
	NormalizedSkillName string       `json:"normalized_skill_name"`
	LockIdentity        LockIdentity `json:"lock_identity,omitempty"`
	CanonicalPath       string       `json:"canonical_path,omitempty"`
	MutatedPath         string       `json:"mutated_path"`
	ObservedStatus      model.Status `json:"observed_status"`
	RawSymlinkTarget    string       `json:"raw_symlink_target"`
	Timestamp           time.Time    `json:"timestamp"`
	State               State        `json:"state"`
}

type VisibilityState struct {
	Version     int                      `json:"version"`
	Skills      map[string]DisabledEntry `json:"skills"`
	Diagnostics []string                 `json:"-"`
}

var HookPostPendingDisableWrite func()

func NewVisibilityState() *VisibilityState {
	return &VisibilityState{
		Version: 1,
		Skills:  make(map[string]DisabledEntry),
	}
}

func NewLockIdentityFromLocal(entry *model.LocalLockEntry) LockIdentity {
	if entry == nil {
		return LockIdentity{}
	}
	return LockIdentity{
		Source:       entry.Source,
		Ref:          entry.Ref,
		SourceType:   entry.SourceType,
		SkillPath:    entry.SkillPath,
		ComputedHash: entry.ComputedHash,
	}
}

func NewLockIdentityFromGlobal(entry *model.GlobalLockEntry) LockIdentity {
	if entry == nil {
		return LockIdentity{}
	}
	return LockIdentity{
		Source:          entry.Source,
		SourceType:      entry.SourceType,
		SourceURL:       entry.SourceURL,
		Ref:             entry.Ref,
		SkillPath:       entry.SkillPath,
		SkillFolderHash: entry.SkillFolderHash,
		InstalledAt:     entry.InstalledAt,
		UpdatedAt:       entry.UpdatedAt,
		PluginName:      entry.PluginName,
	}
}

func ReadState(path string) (*VisibilityState, error) {
	state := NewVisibilityState()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return state, nil
		}
		return state, err
	}
	if err := json.Unmarshal(b, state); err != nil {
		state.Diagnostics = append(state.Diagnostics, fmt.Sprintf("corrupt visibility state JSON: %v", err))
		if state.Skills == nil {
			state.Skills = make(map[string]DisabledEntry)
		}
		return state, fmt.Errorf("corrupt visibility state JSON: %w", err)
	}
	if state.Skills == nil {
		state.Skills = make(map[string]DisabledEntry)
	}
	return state, nil
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	if filepath.Base(dir) == ".lazyskills" {
		giPath := filepath.Join(dir, ".gitignore")
		if _, err := os.Stat(giPath); os.IsNotExist(err) {
			content := "*\n!.gitignore\n"
			if err := os.WriteFile(giPath, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write .gitignore: %w", err)
			}
		}
	}
	return nil
}

func WriteState(path string, state *VisibilityState) error {
	if err := ensureDir(path); err != nil {
		return fmt.Errorf("failed to create directory for visibility state: %w", err)
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal visibility state: %w", err)
	}

	dir := filepath.Dir(path)
	tempFile, err := os.CreateTemp(dir, "visibility-*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file for atomic write: %w", err)
	}
	tempName := tempFile.Name()
	defer func() {
		if tempFile != nil {
			tempFile.Close()
			_ = os.Remove(tempName)
		}
	}()

	if _, err := tempFile.Write(b); err != nil {
		return fmt.Errorf("failed to write to temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		tempFile = nil
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	tempFile = nil

	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("failed to rename temp file to target path: %w", err)
	}
	return nil
}

func ProjectStatePath(cwd string) string {
	return filepath.Join(cwd, ".lazyskills", "visibility.json")
}

func GlobalStatePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("user config dir and home dir not found: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "lazyskills", "visibility.json"), nil
}

func StateKey(scope model.Scope, agentName, normalizedSkillName, canonicalPath, mutatedPath string) string {
	return strings.Join([]string{string(scope), agentName, normalizedSkillName, filepath.Clean(canonicalPath), filepath.Clean(mutatedPath)}, "\x00")
}

func StatePath(cwd string, scope model.Scope) (string, error) {
	if scope == model.ScopeProject {
		return ProjectStatePath(cwd), nil
	}
	return GlobalStatePath()
}

func ResolveAgent(agentName string, cwd string) (agents.Agent, error) {
	for _, a := range agents.RegistryWithEnv(agents.DefaultEnv(), cwd) {
		if a.Name == agentName {
			return a, nil
		}
	}
	return agents.Agent{}, fmt.Errorf("agent %q not found in registry", agentName)
}

func ExpectedAgentDir(agent agents.Agent, scope model.Scope, cwd string) string {
	if scope == model.ScopeProject {
		return filepath.Clean(filepath.Join(cwd, filepath.FromSlash(agent.ProjectDir)))
	}
	return filepath.Clean(agent.GlobalDir)
}

func IsDirectChild(parent, child string) bool {
	absParent, err := filepath.Abs(parent)
	if err != nil {
		return false
	}
	absChild, err := filepath.Abs(child)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absParent, absChild)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !strings.Contains(rel, string(filepath.Separator))
}

func ValidatePath(path string, agent agents.Agent, scope model.Scope, cwd string) error {
	expectedDir := ExpectedAgentDir(agent, scope, cwd)
	if expectedDir == "" {
		return fmt.Errorf("expected agent dir is empty")
	}
	if !IsDirectChild(expectedDir, path) {
		return fmt.Errorf("path %q is not a direct child of expected agent directory %q", path, expectedDir)
	}
	return nil
}

func ValidateEntryPath(cwd string, entry DisabledEntry) error {
	agent, err := ResolveAgent(entry.Agent, cwd)
	if err != nil {
		return err
	}
	return ValidatePath(entry.MutatedPath, agent, entry.Scope, cwd)
}

func EntryPathExists(entry DisabledEntry) (bool, error) {
	_, err := os.Lstat(entry.MutatedPath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func EntryTargetExists(entry DisabledEntry) bool {
	resolvedTarget := entry.RawSymlinkTarget
	if resolvedTarget == "" {
		resolvedTarget = entry.CanonicalPath
	}
	if resolvedTarget == "" {
		return false
	}
	if !filepath.IsAbs(resolvedTarget) {
		resolvedTarget = filepath.Clean(filepath.Join(filepath.Dir(entry.MutatedPath), resolvedTarget))
	}
	_, err := os.Stat(resolvedTarget)
	return err == nil
}

func EntryCanEnable(cwd string, entry DisabledEntry) error {
	if entry.State != StateActive && entry.State != StatePendingDisable {
		return fmt.Errorf("disabled entry state %q cannot be enabled", entry.State)
	}
	if entry.RawSymlinkTarget == "" {
		return fmt.Errorf("raw symlink target is missing")
	}
	if err := ValidateEntryPath(cwd, entry); err != nil {
		return err
	}
	exists, err := EntryPathExists(entry)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("destination path already exists")
	}
	if !EntryTargetExists(entry) {
		return fmt.Errorf("symlink target does not exist")
	}
	return nil
}

func SharedDirectoryAgents(agentName string, scope model.Scope, cwd string) ([]string, error) {
	allAgents := agents.RegistryWithEnv(agents.DefaultEnv(), cwd)
	var targetAgent *agents.Agent
	for i := range allAgents {
		if allAgents[i].Name == agentName {
			targetAgent = &allAgents[i]
			break
		}
	}
	if targetAgent == nil {
		return nil, fmt.Errorf("agent %q not found in registry", agentName)
	}
	if scope == model.ScopeGlobal && !targetAgent.SupportsGlobal {
		return nil, nil
	}

	targetDir := ExpectedAgentDir(*targetAgent, scope, cwd)
	if targetDir == "" {
		return nil, fmt.Errorf("expected directory for agent %q is empty", agentName)
	}
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path of agent dir: %w", err)
	}

	var sharingAgents []string
	for _, a := range allAgents {
		if a.Name == agentName {
			continue
		}
		if scope == model.ScopeGlobal && !a.SupportsGlobal {
			continue
		}
		aDir := ExpectedAgentDir(a, scope, cwd)
		if aDir == "" {
			continue
		}
		absADir, err := filepath.Abs(aDir)
		if err != nil {
			continue
		}
		if absADir == absTarget {
			sharingAgents = append(sharingAgents, a.Name)
		}
	}

	return sharingAgents, nil
}

func Disable(cwd string, skill *model.Skill, agentName string, scope model.Scope) error {
	agent, err := ResolveAgent(agentName, cwd)
	if err != nil {
		return err
	}

	sharing, err := SharedDirectoryAgents(agentName, scope, cwd)
	if err != nil {
		return err
	}
	if len(sharing) > 0 {
		return fmt.Errorf("agent shares directory with other agent(s): %s", strings.Join(sharing, ", "))
	}

	var targetObserved *model.ObservedPath
	for i := range skill.ObservedPaths {
		if skill.ObservedPaths[i].Agent == agentName && skill.ObservedPaths[i].Scope == scope {
			targetObserved = &skill.ObservedPaths[i]
			break
		}
	}

	if targetObserved == nil {
		return fmt.Errorf("no observed path found for agent %q and scope %q", agentName, scope)
	}

	if targetObserved.Status != model.StatusSymlink {
		return fmt.Errorf("disable only supported for symlinks, observed status is %q", targetObserved.Status)
	}

	if err := ValidatePath(targetObserved.Path, agent, scope, cwd); err != nil {
		return fmt.Errorf("path safety validation failed: %w", err)
	}

	info, err := os.Lstat(targetObserved.Path)
	if err != nil {
		return fmt.Errorf("failed to lstat mutated path %q: %w", targetObserved.Path, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("path %q is not a symlink on disk", targetObserved.Path)
	}

	rawTarget, err := os.Readlink(targetObserved.Path)
	if err != nil {
		return fmt.Errorf("failed to read symlink target for %q: %w", targetObserved.Path, err)
	}

	hasLocalLock := (scope == model.ScopeProject && skill.LocalLock != nil)
	hasGlobalLock := (scope == model.ScopeGlobal && skill.GlobalLock != nil)
	hasCanonical := skill.CanonicalPath != ""
	if !hasLocalLock && !hasGlobalLock && !hasCanonical {
		return fmt.Errorf("skill must be lock-backed or have a canonical path")
	}

	var lockIdentity LockIdentity
	if scope == model.ScopeProject && skill.LocalLock != nil {
		lockIdentity = NewLockIdentityFromLocal(skill.LocalLock)
	} else if scope == model.ScopeGlobal && skill.GlobalLock != nil {
		lockIdentity = NewLockIdentityFromGlobal(skill.GlobalLock)
	}

	var statePath string
	if scope == model.ScopeProject {
		statePath = ProjectStatePath(cwd)
	} else {
		var err error
		statePath, err = GlobalStatePath()
		if err != nil {
			return err
		}
	}

	state, err := ReadState(statePath)
	if err != nil {
		// Do not silently ignore corrupt state - report the diagnostic error
		return fmt.Errorf("corrupt visibility state read: %w", err)
	}

	normName := compat.NormalizeName(skill.Name)
	entry := DisabledEntry{
		Version:             1,
		Scope:               scope,
		Agent:               agentName,
		SkillDisplayName:    skill.Name,
		NormalizedSkillName: normName,
		LockIdentity:        lockIdentity,
		CanonicalPath:       skill.CanonicalPath,
		MutatedPath:         targetObserved.Path,
		ObservedStatus:      targetObserved.Status,
		RawSymlinkTarget:    rawTarget,
		Timestamp:           time.Now(),
		State:               StatePendingDisable,
	}

	key := StateKey(scope, agentName, normName, entry.CanonicalPath, entry.MutatedPath)
	state.Skills[key] = entry

	if err := WriteState(statePath, state); err != nil {
		return fmt.Errorf("failed to write pending disable state: %w", err)
	}

	if hook := HookPostPendingDisableWrite; hook != nil {
		hook()
	}

	if err := os.Remove(targetObserved.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove symlink %q: %w", targetObserved.Path, err)
	}

	entry.State = StateActive
	state.Skills[key] = entry
	if err := WriteState(statePath, state); err != nil {
		return fmt.Errorf("failed to write active disable state: %w", err)
	}

	return nil
}

func Enable(cwd string, skill *model.Skill, agentName string, scope model.Scope) error {
	var statePath string
	if scope == model.ScopeProject {
		statePath = ProjectStatePath(cwd)
	} else {
		var err error
		statePath, err = GlobalStatePath()
		if err != nil {
			return err
		}
	}

	state, err := ReadState(statePath)
	if err != nil {
		// Do not silently ignore corrupt state - report the diagnostic error
		return fmt.Errorf("corrupt visibility state read: %w", err)
	}

	entry, key, exists := FindEntryForSkill(state, skill, agentName, scope)
	if !exists {
		return fmt.Errorf("no disabled state found for skill %q, agent %q, scope %q", skillName(skill), agentName, scope)
	}

	if err := EntryCanEnable(cwd, entry); err != nil {
		return fmt.Errorf("disabled entry cannot be enabled: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(entry.MutatedPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory for restored symlink: %w", err)
	}

	if err := os.Symlink(entry.RawSymlinkTarget, entry.MutatedPath); err != nil {
		return fmt.Errorf("failed to recreate symlink: %w", err)
	}

	delete(state.Skills, key)
	if err := WriteState(statePath, state); err != nil {
		return fmt.Errorf("failed to update state after enabling: %w", err)
	}

	return nil
}

func FindEntryForSkill(state *VisibilityState, skill *model.Skill, agentName string, scope model.Scope) (DisabledEntry, string, bool) {
	if state == nil || skill == nil {
		return DisabledEntry{}, "", false
	}
	for key, entry := range state.Skills {
		if entry.Scope != scope || entry.Agent != agentName {
			continue
		}
		if !entryMatchesSkillIdentity(entry, skill, scope) {
			continue
		}
		return entry, key, true
	}
	return DisabledEntry{}, "", false
}

func entryMatchesSkillIdentity(entry DisabledEntry, skill *model.Skill, scope model.Scope) bool {
	if skill == nil {
		return false
	}
	entryHasCanonical := entry.CanonicalPath != "" && filepath.Clean(entry.CanonicalPath) != "."
	skillHasCanonical := skill.CanonicalPath != "" && filepath.Clean(skill.CanonicalPath) != "."
	if entryHasCanonical && skillHasCanonical {
		return filepath.Clean(entry.CanonicalPath) == filepath.Clean(skill.CanonicalPath) && compatibleLockOrName(entry, skill, scope)
	}
	if entryHasCanonical != skillHasCanonical {
		return false
	}

	entryHasLock := !entry.LockIdentity.empty()
	skillHasLock := skillHasLockIdentity(skill, scope)
	if entryHasLock && skillHasLock {
		return lockIdentityMatchesSkill(entry.LockIdentity, skill, scope)
	}
	if entryHasLock != skillHasLock {
		return false
	}

	return entry.NormalizedSkillName != "" && entry.NormalizedSkillName == compat.NormalizeName(skill.Name)
}

func compatibleLockOrName(entry DisabledEntry, skill *model.Skill, scope model.Scope) bool {
	entryHasLock := !entry.LockIdentity.empty()
	skillHasLock := skillHasLockIdentity(skill, scope)
	if entryHasLock && skillHasLock {
		return lockIdentityMatchesSkill(entry.LockIdentity, skill, scope)
	}
	if entryHasLock != skillHasLock {
		return false
	}
	return entry.NormalizedSkillName == "" || entry.NormalizedSkillName == compat.NormalizeName(skill.Name)
}

func (identity LockIdentity) empty() bool {
	return identity.Source == "" && identity.Ref == "" && identity.SourceType == "" && identity.SkillPath == "" && identity.ComputedHash == "" && identity.SourceURL == "" && identity.SkillFolderHash == "" && identity.PluginName == ""
}

func skillHasLockIdentity(skill *model.Skill, scope model.Scope) bool {
	if skill == nil {
		return false
	}
	if scope == model.ScopeProject {
		return skill.LocalLock != nil
	}
	if scope == model.ScopeGlobal {
		return skill.GlobalLock != nil
	}
	return false
}

func lockIdentityMatchesSkill(identity LockIdentity, skill *model.Skill, scope model.Scope) bool {
	if skill == nil {
		return false
	}
	if scope == model.ScopeProject && skill.LocalLock != nil {
		lock := NewLockIdentityFromLocal(skill.LocalLock)
		return identity.Source == lock.Source && identity.SourceType == lock.SourceType && identity.Ref == lock.Ref && identity.SkillPath == lock.SkillPath && identity.ComputedHash == lock.ComputedHash
	}
	if scope == model.ScopeGlobal && skill.GlobalLock != nil {
		lock := NewLockIdentityFromGlobal(skill.GlobalLock)
		return identity.Source == lock.Source && identity.SourceType == lock.SourceType && identity.SourceURL == lock.SourceURL && identity.Ref == lock.Ref && identity.SkillPath == lock.SkillPath && identity.SkillFolderHash == lock.SkillFolderHash
	}
	return false
}

func skillName(skill *model.Skill) string {
	if skill == nil {
		return ""
	}
	return skill.Name
}
