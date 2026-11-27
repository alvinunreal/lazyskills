package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvinunreal/lazyskills/internal/buildinfo"
)

type mockFetcher struct {
	release    *GitHubRelease
	releaseErr error
}

func (m *mockFetcher) FetchRelease(ctx context.Context, url string) (*GitHubRelease, error) {
	if m.releaseErr != nil {
		return nil, m.releaseErr
	}
	return m.release, nil
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"v1.0.0", "v1.0.0", 0},
		{"1.0.0", "v1.0.0", 0},
		{"v1.2.3", "v1.2.4", -1},
		{"v1.3.0", "v1.2.9", 1},
		{"v2.0.0", "v1.9.9", 1},
		{"v1.0.0-alpha", "v1.0.0", -1},
		{"v1.0.0", "v1.0.0-beta", 1},
		{"v1.0.0-alpha", "v1.0.0-beta", -1},
		{"v1.0.0-alpha.2", "v1.0.0-alpha.10", -1},
		{"v1.0.0-rc.1", "v1.0.0", -1},
		{"V1.0.0", "v1.0.0", 0},
		{"dev", "dev", 0},
		{"dev", "v1.0.0", 1},
	}

	for _, tt := range tests {
		got := CompareVersions(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d; want %d", tt.v1, tt.v2, got, tt.want)
		}
	}
}

func TestDetectChannel(t *testing.T) {
	// Temporarily override version
	oldVer := buildinfo.Version
	oldCommit := buildinfo.Commit
	defer func() {
		buildinfo.Version = oldVer
		buildinfo.Commit = oldCommit
	}()

	// 1. Dev build
	buildinfo.Version = "dev"
	ch, conf := DetectChannel("/usr/local/bin/lazyskills")
	if ch != "dev" || conf != "high" {
		t.Errorf("expected dev build, got channel=%s, conf=%s", ch, conf)
	}

	// 2. Normal build, brew path
	buildinfo.Version = "v1.0.0"
	buildinfo.Commit = "abcdef"
	ch, conf = DetectChannel("/opt/homebrew/Cellar/lazyskills/1.0.0/bin/lazyskills")
	if ch != "brew" || conf != "high" {
		t.Errorf("expected brew, got channel=%s, conf=%s", ch, conf)
	}

	ch, conf = DetectChannel("/usr/local/Caskroom/lazyskills/1.0.0/bin/lazyskills")
	if ch != "brew" || conf != "high" {
		t.Errorf("expected brew caskroom, got channel=%s, conf=%s", ch, conf)
	}

	// 3. Scoop path
	ch, conf = DetectChannel("C:/Users/name/scoop/apps/lazyskills/1.0.0/lazyskills.exe")
	if ch != "scoop" || conf != "high" {
		t.Errorf("expected scoop, got channel=%s, conf=%s", ch, conf)
	}

	// 4. WinGet path
	ch, conf = DetectChannel("C:/Users/name/AppData/Local/Microsoft/WinGet/Packages/lazyskills/lazyskills.exe")
	if ch != "winget" || conf != "high" {
		t.Errorf("expected winget, got channel=%s, conf=%s", ch, conf)
	}

	// 5. Go path
	ch, conf = DetectChannel("/home/user/go/bin/lazyskills")
	if ch != "go" || conf != "high" {
		t.Errorf("expected go, got channel=%s, conf=%s", ch, conf)
	}
}

func TestPlanAndCacheTTL(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)
	t.Setenv("HOME", cacheHome) // os.UserCacheDir uses $HOME/Library/Caches on macOS
	oldVer := buildinfo.Version
	oldCommit := buildinfo.Commit
	buildinfo.Version = "v1.0.0"
	buildinfo.Commit = "abcdef"
	defer func() {
		buildinfo.Version = oldVer
		buildinfo.Commit = oldCommit
	}()

	// Mock release info
	mockRel := &GitHubRelease{
		TagName: "v1.1.0",
		HTMLURL: "https://github.com/alvinunreal/lazyskills/releases/tag/v1.1.0",
		Body:    "New features!",
	}

	fetcher := &mockFetcher{release: mockRel}

	// Make sure we have a clean state for caching
	os.Setenv("LAZYSKILLS_NO_UPDATE_CHECK", "")
	cachePath, _ := getCachePath()
	_ = os.Remove(cachePath)

	ctx := context.Background()

	// 1. First plan (uncached)
	plan, err := Plan(ctx, true, fetcher)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if plan.Status != StatusAvailable {
		t.Errorf("expected available update, got status=%s", plan.Status)
	}
	if plan.Latest != "v1.1.0" {
		t.Errorf("expected latest version v1.1.0, got %s", plan.Latest)
	}

	// 2. Cache should now exist and be valid
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		t.Error("cache file was not created")
	}

	// 3. Test TTL expiration by writing old cache date
	oldTime := time.Now().Add(-25 * time.Hour)
	cd := CacheData{
		LastChecked: oldTime,
		Release:     *mockRel,
	}
	data, err := json.Marshal(cd)
	if err != nil {
		t.Fatalf("failed to marshal cache data: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		t.Fatalf("failed to write cache file: %v", err)
	}

	// Fetcher with error, but if TTL cache is expired, Plan will try to fetch again and fail
	errFetcher := &mockFetcher{releaseErr: fmt.Errorf("network error")}
	planRes, err := Plan(ctx, false, errFetcher)
	if err == nil {
		cachedRel, readErr := readCache(24 * time.Hour)
		t.Errorf("expected error from live plan when cache expired, but got nil. cachePath: %s, readErr: %v, cachedRel: %+v, planRes: %+v", cachePath, readErr, cachedRel, planRes)
	}

	// 4. Test LAZYSKILLS_NO_UPDATE_CHECK env opt-out
	os.Setenv("LAZYSKILLS_NO_UPDATE_CHECK", "1")
	planEnv, err := Plan(ctx, false, fetcher)
	if err != nil {
		t.Fatalf("Plan with env fail: %v", err)
	}
	if planEnv.Status != StatusUnknown || planEnv.Reason != "Update checks disabled by LAZYSKILLS_NO_UPDATE_CHECK" {
		t.Errorf("expected unknown/disabled status, got status=%s, reason=%s", planEnv.Status, planEnv.Reason)
	}
	os.Setenv("LAZYSKILLS_NO_UPDATE_CHECK", "")
}

func TestWriteCacheReadCacheRoundTrip(t *testing.T) {
	cacheRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("HOME", cacheRoot) // os.UserCacheDir uses $HOME/Library/Caches on macOS

	rel := &GitHubRelease{
		TagName: "v1.2.3",
		HTMLURL: "https://github.com/alvinunreal/lazyskills/releases/tag/v1.2.3",
		Body:    "cache body",
	}
	if err := writeCache(rel); err != nil {
		t.Fatalf("writeCache: %v", err)
	}

	got, err := readCache(time.Hour)
	if err != nil {
		t.Fatalf("readCache: %v", err)
	}
	if got.TagName != rel.TagName || got.HTMLURL != rel.HTMLURL || got.Body != rel.Body {
		t.Fatalf("readCache = %+v; want %+v", got, rel)
	}

	cachePath, err := getCachePath()
	if err != nil {
		t.Fatalf("getCachePath: %v", err)
	}
	info, err := os.Stat(cachePath)
	if err != nil {
		t.Fatalf("stat cache file: %v", err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("expected new cache file mode 0644, got %v", info.Mode().Perm())
	}
	entries, err := os.ReadDir(filepath.Dir(cachePath))
	if err != nil {
		t.Fatalf("ReadDir cache dir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp-") {
			t.Fatalf("unexpected leftover temp file %q", entry.Name())
		}
	}
}
func TestRecoveryAdvice(t *testing.T) {
	tests := []struct {
		channel  string
		goos     string
		wantInst string
		wantCmd  string
	}{
		{"brew", "darwin", "To upgrade using Homebrew, run:", "brew upgrade --cask alvinunreal/tap/lazyskills"},
		{"brew", "linux", "To upgrade using Homebrew, run:", "brew upgrade --cask alvinunreal/tap/lazyskills"},
		{"scoop", "windows", "To upgrade using Scoop, run:", "scoop update lazyskills"},
		{"winget", "windows", "To upgrade using WinGet, run:", "winget upgrade --id alvinunreal.lazyskills"},
		{"deb", "linux", "To upgrade via apt, run:", "sudo apt update && sudo apt install --only-upgrade lazyskills"},
		{"rpm", "linux", "To upgrade via dnf, run:", "sudo dnf upgrade lazyskills"},
		{"go", "linux", "To rebuild from source, run:", "go install github.com/alvinunreal/lazyskills/cmd/lazyskills@latest"},
		{"dev", "linux", "To rebuild from source, run:", "go install github.com/alvinunreal/lazyskills/cmd/lazyskills@latest"},
		{"manual", "darwin", "To reinstall, run:", "curl -fsSL https://lazyskills.sh/install | sh"},
		{"manual", "linux", "To reinstall, run:", "curl -fsSL https://lazyskills.sh/install | sh"},
		{"manual", "windows", "To reinstall, run in PowerShell:", "irm https://lazyskills.sh/install.ps1 | iex"},
		{"windows", "windows", "To reinstall, run in PowerShell:", "irm https://lazyskills.sh/install.ps1 | iex"},
		{"unknown", "linux", "To reinstall, run:", "curl -fsSL https://lazyskills.sh/install | sh"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s-%s", tt.channel, tt.goos), func(t *testing.T) {
			gotInst, gotCmd := RecoveryAdvice(tt.channel, tt.goos)
			if gotInst != tt.wantInst || gotCmd != tt.wantCmd {
				t.Errorf("RecoveryAdvice(%q, %q) = (%q, %q); want (%q, %q)",
					tt.channel, tt.goos, gotInst, gotCmd, tt.wantInst, tt.wantCmd)
			}
		})
	}
}
