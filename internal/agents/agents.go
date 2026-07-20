package agents

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alvinunreal/lazyskills/internal/model"
)

type Agent struct {
	Name                  string
	Display               string
	ProjectDir            string
	GlobalDir             string
	SupportsGlobal        bool
	Universal             bool
	ShowInUniversalList   bool
	ShowInUniversalPrompt bool
	Detected              bool
}
type Env struct {
	Home       string
	Vars       map[string]string
	ExistsFunc func(string) bool
}

func DefaultEnv() Env {
	home, _ := os.UserHomeDir()
	return Env{Home: home, Vars: map[string]string{}, ExistsFunc: pathExists}
}
func pathExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
func (e Env) EnvValue(key string) string {
	if e.Vars != nil {
		if v, ok := e.Vars[key]; ok {
			return v
		}
	}
	return os.Getenv(key)
}
func (e Env) Exists(path string) bool {
	if e.ExistsFunc != nil {
		return e.ExistsFunc(path)
	}
	return pathExists(path)
}
func (e Env) ConfigHome() string {
	if v := e.EnvValue("XDG_CONFIG_HOME"); v != "" {
		return v
	}
	return filepath.Join(e.Home, ".config")
}
func (e Env) CodexHome() string {
	if v := e.EnvValue("CODEX_HOME"); v != "" {
		return v
	}
	return filepath.Join(e.Home, ".codex")
}
func (e Env) ClaudeHome() string {
	if v := e.EnvValue("CLAUDE_CONFIG_DIR"); v != "" {
		return v
	}
	return filepath.Join(e.Home, ".claude")
}
func (e Env) VibeHome() string {
	if v := e.EnvValue("VIBE_HOME"); v != "" {
		return v
	}
	return filepath.Join(e.Home, ".vibe")
}
func (e Env) HermesHome() string {
	if v := e.EnvValue("HERMES_HOME"); v != "" {
		return v
	}
	return filepath.Join(e.Home, ".hermes")
}
func (e Env) AutohandHome() string {
	if v := e.EnvValue("AUTOHAND_HOME"); v != "" {
		return v
	}
	return filepath.Join(e.Home, ".autohand")
}
func anyExists(e Env, paths ...string) bool {
	for _, p := range paths {
		if e.Exists(p) {
			return true
		}
	}
	return false
}
func openClawGlobalSkillsDir(e Env) string {
	if e.Exists(filepath.Join(e.Home, ".openclaw")) {
		return filepath.Join(e.Home, ".openclaw", "skills")
	}
	if e.Exists(filepath.Join(e.Home, ".clawdbot")) {
		return filepath.Join(e.Home, ".clawdbot", "skills")
	}
	if e.Exists(filepath.Join(e.Home, ".moltbot")) {
		return filepath.Join(e.Home, ".moltbot", "skills")
	}
	return filepath.Join(e.Home, ".openclaw", "skills")
}
func agentDetectRoot(globalDir string) string {
	dir := filepath.Clean(globalDir)
	for _, suffix := range []string{filepath.Join("agent", "skills"), filepath.Join("data", "skills"), "skills"} {
		if filepath.Base(dir) == filepath.Base(suffix) {
			dir = filepath.Dir(dir)
			if filepath.Base(suffix) == "skills" && filepath.Base(dir) == "agent" {
				dir = filepath.Dir(dir)
			}
			return dir
		}
	}
	return filepath.Dir(dir)
}
func RegistryWithEnv(e Env, cwd string) []Agent {
	agents := []Agent{
		{Name: `aider-desk`, Display: `AiderDesk`, ProjectDir: `.aider-desk/skills`, GlobalDir: filepath.Join(e.Home, `.aider-desk/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `amp`, Display: `Amp`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.ConfigHome(), `agents/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `antigravity`, Display: `Antigravity`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.gemini/antigravity/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `antigravity-cli`, Display: `Antigravity CLI`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.gemini/antigravity-cli/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `astrbot`, Display: `AstrBot`, ProjectDir: `data/skills`, GlobalDir: filepath.Join(e.Home, `.astrbot/data/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `autohand-code`, Display: `Autohand Code CLI`, ProjectDir: `.autohand/skills`, GlobalDir: filepath.Join(e.AutohandHome(), `skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `augment`, Display: `Augment`, ProjectDir: `.augment/skills`, GlobalDir: filepath.Join(e.Home, `.augment/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `bob`, Display: `IBM Bob`, ProjectDir: `.bob/skills`, GlobalDir: filepath.Join(e.Home, `.bob/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `claude-code`, Display: `Claude Code`, ProjectDir: `.claude/skills`, GlobalDir: filepath.Join(e.ClaudeHome(), `skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `openclaw`, Display: `OpenClaw`, ProjectDir: `skills`, GlobalDir: openClawGlobalSkillsDir(e), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `cline`, Display: `Cline`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.agents`, `skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `codearts-agent`, Display: `CodeArts Agent`, ProjectDir: `.codeartsdoer/skills`, GlobalDir: filepath.Join(e.Home, `.codeartsdoer/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `codebuddy`, Display: `CodeBuddy`, ProjectDir: `.codebuddy/skills`, GlobalDir: filepath.Join(e.Home, `.codebuddy/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `codemaker`, Display: `Codemaker`, ProjectDir: `.codemaker/skills`, GlobalDir: filepath.Join(e.Home, `.codemaker/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `codestudio`, Display: `Code Studio`, ProjectDir: `.codestudio/skills`, GlobalDir: filepath.Join(e.Home, `.codestudio/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `codex`, Display: `Codex`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.CodexHome(), `skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `command-code`, Display: `Command Code`, ProjectDir: `.commandcode/skills`, GlobalDir: filepath.Join(e.Home, `.commandcode/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `continue`, Display: `Continue`, ProjectDir: `.continue/skills`, GlobalDir: filepath.Join(e.Home, `.continue/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `cortex`, Display: `Cortex Code`, ProjectDir: `.cortex/skills`, GlobalDir: filepath.Join(e.Home, `.snowflake/cortex/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `crush`, Display: `Crush`, ProjectDir: `.crush/skills`, GlobalDir: filepath.Join(e.Home, `.config/crush/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `cursor`, Display: `Cursor`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.cursor/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `deepagents`, Display: `Deep Agents`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.deepagents/agent/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `devin`, Display: `Devin for Terminal`, ProjectDir: `.devin/skills`, GlobalDir: filepath.Join(e.ConfigHome(), `devin/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `dexto`, Display: `Dexto`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.agents/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: false},
		{Name: `droid`, Display: `Droid`, ProjectDir: `.factory/skills`, GlobalDir: filepath.Join(e.Home, `.factory/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `firebender`, Display: `Firebender`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.firebender/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: false},
		{Name: `forgecode`, Display: `ForgeCode`, ProjectDir: `.forge/skills`, GlobalDir: filepath.Join(e.Home, `.forge/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `gemini-cli`, Display: `Gemini CLI`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.gemini/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `github-copilot`, Display: `GitHub Copilot`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.copilot/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `goose`, Display: `Goose`, ProjectDir: `.goose/skills`, GlobalDir: filepath.Join(e.ConfigHome(), `goose/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `hermes-agent`, Display: `Hermes Agent`, ProjectDir: `.hermes/skills`, GlobalDir: filepath.Join(e.HermesHome(), `skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `inference-sh`, Display: `inference.sh`, ProjectDir: `.inferencesh/skills`, GlobalDir: filepath.Join(e.Home, `.inferencesh/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `jazz`, Display: `Jazz`, ProjectDir: `.jazz/skills`, GlobalDir: filepath.Join(e.Home, `.jazz/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `junie`, Display: `Junie`, ProjectDir: `.junie/skills`, GlobalDir: filepath.Join(e.Home, `.junie/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `iflow-cli`, Display: `iFlow CLI`, ProjectDir: `.iflow/skills`, GlobalDir: filepath.Join(e.Home, `.iflow/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `kilo`, Display: `Kilo Code`, ProjectDir: `.kilocode/skills`, GlobalDir: filepath.Join(e.Home, `.kilocode/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `kimi-code-cli`, Display: `Kimi Code CLI`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.agents/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `kiro-cli`, Display: `Kiro CLI`, ProjectDir: `.kiro/skills`, GlobalDir: filepath.Join(e.Home, `.kiro/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `kode`, Display: `Kode`, ProjectDir: `.kode/skills`, GlobalDir: filepath.Join(e.Home, `.kode/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `lingma`, Display: `Lingma`, ProjectDir: `.lingma/skills`, GlobalDir: filepath.Join(e.Home, `.lingma/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `loaf`, Display: `Loaf`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.agents/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: false},
		{Name: `mcpjam`, Display: `MCPJam`, ProjectDir: `.mcpjam/skills`, GlobalDir: filepath.Join(e.Home, `.mcpjam/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `mistral-vibe`, Display: `Mistral Vibe`, ProjectDir: `.vibe/skills`, GlobalDir: filepath.Join(e.VibeHome(), `skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `moxby`, Display: `Moxby`, ProjectDir: `.moxby/skills`, GlobalDir: filepath.Join(e.Home, `.moxby/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `mux`, Display: `Mux`, ProjectDir: `.mux/skills`, GlobalDir: filepath.Join(e.Home, `.mux/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `opencode`, Display: `OpenCode`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.ConfigHome(), `opencode/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `openhands`, Display: `OpenHands`, ProjectDir: `.openhands/skills`, GlobalDir: filepath.Join(e.Home, `.openhands/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `ona`, Display: `Ona`, ProjectDir: `.ona/skills`, GlobalDir: filepath.Join(e.Home, `.ona/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `pi`, Display: `Pi`, ProjectDir: `.pi/skills`, GlobalDir: filepath.Join(e.Home, `.pi/agent/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `qoder`, Display: `Qoder`, ProjectDir: `.qoder/skills`, GlobalDir: filepath.Join(e.Home, `.qoder/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `qoder-cn`, Display: `Qoder CN`, ProjectDir: `.qoder/skills`, GlobalDir: filepath.Join(e.Home, `.qoder-cn/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `qwen-code`, Display: `Qwen Code`, ProjectDir: `.qwen/skills`, GlobalDir: filepath.Join(e.Home, `.qwen/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `replit`, Display: `Replit`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.ConfigHome(), `agents/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: false, ShowInUniversalPrompt: true},
		{Name: `reasonix`, Display: `Reasonix`, ProjectDir: `.reasonix/skills`, GlobalDir: filepath.Join(e.Home, `.reasonix/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `rovodev`, Display: `Rovo Dev`, ProjectDir: `.rovodev/skills`, GlobalDir: filepath.Join(e.Home, `.rovodev/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `roo`, Display: `Roo Code`, ProjectDir: `.roo/skills`, GlobalDir: filepath.Join(e.Home, `.roo/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `tabnine-cli`, Display: `Tabnine CLI`, ProjectDir: `.tabnine/agent/skills`, GlobalDir: filepath.Join(e.Home, `.tabnine/agent/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `terramind`, Display: `Terramind`, ProjectDir: `.terramind/skills`, GlobalDir: filepath.Join(e.Home, `.terramind/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `tinycloud`, Display: `Tinycloud`, ProjectDir: `.tinycloud/skills`, GlobalDir: filepath.Join(e.Home, `.tinycloud/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `trae`, Display: `Trae`, ProjectDir: `.trae/skills`, GlobalDir: filepath.Join(e.Home, `.trae/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `trae-cn`, Display: `Trae CN`, ProjectDir: `.trae/skills`, GlobalDir: filepath.Join(e.Home, `.trae-cn/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `warp`, Display: `Warp`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.agents/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `windsurf`, Display: `Windsurf`, ProjectDir: `.windsurf/skills`, GlobalDir: filepath.Join(e.Home, `.codeium/windsurf/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `zed`, Display: `Zed`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.Home, `.agents/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `zencoder`, Display: `Zencoder`, ProjectDir: `.zencoder/skills`, GlobalDir: filepath.Join(e.Home, `.zencoder/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `zenflow`, Display: `Zenflow`, ProjectDir: `.zencoder/skills`, GlobalDir: filepath.Join(e.Home, `.zencoder/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `neovate`, Display: `Neovate`, ProjectDir: `.neovate/skills`, GlobalDir: filepath.Join(e.Home, `.neovate/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `pochi`, Display: `Pochi`, ProjectDir: `.pochi/skills`, GlobalDir: filepath.Join(e.Home, `.pochi/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `promptscript`, Display: `PromptScript`, ProjectDir: `.agents/skills`, GlobalDir: "", SupportsGlobal: false, Universal: true, ShowInUniversalList: true, ShowInUniversalPrompt: false},
		{Name: `adal`, Display: `AdaL`, ProjectDir: `.adal/skills`, GlobalDir: filepath.Join(e.Home, `.adal/skills`), SupportsGlobal: true, Universal: false, ShowInUniversalList: true, ShowInUniversalPrompt: true},
		{Name: `universal`, Display: `Universal`, ProjectDir: `.agents/skills`, GlobalDir: filepath.Join(e.ConfigHome(), `agents/skills`), SupportsGlobal: true, Universal: true, ShowInUniversalList: false, ShowInUniversalPrompt: true},
	}
	for i := range agents {
		agents[i].Detected = detectAgentInstalled(agents[i], e, cwd)
	}
	return agents
}

func detectAgentInstalled(agent Agent, e Env, cwd string) bool {
	switch agent.Name {
	case "amp":
		return e.Exists(filepath.Join(e.ConfigHome(), "amp"))
	case "cline":
		return e.Exists(filepath.Join(e.Home, ".cline"))
	case "dexto":
		return e.Exists(filepath.Join(e.Home, ".dexto"))
	case "kimi-code-cli":
		return anyExists(e, filepath.Join(e.Home, ".kimi-code"), filepath.Join(e.Home, ".kimi"))
	case "loaf":
		return e.Exists(filepath.Join(e.Home, ".loaf"))
	case "warp":
		return e.Exists(filepath.Join(e.Home, ".warp"))
	case "pi":
		return e.Exists(filepath.Join(e.Home, ".pi", "agent"))
	case "zed":
		paths := []string{filepath.Join(e.ConfigHome(), "zed")}
		if appData := e.EnvValue("APPDATA"); appData != "" {
			paths = append(paths, filepath.Join(appData, "Zed"))
		}
		if flatpakConfig := e.EnvValue("FLATPAK_XDG_CONFIG_HOME"); flatpakConfig != "" {
			paths = append(paths, filepath.Join(flatpakConfig, "zed"))
		}
		return anyExists(e, paths...)
	case "claude-code":
		return e.Exists(e.ClaudeHome())
	case "codex":
		return anyExists(e, e.CodexHome(), "/etc/codex")
	case "promptscript":
		return anyExists(e, filepath.Join(cwd, ".promptscript"), filepath.Join(cwd, "promptscript.yaml"))
	case "openclaw":
		return anyExists(e, filepath.Join(e.Home, ".openclaw"), filepath.Join(e.Home, ".clawdbot"), filepath.Join(e.Home, ".moltbot"))
	case "astrbot":
		return anyExists(e, filepath.Join(cwd, "data", "skills"), filepath.Join(e.Home, ".astrbot"))
	case "codebuddy":
		return anyExists(e, filepath.Join(cwd, ".codebuddy"), filepath.Join(e.Home, ".codebuddy"))
	case "continue":
		return anyExists(e, filepath.Join(cwd, ".continue"), filepath.Join(e.Home, ".continue"))
	case "jazz":
		return anyExists(e, filepath.Join(e.Home, ".jazz"), filepath.Join(cwd, ".jazz"))
	case "replit":
		return e.Exists(filepath.Join(cwd, ".replit"))
	case "universal":
		return false
	default:
		return e.Exists(agentDetectRoot(agent.GlobalDir))
	}
}
func DetectInstalled(cwd string) []Agent {
	out := []Agent{}
	for _, a := range RegistryWithEnv(DefaultEnv(), cwd) {
		if a.Detected {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

type Location struct {
	Root      string
	Scope     model.Scope
	AgentName string
	Canonical bool
	// SharedRoot reports whether Root is reached through a symlinked path
	// component, meaning the files under it are shared with whatever the
	// link resolves to (e.g. another agent's canonical skills directory).
	SharedRoot bool
	// SharedRootLink is the symlinked path component (logical path), for
	// diagnostic messages. Empty unless SharedRoot is true.
	SharedRootLink string
	// SharedRootTarget is the symlink's resolved (absolute, cleaned) target,
	// for diagnostic messages. Empty unless SharedRoot is true.
	SharedRootTarget string
}

func Locations(cwd string) []Location { return LocationsWithEnv(cwd, DefaultEnv()) }
func LocationsWithEnv(cwd string, e Env) []Location {
	var out []Location
	seen := map[string]bool{}
	globalRoots := map[string]bool{}
	homeIsCwd := filepath.Clean(cwd) == filepath.Clean(e.Home)
	sharedCache := map[string]sharedRootResult{}
	sharedFor := func(root, anchor string) sharedRootResult {
		key := root + "\x00" + anchor
		if result, ok := sharedCache[key]; ok {
			return result
		}
		link, target, shared := sharedRootInfo(root, anchor)
		result := sharedRootResult{link: link, target: target, shared: shared}
		sharedCache[key] = result
		return result
	}
	add := func(loc Location) {
		key := string(loc.Scope) + "\x00" + loc.AgentName + "\x00" + loc.Root
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, loc)
	}
	globalCanonical := filepath.Join(e.Home, ".agents", "skills")
	registry := RegistryWithEnv(e, cwd)
	for _, a := range registry {
		if a.Universal && a.SupportsGlobal {
			globalRoots[filepath.Clean(globalCanonical)] = true
		}
		if a.SupportsGlobal && a.GlobalDir != "" && a.GlobalDir != globalCanonical {
			globalRoots[filepath.Clean(a.GlobalDir)] = true
		}
	}
	for _, a := range registry {
		projectRoot := filepath.Join(cwd, filepath.FromSlash(a.ProjectDir))
		// Skip project locations that are, or live inside, a global skills root.
		// Otherwise running from within the global tree (e.g. inside
		// ~/.agents/skills/<bundle>) makes a cwd-relative project dir physically
		// coincide with a global bundle's contents and mislabels those skills as
		// project-scoped. homeIsCwd handles the cwd==home special case; this guard
		// covers the general "project root under a global root" overlap.
		if !homeIsCwd && !isUnderAnyGlobalRoot(filepath.Clean(projectRoot), globalRoots) {
			shared := sharedFor(projectRoot, cwd)
			add(Location{Root: projectRoot, Scope: model.ScopeProject, AgentName: a.Name, Canonical: a.Universal, SharedRoot: shared.shared, SharedRootLink: shared.link, SharedRootTarget: shared.target})
		}
		if a.Universal && a.SupportsGlobal {
			shared := sharedFor(globalCanonical, e.Home)
			add(Location{Root: globalCanonical, Scope: model.ScopeGlobal, AgentName: a.Name, Canonical: true, SharedRoot: shared.shared, SharedRootLink: shared.link, SharedRootTarget: shared.target})
		}
		if a.SupportsGlobal && a.GlobalDir != "" && a.GlobalDir != globalCanonical {
			shared := sharedFor(a.GlobalDir, e.Home)
			add(Location{Root: a.GlobalDir, Scope: model.ScopeGlobal, AgentName: a.Name, Canonical: false, SharedRoot: shared.shared, SharedRootLink: shared.link, SharedRootTarget: shared.target})
		}
	}
	return out
}

type sharedRootResult struct {
	link   string
	target string
	shared bool
}

// sharedRootInfo reports whether root is reached through a symlinked path
// component below anchor. anchor is cwd for project-scope locations and the
// user's home directory for global-scope locations.
//
// When root sits under anchor, each path component of root strictly below
// anchor is Lstat-ed in turn; the first symlinked component means everything
// under it (including root) is shared with wherever the link resolves to.
// A missing component means nothing below it exists either, so the walk
// stops there and reports not-shared rather than continuing.
//
// When root is NOT under anchor (e.g. CODEX_HOME pointing outside the home
// directory), only root itself is Lstat-ed and reported if it is a symlink.
// The walk deliberately does not continue up to the filesystem root in that
// case — doing so would false-positive on system-level symlinks such as
// macOS's /var -> /private/var.
func sharedRootInfo(root, anchor string) (link, target string, shared bool) {
	root = filepath.Clean(root)
	anchor = filepath.Clean(anchor)
	if root == anchor {
		return "", "", false
	}
	prefix := anchor + string(filepath.Separator)
	if !strings.HasPrefix(root, prefix) {
		return symlinkAt(root)
	}
	rel := strings.TrimPrefix(root, prefix)
	current := anchor
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return "", "", false
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return symlinkAt(current)
		}
	}
	return "", "", false
}

// symlinkAt reports whether path itself is a symlink, returning path as the
// link and its resolved, cleaned target when so.
func symlinkAt(path string) (link, target string, shared bool) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return "", "", false
	}
	raw, err := os.Readlink(path)
	if err != nil {
		return "", "", false
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(filepath.Dir(path), raw)
	}
	return path, filepath.Clean(raw), true
}

// isUnderAnyGlobalRoot reports whether path is equal to, or nested inside, any
// of the global skills roots. Matching is path-boundary aware so that
// ~/.agents/skills-foo is not treated as inside ~/.agents/skills.
func isUnderAnyGlobalRoot(path string, roots map[string]bool) bool {
	for r := range roots {
		r = filepath.Clean(r)
		if path == r {
			return true
		}
		if strings.HasPrefix(path, r+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
