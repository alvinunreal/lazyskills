package agents

import (
	"os"
	"path/filepath"

	"lazyskills/internal/model"
)

type Agent struct {
	Name       string
	Display    string
	ProjectDir string
	GlobalDir  string
	Universal  bool
}

func configHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config")
}

func homeJoin(parts ...string) string {
	home, _ := os.UserHomeDir()
	all := append([]string{home}, parts...)
	return filepath.Join(all...)
}

func InitialAgents() []Agent {
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = homeJoin(".codex")
	}
	claudeHome := os.Getenv("CLAUDE_CONFIG_DIR")
	if claudeHome == "" {
		claudeHome = homeJoin(".claude")
	}

	return []Agent{
		{Name: "universal", Display: "Universal", ProjectDir: ".agents/skills", GlobalDir: homeJoin(".agents", "skills"), Universal: true},
		{Name: "opencode", Display: "OpenCode", ProjectDir: ".agents/skills", GlobalDir: filepath.Join(configHome(), "opencode", "skills"), Universal: true},
		{Name: "claude-code", Display: "Claude Code", ProjectDir: ".claude/skills", GlobalDir: filepath.Join(claudeHome, "skills")},
		{Name: "cursor", Display: "Cursor", ProjectDir: ".agents/skills", GlobalDir: homeJoin(".cursor", "skills"), Universal: true},
		{Name: "codex", Display: "Codex", ProjectDir: ".agents/skills", GlobalDir: filepath.Join(codexHome, "skills"), Universal: true},
	}
}

type Location struct {
	Root      string
	Scope     model.Scope
	AgentName string
	Canonical bool
}

func Locations(cwd string) []Location {
	var out []Location
	globalCanonical := homeJoin(".agents", "skills")
	for _, a := range InitialAgents() {
		out = append(out, Location{
			Root:      filepath.Join(cwd, filepath.FromSlash(a.ProjectDir)),
			Scope:     model.ScopeProject,
			AgentName: a.Name,
			Canonical: a.ProjectDir == ".agents/skills",
		})
		if a.Universal {
			out = append(out, Location{Root: globalCanonical, Scope: model.ScopeGlobal, AgentName: a.Name, Canonical: true})
		}
		if a.GlobalDir != "" && a.GlobalDir != globalCanonical {
			out = append(out, Location{Root: a.GlobalDir, Scope: model.ScopeGlobal, AgentName: a.Name, Canonical: false})
		}
	}
	return out
}
