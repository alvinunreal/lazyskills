package frontmatter

import (
	"errors"
	"os"
	"regexp"

	"lazyskills/internal/compat"

	"gopkg.in/yaml.v3"
)

var frontmatterRE = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n?(.*)$`)

type SkillDoc struct {
	Name        string
	Description string
	Content     string
	Raw         string
}

func ParseFile(path string) (SkillDoc, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return SkillDoc{}, err
	}
	return Parse(string(b))
}

func Parse(raw string) (SkillDoc, error) {
	match := frontmatterRE.FindStringSubmatch(raw)
	if match == nil {
		return SkillDoc{Raw: raw, Content: raw}, errors.New("missing YAML frontmatter")
	}

	var data map[string]any
	if err := yaml.Unmarshal([]byte(match[1]), &data); err != nil {
		return SkillDoc{Raw: raw, Content: match[2]}, err
	}

	name, ok := data["name"].(string)
	if !ok || name == "" {
		return SkillDoc{Raw: raw, Content: match[2]}, errors.New("frontmatter name must be a string")
	}
	desc, ok := data["description"].(string)
	if !ok || desc == "" {
		return SkillDoc{Raw: raw, Content: match[2]}, errors.New("frontmatter description must be a string")
	}

	name = compat.SanitizeMetadata(name)
	desc = compat.SanitizeMetadata(desc)
	if name == "" || desc == "" {
		return SkillDoc{Raw: raw, Content: match[2]}, errors.New("frontmatter name and description must be non-empty after sanitization")
	}
	return SkillDoc{Name: name, Description: desc, Content: match[2], Raw: raw}, nil
}
