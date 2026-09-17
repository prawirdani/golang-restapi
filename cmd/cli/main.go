// Command cli groups the project's developer utilities behind subcommands.
//
//	cli permissions [-out file]   dump registered permission codes as a JS array
//	cli help                      list available commands
//
// Run `cli help` for the current command list.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

// command is one CLI subcommand.
type command struct {
	name    string
	summary string
	run     func(args []string) error
}

// commands is the ordered subcommand registry (also the `help` listing order).
var commands = []command{
	permissionsCmd,
}

func main() {
	if err := dispatch(os.Args[1:]); err != nil {
		// --help is not an error.
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// dispatch routes args[0] to a subcommand, passing the remaining args through.
func dispatch(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return nil
	}

	for _, cmd := range commands {
		if cmd.name == args[0] {
			return cmd.run(args[1:])
		}
	}

	printUsage(os.Stderr)
	return fmt.Errorf("unknown command %q", args[0])
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: cli <command> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "commands:")
	for _, cmd := range commands {
		fmt.Fprintf(w, "  %-14s %s\n", cmd.name, cmd.summary)
	}
}
