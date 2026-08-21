package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ompatel-24/rome/internal/app"
	"github.com/ompatel-24/rome/internal/command"
	"github.com/ompatel-24/rome/internal/terminal"
	"github.com/ompatel-24/rome/internal/version"
)

const helpText = `Rome runs an interactive command inside a pseudo-terminal.

Usage:
  rome <command> [args...]
  rome -- <command> [args...]
  rome help
  rome version

The -- separator is optional. Use it to run a command named help or version,
or whenever an explicit end to Rome's arguments is useful.

Examples:
  rome bash
  rome claude
  rome aider --model sonnet
  rome -- command --with-flags

Environment:
  ROME_LISTEN=<host:port>  Enable the authenticated local transport on one
                          concrete loopback or LAN interface.
  ROME_WEB_DIR=<path>      Override embedded mobile assets for development.
`

func main() {
	os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func runCLI(ctx context.Context, args []string, stdin *os.File, stdout, stderr io.Writer) int {
	invocation, err := command.Parse(args)
	if err != nil {
		fmt.Fprintf(stderr, "rome: %s\n", err)
		fmt.Fprintln(stderr, "Try 'rome help' for usage.")
		return 2
	}

	switch invocation.Action {
	case command.ActionHelp:
		fmt.Fprint(stdout, helpText)
		return 0
	case command.ActionVersion:
		fmt.Fprintf(stdout, "rome %s\n", version.Value)
		return 0
	case command.ActionRun:
		runner := app.Runner{
			ListenAddress: os.Getenv("ROME_LISTEN"),
			WebRoot:       os.Getenv("ROME_WEB_DIR"),
			Terminal: terminal.Runner{
				Stdin:  stdin,
				Stdout: stdout,
				Stderr: stderr,
				Debug:  debugEnabled(os.Getenv("ROME_DEBUG")),
			},
		}
		result, runErr := runner.Run(ctx, invocation.Args)
		if runErr != nil {
			fmt.Fprintf(stderr, "rome: %s\n", runErr)
			return terminal.ErrorCode(runErr)
		}
		return result.ExitCode
	default:
		fmt.Fprintln(stderr, "rome: internal error: unknown command action")
		return 1
	}
}

func debugEnabled(value string) bool {
	value = strings.TrimSpace(strings.ToLower(value))
	return value != "" && value != "0" && value != "false" && value != "no"
}
