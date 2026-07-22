package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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

func TestCLIUpdate(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

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

	// 1. Check only
	out, err := captureRunStdout(t, []string{"update", "--check"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Update available: v1.1.0") {
		t.Errorf("expected check output to mention Update available, got: %q", out)
	}

	// 2. Print command
	out, err = captureRunStdout(t, []string{"update", "--print-command"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "lazyskills update --yes") {
		t.Errorf("print-command must not advertise removed self-update command, got: %q", out)
	}
	// Default channel should be "manual" on unix, or "windows" on windows.
	// Since we mock it, we just check if it returns one of the guidance/command values.
	if !strings.Contains(out, "To upgrade, please download") && !strings.Contains(out, "lazyskills.sh/install") {
		t.Errorf("expected command preview or download guidance, got: %q", out)
	}

	// 3. Default (no --yes)
	out, err = captureRunStdout(t, []string{"update"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Update available: v1.1.0") {
		t.Errorf("expected default output to mention Update available, got: %q", out)
	}

	// 4. Test --yes returns non-zero error before calling Plan
	_, errYes := captureRunStdout(t, []string{"update", "--yes"})
	if errYes == nil {
		t.Fatal("expected error with --yes, got nil")
	}
	expectedErrMsg := "automatic updates have been removed; run lazyskills update for manual upgrade instructions"
	if !strings.Contains(errYes.Error(), expectedErrMsg) {
		t.Errorf("expected error message to contain %q, got: %v", expectedErrMsg, errYes)
	}

	// 5. Already up-to-date behavior.
	buildinfo.Version = "v1.1.0"
	out, err = captureRunStdout(t, []string{"update"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Already up to date.") {
		t.Errorf("expected already up to date, got: %q", out)
	}

	// 6. Test LAZYSKILLS_NO_UPDATE_CHECK prints the disable reason instead of Already up to date
	os.Setenv("LAZYSKILLS_NO_UPDATE_CHECK", "1")
	defer os.Setenv("LAZYSKILLS_NO_UPDATE_CHECK", "")
	out, err = captureRunStdout(t, []string{"update"})
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"skills": [
				{"id": "owner/one/skill-1", "skillId": "skill-1", "name": "Skill One", "installs": 10, "source": "github.com/one"}
			]
		}`))
	}))
	defer server.Close()

	t.Setenv("SKILLS_API_URL", server.URL)

	// 1. Missing json flag
	_, err := captureRunStdout(t, []string{"find", "test-query"})
	if err == nil || !strings.Contains(err.Error(), "usage: lazyskills find --json <query>") {
		t.Fatalf("expected usage error without --json, got: %v", err)
	}

	// 2. Missing query
	_, err = captureRunStdout(t, []string{"find", "--json"})
	if err == nil || !strings.Contains(err.Error(), "usage: lazyskills find --json <query>") {
		t.Fatalf("expected usage error without query, got: %v", err)
	}

	// 3. Multi query or too many args
	_, err = captureRunStdout(t, []string{"find", "--json", "q1", "q2"})
	if err == nil || !strings.Contains(err.Error(), "usage: lazyskills find --json <query>") {
		t.Fatalf("expected usage error with multiple queries, got: %v", err)
	}

	// 4. Success JSON
	out, err := captureRunStdout(t, []string{"find", "--json", "test-query"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check if output is stable indented JSON containing the query and skills
	var res struct {
		Query   string           `json:"query"`
		Results []registry.Skill `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nOutput was:\n%s", err, out)
	}

	if res.Query != "test-query" {
		t.Errorf("expected query %q, got %q", "test-query", res.Query)
	}

	if len(res.Results) != 1 || res.Results[0].Slug != "skill-1" {
		t.Errorf("unexpected results: %+v", res.Results)
	}
}

func captureRunStdout(t *testing.T, args []string) (string, error) {
	t.Helper()
	oldOut := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = oldOut }()

	runErr := run(args)
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String(), runErr
}
