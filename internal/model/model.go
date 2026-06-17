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

// Skill represents the consolidated view of a skill.
type Skill struct {
	Name          string           `json:"name"`
	Description   string           `json:"description"`
	Scope         Scope            `json:"scope"`
	CanonicalPath string           `json:"canonical_path,omitempty"`
	SkillPath     string           `json:"skill_path,omitempty"`
	Preview       string           `json:"-"`
	ObservedPaths []ObservedPath   `json:"observed_paths"`
	LocalLock     *LocalLockEntry  `json:"local_lock,omitempty"`
	GlobalLock    *GlobalLockEntry `json:"global_lock,omitempty"`
	HealthIssues  []HealthIssue    `json:"health_issues"`
}

// ScanResult is the root schema produced by lazyskills scan --json.
type ScanResult struct {
	Cwd          string        `json:"cwd"`
	GlobalLock   string        `json:"global_lock_path,omitempty"`
	ProjectLock  string        `json:"project_lock_path,omitempty"`
	Skills       []*Skill      `json:"skills"`
	HealthIssues []HealthIssue `json:"health_issues,omitempty"`
}
