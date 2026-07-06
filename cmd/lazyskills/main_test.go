package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/alvinunreal/lazyskills/internal/buildinfo"
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
	if !strings.Contains(out, "lazyskills update") &&
		!strings.Contains(out, "To upgrade, please download") &&
		!strings.Contains(out, "Please download the release manually") {
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
