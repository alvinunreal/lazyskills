package main

import (
	"strings"
	"testing"

	"github.com/alvinunreal/lazyskills/internal/model"
	"github.com/alvinunreal/lazyskills/internal/runner"
)

func TestRestoreGlobalOnly(t *testing.T) {
	scans := 0
	harness := useRestoreHarness(t, func(cwd string) (model.ScanResult, error) {
		scans++
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
	// initial + post-confirm plan check + one pre-exec recheck
	if scans != 3 {
		t.Fatalf("expected 3 scans for one successful restore, got %d", scans)
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
	scans := 0
	harness := useRestoreHarness(t, func(cwd string) (model.ScanResult, error) {
		scans++
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
	// initial + post-confirm plan check + one pre-exec recheck
	if scans != 3 {
		t.Fatalf("expected 3 scans for one successful restore, got %d", scans)
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

func TestRestoreAbortsWhenInstalledAfterFinalScan(t *testing.T) {
	scans := 0
	harness := useRestoreHarness(t, func(cwd string) (model.ScanResult, error) {
		scans++
		skill := missingRestoreSkill("Global Skill", model.ScopeGlobal)
		// scans 1-2: initial + post-confirm check still missing; scan 3: pre-exec finds it installed
		if scans >= 3 {
			skill.HealthIssues = nil
			skill.CanonicalPath = "/tmp/global-skill"
		}
		return model.ScanResult{Cwd: cwd, Skills: []*model.Skill{skill}}, nil
	})

	_, err := captureRunStdout(t, []string{"restore", "--global", "--yes", "--cwd", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "no longer missing") {
		t.Fatalf("expected no-longer-missing error, got %v", err)
	}
	if len(harness.calls) != 0 {
		t.Fatal("restore ran after post-final-scan install")
	}
	if scans != 3 {
		t.Fatalf("expected 3 scans, got %d", scans)
	}
}

func TestRestoreAbortsWhenLockMetadataChangesOnPostConfirmScan(t *testing.T) {
	scans := 0
	harness := useRestoreHarness(t, func(cwd string) (model.ScanResult, error) {
		scans++
		skill := missingRestoreSkill("Global Skill", model.ScopeGlobal)
		// scan 2: after preview / before any exec — lock identity diverges from initial plan
		if scans >= 2 {
			skill.GlobalLock = &model.GlobalLockEntry{Source: "owner/changed"}
		}
		return model.ScanResult{Cwd: cwd, Skills: []*model.Skill{skill}}, nil
	})

	_, err := captureRunStdout(t, []string{"restore", "--global", "--yes", "--cwd", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "skill state changed") {
		t.Fatalf("expected state-changed error, got %v", err)
	}
	if len(harness.calls) != 0 {
		t.Fatal("restore ran after lock metadata changed on post-confirm scan")
	}
	if scans != 2 {
		t.Fatalf("expected abort after post-confirm scan only, got %d scans", scans)
	}
}

func TestRestoreAbortsWhenLockMetadataChangesBeforeExec(t *testing.T) {
	scans := 0
	harness := useRestoreHarness(t, func(cwd string) (model.ScanResult, error) {
		scans++
		skill := missingRestoreSkill("Global Skill", model.ScopeGlobal)
		if scans >= 3 {
			skill.GlobalLock = &model.GlobalLockEntry{Source: "owner/changed"}
		}
		return model.ScanResult{Cwd: cwd, Skills: []*model.Skill{skill}}, nil
	})

	_, err := captureRunStdout(t, []string{"restore", "--global", "--yes", "--cwd", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "lock metadata changed") {
		t.Fatalf("expected lock-metadata error, got %v", err)
	}
	if len(harness.calls) != 0 {
		t.Fatal("restore ran after lock metadata changed")
	}
	if scans != 3 {
		t.Fatalf("expected 3 scans, got %d", scans)
	}
}

func TestRestoreAbortsWhenOneOfMultipleInstalledOnPostConfirmScan(t *testing.T) {
	scans := 0
	harness := useRestoreHarness(t, func(cwd string) (model.ScanResult, error) {
		scans++
		first := missingRestoreSkill("First", model.ScopeGlobal)
		second := missingRestoreSkill("Second", model.ScopeGlobal)
		// scan 2: one planned candidate is already installed — must not exec the other
		if scans >= 2 {
			first.HealthIssues = nil
			first.CanonicalPath = "/tmp/first"
		}
		return model.ScanResult{Cwd: cwd, Skills: []*model.Skill{first, second}}, nil
	})

	_, err := captureRunStdout(t, []string{"restore", "--global", "--yes", "--cwd", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "skill state changed") {
		t.Fatalf("expected state-changed error, got %v", err)
	}
	if len(harness.calls) != 0 {
		t.Fatalf("expected no exec when post-confirm plan diverges, got %#v", harness.calls)
	}
	if scans != 2 {
		t.Fatalf("expected abort after post-confirm scan only, got %d scans", scans)
	}
}

func TestRestorePartialWhenLaterSkillInstalledAfterFirstSuccess(t *testing.T) {
	scans := 0
	harness := useRestoreHarness(t, func(cwd string) (model.ScanResult, error) {
		scans++
		first := missingRestoreSkill("First", model.ScopeGlobal)
		second := missingRestoreSkill("Second", model.ScopeGlobal)
		// After first success, pre-exec recheck for second (scan 4) sees it installed.
		if scans >= 4 {
			second.HealthIssues = nil
			second.CanonicalPath = "/tmp/second"
		}
		return model.ScanResult{Cwd: cwd, Skills: []*model.Skill{first, second}}, nil
	})

	_, err := captureRunStdout(t, []string{"restore", "--global", "--yes", "--cwd", t.TempDir()})
	if err == nil {
		t.Fatal("expected partial restore error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "partial restore") || !strings.Contains(msg, "[global] First") || !strings.Contains(msg, "no longer missing") {
		t.Fatalf("expected partial success naming First, got %v", err)
	}
	if len(harness.calls) != 1 || !containsRestoreArg(harness.calls[0].Args, "First") {
		t.Fatalf("expected only first restore exec, got %#v", harness.calls)
	}
	if scans != 4 {
		t.Fatalf("expected 4 scans, got %d", scans)
	}
}

func TestRestorePartialWhenExecutorFailsAfterFirstSuccess(t *testing.T) {
	scans := 0
	harness := useRestoreHarness(t, func(cwd string) (model.ScanResult, error) {
		scans++
		return model.ScanResult{Cwd: cwd, Skills: []*model.Skill{
			missingRestoreSkill("First", model.ScopeGlobal),
			missingRestoreSkill("Second", model.ScopeGlobal),
			missingRestoreSkill("Third", model.ScopeGlobal),
		}}, nil
	})
	oldRun := restoreRunExec
	t.Cleanup(func() { restoreRunExec = oldRun })
	restoreRunExec = func(spec runner.ExecSpec) runner.Result {
		harness.calls = append(harness.calls, spec)
		if len(harness.calls) >= 2 {
			return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 2, Err: "installer failed"}
		}
		return runner.Result{Program: spec.Program, Args: spec.Args, Cwd: spec.Cwd, ExitCode: 0}
	}

	_, err := captureRunStdout(t, []string{"restore", "--global", "--yes", "--cwd", t.TempDir()})
	if err == nil {
		t.Fatal("expected partial restore error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "partial restore") || !strings.Contains(msg, "[global] First") || !strings.Contains(msg, "[global] Second") || !strings.Contains(msg, "installer failed") {
		t.Fatalf("expected partial executor failure naming First and Second, got %v", err)
	}
	if len(harness.calls) != 2 {
		t.Fatalf("expected first success and second failure only (third skipped), got %#v", harness.calls)
	}
	if !containsRestoreArg(harness.calls[0].Args, "First") || !containsRestoreArg(harness.calls[1].Args, "Second") {
		t.Fatalf("unexpected exec order: %#v", harness.calls)
	}
	// initial + post-confirm plan check + pre-exec for first and second only
	if scans != 4 {
		t.Fatalf("expected 4 scans, got %d", scans)
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
