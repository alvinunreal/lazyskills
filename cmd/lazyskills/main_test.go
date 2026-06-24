package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/buildinfo"
	"github.com/alvinunreal/lazyskills/internal/registry"
	"github.com/alvinunreal/lazyskills/internal/selfupdate"
)

type mockFetcher struct {
	release    *selfupdate.GitHubRelease
	releaseErr error
}

func (m *mockFetcher) FetchRelease(ctx context.Context, url string) (*selfupdate.GitHubRelease, error) {
	if m.releaseErr != nil {
		return nil, m.releaseErr
	}
	return m.release, nil
}

func (m *mockFetcher) Download(ctx context.Context, url string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestCLIUpdate(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	oldOut := os.Stdout
	defer func() { os.Stdout = oldOut }()

	oldVer := buildinfo.Version
	oldCommit := buildinfo.Commit
	buildinfo.Version = "v1.0.0"
	buildinfo.Commit = "abcdef"
	defer func() {
		buildinfo.Version = oldVer
		buildinfo.Commit = oldCommit
	}()

	mockRel := &selfupdate.GitHubRelease{
		TagName: "v1.1.0",
		HTMLURL: "https://github.com/alvinunreal/lazyskills/releases/tag/v1.1.0",
		Body:    "Version 1.1.0 notes",
	}

	selfupdate.DefaultPlanFetcher = &mockFetcher{release: mockRel}
	defer func() { selfupdate.DefaultPlanFetcher = nil }()

	// Help function to capture stdout during run
	captureStdout := func(args []string) (string, error) {
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := run(args)

		w.Close()
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		os.Stdout = oldOut

		return buf.String(), err
	}

	// 1. Check only
	out, err := captureStdout([]string{"update", "--check"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Update available: v1.1.0") {
		t.Errorf("expected check output to mention Update available, got: %q", out)
	}

	// 2. Print command
	out, err = captureStdout([]string{"update", "--print-command"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Default channel should be "manual" on unix, or "windows" on windows.
	// Since we mock it, we just check if it returns one of the guidance/command values.
	if !strings.Contains(out, "lazyskills update") && !strings.Contains(out, "To upgrade, please download") {
		t.Errorf("expected command preview or download guidance, got: %q", out)
	}

	// 3. Default (no --yes)
	out, err = captureStdout([]string{"update"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Update available: v1.1.0") {
		t.Errorf("expected default output to mention Update available, got: %q", out)
	}

	// 4. --yes with CanExecute = false check (if we mock it to a managed channel like brew)
	// We can test this by changing the channel of the executable.
	// Let's verify already up-to-date behavior.
	buildinfo.Version = "v1.1.0"
	out, err = captureStdout([]string{"update"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Already up to date.") {
		t.Errorf("expected already up to date, got: %q", out)
	}

	// 5. Test LAZYSKILLS_NO_UPDATE_CHECK prints the disable reason instead of Already up to date
	os.Setenv("LAZYSKILLS_NO_UPDATE_CHECK", "1")
	defer os.Setenv("LAZYSKILLS_NO_UPDATE_CHECK", "")
	out, err = captureStdout([]string{"update"})
	if err != nil {
		t.Fatalf("unexpected error with env check: %v", err)
	}
	if !strings.Contains(out, "Update checks disabled by LAZYSKILLS_NO_UPDATE_CHECK") {
		t.Errorf("expected disabled reason, got: %q", out)
	}
	if strings.Contains(out, "Already up to date.") {
		t.Error("should not print Already up to date when update checks are disabled")
	}
}

func TestCLIFind(t *testing.T) {
	oldLookPath := actions.LookPath
	actions.LookPath = func(name string) (string, error) {
		return "/mock/bin/" + name, nil
	}
	defer func() { actions.LookPath = oldLookPath }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"skills": [
				{"id": "owner/one/skill-1", "skillId": "skill-1", "name": "Skill One", "installs": 10, "source": "github.com/one"}
				,{"id": "owner/two/skill-2", "skillId": "skill-2", "name": "Skill Two", "installs": 5, "source": "owner/\u001brepo"}
			]
		}`))
	}))
	defer server.Close()

	t.Setenv("SKILLS_API_URL", server.URL)

	oldOut := os.Stdout
	defer func() { os.Stdout = oldOut }()

	captureStdout := func(args []string) (string, error) {
		r, w, _ := os.Pipe()
		os.Stdout = w
		err := run(args)
		w.Close()
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		os.Stdout = oldOut
		return buf.String(), err
	}

	// 1. Missing json flag
	_, err := captureStdout([]string{"find", "test-query"})
	if err == nil || !strings.Contains(err.Error(), "usage: lazyskills find --json <query>") {
		t.Fatalf("expected usage error without --json, got: %v", err)
	}

	// 2. Missing query
	_, err = captureStdout([]string{"find", "--json"})
	if err == nil || !strings.Contains(err.Error(), "usage: lazyskills find --json <query>") {
		t.Fatalf("expected usage error without query, got: %v", err)
	}

	// 3. Multi query or too many args
	_, err = captureStdout([]string{"find", "--json", "q1", "q2"})
	if err == nil || !strings.Contains(err.Error(), "usage: lazyskills find --json <query>") {
		t.Fatalf("expected usage error with multiple queries, got: %v", err)
	}

	// 4. Success JSON
	out, err := captureStdout([]string{"find", "--json", "test-query"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check if output is stable indented JSON containing the query and skills
	var res struct {
		Query           string           `json:"query"`
		Results         []registry.Skill `json:"results"`
		InstallCommands []struct {
			Slug    string `json:"slug"`
			Source  string `json:"source"`
			Project []struct {
				Available bool   `json:"available"`
				Program   string `json:"program"`
				Command   string `json:"command"`
				Reason    string `json:"reason"`
			} `json:"project"`
			Global []struct {
				Available bool   `json:"available"`
				Command   string `json:"command"`
				Reason    string `json:"reason"`
			} `json:"global"`
		} `json:"install_commands"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput was:\n%s", err, out)
	}

	if res.Query != "test-query" {
		t.Errorf("expected query %q, got %q", "test-query", res.Query)
	}

	if len(res.Results) != 2 || res.Results[0].Slug != "skill-1" {
		t.Errorf("unexpected results: %+v", res.Results)
	}

	if len(res.InstallCommands) != 2 {
		t.Fatalf("expected install_commands for each result, got %+v", res.InstallCommands)
	}
	if len(res.InstallCommands[0].Project) == 0 || len(res.InstallCommands[0].Global) == 0 {
		t.Fatalf("expected project/global command previews for safe result, got %+v", res.InstallCommands[0])
	}
	if !res.InstallCommands[0].Project[0].Available || !strings.Contains(res.InstallCommands[0].Project[0].Command, "skills add github.com/one --skill skill-1 --yes") {
		t.Fatalf("unexpected safe project install preview: %+v", res.InstallCommands[0].Project[0])
	}
	if !strings.Contains(res.InstallCommands[0].Global[0].Command, "-g") {
		t.Fatalf("expected global install preview to include -g, got %+v", res.InstallCommands[0].Global[0])
	}
	if len(res.InstallCommands[1].Project) == 0 || res.InstallCommands[1].Project[0].Available {
		t.Fatalf("expected unsafe result to surface unavailable project preview, got %+v", res.InstallCommands[1])
	}
	if res.InstallCommands[1].Project[0].Reason == "" {
		t.Fatalf("expected unsafe result to carry a reason, got %+v", res.InstallCommands[1].Project[0])
	}
	if res.InstallCommands[1].Project[0].Program != "" || res.InstallCommands[1].Project[0].Command != "" {
		t.Fatalf("expected unsafe result not to expose executable preview fields, got %+v", res.InstallCommands[1].Project[0])
	}
}
