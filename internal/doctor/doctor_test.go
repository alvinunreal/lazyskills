package doctor

import (
	"strings"
	"testing"

	"github.com/alvinunreal/lazyskills/internal/model"
)

func TestBuildPrioritizesAndSanitizesFindings(t *testing.T) {
	report := Build(model.ScanResult{
		Cwd: "/tmp/work\x1b[31m",
		HealthIssues: []model.HealthIssue{
			{Type: "corrupt_project_lock", Severity: "warning", Message: "bad\x1b[31m lock", Path: "/tmp/lock"},
		},
		Skills: []*model.Skill{
			{Name: "Zulu", Scope: model.ScopeGlobal, HealthIssues: []model.HealthIssue{{Type: "lock_without_files", Severity: "warning"}}},
			{Name: "Alpha", Scope: model.ScopeProject, HealthIssues: []model.HealthIssue{{Type: "broken_symlink", Severity: "error", Path: "/tmp/a"}}},
		},
	})

	if report.Summary.Errors != 1 || report.Summary.Warnings != 2 || report.Summary.Info != 0 {
		t.Fatalf("unexpected summary: %#v", report.Summary)
	}
	if len(report.Findings) != 3 {
		t.Fatalf("expected three findings, got %#v", report.Findings)
	}
	if report.Findings[0].Subject != "Alpha" || report.Findings[0].Severity != "error" {
		t.Fatalf("expected error first, got %#v", report.Findings)
	}
	if report.Findings[1].Subject != "Workspace" || report.Findings[2].Subject != "Zulu" {
		t.Fatalf("expected warnings sorted by subject, got %#v", report.Findings)
	}
	if !strings.Contains(report.Findings[2].Advice, "lazyskills restore --global 'Zulu'") {
		t.Fatalf("expected global restore advice, got %q", report.Findings[2].Advice)
	}
	if strings.Contains(report.Cwd, "\x1b") || strings.Contains(report.Findings[1].Message, "\x1b") {
		t.Fatalf("expected sanitized report: %#v", report)
	}
}

func TestBuildIncludesMissingCommandTools(t *testing.T) {
	report := Build(model.ScanResult{Preflight: &model.Preflight{CanRunSkills: false}})
	if len(report.Findings) != 1 || report.Findings[0].Type != "missing_skill_tools" || report.Findings[0].Severity != "error" {
		t.Fatalf("expected missing tools error, got %#v", report)
	}
	if !strings.Contains(report.Findings[0].Advice, "Node.js and npm") {
		t.Fatalf("unexpected missing tools advice: %q", report.Findings[0].Advice)
	}
}

func TestRestoreAdviceQuotesShellMetacharacters(t *testing.T) {
	report := Build(model.ScanResult{Skills: []*model.Skill{{
		Name:  "$(bad)'name",
		Scope: model.ScopeProject,
		HealthIssues: []model.HealthIssue{{
			Type: "lock_without_files", Severity: "warning",
		}},
	}}})
	if got, want := report.Findings[0].Advice, "lazyskills restore --project '$(bad)'\"'\"'name'"; got != "Restore it with: "+want {
		t.Fatalf("unexpected shell quoting: %q", got)
	}
}

func TestFormatTextForHealthyReport(t *testing.T) {
	out := FormatText(Build(model.ScanResult{Cwd: "/tmp/work"}))
	if !strings.Contains(out, "Healthy. No health issues found.") {
		t.Fatalf("unexpected healthy output: %q", out)
	}
}
