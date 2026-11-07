// Command changelog is the release job's interface to CHANGELOG.md.
//
//	changelog check               validate Unreleased; exit 1 on any problem
//	changelog notes               print the Unreleased entries
//	changelog next vX.Y.Z         print the version Unreleased calls for after
//	                              vX.Y.Z, or nothing when Unreleased is empty
//	changelog release vX.Y.Z DATE move Unreleased under "## vX.Y.Z - DATE"
//
// -file names the changelog (default CHANGELOG.md).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jsdrews/tuilib/internal/changelog"
)

func main() {
	file := flag.String("file", "CHANGELOG.md", "changelog to read")
	flag.Parse()
	if err := run(*file, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "changelog:", err)
		os.Exit(1)
	}
}

func run(file string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("want a command: check, notes, next or release")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	log, err := changelog.Parse(string(raw))
	if err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	switch cmd, rest := args[0], args[1:]; {
	case cmd == "check" && len(rest) == 0:
		errs := log.Check()
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "%s: %v\n", file, e)
		}
		if len(errs) > 0 {
			return fmt.Errorf("%d problem(s) in Unreleased", len(errs))
		}
		return nil
	case cmd == "notes" && len(rest) == 0:
		fmt.Println(log.Notes())
		return nil
	case cmd == "next" && len(rest) == 1:
		next, err := log.Next(rest[0])
		if err != nil {
			return err
		}
		if next != "" {
			fmt.Println(next)
		}
		return nil
	case cmd == "release" && len(rest) == 2:
		if errs := log.Check(); len(errs) > 0 {
			return fmt.Errorf("Unreleased is malformed (run check): %v", errs[0])
		}
		if log.Empty() {
			return fmt.Errorf("Unreleased is empty; nothing to release")
		}
		return os.WriteFile(file, []byte(log.Release(rest[0], rest[1])), 0o644)
	}
	return fmt.Errorf("bad usage: %v", args)
}
