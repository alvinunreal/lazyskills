package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/model"
	"github.com/alvinunreal/lazyskills/internal/runner"
	"github.com/alvinunreal/lazyskills/internal/scan"
)

var (
	restoreScan                           = scan.Run
	restoreResolveSkillsCommand           = actions.ResolveSkillsCommand
	restoreRunExec                        = func(spec runner.ExecSpec) runner.Result { return (runner.OSRunner{}).Run(spec) }
	restoreInput                io.Reader = os.Stdin
)

type restorePlanItem struct {
	name       string
	scope      model.Scope
	localLock  *model.LocalLockEntry
	globalLock *model.GlobalLockEntry
	program    string
	args       []string
}

func runRestore(args []string) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	global := fs.Bool("global", false, "restore global skills only")
	project := fs.Bool("project", false, "restore project skills only")
	all := fs.Bool("all", false, "restore project and global skills")
	yes := fs.Bool("yes", false, "skip confirmation")
	cwd := fs.String("cwd", "", "project working directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	selectedScopes := 0
	for _, selected := range []bool{*global, *project, *all} {
		if selected {
			selectedScopes++
		}
	}
	if selectedScopes > 1 {
		return fmt.Errorf("choose only one restore scope: --global, --project, or --all")
	}
	if *cwd == "" {
		var err error
		*cwd, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	result, err := restoreScan(*cwd)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	for _, name := range fs.Args() {
		names[strings.ToLower(name)] = true
	}
	candidates := restoreCandidates(result.Skills, *global, *project, names)
	if len(candidates) == 0 {
		fmt.Fprintln(os.Stdout, "No missing locked skills.")
		return nil
	}

	// Immutable plan from the user-visible preview baseline.
	plan, err := restoreBuildPlan(candidates, restoreResolveSkillsCommand)
	if err != nil {
		return err
	}
	preview := actions.ForRestoreWithResolver(candidates, restoreResolveSkillsCommand)
	if !preview.Available {
		return fmt.Errorf("restore unavailable: %s", preview.Reason)
	}
	fmt.Fprintln(os.Stdout, preview.Title)
	for _, skill := range candidates {
		fmt.Fprintf(os.Stdout, "  [%s] %s\n", skill.Scope, skill.Name)
	}
	fmt.Fprintf(os.Stdout, "Command: %s\n", preview.Command)
	if !*yes {
		fmt.Fprint(os.Stdout, "Continue? [y/N] ")
		answer, _ := bufio.NewReader(restoreInput).ReadString('\n')
		answer = strings.ToLower(strings.TrimSpace(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(os.Stdout, "Restore cancelled.")
			return nil
		}
	}

	latest, err := restoreScan(*cwd)
	if err != nil {
		return err
	}
	latestPlan, err := restoreBuildPlan(restoreCandidates(latest.Skills, *global, *project, names), restoreResolveSkillsCommand)
	if err != nil {
		return fmt.Errorf("skill state changed after preview; %w", err)
	}
	if !restorePlansEqual(plan, latestPlan) {
		return fmt.Errorf("skill state changed after preview; run restore again")
	}

	var succeeded []string
	for _, item := range plan {
		current, err := restoreScan(*cwd)
		if err != nil {
			return restorePartialError(succeeded, err)
		}
		skill := restoreLookupMissing(current.Skills, item.name, item.scope)
		if skill == nil {
			return restorePartialError(succeeded, fmt.Errorf("skill state changed after preview; [%s] %s is no longer missing; run restore again", item.scope, item.name))
		}
		if !restoreLockIdentityEqual(skill, item) {
			return restorePartialError(succeeded, fmt.Errorf("skill state changed after preview; [%s] %s lock metadata changed; run restore again", item.scope, item.name))
		}
		program, args, ok, reason := restoreSkillCommand(skill, restoreResolveSkillsCommand)
		if !ok {
			return restorePartialError(succeeded, fmt.Errorf("skill state changed after preview; [%s] %s restore command unavailable: %s; run restore again", item.scope, item.name, reason))
		}
		if program != item.program || !slices.Equal(args, item.args) {
			return restorePartialError(succeeded, fmt.Errorf("skill state changed after preview; [%s] %s restore command changed; run restore again", item.scope, item.name))
		}

		execResult := restoreRunExec(runner.ExecSpec{Program: program, Args: args, Cwd: *cwd})
		if execResult.ExitCode != 0 || execResult.Err != "" {
			if execResult.Stderr != "" {
				fmt.Fprintln(os.Stderr, execResult.Stderr)
			}
			detail := execResult.Err
			if detail == "" {
				detail = fmt.Sprintf("exit code %d", execResult.ExitCode)
			}
			return restorePartialError(succeeded, fmt.Errorf("restore command failed for [%s] %s: %s", item.scope, item.name, detail))
		}
		succeeded = append(succeeded, restoreQualifiedName(item.scope, item.name))
	}
	fmt.Fprintln(os.Stdout, "Restore complete.")
	return nil
}

func restoreCandidates(skills []*model.Skill, globalOnly, projectOnly bool, names map[string]bool) []*model.Skill {
	candidates := make([]*model.Skill, 0, len(skills))
	for _, skill := range skills {
		if !actions.IsMissingLockedSkill(skill) {
			continue
		}
		if globalOnly && skill.Scope != model.ScopeGlobal {
			continue
		}
		if projectOnly && skill.Scope != model.ScopeProject {
			continue
		}
		if len(names) > 0 && !names[strings.ToLower(skill.Name)] {
			continue
		}
		candidates = append(candidates, skill)
	}
	return candidates
}

func restoreBuildPlan(candidates []*model.Skill, resolve actions.SkillsResolver) ([]restorePlanItem, error) {
	plan := make([]restorePlanItem, 0, len(candidates))
	for _, skill := range candidates {
		program, args, ok, reason := restoreSkillCommand(skill, resolve)
		if !ok {
			return nil, fmt.Errorf("restore unavailable: %s", reason)
		}
		plan = append(plan, restorePlanItem{
			name:       skill.Name,
			scope:      skill.Scope,
			localLock:  cloneLocalLock(skill.LocalLock),
			globalLock: cloneGlobalLock(skill.GlobalLock),
			program:    program,
			args:       args,
		})
	}
	return plan, nil
}

func restorePlansEqual(a, b []restorePlanItem) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].name != b[i].name || a[i].scope != b[i].scope {
			return false
		}
		if !reflect.DeepEqual(a[i].localLock, b[i].localLock) || !reflect.DeepEqual(a[i].globalLock, b[i].globalLock) {
			return false
		}
		if a[i].program != b[i].program || !slices.Equal(a[i].args, b[i].args) {
			return false
		}
	}
	return true
}

func restoreSkillCommand(skill *model.Skill, resolve actions.SkillsResolver) (string, []string, bool, string) {
	preview := actions.ForRestoreWithResolver([]*model.Skill{skill}, resolve)
	if !preview.Available {
		return "", nil, false, preview.Reason
	}
	if len(preview.Exec.Batch) != 1 {
		return "", nil, false, "unexpected restore command batch"
	}
	spec := preview.Exec.Batch[0]
	return spec.Program, append([]string{}, spec.Args...), true, ""
}

func restoreLookupMissing(skills []*model.Skill, name string, scope model.Scope) *model.Skill {
	for _, skill := range skills {
		if skill == nil || skill.Name != name || skill.Scope != scope {
			continue
		}
		if actions.IsMissingLockedSkill(skill) {
			return skill
		}
		return nil
	}
	return nil
}

func restoreLockIdentityEqual(skill *model.Skill, item restorePlanItem) bool {
	if skill == nil {
		return false
	}
	return reflect.DeepEqual(skill.LocalLock, item.localLock) && reflect.DeepEqual(skill.GlobalLock, item.globalLock)
}

func restoreQualifiedName(scope model.Scope, name string) string {
	return fmt.Sprintf("[%s] %s", scope, name)
}

func restorePartialError(succeeded []string, err error) error {
	if err == nil {
		return nil
	}
	if len(succeeded) == 0 {
		return err
	}
	return fmt.Errorf("partial restore; already restored: %s; %w", strings.Join(succeeded, ", "), err)
}

func cloneLocalLock(lock *model.LocalLockEntry) *model.LocalLockEntry {
	if lock == nil {
		return nil
	}
	copy := *lock
	return &copy
}

func cloneGlobalLock(lock *model.GlobalLockEntry) *model.GlobalLockEntry {
	if lock == nil {
		return nil
	}
	copy := *lock
	return &copy
}
