package display

import (
	"fmt"
	"strings"

	"lazyskills/internal/compat"
	"lazyskills/internal/model"
)

type SkillView struct {
	Name          string
	Description   string
	Scope         string
	CanonicalPath string
	SkillPath     string
	Observed      []ObservedPathView
	Visibility    []VisibilityView
	LocalLock     *compat.LocalLockDisplay
	GlobalLock    *compat.GlobalLockDisplay
	HealthIssues  []HealthIssueView
	Preview       string
}

type ObservedPathView struct {
	Path       string
	Scope      string
	Agent      string
	Status     string
	TargetPath string
}

type HealthIssueView struct {
	Type     string
	Severity string
	Message  string
	Path     string
}

type VisibilityView struct {
	Agent   string
	Display string
	Visible bool
	Reason  string
	Path    string
	Status  string
}

func Skill(sk *model.Skill) SkillView {
	if sk == nil {
		return SkillView{}
	}
	view := SkillView{
		Name:          compat.SanitizeMetadata(sk.Name),
		Description:   compat.SanitizeMetadata(sk.Description),
		Scope:         compat.SanitizeMetadata(string(sk.Scope)),
		CanonicalPath: compat.SanitizeMetadata(sk.CanonicalPath),
		SkillPath:     compat.SanitizeMetadata(sk.SkillPath),
	}
	for _, p := range sk.ObservedPaths {
		view.Observed = append(view.Observed, ObservedPathView{
			Path:       compat.SanitizeMetadata(p.Path),
			Scope:      compat.SanitizeMetadata(string(p.Scope)),
			Agent:      compat.SanitizeMetadata(p.Agent),
			Status:     compat.SanitizeMetadata(string(p.Status)),
			TargetPath: compat.SanitizeMetadata(p.TargetPath),
		})
	}
	for _, visibility := range sk.Visibility {
		view.Visibility = append(view.Visibility, VisibilityView{
			Agent:   compat.SanitizeMetadata(visibility.Agent),
			Display: compat.SanitizeMetadata(visibility.Display),
			Visible: visibility.Visible,
			Reason:  compat.SanitizeMetadata(visibility.Reason),
			Path:    compat.SanitizeMetadata(visibility.Path),
			Status:  compat.SanitizeMetadata(string(visibility.Status)),
		})
	}
	if sk.LocalLock != nil {
		lock := compat.SanitizeLocalLockDisplay(*sk.LocalLock)
		view.LocalLock = &lock
	}
	if sk.GlobalLock != nil {
		lock := compat.SanitizeGlobalLockDisplay(*sk.GlobalLock)
		view.GlobalLock = &lock
	}
	for _, issue := range sk.HealthIssues {
		view.HealthIssues = append(view.HealthIssues, HealthIssueView{
			Type:     compat.SanitizeMetadata(issue.Type),
			Severity: compat.SanitizeMetadata(issue.Severity),
			Message:  compat.SanitizeMetadata(issue.Message),
			Path:     compat.SanitizeMetadata(issue.Path),
		})
	}
	view.Preview = compat.SanitizePreviewContent(sk.Preview)
	return view
}

func LockSummary(view SkillView) string {
	parts := []string{}
	if view.LocalLock != nil {
		parts = append(parts, fmt.Sprintf("project: %s", firstNonEmpty(view.LocalLock.Source, view.LocalLock.SkillPath, "tracked")))
	}
	if view.GlobalLock != nil {
		parts = append(parts, fmt.Sprintf("global: %s", firstNonEmpty(view.GlobalLock.Source, view.GlobalLock.SourceURL, view.GlobalLock.SkillPath, "tracked")))
	}
	if len(parts) == 0 {
		return "not tracked"
	}
	return strings.Join(parts, " | ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
