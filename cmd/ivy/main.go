package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ompatel-24/ivy/internal/app"
	"github.com/ompatel-24/ivy/internal/command"
	"github.com/ompatel-24/ivy/internal/terminal"
	"github.com/ompatel-24/ivy/internal/version"
)

const helpText = `Ivy runs an interactive command inside a pseudo-terminal.

Usage:
  ivy <command> [args...]
  ivy -- <command> [args...]
  ivy help
  ivy version

The -- separator is optional. Use it to run a command named help or version,
or whenever an explicit end to Ivy's arguments is useful.

Examples:
  ivy bash
  ivy claude
  ivy aider --model sonnet
  ivy -- command --with-flags

Environment:
  IVY_LISTEN=<host:port>  Enable the authenticated local transport on one
                          concrete loopback or LAN interface.
`

func main() {
	os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func runCLI(ctx context.Context, args []string, stdin *os.File, stdout, stderr io.Writer) int {
	invocation, err := command.Parse(args)
	if err != nil {
		fmt.Fprintf(stderr, "ivy: %s\n", err)
		fmt.Fprintln(stderr, "Try 'ivy help' for usage.")
		return 2
	}

	switch invocation.Action {
	case command.ActionHelp:
		fmt.Fprint(stdout, helpText)
		return 0
	case command.ActionVersion:
		fmt.Fprintf(stdout, "ivy %s\n", version.Value)
		return 0
	case command.ActionRun:
		runner := app.Runner{
			ListenAddress: os.Getenv("IVY_LISTEN"),
			Terminal: terminal.Runner{
				Stdin:  stdin,
				Stdout: stdout,
				Stderr: stderr,
				Debug:  debugEnabled(os.Getenv("IVY_DEBUG")),
			},
		}
		result, runErr := runner.Run(ctx, invocation.Args)
		if runErr != nil {
			fmt.Fprintf(stderr, "ivy: %s\n", runErr)
			return terminal.ErrorCode(runErr)
		}
		return result.ExitCode
	default:
		fmt.Fprintln(stderr, "ivy: internal error: unknown command action")
		return 1
	}
}

func debugEnabled(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return value != "" && value != "0" && value != "false" && value != "no"
}
