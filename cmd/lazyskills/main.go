package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/alvinunreal/lazyskills/internal/actions"
	"github.com/alvinunreal/lazyskills/internal/buildinfo"
	"github.com/alvinunreal/lazyskills/internal/registry"
	"github.com/alvinunreal/lazyskills/internal/scan"
	"github.com/alvinunreal/lazyskills/internal/selfupdate"
	"github.com/alvinunreal/lazyskills/internal/tui"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type findJSONResponse struct {
	Query           string                    `json:"query"`
	Results         []registry.Skill          `json:"results"`
	InstallCommands []findJSONInstallCommands `json:"install_commands,omitempty"`
}

type findJSONInstallCommands struct {
	Slug    string                  `json:"slug"`
	Source  string                  `json:"source"`
	Project []findJSONCommandPreview `json:"project"`
	Global  []findJSONCommandPreview `json:"global"`
}

type findJSONCommandPreview struct {
	ID              string                 `json:"id,omitempty"`
	Title           string                 `json:"title,omitempty"`
	Program         string                 `json:"program,omitempty"`
	Args            []string               `json:"args,omitempty"`
	Exec            findJSONExecSpec       `json:"exec,omitempty"`
	Command         string                 `json:"command,omitempty"`
	Description     string                 `json:"description,omitempty"`
	Mutates         bool                   `json:"mutates,omitempty"`
	RequiresConfirm bool                   `json:"requires_confirm,omitempty"`
	Dangerous       bool                   `json:"dangerous,omitempty"`
	ConfirmValue    string                 `json:"confirm_value,omitempty"`
	Available       bool                   `json:"available"`
	Reason          string                 `json:"reason,omitempty"`
}

type findJSONExecSpec struct {
	Program     string               `json:"program,omitempty"`
	Args        []string             `json:"args,omitempty"`
	Batch       []findJSONExecSpec   `json:"batch,omitempty"`
	Cwd         string               `json:"cwd,omitempty"`
	Interactive bool                 `json:"interactive,omitempty"`
	Internal    string               `json:"internal,omitempty"`
}

func convertCommandPreview(preview actions.CommandPreview) findJSONCommandPreview {
	return findJSONCommandPreview{
		ID:              preview.ID,
		Title:           preview.Title,
		Program:         preview.Program,
		Args:            append([]string(nil), preview.Args...),
		Exec:            convertExecSpec(preview.Exec),
		Command:         preview.Command,
		Description:     preview.Description,
		Mutates:         preview.Mutates,
		RequiresConfirm: preview.RequiresConfirm,
		Dangerous:       preview.Dangerous,
		ConfirmValue:    preview.ConfirmValue,
		Available:       preview.Available,
		Reason:          preview.Reason,
	}
}

func convertExecSpec(spec actions.ExecSpec) findJSONExecSpec {
	batch := make([]findJSONExecSpec, 0, len(spec.Batch))
	for _, child := range spec.Batch {
		batch = append(batch, convertExecSpec(child))
	}
	return findJSONExecSpec{
		Program:     spec.Program,
		Args:        append([]string(nil), spec.Args...),
		Batch:       batch,
		Cwd:         spec.Cwd,
		Interactive: spec.Interactive,
		Internal:    spec.Internal,
	}
}

func buildFindInstallCommands(results []registry.Skill) []findJSONInstallCommands {
	if len(results) == 0 {
		return nil
	}
	installCommands := make([]findJSONInstallCommands, 0, len(results))
	for _, result := range results {
		project := actions.ForAvailableSkillWithOptions(result.Source, actions.InstallOptions{
			DisplayName: result.DisplayName,
			Slug:        result.Slug,
			Global:      false,
		})
		global := actions.ForAvailableSkillWithOptions(result.Source, actions.InstallOptions{
			DisplayName: result.DisplayName,
			Slug:        result.Slug,
			Global:      true,
		})
		if result.Invalid {
			reason := result.Reason
			if reason == "" {
				reason = "registry result cannot be safely installed"
			}
			for i := range project {
				project[i] = unavailableFindPreview(project[i], reason)
			}
			for i := range global {
				global[i] = unavailableFindPreview(global[i], reason)
			}
		}
		installCommands = append(installCommands, findJSONInstallCommands{
			Slug:    result.Slug,
			Source:  result.Source,
			Project: convertCommandPreviews(project),
			Global:  convertCommandPreviews(global),
		})
	}
	return installCommands
}

func unavailableFindPreview(preview actions.CommandPreview, reason string) actions.CommandPreview {
	return actions.CommandPreview{
		ID:          preview.ID,
		Title:       preview.Title,
		Description: preview.Description,
		Available:   false,
		Reason:      reason,
	}
}

func convertCommandPreviews(previews []actions.CommandPreview) []findJSONCommandPreview {
	if len(previews) == 0 {
		return nil
	}
	out := make([]findJSONCommandPreview, 0, len(previews))
	for _, preview := range previews {
		out = append(out, convertCommandPreview(preview))
	}
	return out
}

func init() {
	if version != "dev" && version != "" {
		buildinfo.Version = version
	}
	if commit != "none" && commit != "" {
		buildinfo.Commit = commit
	}
	if date != "unknown" && date != "" {
		buildinfo.Date = date
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version" || args[0] == "-v") {
		fmt.Fprintf(os.Stdout, "lazyskills %s\ncommit: %s\nbuilt: %s\n", buildinfo.Version, buildinfo.Commit, buildinfo.Date)
		return nil
	}
	if len(args) > 0 && args[0] == "find" {
		fs := flag.NewFlagSet("find", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		jsonOut := fs.Bool("json", false, "output JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if !*jsonOut {
			return fmt.Errorf("usage: lazyskills find --json <query>")
		}
		if fs.NArg() != 1 {
			return fmt.Errorf("usage: lazyskills find --json <query>")
		}
		query := fs.Arg(0)

		client := registry.NewClient()
		ctx := context.Background()
		skills, err := client.Search(ctx, query, 0)
		if err != nil {
			return err
		}

		res := struct {
			Query           string                    `json:"query"`
			Results         []registry.Skill          `json:"results"`
			InstallCommands []findJSONInstallCommands `json:"install_commands,omitempty"`
		}{
			Query:           query,
			Results:         skills,
			InstallCommands: buildFindInstallCommands(skills),
		}
		if res.Results == nil {
			res.Results = []registry.Skill{}
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if len(args) > 0 && args[0] == "update" {
		fs := flag.NewFlagSet("update", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		checkOnly := fs.Bool("check", false, "only check if update is available")
		printCmd := fs.Bool("print-command", false, "print upgrade command and exit")
		yes := fs.Bool("yes", false, "apply update automatically without confirmation")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		ctx := context.Background()
		plan, err := selfupdate.Plan(ctx, true, nil)
		if err != nil {
			return fmt.Errorf("update check failed: %w", err)
		}

		if *checkOnly {
			if plan.Status == selfupdate.StatusAvailable {
				fmt.Fprintf(os.Stdout, "Update available: %s (current: %s)\n", plan.Latest, plan.Current)
			} else if plan.Status == selfupdate.StatusUnknown {
				if plan.Reason != "" {
					fmt.Fprintln(os.Stdout, plan.Reason)
				} else {
					fmt.Fprintln(os.Stdout, "Update status unknown.")
				}
			} else {
				fmt.Fprintln(os.Stdout, "Already up to date.")
			}
			return nil
		}

		if *printCmd {
			if plan.Status == selfupdate.StatusAvailable {
				if plan.CommandPreview != "" {
					fmt.Fprintln(os.Stdout, plan.CommandPreview)
				} else {
					fmt.Fprintln(os.Stdout, plan.Reason)
				}
			} else if plan.Status == selfupdate.StatusUnknown {
				if plan.Reason != "" {
					fmt.Fprintln(os.Stdout, plan.Reason)
				} else {
					fmt.Fprintln(os.Stdout, "Update status unknown.")
				}
			} else {
				fmt.Fprintln(os.Stdout, "Already up to date.")
			}
			return nil
		}

		if plan.Status == selfupdate.StatusUnknown {
			if plan.Reason != "" {
				fmt.Fprintln(os.Stdout, plan.Reason)
			} else {
				fmt.Fprintln(os.Stdout, "Update status unknown.")
			}
			return nil
		}

		if plan.Status == selfupdate.StatusAvailable {
			if *yes {
				if plan.CanExecute {
					fmt.Fprintf(os.Stdout, "Updating lazyskills to %s...\n", plan.Latest)
					if err := selfupdate.Apply(ctx, plan, nil); err != nil {
						return fmt.Errorf("update failed: %w", err)
					}
					fmt.Fprintln(os.Stdout, "Update applied successfully. Please restart lazyskills.")
					return nil
				} else {
					fmt.Fprintf(os.Stdout, "Auto-update not supported for install channel: %s\n%s\n", plan.Channel, plan.Reason)
					if plan.CommandPreview != "" {
						fmt.Fprintf(os.Stdout, "Command: %s\n", plan.CommandPreview)
					}
					return nil
				}
			} else {
				fmt.Fprintf(os.Stdout, "Update available: %s (current: %s)\n", plan.Latest, plan.Current)
				fmt.Fprintf(os.Stdout, "%s\n", plan.Reason)
				if plan.CommandPreview != "" {
					fmt.Fprintf(os.Stdout, "Command: %s\n", plan.CommandPreview)
				}
				return nil
			}
		}

		fmt.Fprintln(os.Stdout, "Already up to date.")
		return nil
	}
	if len(args) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return tui.Run(cwd)
	}
	if args[0] != "scan" {
		fs := flag.NewFlagSet("lazyskills", flag.ContinueOnError)
		fs.SetOutput(os.Stderr)
		cwd := fs.String("cwd", "", "project working directory")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() > 0 {
			return fmt.Errorf("usage: lazyskills [--cwd <path>] | lazyskills scan --json [--cwd <path>] | lazyskills update [--check] [--print-command] [--yes] | lazyskills find --json <query> | lazyskills version")
		}
		if *cwd == "" {
			var err error
			*cwd, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		return tui.Run(*cwd)
	}
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	cwd := fs.String("cwd", "", "project working directory")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if !*jsonOut {
		return fmt.Errorf("scan currently requires --json")
	}
	if *cwd == "" {
		var err error
		*cwd, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	result, err := scan.Run(*cwd)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
