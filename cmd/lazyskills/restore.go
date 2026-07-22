package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
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
	latestPreview := actions.ForRestoreWithResolver(restoreCandidates(latest.Skills, *global, *project, names), restoreResolveSkillsCommand)
	if !latestPreview.Available || latestPreview.Command != preview.Command {
		return fmt.Errorf("skill state changed after preview; run restore again")
	}
	preview = latestPreview

	for _, spec := range preview.Exec.Batch {
		result := restoreRunExec(runner.ExecSpec{Program: spec.Program, Args: spec.Args, Cwd: *cwd})
		if result.ExitCode != 0 || result.Err != "" {
			if result.Stderr != "" {
				fmt.Fprintln(os.Stderr, result.Stderr)
			}
			detail := result.Err
			if detail == "" {
				detail = fmt.Sprintf("exit code %d", result.ExitCode)
			}
			return fmt.Errorf("restore command failed: %s", detail)
		}
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
