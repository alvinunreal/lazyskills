package main

import (
	"strings"
	"testing"

	"github.com/alvinunreal/lazyskills/internal/model"
	"github.com/alvinunreal/lazyskills/internal/runner"
)

func TestRestoreGlobalOnly(t *testing.T) {
	harness := useRestoreHarness(t, func(cwd string) (model.ScanResult, error) {
		return model.ScanResult{Cwd: cwd, Skills: []*model.Skill{
			missingRestoreSkill("Global Missing", model.ScopeGlobal),
			missingRestoreSkill("Project Missing", model.ScopeProject),
		}}, nil
	})

	out, err := captureRunStdout(t, []string{"restore", "--global", "--yes", "--cwd", t.TempDir()})
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if len(harness.calls) != 1 {
		t.Fatalf("expected one global restore command, got %#v", harness.calls)
	}
	if !containsRestoreArg(harness.calls[0].Args, "-g") || !containsRestoreArg(harness.calls[0].Args, "Global Missing") {
		t.Fatalf("unexpected global restore command: %#v", harness.calls[0])
	}
	if strings.Contains(strings.Join(harness.calls[0].Args, " "), "Project Missing") {
		t.Fatalf("project skill leaked into global restore: %#v", harness.calls[0])
	}
	if !strings.Contains(out, "Restore 1 missing skill") || !strings.Contains(out, "Global Missing") {
		t.Fatalf("expected restore preview, got %q", out)
	}
}

func TestRestoreConfirmationDefaultsToCancel(t *testing.T) {
	oldInput := restoreInput
	t.Cleanup(func() { restoreInput = oldInput })
	harness := useRestoreHarness(t, func(cwd string) (model.ScanResult, error) {
		return model.ScanResult{Cwd: cwd, Skills: []*model.Skill{missingRestoreSkill("Project Missing", model.ScopeProject)}}, nil
	})
	restoreInput = strings.NewReader("\n")

	out, err := captureRunStdout(t, []string{"restore", "--cwd", t.TempDir()})
	if err != nil {
		t.Fatalf("cancelled restore returned an error: %v", err)
	}
	if len(harness.calls) != 0 {
		t.Fatal("restore ran despite default confirmation cancellation")
	}
	if !strings.Contains(out, "Continue? [y/N]") || !strings.Contains(out, "Restore cancelled.") {
		t.Fatalf("expected confirmation and cancellation output, got %q", out)
	}
}

func TestRestoreSelectedProjectSkill(t *testing.T) {
	harness := useRestoreHarness(t, func(cwd string) (model.ScanResult, error) {
		return model.ScanResult{Cwd: cwd, Skills: []*model.Skill{
			missingRestoreSkill("Project One", model.ScopeProject),
			missingRestoreSkill("Project Two", model.ScopeProject),
			missingRestoreSkill("Project One", model.ScopeGlobal),
		}}, nil
	})

	out, err := captureRunStdout(t, []string{"restore", "--project", "--yes", "--cwd", t.TempDir(), "project one"})
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if len(harness.calls) != 1 || containsRestoreArg(harness.calls[0].Args, "-g") || !containsRestoreArg(harness.calls[0].Args, "Project One") {
		t.Fatalf("expected only selected project restore, got %#v", harness.calls)
	}
	if strings.Contains(out, "Project Two") || strings.Contains(out, "[global]") {
		t.Fatalf("unselected skill leaked into preview: %q", out)
	}
}

func TestRestoreAbortsWhenMissingStateChanges(t *testing.T) {
	scans := 0
	harness := useRestoreHarness(t, func(cwd string) (model.ScanResult, error) {
		scans++
		skill := missingRestoreSkill("Global Skill", model.ScopeGlobal)
		if scans == 1 {
		} else {
			skill.HealthIssues = nil
			skill.CanonicalPath = "/tmp/global-skill"
		}
		return model.ScanResult{Cwd: cwd, Skills: []*model.Skill{skill}}, nil
	})

	_, err := captureRunStdout(t, []string{"restore", "--global", "--yes", "--cwd", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "skill state changed") {
		t.Fatalf("expected changed-state error, got %v", err)
	}
	if len(harness.calls) != 0 {
		t.Fatal("restore ran after the skill was no longer missing")
	}
}

type restoreHarness struct {
	calls []runner.ExecSpec
}

func useRestoreHarness(t *testing.T, scanFn func(string) (model.ScanResult, error)) *restoreHarness {
	t.Helper()
	oldScan := restoreScan
	oldResolve := restoreResolveSkillsCommand
	oldRun := restoreRunExec
	t.Cleanup(func() {
		restoreScan = oldScan
		restoreResolveSkillsCommand = oldResolve
		restoreRunExec = oldRun
	})

	harness := &restoreHarness{}
	restoreScan = scanFn
	restoreResolveSkillsCommand = func() (string, []string) { return "skills", nil }
	restoreRunExec = func(spec runner.ExecSpec) runner.Result {
		harness.calls = append(harness.calls, spec)
		return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 0}
	}
	return harness
}

func missingRestoreSkill(name string, scope model.Scope) *model.Skill {
	skill := &model.Skill{Name: name, Scope: scope, HealthIssues: []model.HealthIssue{{Type: "lock_without_files"}}}
	if scope == model.ScopeGlobal {
		skill.GlobalLock = &model.GlobalLockEntry{Source: "owner/global"}
	} else {
		skill.LocalLock = &model.LocalLockEntry{Source: "owner/project"}
	}
	return skill
}

func containsRestoreArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
