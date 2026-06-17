package agents

import (
	"path/filepath"
	"testing"
)

func testEnv(home string, existing ...string) Env {
	seen := map[string]bool{}
	for _, path := range existing {
		seen[path] = true
	}
	return Env{
		Home: home,
		Vars: map[string]string{},
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

	visible := VisibleUniversalAgents()
	for _, hidden := range []string{"dexto", "firebender", "loaf", "promptscript", "replit", "universal"} {
		for _, agent := range visible {
			if agent.Name == hidden {
				t.Fatalf("%s should not be in visible universal agents", hidden)
			}
		}
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
