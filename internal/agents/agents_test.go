package agents

import (
	"path/filepath"
	"testing"

	"github.com/alvinunreal/lazyskills/internal/model"
)

func testEnv(home string, existing ...string) Env {
	seen := map[string]bool{}
	for _, path := range existing {
		seen[path] = true
	}
	return Env{
		Home: home,
		Vars: map[string]string{
			"APPDATA":                 "",
			"AUTOHAND_HOME":           "",
			"CLAUDE_CONFIG_DIR":       "",
			"CODEX_HOME":              "",
			"FLATPAK_XDG_CONFIG_HOME": "",
			"HERMES_HOME":             "",
			"VIBE_HOME":               "",
			"XDG_CONFIG_HOME":         "",
		},
		ExistsFunc: func(path string) bool {
			return seen[path]
		},
	}
}

func agentByName(t *testing.T, list []Agent, name string) Agent {
	t.Helper()
	for _, agent := range list {
		if agent.Name == name {
			return agent
		}
	}
	t.Fatalf("agent %q not found", name)
	return Agent{}
}

func TestRegistryIncludesRepresentativeUpstreamAgents(t *testing.T) {
	registry := RegistryWithEnv(testEnv("/home/test"), "/repo")
	for _, name := range []string{"opencode", "claude-code", "codex", "promptscript", "zed"} {
		agentByName(t, registry, name)
	}
	if len(registry) < 70 {
		t.Fatalf("expected full upstream registry, got %d agents", len(registry))
	}
}

func TestPromptScriptDoesNotSupportGlobal(t *testing.T) {
	agent := agentByName(t, RegistryWithEnv(testEnv("/home/test"), "/repo"), "promptscript")
	if agent.SupportsGlobal {
		t.Fatalf("promptscript should not support global skills")
	}
	if agent.GlobalDir != "" {
		t.Fatalf("promptscript global dir should be empty, got %q", agent.GlobalDir)
	}
}

func TestUniversalClassificationAndVisibility(t *testing.T) {
	registry := RegistryWithEnv(testEnv("/home/test"), "/repo")
	if !agentByName(t, registry, "opencode").Universal {
		t.Fatalf("opencode should be universal")
	}
	if agentByName(t, registry, "claude-code").Universal {
		t.Fatalf("claude-code should not be universal")
	}
	if agentByName(t, registry, "replit").ShowInUniversalList {
		t.Fatalf("replit should be hidden from universal list")
	}
	if agentByName(t, registry, "firebender").ShowInUniversalPrompt {
		t.Fatalf("firebender should be hidden from universal prompt")
	}
}

func TestEnvOverridesForAgentHomes(t *testing.T) {
	env := testEnv("/home/test")
	env.Vars["CODEX_HOME"] = "/custom/codex"
	env.Vars["CLAUDE_CONFIG_DIR"] = "/custom/claude"
	registry := RegistryWithEnv(env, "/repo")
	if got := agentByName(t, registry, "codex").GlobalDir; got != filepath.Join("/custom/codex", "skills") {
		t.Fatalf("unexpected codex global dir: %q", got)
	}
	if got := agentByName(t, registry, "claude-code").GlobalDir; got != filepath.Join("/custom/claude", "skills") {
		t.Fatalf("unexpected claude global dir: %q", got)
	}
}

func TestOpenCodeUsesNativeProjectDirAndKeepsLegacyAlias(t *testing.T) {
	const home = "/home/test"
	const cwd = "/repo"
	agent := agentByName(t, RegistryWithEnv(testEnv(home), cwd), "opencode")
	if agent.ProjectDir != ".opencode/skills" {
		t.Fatalf("expected OpenCode native project dir, got %q", agent.ProjectDir)
	}
	if len(agent.LegacyProjectDirs) != 1 || agent.LegacyProjectDirs[0] != ".agents/skills" {
		t.Fatalf("expected OpenCode legacy alias to keep .agents/skills, got %#v", agent.LegacyProjectDirs)
	}
	if dirs := agent.ProjectDirs(); len(dirs) != 2 || dirs[0] != ".opencode/skills" || dirs[1] != ".agents/skills" {
		t.Fatalf("expected OpenCode project dirs to preserve native and legacy paths, got %#v", dirs)
	}

	locations := LocationsWithEnv(cwd, testEnv(home))
	projectRoots := map[string]bool{}
	canonicalRoots := map[string]bool{}
	for _, loc := range locations {
		if loc.AgentName == "opencode" && loc.Scope == model.ScopeProject {
			projectRoots[filepath.Clean(loc.Root)] = true
			if loc.Canonical {
				canonicalRoots[filepath.Clean(loc.Root)] = true
			}
		}
	}
	for _, root := range []string{filepath.Join(cwd, ".opencode", "skills"), filepath.Join(cwd, ".agents", "skills")} {
		if !projectRoots[root] {
			t.Fatalf("expected OpenCode project location %q to be registered, got %#v", root, projectRoots)
		}
		if !canonicalRoots[root] {
			t.Fatalf("expected OpenCode project location %q to be canonical, got %#v", root, canonicalRoots)
		}
	}
}

func TestOpenClawLegacyGlobalDirFallback(t *testing.T) {
	home := "/home/test"
	if got := agentByName(t, RegistryWithEnv(testEnv(home), "/repo"), "openclaw").GlobalDir; got != filepath.Join(home, ".openclaw", "skills") {
		t.Fatalf("expected openclaw fallback, got %q", got)
	}
	if got := agentByName(t, RegistryWithEnv(testEnv(home, filepath.Join(home, ".clawdbot")), "/repo"), "openclaw").GlobalDir; got != filepath.Join(home, ".clawdbot", "skills") {
		t.Fatalf("expected clawdbot legacy path, got %q", got)
	}
}

func TestDetectInstalledCwdBasedAgents(t *testing.T) {
	cwd := "/repo"
	env := testEnv("/home/test", filepath.Join(cwd, ".promptscript"), filepath.Join(cwd, ".replit"))
	registry := RegistryWithEnv(env, cwd)
	if !agentByName(t, registry, "promptscript").Detected {
		t.Fatalf("promptscript should be detected from cwd")
	}
	if !agentByName(t, registry, "replit").Detected {
		t.Fatalf("replit should be detected from cwd")
	}
}

func TestDetectorPathsMatchTrickyUpstreamAgents(t *testing.T) {
	home := "/home/test"
	cwd := "/repo"
	cases := []struct {
		name string
		path string
	}{
		{"amp", filepath.Join(home, ".config", "amp")},
		{"cline", filepath.Join(home, ".cline")},
		{"dexto", filepath.Join(home, ".dexto")},
		{"kimi-code-cli", filepath.Join(home, ".kimi-code")},
		{"loaf", filepath.Join(home, ".loaf")},
		{"warp", filepath.Join(home, ".warp")},
		{"pi", filepath.Join(home, ".pi", "agent")},
		{"claude-code", filepath.Join(home, ".claude")},
		{"codex", filepath.Join(home, ".codex")},
		{"promptscript", filepath.Join(cwd, "promptscript.yaml")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testEnv(home, tc.path)
			if !agentByName(t, RegistryWithEnv(env, cwd), tc.name).Detected {
				t.Fatalf("expected %s detected by %s", tc.name, tc.path)
			}
		})
	}
}

func TestZedDetectorUsesConfigAppDataAndFlatpak(t *testing.T) {
	home := "/home/test"
	cwd := "/repo"
	if agentByName(t, RegistryWithEnv(testEnv(home, "zed", "Zed"), cwd), "zed").Detected {
		t.Fatalf("zed should not be detected from relative zed/Zed paths when env vars are unset")
	}

	env := testEnv(home, filepath.Join("/appdata", "Zed"))
	env.Vars["APPDATA"] = "/appdata"
	if !agentByName(t, RegistryWithEnv(env, cwd), "zed").Detected {
		t.Fatalf("expected zed detected from APPDATA")
	}

	env = testEnv(home, filepath.Join("/flatpak", "zed"))
	env.Vars["FLATPAK_XDG_CONFIG_HOME"] = "/flatpak"
	if !agentByName(t, RegistryWithEnv(env, cwd), "zed").Detected {
		t.Fatalf("expected zed detected from Flatpak config")
	}
}

func TestLocationsDoNotTreatHomeGlobalDirsAsProjectDirs(t *testing.T) {
	home := "/home/test"
	locations := LocationsWithEnv(home, testEnv(home))

	for _, loc := range locations {
		if loc.Scope != "project" {
			continue
		}
		switch filepath.Clean(loc.Root) {
		case filepath.Join(home, ".agents", "skills"), filepath.Join(home, ".claude", "skills"), filepath.Join(home, ".codex", "skills"):
			t.Fatalf("home global dir should not be project location: %#v", loc)
		}
	}
}

func TestLocationsKeepProjectDirsOutsideHome(t *testing.T) {
	home := "/home/test"
	cwd := "/repo"
	locations := LocationsWithEnv(cwd, testEnv(home))

	want := filepath.Join(cwd, ".agents", "skills")
	for _, loc := range locations {
		if loc.Scope == "project" && loc.Root == want {
			return
		}
	}
	t.Fatalf("expected project location %q outside home", want)
}
