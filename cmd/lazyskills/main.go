package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

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
			Query   string           `json:"query"`
			Results []registry.Skill `json:"results"`
		}{
			Query:   query,
			Results: skills,
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
		yes := fs.Bool("yes", false, "unsupported; automatic updates have been removed")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}

		if *yes {
			return fmt.Errorf("automatic updates have been removed; run lazyskills update for manual upgrade instructions")
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
			fmt.Fprintf(os.Stdout, "Update available: %s (current: %s)\n", plan.Latest, plan.Current)
			if plan.Reason != "" {
				fmt.Fprintf(os.Stdout, "%s\n", plan.Reason)
			}
			if plan.CommandPreview != "" {
				fmt.Fprintf(os.Stdout, "Command: %s\n", plan.CommandPreview)
			}
			if plan.ReleaseURL != "" {
				fmt.Fprintf(os.Stdout, "Release URL: %s\n", plan.ReleaseURL)
			}
			return nil
		}

		fmt.Fprintln(os.Stdout, "Already up to date.")
		return nil
	}
	if len(args) > 0 && args[0] == "restore" {
		return runRestore(args[1:])
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
			return fmt.Errorf("usage: lazyskills [--cwd <path>] | lazyskills scan --json [--cwd <path>] | lazyskills restore [--global|--project|--all] [--yes] [skills...] | lazyskills update [--check] [--print-command] | lazyskills find --json <query> | lazyskills version")
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
