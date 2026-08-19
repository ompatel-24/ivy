// Package terminal runs arbitrary commands inside Unix pseudo-terminals.
package terminal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

const (
	defaultRows = 24
	defaultCols = 80
)

// Result describes how the child process exited.
type Result struct {
	ExitCode int
}

// Runner connects a child PTY to local streams. Signals is optional and exists
// primarily to make signal behavior deterministic in integration tests.
type Runner struct {
	Stdin  *os.File
	Stdout io.Writer
	Stderr io.Writer

	Signals     <-chan os.Signal
	GracePeriod time.Duration
	Debug       bool
}

// RunError is an Ivy failure carrying the shell-compatible exit code that the
// CLI should return.
type RunError struct {
	Code    int
	Message string
	Err     error
}

func (e *RunError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "terminal error"
}

func (e *RunError) Unwrap() error {
	return e.Err
}

// ErrorCode extracts a CLI exit code from an Ivy runtime error.
func ErrorCode(err error) int {
	var runErr *RunError
	if errors.As(err, &runErr) && runErr.Code > 0 {
		return runErr.Code
	}
	return 1
}

func (r Runner) gracePeriod() time.Duration {
	if r.GracePeriod > 0 {
		return r.GracePeriod
	}
	return 2 * time.Second
}

func (r Runner) debugf(format string, args ...any) {
	if !r.Debug || r.Stderr == nil {
		return
	}
	fmt.Fprintf(r.Stderr, "ivy: debug: "+format+"\n", args...)
}
