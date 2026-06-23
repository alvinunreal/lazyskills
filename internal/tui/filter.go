package tui

import (
	"sort"
	"strings"

	"github.com/alvinunreal/lazyskills/internal/agents"
	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/model"
)

func (m appModel) filteredSkills() []*model.Skill {
	query := strings.ToLower(m.search)
	out := make([]*model.Skill, 0, len(m.result.Skills))
	for _, sk := range m.result.Skills {
		if m.filter == scopeProject && sk.Scope != model.ScopeProject {
			continue
		}
		if m.filter == scopeGlobal && sk.Scope != model.ScopeGlobal {
			continue
		}
		if m.agent != "" && !skillRelevantToAgent(sk, m.agent) {
			continue
		}
		if query != "" {
			haystack := m.cachedSkillSearchText(sk)
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		out = append(out, sk)
	}
	return out
}

func sortSkills(skills []*model.Skill) {
	sort.SliceStable(skills, func(i, j int) bool {
		leftGroup := listGroupLabel(skills[i])
		rightGroup := listGroupLabel(skills[j])
		if leftGroup != rightGroup {
			return leftGroup < rightGroup
		}
		left := strings.ToLower(compat.SanitizeMetadata(skills[i].Name))
		right := strings.ToLower(compat.SanitizeMetadata(skills[j].Name))
		if left != right {
			return left < right
		}
		return string(skills[i].Scope) < string(skills[j].Scope)
	})
}

func (m *appModel) rebuildSkillSearchText() {
	if len(m.result.Skills) == 0 {
		m.skillSearchText = nil
		return
	}
	m.skillSearchText = make(map[*model.Skill]string, len(m.result.Skills))
	for _, sk := range m.result.Skills {
		m.skillSearchText[sk] = buildSkillSearchText(sk)
	}
}

func (m appModel) cachedSkillSearchText(sk *model.Skill) string {
	if m.skillSearchText != nil {
		if text, ok := m.skillSearchText[sk]; ok {
			return text
		}
	}
	return buildSkillSearchText(sk)
}

func buildSkillSearchText(sk *model.Skill) string {
	if sk == nil {
		return ""
	}
	return strings.ToLower(compat.SanitizeMetadata(sk.Name) + " " + compat.SanitizeMetadata(sk.Description))
}

func (m appModel) agentFilters() []string {
	var detected []string
	if len(m.result.Agents) == 0 {
		for _, agent := range agents.DetectInstalled(m.cwd) {
			if agent.Name == "universal" {
				continue
			}
			detected = append(detected, agent.Name)
		}
	} else {
		for _, agent := range m.result.Agents {
			if agent.Name == "universal" {
				continue
			}
			if agent.Detected {
				detected = append(detected, agent.Name)
			}
		}
	}
	sort.Strings(detected)
	return detected
}

func (m appModel) nextAgentFilter() string {
	agents := m.agentFilters()
	if len(agents) == 0 {
		return ""
	}
	if m.agent == "" {
		return agents[0]
	}
	for i, agent := range agents {
		if agent == m.agent {
			if i == len(agents)-1 {
				return ""
			}
			return agents[i+1]
		}
	}
	return ""
}

func skillObservedByAgent(sk *model.Skill, agent string) bool {
	for _, observed := range sk.ObservedPaths {
		if compat.SanitizeMetadata(observed.Agent) == agent {
			return true
		}
	}
	return false
}

func skillRelevantToAgent(sk *model.Skill, agent string) bool {
	if skillObservedByAgent(sk, agent) {
		return true
	}
	if sk.CanonicalPath == "" {
		return false
	}
	for _, visibility := range sk.Visibility {
		if visibility.Agent == agent {
			return true
		}
	}
	return false
}

func agentVisibilityBadge(sk *model.Skill, agent string) string {
	for _, visibility := range sk.Visibility {
		if visibility.Agent != agent {
			continue
		}
		if visibility.Visible {
			return successStyle.Render("✓")
		}
		return "×"
	}
	if skillObservedByAgent(sk, agent) {
		return successStyle.Render("✓")
	}
	return "×"
}

func (m appModel) agentLabel() string {
	if m.agent == "" {
		if len(m.agentFilters()) == 0 {
			return "all (none detected)"
		}
		return "all"
	}
	for _, agent := range m.result.Agents {
		if agent.Name == m.agent {
			return compat.SanitizeMetadata(agent.Display)
		}
	}
	return compat.SanitizeMetadata(m.agent)
}
