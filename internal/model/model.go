package model

// Scope represents project-local or machine-global scope.
type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

// Status represents the relationship of a path to the skill (canonical repository vs link/copy).
type Status string

const (
	StatusCanonical     Status = "canonical"
	StatusSymlink       Status = "symlink"
	StatusCopy          Status = "copy"
	StatusBrokenSymlink Status = "broken_symlink"
	StatusDisabled      Status = "disabled"
)

// ObservedPath represents one location on disk where this skill was found.
type ObservedPath struct {
	Path       string `json:"path"`
	Scope      Scope  `json:"scope"`
	Agent      string `json:"agent"`
	Status     Status `json:"status"`
	TargetPath string `json:"target_path,omitempty"` // For symlinks, what they point to
}

// HealthIssue represents a configuration or discovery issue with a skill.
type HealthIssue struct {
	Type     string `json:"type"`     // e.g. "invalid_frontmatter", "broken_symlink", "duplicate_name"
	Severity string `json:"severity"` // "error" or "warning"
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
}

// AgentState describes LazySkills' compatibility view of a supported agent.
type AgentState struct {
	Name             string `json:"name"`
	Display          string `json:"display"`
	Supported        bool   `json:"supported"`
	Detected         bool   `json:"detected"`
	Universal        bool   `json:"universal"`
	SupportsGlobal   bool   `json:"supports_global"`
	ProjectDir       string `json:"project_dir"`
	GlobalDir        string `json:"global_dir,omitempty"`
	ProjectDirExists bool   `json:"project_dir_exists"`
	GlobalDirExists  bool   `json:"global_dir_exists,omitempty"`
}

// SkillVisibility explains whether one supported agent can see a skill.
type SkillVisibility struct {
	Agent   string `json:"agent"`
	Display string `json:"display"`
	Visible bool   `json:"visible"`
	Reason  string `json:"reason"`
	Path    string `json:"path,omitempty"`
	Status  Status `json:"status,omitempty"`
}

// AddHealthIssue appends a health issue unless an identical issue already exists.
func (sk *Skill) AddHealthIssue(issue HealthIssue) {
	for _, existing := range sk.HealthIssues {
		if existing.Type == issue.Type && existing.Severity == issue.Severity && existing.Message == issue.Message && existing.Path == issue.Path {
			return
		}
	}
	sk.HealthIssues = append(sk.HealthIssues, issue)
}

// LocalLockEntry is the lock entry in project-local skills-lock.json
type LocalLockEntry struct {
	Source       string `json:"source"`
	Ref          string `json:"ref,omitempty"`
	SourceType   string `json:"sourceType"`
	SkillPath    string `json:"skillPath,omitempty"`
	ComputedHash string `json:"computedHash"`
}

// LocalLockFile represents project-local skills-lock.json
type LocalLockFile struct {
	Version int                       `json:"version"`
	Skills  map[string]LocalLockEntry `json:"skills"`
}

// GlobalLockEntry is the lock entry in global .skill-lock.json
type GlobalLockEntry struct {
	Source          string `json:"source"`
	SourceType      string `json:"sourceType"`
	SourceURL       string `json:"sourceUrl"`
	Ref             string `json:"ref,omitempty"`
	SkillPath       string `json:"skillPath,omitempty"`
	SkillFolderHash string `json:"skillFolderHash"`
	InstalledAt     string `json:"installedAt"`
	UpdatedAt       string `json:"updatedAt"`
	PluginName      string `json:"pluginName,omitempty"`
}

// GlobalLockFile represents global .skill-lock.json
type GlobalLockFile struct {
	Version int                        `json:"version"`
	Skills  map[string]GlobalLockEntry `json:"skills"`
}

// SkillBundle captures the project-scoped skills required to reproduce a team
// onboarding setup without depending on a package manager-specific manifest.
type SkillBundle struct {
	Version int              `json:"version"`
	Scope   Scope            `json:"scope"`
	Skills  []SkillBundleSkill `json:"skills"`
}

// SkillBundleSkill records the minimal bundle metadata needed to reinstall a
// skill while preserving its lock identity.
type SkillBundleSkill struct {
	Name         string                 `json:"name"`
	Source       string                 `json:"source"`
	Reference    string                 `json:"reference,omitempty"`
	SkillPath    string                 `json:"skill_path,omitempty"`
	Scope        Scope                  `json:"scope"`
	LockIdentity SkillBundleLockIdentity `json:"lock_identity"`
}

// SkillBundleLockIdentity mirrors the project lock identity fields so imports
// can compare exact source/ref/path matches before applying changes.
type SkillBundleLockIdentity struct {
	Source       string `json:"source,omitempty"`
	SourceType   string `json:"sourceType,omitempty"`
	Reference    string `json:"reference,omitempty"`
	SkillPath    string `json:"skillPath,omitempty"`
	ComputedHash string `json:"computedHash,omitempty"`
}

// Skill represents the consolidated view of a skill.
type Skill struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Scope         Scope             `json:"scope"`
	CanonicalPath string            `json:"canonical_path,omitempty"`
	SkillPath     string            `json:"skill_path,omitempty"`
	Preview       string            `json:"-"`
	ObservedPaths []ObservedPath    `json:"observed_paths"`
	Visibility    []SkillVisibility `json:"visibility,omitempty"`
	LocalLock     *LocalLockEntry   `json:"local_lock,omitempty"`
	GlobalLock    *GlobalLockEntry  `json:"global_lock,omitempty"`
	HealthIssues  []HealthIssue     `json:"health_issues"`
	Disabled      bool              `json:"disabled"`
}

// ToolStatus represents the location and status of a development tool.
type ToolStatus struct {
	Exists bool   `json:"exists"`
	Path   string `json:"path,omitempty"`
}

// Preflight details the health and status of critical CLI dependencies.
type Preflight struct {
	CanRunSkills bool                  `json:"can_run_skills"`
	Tools        map[string]ToolStatus `json:"tools"`
}

// ScanResult is the root schema produced by lazyskills scan --json.
type ScanResult struct {
	Cwd          string        `json:"cwd"`
	GlobalLock   string        `json:"global_lock_path,omitempty"`
	ProjectLock  string        `json:"project_lock_path,omitempty"`
	Agents       []AgentState  `json:"agents,omitempty"`
	Skills       []*Skill      `json:"skills"`
	HealthIssues []HealthIssue `json:"health_issues,omitempty"`
	Preflight    *Preflight    `json:"preflight,omitempty"`
}
