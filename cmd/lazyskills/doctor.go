package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/alvinunreal/lazyskills/internal/doctor"
	"github.com/alvinunreal/lazyskills/internal/scan"
)

func runDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "output JSON")
	cwd := fs.String("cwd", "", "project working directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: lazyskills doctor [--json] [--cwd <path>]")
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
	report := doctor.Build(result)
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	_, err = fmt.Fprint(os.Stdout, doctor.FormatText(report))
	return err
}
