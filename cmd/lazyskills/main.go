package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"lazyskills/internal/scan"
	"lazyskills/internal/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
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
			return fmt.Errorf("usage: lazyskills [--cwd <path>] | lazyskills scan --json [--cwd <path>]")
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
