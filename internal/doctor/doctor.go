// Package doctor turns a scan snapshot into a deterministic, read-only health report.
package doctor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alvinunreal/lazyskills/internal/compat"
	"github.com/alvinunreal/lazyskills/internal/model"
)

type Summary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Info     int `json:"info"`
}

type Finding struct {
	Subject  string `json:"subject"`
	Scope    string `json:"scope,omitempty"`
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Message  string `json:"message"`
	Path     string `json:"path,omitempty"`
	Advice   string `json:"advice"`
}

type Report struct {
	Cwd      string    `json:"cwd"`
	Summary  Summary   `json:"summary"`
	Findings []Finding `json:"findings"`
}

// Build collects workspace and skill health issues into a report. It does not
// inspect or modify the filesystem; callers pass the scan snapshot they own.
func Build(result model.ScanResult) Report {
	report := Report{
		Cwd:      compat.SanitizeMetadata(result.Cwd),
		Findings: make([]Finding, 0, len(result.HealthIssues)),
	}
	if result.Preflight != nil && !result.Preflight.CanRunSkills {
		report.add("Workspace", "", "", model.HealthIssue{
			Type:     "missing_skill_tools",
			Severity: "error",
			Message:  "LazySkills cannot run skill commands because neither the skills CLI nor Node.js with npm and npx is available.",
		})
	}
	for _, issue := range result.HealthIssues {
		report.add("Workspace", "", "", issue)
	}
	for _, skill := range result.Skills {
		if skill == nil {
			continue
		}
		subject := compat.FirstNonEmpty(compat.SanitizeMetadata(skill.Name), "Unnamed skill")
		for _, issue := range skill.HealthIssues {
			report.add(subject, string(skill.Scope), skill.Name, issue)
		}
	}
	sort.SliceStable(report.Findings, func(i, j int) bool {
		left, right := report.Findings[i], report.Findings[j]
		if severityRank(left.Severity) != severityRank(right.Severity) {
			return severityRank(left.Severity) < severityRank(right.Severity)
		}
		if left.Subject != right.Subject {
			return left.Subject < right.Subject
		}
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		return left.Path < right.Path
	})
	for _, finding := range report.Findings {
		switch finding.Severity {
		case "error":
			report.Summary.Errors++
		case "warning":
			report.Summary.Warnings++
		default:
			report.Summary.Info++
		}
	}
	return report
}

func (r *Report) add(subject, scope, skillName string, issue model.HealthIssue) {
	severity := normalizedSeverity(issue.Severity)
	r.Findings = append(r.Findings, Finding{
		Subject:  compat.SanitizeMetadata(subject),
		Scope:    compat.SanitizeMetadata(scope),
		Type:     compat.SanitizeMetadata(issue.Type),
		Severity: severity,
		Title:    title(issue.Type),
		Message:  message(issue.Type, issue.Message),
		Path:     compat.SanitizeMetadata(issue.Path),
		Advice:   advice(issue.Type, scope, skillName),
	})
}

func normalizedSeverity(value string) string {
	switch strings.ToLower(compat.SanitizeMetadata(value)) {
	case "error", "critical":
		return "error"
	case "warning", "warn":
		return "warning"
	default:
		return "info"
	}
}

func severityRank(value string) int {
	switch value {
	case "error":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

func title(issueType string) string {
	switch issueType {
	case "missing_skill_md":
		return "Missing SKILL.md"
	case "invalid_frontmatter":
		return "Invalid frontmatter"
	case "broken_symlink":
		return "Broken symlink"
	case "missing_project_lock":
		return "Missing project lock entry"
	case "missing_global_lock":
		return "Missing global lock entry"
	case "lock_without_files":
		return "Locked skill is missing files"
	case "ghost_agent_skill":
		return "Agent-specific skill"
	case "duplicate_name":
		return "Duplicate skill name"
	case "project_global_shadowing":
		return "Project skill shadows global skill"
	case "corrupt_project_lock":
		return "Corrupt project lock file"
	case "corrupt_global_lock":
		return "Corrupt global lock file"
	case "shared_scope_root":
		return "Shared skills root"
	case "missing_skill_tools":
		return "Missing skill command tools"
	default:
		return strings.ReplaceAll(compat.SanitizeMetadata(issueType), "_", " ")
	}
}

func message(issueType, value string) string {
	if issueType == "ghost_agent_skill" {
		return "This skill is installed only for specific agents and is not tracked by the canonical skills directory."
	}
	return compat.SanitizeMetadata(value)
}

func advice(issueType, scope, skillName string) string {
	if issueType == "lock_without_files" && skillName != "" {
		flag := "--project"
		if scope == string(model.ScopeGlobal) {
			flag = "--global"
		}
		return fmt.Sprintf("Restore it with: lazyskills restore %s %s", flag, shellQuote(compat.SanitizeMetadata(skillName)))
	}
	switch issueType {
	case "broken_symlink":
		return "Review the broken link in LazySkills before removing it."
	case "missing_skill_md", "invalid_frontmatter":
		return "Repair the skill's SKILL.md file, then run lazyskills doctor again."
	case "missing_project_lock", "missing_global_lock":
		return "Reinstall or update the skill from its source to record it in the lock file."
	case "corrupt_project_lock", "corrupt_global_lock":
		return "Back up and repair the lock file before running installs or updates."
	case "duplicate_name", "project_global_shadowing":
		return "Choose one copy or rename a conflicting skill, then run lazyskills doctor again."
	case "shared_scope_root":
		return "This location is shared through a symlinked skills root. Repair it at the canonical source."
	case "ghost_agent_skill":
		return "Move or reinstall it through the canonical skills directory if it should be shared."
	case "missing_skill_tools":
		return "Install the skills CLI, or install Node.js and npm so LazySkills can use npx."
	default:
		return "Review this configuration and run lazyskills doctor again."
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func FormatText(report Report) string {
	var out strings.Builder
	out.WriteString("LazySkills doctor\n")
	if report.Cwd != "" {
		fmt.Fprintf(&out, "Workspace: %s\n", report.Cwd)
	}
	if len(report.Findings) == 0 {
		out.WriteString("\nHealthy. No health issues found.\n")
		return out.String()
	}
	fmt.Fprintf(&out, "\n%d error(s), %d warning(s), %d info item(s)\n", report.Summary.Errors, report.Summary.Warnings, report.Summary.Info)
	currentSeverity := ""
	for _, finding := range report.Findings {
		if finding.Severity != currentSeverity {
			currentSeverity = finding.Severity
			fmt.Fprintf(&out, "\n%s\n", sectionTitle(currentSeverity))
		}
		subject := finding.Subject
		if finding.Scope != "" {
			subject += " [" + finding.Scope + "]"
		}
		fmt.Fprintf(&out, "- %s: %s\n", subject, finding.Title)
		if finding.Message != "" {
			fmt.Fprintf(&out, "  %s\n", finding.Message)
		}
		if finding.Path != "" {
			fmt.Fprintf(&out, "  Path: %s\n", finding.Path)
		}
		fmt.Fprintf(&out, "  Next: %s\n", finding.Advice)
	}
	return out.String()
}

func sectionTitle(severity string) string {
	switch severity {
	case "error":
		return "Errors"
	case "warning":
		return "Warnings"
	default:
		return "Info"
	}
}
