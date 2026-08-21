// Package terminal runs arbitrary commands inside Unix pseudo-terminals.
package terminal

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ompatel-24/rome/internal/session"
)

const (
	defaultRows = 24
	defaultCols = 80
)

// Result describes how the child process exited.
type Result = session.Result

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

// RunError is an Rome failure carrying the shell-compatible exit code that the
// CLI should return.
type RunError = session.RunError

// ErrorCode extracts a CLI exit code from an Rome runtime error.
func ErrorCode(err error) int {
	return session.ErrorCode(err)
}

func (r Runner) debugf(format string, args ...any) {
	if !r.Debug || r.Stderr == nil {
		return
	}
	fmt.Fprintf(r.Stderr, "rome: debug: "+format+"\n", args...)
}
