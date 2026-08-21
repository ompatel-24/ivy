// Package command parses Rome's small command-line surface without interpreting
// arguments intended for the child process.
package command

import (
	"errors"
	"fmt"
)

// Action identifies what Rome should do with a parsed invocation.
type Action uint8

const (
	ActionRun Action = iota
	ActionHelp
	ActionVersion
)

// Invocation is the result of parsing Rome's command-line arguments.
type Invocation struct {
	Action Action
	Args   []string
}

// ErrMissingCommand reports that Rome was invoked without a child command.
var ErrMissingCommand = errors.New("missing command")

// Parse separates Rome's built-in commands from an arbitrary child argv.
func Parse(args []string) (Invocation, error) {
	if len(args) == 0 {
		return Invocation{}, ErrMissingCommand
	}

	if args[0] == "--" {
		if len(args) == 1 {
			return Invocation{}, ErrMissingCommand
		}
		return Invocation{Action: ActionRun, Args: clone(args[1:])}, nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		if len(args) != 1 {
			return Invocation{}, fmt.Errorf("%s does not accept arguments", args[0])
		}
		return Invocation{Action: ActionHelp}, nil
	case "version", "--version":
		if len(args) != 1 {
			return Invocation{}, fmt.Errorf("%s does not accept arguments", args[0])
		}
		return Invocation{Action: ActionVersion}, nil
	default:
		return Invocation{Action: ActionRun, Args: clone(args)}, nil
	}
}

func clone(values []string) []string {
	return append([]string(nil), values...)
}
