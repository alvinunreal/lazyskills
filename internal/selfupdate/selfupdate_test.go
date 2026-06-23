package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
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
	release      *GitHubRelease
	releaseErr   error
	downloadData map[string][]byte
	downloadErr  map[string]error
}

func (m *mockFetcher) FetchRelease(ctx context.Context, url string) (*GitHubRelease, error) {
	if m.releaseErr != nil {
		return nil, m.releaseErr
	}
	return m.release, nil
}

func (m *mockFetcher) Download(ctx context.Context, url string) ([]byte, error) {
	if m.downloadErr != nil {
		if err := m.downloadErr[url]; err != nil {
			return nil, err
		}
	}
	if data, ok := m.downloadData[url]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("mock download error for URL: %s", url)
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
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
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
	mockRel.Assets = append(mockRel.Assets, struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{
		Name:               "lazyskills_Linux_x86_64.tar.gz",
		BrowserDownloadURL: "https://github.com/downloads/archive.tar.gz",
	})

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

func TestApplyAndChecksumMismatch(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	// Create mock binary data and tar.gz
	var binBuf bytes.Buffer
	binBuf.WriteString("dummy binary content")
	binBytes := binBuf.Bytes()

	var tarBuf bytes.Buffer
	gw := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gw)
	hdr := &tar.Header{
		Name:     "lazyskills",
		Mode:     0755,
		Size:     int64(len(binBytes)),
		Typeflag: tar.TypeReg,
	}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write(binBytes)
	_ = tw.Close()
	_ = gw.Close()
	tarBytes := tarBuf.Bytes()

	correctHash := sha256.Sum256(tarBytes)
	correctHashStr := fmt.Sprintf("%x", correctHash)

	// Write temp target executable paths
	tmpDir, err := os.MkdirTemp("", "lazyskills-test-*")
	if err != nil {
		t.Fatalf("MkdirTemp failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	execPath := filepath.Join(tmpDir, "lazyskills")
	_ = os.WriteFile(execPath, []byte("old binary content"), 0755)

	plan := &UpdatePlan{
		Current:        "v1.0.0",
		Latest:         "v1.1.0",
		Status:         StatusAvailable,
		Channel:        "manual",
		ExecutablePath: execPath,
		CanExecute:     true,
	}

	// Helper URLs
	archiveURL := "https://github.com/downloads/lazyskills_linux_amd64.tar.gz" // not exact, mock matches anyway
	checksumsURL := "https://github.com/downloads/checksums.txt"

	// Mock release info with asset list matching our helper URLs
	mockRel := &GitHubRelease{
		TagName: "v1.1.0",
	}
	// We will determine our asset based on runtime OS/Arch
	osName := "Linux"
	archName := "x86_64"
	mockRel.Assets = append(mockRel.Assets, struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{
		Name:               fmt.Sprintf("lazyskills_%s_%s.tar.gz", osName, archName),
		BrowserDownloadURL: archiveURL,
	})
	mockRel.Assets = append(mockRel.Assets, struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	}{
		Name:               "checksums.txt",
		BrowserDownloadURL: checksumsURL,
	})

	// 1. Checksum mismatch test
	mismatchFetcher := &mockFetcher{
		release: mockRel,
		downloadData: map[string][]byte{
			checksumsURL: []byte(fmt.Sprintf("badhash1234567890abcdef  lazyskills_%s_%s.tar.gz\n", osName, archName)),
			archiveURL:   tarBytes,
		},
	}

	ctx := context.Background()
	err = Apply(ctx, plan, mismatchFetcher)
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("checksum mismatch")) {
		t.Errorf("expected checksum mismatch error, got %v", err)
	}

	// 2. Successful Apply test
	successFetcher := &mockFetcher{
		release: mockRel,
		downloadData: map[string][]byte{
			checksumsURL: []byte(fmt.Sprintf("%s  lazyskills_%s_%s.tar.gz\n", correctHashStr, osName, archName)),
			archiveURL:   tarBytes,
		},
	}

	err = Apply(ctx, plan, successFetcher)
	if err != nil {
		t.Fatalf("expected successful Apply, got err: %v", err)
	}

	if !plan.RestartRequired {
		t.Error("expected RestartRequired to be true")
	}

	newContent, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatalf("failed to read updated executable: %v", err)
	}
	if string(newContent) != "dummy binary content" {
		t.Errorf("binary content not updated properly: got %q", string(newContent))
	}

	// 3. Test TOCTOU tag mismatch (when ArchiveURL is empty and latest release tag has changed)
	mismatchTagFetcher := &mockFetcher{
		release: &GitHubRelease{TagName: "v1.2.0"}, // different from plan.Latest which is "v1.1.0"
	}
	planNoURLs := &UpdatePlan{
		Current:        "v1.0.0",
		Latest:         "v1.1.0",
		Status:         StatusAvailable,
		Channel:        "manual",
		ExecutablePath: execPath,
		CanExecute:     true,
	}
	err = Apply(ctx, planNoURLs, mismatchTagFetcher)
	if err == nil || !strings.Contains(err.Error(), "release mismatch") {
		t.Errorf("expected release mismatch error, got %v", err)
	}

	// 4. Test target path is not a regular file
	dirPath := filepath.Join(tmpDir, "some-dir-target")
	_ = os.Mkdir(dirPath, 0755)
	planDir := &UpdatePlan{
		Current:        "v1.0.0",
		Latest:         "v1.1.0",
		Status:         StatusAvailable,
		Channel:        "manual",
		ExecutablePath: dirPath,
		CanExecute:     true,
	}
	err = Apply(ctx, planDir, successFetcher)
	if err == nil || !strings.Contains(err.Error(), "is not a regular file") {
		t.Errorf("expected is not a regular file error, got %v", err)
	}

	// 5. Test symlink pointing to an unsafe target must not be executable/updatable
	baseUnsafeDir, err := os.MkdirTemp("/var/tmp", "lazyskills-test-*")
	if err == nil {
		defer os.RemoveAll(baseUnsafeDir)

		unsafeTarget := filepath.Join(baseUnsafeDir, "opt/vendor/lazyskills")
		_ = os.MkdirAll(filepath.Dir(unsafeTarget), 0755)
		_ = os.WriteFile(unsafeTarget, []byte("unsafe binary"), 0755)

		safeSymlink := filepath.Join(tmpDir, "usr/local/bin/lazyskills")
		_ = os.MkdirAll(filepath.Dir(safeSymlink), 0755)

		err = os.Symlink(unsafeTarget, safeSymlink)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(safeSymlink)
			if err != nil {
				t.Fatalf("failed to resolve symlink: %v", err)
			}
			if isManualInstallSafe(resolved) {
				t.Errorf("expected resolved path %q to be unsafe", resolved)
			}

			planUnsafeSymlink := &UpdatePlan{
				Current:        "v1.0.0",
				Latest:         "v1.1.0",
				Status:         StatusAvailable,
				Channel:        "manual",
				ExecutablePath: safeSymlink,
				CanExecute:     true,
			}
			err = Apply(ctx, planUnsafeSymlink, successFetcher)
			if err == nil || !strings.Contains(err.Error(), "unsafe system directory") {
				t.Errorf("expected unsafe system directory error for symlink target, got %v", err)
			}
		}
	} else {
		t.Logf("skipping /var/tmp symlink safety test because /var/tmp is not writeable or available: %v", err)
	}
}
