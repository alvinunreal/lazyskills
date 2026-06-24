package compat

import (
	"regexp"
	"strings"

	"github.com/alvinunreal/lazyskills/internal/model"
)

var unsafeNameChars = regexp.MustCompile(`[^a-z0-9._]+`)
var trimNameChars = regexp.MustCompile(`^[.\-]+|[.\-]+$`)

// SanitizeName mirrors vercel-labs/skills installer.ts sanitizeName.
func SanitizeName(name string) string {
	s := strings.ToLower(name)
	s = unsafeNameChars.ReplaceAllString(s, "-")
	s = trimNameChars.ReplaceAllString(s, "")
	if len(s) > 255 {
		s = s[:255]
	}
	if s == "" {
		return "unnamed-skill"
	}
	return s
}

func NormalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

var csiRE = regexp.MustCompile(`\x1b\[[\x30-\x3f]*[\x20-\x2f]*[\x40-\x7e]`)
var oscRE = regexp.MustCompile(`\x1b\][\s\S]*?(?:\x07|\x1b\\)`)
var dcsPMAPCRE = regexp.MustCompile(`\x1b[P^_][\s\S]*?(?:\x1b\\)`)
var simpleEscRE = regexp.MustCompile(`\x1b[\x20-\x7e]`)
var c1RE = regexp.MustCompile(`[\x80-\x9f]`)
var controlRE = regexp.MustCompile(`[\x00-\x06\x07\x08\x0b\x0c\x0d-\x1b\x1c-\x1f\x7f]`)
var newlineRE = regexp.MustCompile(`[\r\n]+`)

func StripTerminalEscapes(str string) string {
	str = oscRE.ReplaceAllString(str, "")
	str = dcsPMAPCRE.ReplaceAllString(str, "")
	str = csiRE.ReplaceAllString(str, "")
	str = simpleEscRE.ReplaceAllString(str, "")
	str = c1RE.ReplaceAllString(str, "")
	return controlRE.ReplaceAllString(str, "")
}

func SanitizeMetadata(str string) string {
	return strings.TrimSpace(newlineRE.ReplaceAllString(StripTerminalEscapes(str), " "))
}

// SanitizePreviewContent strips terminal controls while preserving markdown line breaks.
func SanitizePreviewContent(str string) string {
	return StripTerminalEscapes(str)
}

// FirstNonEmpty returns the first value that is not the empty string, or "" if none are set.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type LocalLockDisplay struct {
	Source     string `json:"source,omitempty"`
	Ref        string `json:"ref,omitempty"`
	SourceType string `json:"sourceType,omitempty"`
	SkillPath  string `json:"skillPath,omitempty"`
	ComputedHash string `json:"computedHash,omitempty"`
}

type GlobalLockDisplay struct {
	Source     string `json:"source,omitempty"`
	SourceType string `json:"sourceType,omitempty"`
	SourceURL  string `json:"sourceUrl,omitempty"`
	Ref        string `json:"ref,omitempty"`
	SkillPath  string `json:"skillPath,omitempty"`
	PluginName string `json:"pluginName,omitempty"`
	SkillFolderHash string `json:"skillFolderHash,omitempty"`
}

func SanitizeLocalLockDisplay(entry model.LocalLockEntry) LocalLockDisplay {
	return LocalLockDisplay{
		Source:     SanitizeMetadata(entry.Source),
		Ref:        SanitizeMetadata(entry.Ref),
		SourceType: SanitizeMetadata(entry.SourceType),
		SkillPath:  SanitizeMetadata(entry.SkillPath),
		ComputedHash: SanitizeMetadata(entry.ComputedHash),
	}
}

func SanitizeGlobalLockDisplay(entry model.GlobalLockEntry) GlobalLockDisplay {
	return GlobalLockDisplay{
		Source:     SanitizeMetadata(entry.Source),
		SourceType: SanitizeMetadata(entry.SourceType),
		SourceURL:  SanitizeMetadata(entry.SourceURL),
		Ref:        SanitizeMetadata(entry.Ref),
		SkillPath:  SanitizeMetadata(entry.SkillPath),
		PluginName: SanitizeMetadata(entry.PluginName),
		SkillFolderHash: SanitizeMetadata(entry.SkillFolderHash),
	}
}
