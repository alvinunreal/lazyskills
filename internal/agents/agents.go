package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
// component. anchor is cwd for project-scope locations and the user's home
// directory for global-scope locations.
//
// When root sits under a non-empty anchor, each path component of root
// strictly below anchor is Lstat-ed in turn; the first symlinked component
// means everything under it (including root) is shared with wherever the
// link resolves to. Restricting the walk to below anchor ignores system
// symlinks above the scope boundary when home itself lives under them.
//
// When root is NOT under anchor (e.g. CODEX_HOME=/alias pointing outside the
// home directory), every path component needed to reach root is walked from
// the volume root so ancestor symlinks such as /alias -> ~/.claude are
// detected even when /alias/skills itself is an ordinary directory. A normal
// non-symlink external root remains usable.
//
// On Darwin only, the exact volume-root OS aliases /var -> /private/var,
// /tmp -> /private/tmp, and /etc -> /private/etc are skipped while walking
// so macOS temp paths under /var/folders are not false-positive shared roots.
// No basename heuristic and no user-created link is trusted.
//
// A missing component after an earlier (non-skipped) symlink is never
// reached: the walk returns shared at the first such symlink. A missing
// component with no prior shared symlink means the path does not exist yet
// and is reported not-shared. Does not use EvalSymlinks.
func sharedRootInfo(root, anchor string) (link, target string, shared bool) {
	root = filepath.Clean(root)
	anchor = filepath.Clean(anchor)
	if root == "" || root == "." {
		return "", "", false
	}
	if root == anchor {
		return "", "", false
	}

	start := pathWalkStart(root)
	if pathInside(root, anchor) && anchor != "" && anchor != "." {
		// Restrict the walk to components strictly below anchor so system
		// symlinks above the scope boundary are not false-positives when the
		// anchor itself lives under them (e.g. home under /var/folders).
		start = anchor
	}
	if root == start {
		return symlinkAt(root)
	}

	rel, err := filepath.Rel(start, root)
	if err != nil || rel == "." || !pathInside(root, start) {
		// Fall back to walking every absolute component of root.
		return walkSymlinkComponents(root, pathWalkStart(root))
	}
	return walkSymlinkComponents(root, start)
}

// walkSymlinkComponents Lstats each path component from start down to root
// (exclusive of start, inclusive of root). Returns at the first symlink that
// is not a Darwin system path alias. Missing components (ErrNotExist) with
// no earlier shared symlink mean the path is not a live shared root. Any
// other inspection error fails closed as shared.
func walkSymlinkComponents(root, start string) (link, target string, shared bool) {
	root = filepath.Clean(root)
	start = filepath.Clean(start)
	if root == start {
		return symlinkAt(root)
	}
	rel, err := filepath.Rel(start, root)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", false
	}
	current := start
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				// Missing component with no earlier symlink: not a live shared root.
				return "", "", false
			}
			// Inspection error: fail closed.
			return current, "", true
		}
		if info.Mode()&os.ModeSymlink != 0 {
			// Reuse the Lstat result; only Readlink is needed for the target.
			link, target, shared = readlinkDetails(current)
			if shared && isDarwinSystemPathAlias(link, target) {
				// Exact Darwin OS prefix alias: keep walking.
				continue
			}
			return link, target, shared
		}
	}
	return "", "", false
}

// pathWalkStart returns the volume root used when walking absolute path
// components of an external custom root (e.g. "/" on Unix, "C:\\" on Windows).
func pathWalkStart(path string) string {
	path = filepath.Clean(path)
	vol := filepath.VolumeName(path)
	if vol != "" {
		// Windows volume root: "C:" + separator
		return vol + string(filepath.Separator)
	}
	if filepath.IsAbs(path) {
		return string(filepath.Separator)
	}
	// Relative path: walk from "." is not meaningful for shared-root
	// detection; callers typically pass absolute roots.
	return ""
}

// darwinSystemPathAliases is the exact Darwin volume-root symlink set that
// rewrites legacy paths to /private/... Without skipping these, every walk
// from "/" through /var/folders temp dirs false-positives as shared.
// Keys and values are filepath.Clean absolute paths only.
var darwinSystemPathAliases = map[string]string{
	"/var": "/private/var",
	"/tmp": "/private/tmp",
	"/etc": "/private/etc",
}

// isDarwinSystemPathAlias reports whether link -> target is one of the exact
// Darwin OS volume-root path aliases. Non-Darwin platforms always return
// false. Same-basename user links (e.g. /alias -> /somewhere/alias) and
// near-miss targets are never trusted.
func isDarwinSystemPathAlias(link, target string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	if link == "" || target == "" {
		return false
	}
	want, ok := darwinSystemPathAliases[filepath.Clean(link)]
	return ok && want == filepath.Clean(target)
}

// pathInside reports whether path is equal to root or a descendant of root
// using filepath-aware containment (not raw string-prefix matching), so
// siblings like root+"-foo" are not treated as inside root.
func pathInside(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if path == "" || root == "" {
		return false
	}
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// symlinkAt reports whether path itself is a symlink, returning path as the
// link and its resolved, cleaned target when so. Lstat/Readlink inspection
// errors on a path that appears to be a symlink fail closed as shared.
func symlinkAt(path string) (link, target string, shared bool) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", false
		}
		return path, "", true
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "", "", false
	}
	return readlinkDetails(path)
}

// readlinkDetails resolves a path already known (via Lstat) to be a symlink.
// Avoids a second Lstat; Readlink failures fail closed as shared.
func readlinkDetails(path string) (link, target string, shared bool) {
	raw, err := os.Readlink(path)
	if err != nil {
		return path, "", true
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(filepath.Dir(path), raw)
	}
	return path, filepath.Clean(raw), true
}

// CheckDestructivePath reports whether path is currently safe to mutate.
// Path must sit under a known agent location root. Live checks cover:
//  1. symlink ancestry of the matched scope root (below its anchor), and
//  2. every path component from that root through filepath.Dir(path)
//     (nested/shelf parents such as .lazyskills-disabled).
//
// The final skill entry leaf may itself be a symlink and is not rejected for
// that alone. Fail closed when the path's location is unknown or inspection
// errors. Do not use scan-time ObservedPath.SharedRoot alone at execution time.
func CheckDestructivePath(path, cwd string) error {
	return CheckDestructivePathWithEnv(path, cwd, DefaultEnv())
}

// CheckDestructivePathWithEnv is CheckDestructivePath with an explicit Env
// (for tests and callers that already have one).
func CheckDestructivePathWithEnv(path, cwd string, e Env) error {
	path = filepath.Clean(path)
	if path == "" || path == "." {
		return fmt.Errorf("path is empty or invalid")
	}
	locs := LocationsWithEnv(cwd, e)
	loc, ok := locationContainingPath(path, locs)
	if !ok {
		return fmt.Errorf("path is outside known skill locations; refusing mutation: %s", path)
	}
	root := filepath.Clean(loc.Root)
	anchor := e.Home
	if loc.Scope == model.ScopeProject {
		anchor = cwd
	}
	// Live, uncached ancestry walk for the matched scope root.
	_, _, shared := sharedRootInfo(root, anchor)
	if shared {
		return fmt.Errorf("path is reached through a symlinked skills root; refusing mutation: %s", path)
	}
	// Live-validate nested parents below the scope root through Dir(path),
	// excluding only the final skill entry. Catches a .lazyskills-disabled
	// (or other intermediate) component replaced by a symlink after scan.
	if err := checkNestedParentsSafe(path, root); err != nil {
		return err
	}
	return nil
}

// checkNestedParentsSafe walks symlink components strictly below scopeRoot
// through filepath.Dir(path). path itself (the skill entry) is not checked.
func checkNestedParentsSafe(path, scopeRoot string) error {
	path = filepath.Clean(path)
	scopeRoot = filepath.Clean(scopeRoot)
	if path == scopeRoot {
		return nil
	}
	if !pathInside(path, scopeRoot) {
		return fmt.Errorf("path is outside known skill locations; refusing mutation: %s", path)
	}
	parent := filepath.Dir(path)
	if parent == path || parent == scopeRoot {
		// Direct child of the scope root: no nested parent components.
		return nil
	}
	_, _, shared := walkSymlinkComponents(parent, scopeRoot)
	if shared {
		return fmt.Errorf("path is reached through a symlinked skills root; refusing mutation: %s", path)
	}
	return nil
}

// DestructiveSkillInstallPaths returns every install path a skills CLI
// remove/add for skillName would touch under current agent locations at the
// given scope. Includes empty/unobserved roots. agentFilter, when non-empty,
// restricts to those agent names; otherwise all agents at scope are included
// (matching skills remove without --agent, which targets every known agent).
// Fail closed when skillName is empty or no locations match.
func DestructiveSkillInstallPaths(cwd, skillName string, scope model.Scope, agentFilter []string) ([]string, error) {
	return DestructiveSkillInstallPathsWithEnv(cwd, skillName, scope, agentFilter, DefaultEnv())
}

// DestructiveSkillInstallPathsWithEnv is DestructiveSkillInstallPaths with an
// explicit Env.
func DestructiveSkillInstallPathsWithEnv(cwd, skillName string, scope model.Scope, agentFilter []string, e Env) ([]string, error) {
	skillName = strings.TrimSpace(skillName)
	if skillName == "" || skillName == "." || skillName == ".." {
		return nil, fmt.Errorf("skill identity is empty or invalid")
	}
	// Install identity is a single path base (skills CLI sanitizeName / basename).
	if filepath.Base(skillName) != skillName || strings.Contains(skillName, string(filepath.Separator)) {
		return nil, fmt.Errorf("skill identity must be a single path base")
	}

	filter := map[string]bool{}
	for _, a := range agentFilter {
		if a != "" {
			filter[a] = true
		}
	}
	locs := LocationsWithEnv(cwd, e)
	seen := map[string]bool{}
	var paths []string
	matchedLocs := 0
	for _, loc := range locs {
		if loc.Scope != scope {
			continue
		}
		if len(filter) > 0 && !filter[loc.AgentName] {
			continue
		}
		matchedLocs++
		p := filepath.Join(filepath.Clean(loc.Root), skillName)
		if seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	if matchedLocs == 0 {
		return nil, fmt.Errorf("no agent locations match scope %s for live shared-root validation", scope)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("unable to resolve skill install paths for live shared-root validation")
	}
	return paths, nil
}

// locationContainingPath returns the most specific (longest root) known
// location that contains path, if any.
func locationContainingPath(path string, locs []Location) (Location, bool) {
	path = filepath.Clean(path)
	var best Location
	found := false
	for _, loc := range locs {
		root := filepath.Clean(loc.Root)
		if !pathInside(path, root) {
			continue
		}
		if !found || len(root) > len(filepath.Clean(best.Root)) {
			best = loc
			found = true
		}
	}
	return best, found
}

// isUnderAnyGlobalRoot reports whether path is equal to, or nested inside, any
// of the global skills roots. Matching is path-boundary aware so that
// ~/.agents/skills-foo is not treated as inside ~/.agents/skills.
func isUnderAnyGlobalRoot(path string, roots map[string]bool) bool {
	for r := range roots {
		if pathInside(path, r) {
			return true
		}
	}
	return false
}
