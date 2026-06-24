package tui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alvinunreal/lazyskills/internal/compat"
)

var radarCacheDir = os.UserCacheDir

type radarCacheFile struct {
	Sources map[string]radarCacheEntry `json:"sources"`
}

type radarCacheEntry struct {
	ScannedAt string            `json:"scanned_at"`
	Skills    []DiscoveredSkill `json:"skills"`
}

func radarCachePath(cwd string) (string, error) {
	base, err := radarCacheDir()
	if err != nil {
		return "", err
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(absCwd))
	return filepath.Join(base, "lazyskills", "radar", hex.EncodeToString(sum[:])+".json"), nil
}

func loadRadarCache(cwd string) (map[string]SourceDiscovery, error) {
	path, err := radarCachePath(cwd)
	if err != nil {
		return map[string]SourceDiscovery{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]SourceDiscovery{}, nil
		}
		return map[string]SourceDiscovery{}, err
	}

	var file radarCacheFile
	if err := json.Unmarshal(data, &file); err != nil {
		return map[string]SourceDiscovery{}, err
	}
	out := make(map[string]SourceDiscovery, len(file.Sources))
	for group, entry := range file.Sources {
		out[group] = SourceDiscovery{
			Status:    DiscoveryReady,
			Skills:    cloneDiscoveredSkills(entry.Skills),
			ScannedAt: mustParseRadarTimestamp(entry.ScannedAt),
		}
	}
	return out, nil
}

func saveRadarCache(cwd string, discoveries map[string]SourceDiscovery) error {
	path, err := radarCachePath(cwd)
	if err != nil {
		return err
	}
	file := radarCacheFile{Sources: map[string]radarCacheEntry{}}
	if data, readErr := os.ReadFile(path); readErr == nil {
		_ = json.Unmarshal(data, &file)
	} else if !os.IsNotExist(readErr) {
		return readErr
	}
	if file.Sources == nil {
		file.Sources = map[string]radarCacheEntry{}
	}
	for group, disc := range discoveries {
		if disc.Status != DiscoveryReady {
			continue
		}
		file.Sources[group] = radarCacheEntry{
			ScannedAt: disc.ScannedAt.Format(timeLayout()),
			Skills:    cloneDiscoveredSkills(disc.Skills),
		}
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func cloneDiscoveredSkills(skills []DiscoveredSkill) []DiscoveredSkill {
	if len(skills) == 0 {
		return nil
	}
	return append([]DiscoveredSkill(nil), skills...)
}

func partitionDiscoveredSkills(skills []DiscoveredSkill, installed map[string]bool) (available []DiscoveredSkill, newly []DiscoveredSkill) {
	for _, skill := range skills {
		if isSkillNameInstalled(skill.Name, installed) {
			continue
		}
		if skill.NewSinceLastScan {
			newly = append(newly, skill)
			continue
		}
		available = append(available, skill)
	}
	return available, newly
}

func classifyDiscoveredSkills(current, previous []DiscoveredSkill, installed map[string]bool, baselineEstablished bool) []DiscoveredSkill {
	prev := make(map[string]bool, len(previous))
	for _, skill := range previous {
		prev[radarSkillKey(skill)] = true
	}
	classified := make([]DiscoveredSkill, 0, len(current))
	for _, skill := range current {
		skill.NewSinceLastScan = baselineEstablished && !isSkillNameInstalled(skill.Name, installed) && !prev[radarSkillKey(skill)]
		classified = append(classified, skill)
	}
	sort.SliceStable(classified, func(i, j int) bool {
		left, right := strings.ToLower(classified[i].Name), strings.ToLower(classified[j].Name)
		if left == right {
			return strings.ToLower(classified[i].RelativePath) < strings.ToLower(classified[j].RelativePath)
		}
		return left < right
	})
	return classified
}

func radarSkillKey(skill DiscoveredSkill) string {
	if skill.RelativePath != "" {
		return strings.ToLower(filepath.ToSlash(skill.RelativePath))
	}
	if skill.SkillPath != "" {
		return strings.ToLower(filepath.ToSlash(skill.SkillPath))
	}
	return strings.ToLower(compat.NormalizeName(skill.Name))
}

func mustParseRadarTimestamp(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(timeLayout(), value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func timeLayout() string {
	return time.RFC3339Nano
}

func (m appModel) trackedSourceGroups() []string {
	seen := make(map[string]bool)
	groups := make([]string, 0)
	for _, skill := range m.result.Skills {
		group := sourceGroupLabel(skill)
		if group == "" || seen[group] {
			continue
		}
		seen[group] = true
		groups = append(groups, group)
	}
	sort.Strings(groups)
	return groups
}
