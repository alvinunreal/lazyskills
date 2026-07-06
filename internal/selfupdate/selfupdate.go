package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/alvinunreal/lazyskills/internal/buildinfo"
)

type UpdateStatus string

const (
	StatusAlreadyLatest UpdateStatus = "already_latest"
	StatusAvailable     UpdateStatus = "available"
	StatusError         UpdateStatus = "error"
	StatusUnknown       UpdateStatus = "unknown"
)

type UpdatePlan struct {
	Current         string       `json:"current"`
	Latest          string       `json:"latest"`
	Status          UpdateStatus `json:"status"`
	Channel         string       `json:"channel"`
	Confidence      string       `json:"confidence"`
	ExecutablePath  string       `json:"executable_path"`
	CommandPreview  string       `json:"command_preview"`
	CanExecute      bool         `json:"can_execute"`
	Reason          string       `json:"reason"`
	RestartRequired bool         `json:"restart_required"`
	ReleaseNotes    string       `json:"release_notes"`
	ReleaseURL      string       `json:"release_url"`
	ArchiveURL      string       `json:"archive_url"`
	ChecksumsURL    string       `json:"checksums_url"`
}

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Body    string `json:"body"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

var DefaultPlanFetcher Fetcher

type Fetcher interface {
	FetchRelease(ctx context.Context, url string) (*GitHubRelease, error)
	Download(ctx context.Context, url string) ([]byte, error)
}

type DefaultFetcher struct {
	Timeout time.Duration
}

func (f *DefaultFetcher) FetchRelease(ctx context.Context, url string) (*GitHubRelease, error) {
	client := &http.Client{Timeout: f.Timeout}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "lazyskills-updater")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}
	var rel GitHubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func (f *DefaultFetcher) Download(ctx context.Context, url string) ([]byte, error) {
	client := &http.Client{Timeout: f.Timeout}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "lazyskills-updater")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download from %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

type CacheData struct {
	LastChecked time.Time     `json:"last_checked"`
	Release     GitHubRelease `json:"release"`
}

func getCachePath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "lazyskills", "update-check.json"), nil
}

func readCache(ttl time.Duration) (*GitHubRelease, error) {
	path, err := getCachePath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cd CacheData
	if err := json.NewDecoder(f).Decode(&cd); err != nil {
		return nil, err
	}
	if time.Since(cd.LastChecked) > ttl {
		return nil, fmt.Errorf("cache expired")
	}
	return &cd.Release, nil
}

func writeCache(rel *GitHubRelease) error {
	path, err := getCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	cd := CacheData{
		LastChecked: time.Now(),
		Release:     *rel,
	}
	return json.NewEncoder(f).Encode(cd)
}

func CleanVersion(v string) string {
	v = strings.TrimSpace(v)
	if len(v) > 0 && (v[0] == 'v' || v[0] == 'V') {
		return v[1:]
	}
	return v
}

func CompareVersions(v1, v2 string) int {
	v1 = CleanVersion(v1)
	v2 = CleanVersion(v2)

	if v1 == v2 {
		return 0
	}

	parts1 := strings.SplitN(v1, "-", 2)
	parts2 := strings.SplitN(v2, "-", 2)

	main1 := parts1[0]
	main2 := parts2[0]

	nums1 := parseVersionComponents(main1)
	nums2 := parseVersionComponents(main2)

	for i := 0; i < len(nums1) || i < len(nums2); i++ {
		n1 := 0
		if i < len(nums1) {
			n1 = nums1[i]
		}
		n2 := 0
		if i < len(nums2) {
			n2 = nums2[i]
		}
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}

	if len(parts1) == 1 && len(parts2) > 1 {
		return 1
	}
	if len(parts1) > 1 && len(parts2) == 1 {
		return -1
	}
	if len(parts1) > 1 && len(parts2) > 1 {
		if parts1[1] < parts2[1] {
			return -1
		}
		if parts1[1] > parts2[1] {
			return 1
		}
	}
	return 0
}

func parseVersionComponents(v string) []int {
	parts := strings.Split(v, ".")
	res := make([]int, len(parts))
	for i, p := range parts {
		var n int
		fmt.Sscanf(p, "%d", &n)
		res[i] = n
	}
	return res
}

func isManagedByDpkg(path string) bool {
	if _, err := exec.LookPath("dpkg"); err != nil {
		return false
	}
	cmd := exec.Command("dpkg", "-S", path)
	err := cmd.Run()
	return err == nil
}

func isManagedByRpm(path string) bool {
	if _, err := exec.LookPath("rpm"); err != nil {
		return false
	}
	cmd := exec.Command("rpm", "-qf", path)
	err := cmd.Run()
	return err == nil
}

func isBrewPath(path string) bool {
	path = filepath.ToSlash(path)
	return strings.Contains(path, "/Cellar/") ||
		strings.Contains(path, "/Caskroom/") ||
		strings.Contains(path, "/homebrew/") ||
		strings.Contains(path, "/opt/homebrew/") ||
		strings.Contains(path, "/usr/local/Cellar/") ||
		strings.Contains(path, "/usr/local/Caskroom/")
}

func isScoopPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(path, "/scoop/apps/") || strings.Contains(path, "/scoop/shims/")
}

func isWinGetPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	return strings.Contains(path, "/winget/") || strings.Contains(path, "/local/microsoft/winget/")
}

func isGoPath(path string) bool {
	path = strings.ToLower(filepath.ToSlash(path))
	if strings.Contains(path, "/go/bin/") {
		return true
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		gopath = strings.ToLower(filepath.ToSlash(gopath))
		if strings.Contains(path, gopath) {
			return true
		}
	}
	return false
}

func isManualInstallSafe(path string) bool {
	path = filepath.Clean(filepath.ToSlash(path))
	if strings.HasPrefix(path, "/usr/bin/") ||
		strings.HasPrefix(path, "/usr/sbin/") ||
		strings.HasPrefix(path, "/bin/") ||
		strings.HasPrefix(path, "/sbin/") ||
		strings.HasPrefix(path, "/opt/") ||
		strings.HasPrefix(path, "/var/") {
		return false
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		home = filepath.Clean(filepath.ToSlash(home))
		if strings.HasPrefix(path, home+"/") {
			return true
		}
	}
	if strings.HasPrefix(path, "/usr/local/bin/") || strings.HasPrefix(path, "/usr/local/sbin/") {
		return true
	}
	tempDir := filepath.Clean(filepath.ToSlash(os.TempDir()))
	if strings.HasPrefix(path, tempDir+"/") {
		return true
	}
	if evalTempDir, err := filepath.EvalSymlinks(os.TempDir()); err == nil {
		evalTempDir = filepath.Clean(filepath.ToSlash(evalTempDir))
		if strings.HasPrefix(path, evalTempDir+"/") {
			return true
		}
	}
	if strings.HasPrefix(path, "/tmp/") {
		return true
	}
	return false
}

func DetectChannel(execPath string) (string, string) {
	evalPath, err := filepath.EvalSymlinks(execPath)
	if err == nil {
		execPath = evalPath
	}

	v := strings.TrimSpace(buildinfo.Version)
	if v == "dev" || v == "" || v == "(devel)" {
		return "dev", "high"
	}

	if isBrewPath(execPath) {
		return "brew", "high"
	}
	if isScoopPath(execPath) {
		return "scoop", "high"
	}
	if isWinGetPath(execPath) {
		return "winget", "high"
	}
	if isManagedByDpkg(execPath) {
		return "deb", "high"
	}
	if isManagedByRpm(execPath) {
		return "rpm", "high"
	}
	if isGoPath(execPath) {
		return "go", "high"
	}

	if runtime.GOOS == "windows" {
		return "windows", "low"
	}
	return "manual", "high"
}

func Plan(ctx context.Context, forceLive bool, fetcher Fetcher) (*UpdatePlan, error) {
	current := buildinfo.Version

	execPath, err := os.Executable()
	if err != nil {
		execPath = "lazyskills"
	}
	resolvedPath, err := filepath.EvalSymlinks(execPath)
	if err == nil {
		execPath = resolvedPath
	}

	channel, confidence := DetectChannel(execPath)

	plan := &UpdatePlan{
		Current:        current,
		Channel:        channel,
		Confidence:     confidence,
		ExecutablePath: execPath,
		Status:         StatusUnknown,
	}

	if os.Getenv("LAZYSKILLS_NO_UPDATE_CHECK") != "" {
		plan.Reason = "Update checks disabled by LAZYSKILLS_NO_UPDATE_CHECK"
		return plan, nil
	}

	var release *GitHubRelease
	if !forceLive {
		if cached, err := readCache(24 * time.Hour); err == nil {
			release = cached
		}
	}

	if release == nil {
		if fetcher == nil {
			if DefaultPlanFetcher != nil {
				fetcher = DefaultPlanFetcher
			} else {
				fetcher = &DefaultFetcher{Timeout: 10 * time.Second}
			}
		}
		url := "https://api.github.com/repos/alvinunreal/lazyskills/releases/latest"
		rel, err := fetcher.FetchRelease(ctx, url)
		if err != nil {
			plan.Status = StatusError
			plan.Reason = fmt.Sprintf("Failed to query latest release: %v", err)
			return plan, err
		}
		release = rel
		_ = writeCache(rel)
	}

	plan.Latest = release.TagName
	plan.ReleaseNotes = release.Body
	plan.ReleaseURL = release.HTMLURL

	osName := strings.Title(runtime.GOOS)
	archName := runtime.GOARCH
	if archName == "amd64" {
		archName = "x86_64"
	}
	archiveName := fmt.Sprintf("lazyskills_%s_%s.tar.gz", osName, archName)
	if runtime.GOOS == "windows" {
		archiveName = fmt.Sprintf("lazyskills_%s_%s.zip", osName, archName)
	}

	for _, asset := range release.Assets {
		if asset.Name == archiveName {
			plan.ArchiveURL = asset.BrowserDownloadURL
		} else if asset.Name == "checksums.txt" {
			plan.ChecksumsURL = asset.BrowserDownloadURL
		}
	}

	currClean := strings.TrimSpace(current)
	if currClean == "dev" || currClean == "" || currClean == "(devel)" {
		plan.Status = StatusAlreadyLatest
		plan.CanExecute = false
		plan.Reason = "Running a development build. Rebuild from source."
		plan.CommandPreview = "go install github.com/alvinunreal/lazyskills/cmd/lazyskills@latest"
		return plan, nil
	}

	cmp := CompareVersions(current, release.TagName)
	if cmp >= 0 {
		plan.Status = StatusAlreadyLatest
		plan.CanExecute = false
		plan.Reason = "You are already running the latest version."
		return plan, nil
	}

	plan.Status = StatusAvailable

	switch channel {
	case "brew":
		plan.CommandPreview = "brew upgrade --cask alvinunreal/tap/lazyskills"
		plan.Reason = "Homebrew managed install. Please upgrade using Homebrew."
		plan.CanExecute = false
	case "go":
		plan.CommandPreview = "go install github.com/alvinunreal/lazyskills/cmd/lazyskills@latest"
		plan.Reason = "Installed via Go. Please run go install to upgrade."
		plan.CanExecute = false
	case "scoop":
		plan.CommandPreview = "scoop update lazyskills"
		plan.Reason = "Scoop managed install. Please upgrade using Scoop."
		plan.CanExecute = false
	case "winget":
		plan.CommandPreview = "winget upgrade --id alvinunreal.lazyskills"
		plan.Reason = "WinGet managed install. Please upgrade using WinGet."
		plan.CanExecute = false
	case "deb":
		plan.CommandPreview = "sudo apt update && sudo apt install --only-upgrade lazyskills"
		plan.Reason = "Installed via DEB package. If a repository is configured, upgrade via apt. Otherwise, download the latest DEB from the releases page: https://github.com/alvinunreal/lazyskills/releases/latest"
		plan.CanExecute = false
	case "rpm":
		plan.CommandPreview = "sudo dnf upgrade lazyskills"
		plan.Reason = "Installed via RPM package. If a repository is configured, upgrade via dnf/yum. Otherwise, download the latest RPM from the releases page: https://github.com/alvinunreal/lazyskills/releases/latest"
		plan.CanExecute = false
	case "dev":
		plan.CommandPreview = "go install github.com/alvinunreal/lazyskills/cmd/lazyskills@latest"
		plan.Reason = "Running a development build. Rebuild from source."
		plan.CanExecute = false
	case "windows":
		plan.CommandPreview = ""
		plan.Reason = "To upgrade, please download the latest release from: " + release.HTMLURL
		plan.CanExecute = false
	default:
		if runtime.GOOS == "windows" {
			plan.CommandPreview = ""
			plan.Reason = "To upgrade, please download the latest release from: " + release.HTMLURL
			plan.CanExecute = false
		} else {
			if !isManualInstallSafe(execPath) {
				plan.CommandPreview = ""
				plan.Reason = "Update available, but automatic replacement is disabled because the executable path (" + execPath + ") is in a system directory. Please download the release manually from: " + release.HTMLURL
				plan.CanExecute = false
			} else {
				plan.CommandPreview = "lazyskills update --yes"
				plan.Reason = "Manual binary install. Can update directly."
				plan.CanExecute = true
			}
		}
	}

	return plan, nil
}

func Apply(ctx context.Context, plan *UpdatePlan, fetcher Fetcher) error {
	if !plan.CanExecute {
		return fmt.Errorf("update execution not supported for channel %q", plan.Channel)
	}

	if fetcher == nil {
		if DefaultPlanFetcher != nil {
			fetcher = DefaultPlanFetcher
		} else {
			fetcher = &DefaultFetcher{Timeout: 30 * time.Second}
		}
	}

	osName := strings.Title(runtime.GOOS)
	archName := runtime.GOARCH
	if archName == "amd64" {
		archName = "x86_64"
	}

	archiveName := fmt.Sprintf("lazyskills_%s_%s.tar.gz", osName, archName)

	archiveURL := plan.ArchiveURL
	checksumsURL := plan.ChecksumsURL

	if archiveURL == "" || checksumsURL == "" {
		url := "https://api.github.com/repos/alvinunreal/lazyskills/releases/latest"
		rel, err := fetcher.FetchRelease(ctx, url)
		if err != nil {
			return fmt.Errorf("failed to fetch release details for download: %v", err)
		}
		if rel.TagName != plan.Latest {
			return fmt.Errorf("release mismatch: plan latest version is %s, but fetched version was %s", plan.Latest, rel.TagName)
		}
		for _, asset := range rel.Assets {
			if asset.Name == archiveName {
				archiveURL = asset.BrowserDownloadURL
			} else if asset.Name == "checksums.txt" {
				checksumsURL = asset.BrowserDownloadURL
			}
		}
	}

	if archiveURL == "" {
		return fmt.Errorf("release archive asset %q not found", archiveName)
	}
	if checksumsURL == "" {
		return fmt.Errorf("release checksums asset %q not found", "checksums.txt")
	}

	checksumsBytes, err := fetcher.Download(ctx, checksumsURL)
	if err != nil {
		return fmt.Errorf("failed to download checksums: %v", err)
	}

	archiveBytes, err := fetcher.Download(ctx, archiveURL)
	if err != nil {
		return fmt.Errorf("failed to download archive: %v", err)
	}

	hash := sha256.Sum256(archiveBytes)
	hashStr := fmt.Sprintf("%x", hash)

	var expectedSHA string
	for _, line := range strings.Split(string(checksumsBytes), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == archiveName {
			expectedSHA = parts[0]
			break
		}
	}

	if expectedSHA == "" {
		return fmt.Errorf("checksum for %q not found in checksums.txt", archiveName)
	}

	if hashStr != expectedSHA {
		return fmt.Errorf("checksum mismatch: got %s, expected %s", hashStr, expectedSHA)
	}

	binaryName := "lazyskills"
	binaryBytes, err := extractBinaryFromTarGz(archiveBytes, binaryName)
	if err != nil {
		return fmt.Errorf("failed to extract binary from archive: %v", err)
	}

	targetPath := plan.ExecutablePath
	evalPath, err := filepath.EvalSymlinks(targetPath)
	if err == nil {
		targetPath = evalPath
	}

	if !isManualInstallSafe(targetPath) {
		return fmt.Errorf("target path %q is in an unsafe system directory for manual replacement", targetPath)
	}

	var targetMode os.FileMode = 0755
	info, statErr := os.Lstat(targetPath)
	if statErr == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("target path %q is not a regular file", targetPath)
		}
		targetMode = info.Mode()
	}

	execDir := filepath.Dir(targetPath)
	dirInfo, err := os.Stat(execDir)
	if err != nil {
		return fmt.Errorf("failed to access directory %q: %w", execDir, err)
	}
	if !dirInfo.IsDir() {
		return fmt.Errorf("target path parent %q is not a directory", execDir)
	}

	tempFile, err := os.CreateTemp(execDir, "lazyskills-update-")
	if err != nil {
		return fmt.Errorf("failed to create temp file (directory may not be writable): %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err := io.Copy(tempFile, bytes.NewReader(binaryBytes)); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write binary to temp file: %w", err)
	}
	tempFile.Close()

	if err := os.Chmod(tempPath, targetMode); err != nil {
		return fmt.Errorf("failed to chmod temp file: %w", err)
	}

	if err := os.Rename(tempPath, targetPath); err != nil {
		return fmt.Errorf("failed to replace executable: %w", err)
	}

	plan.RestartRequired = true
	return nil
}

func extractBinaryFromTarGz(tarGzBytes []byte, binaryName string) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(tarGzBytes))
	if err != nil {
		return nil, err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(header.Name) == binaryName && header.Typeflag == tar.TypeReg {
			var buf bytes.Buffer
			if _, err := io.Copy(&buf, tr); err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", binaryName)
}
