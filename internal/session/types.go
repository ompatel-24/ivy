// Package session owns interactive child processes, their pseudo-terminals,
// bounded output history, and output subscriptions.
package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	// DefaultHistoryBytes bounds retained raw PTY output at 512 KiB.
	DefaultHistoryBytes = 512 * 1024
	// DefaultSubscriberBuffer bounds each subscriber at 64 output chunks.
	DefaultSubscriberBuffer = 64
	defaultRows             = 24
	defaultCols             = 80
)

var (
	// ErrClosed reports an operation attempted after a session ended.
	ErrClosed = errors.New("session is closed")
	// ErrManagerClosed reports a start attempted after manager shutdown.
	ErrManagerClosed = errors.New("session manager is closed")
	// ErrSlowSubscriber reports eviction of a subscriber whose bounded queue
	// could not keep up with PTY output.
	ErrSlowSubscriber = errors.New("session subscriber is too slow")
)

// State describes the externally observable lifecycle of a Session.
type State string

const (
	StateRunning State = "running"
	StateExited  State = "exited"
)

// Metadata is an immutable snapshot of session identity and lifecycle state.
// ExitCode is meaningful only when State is StateExited.
type Metadata struct {
	ID       string
	Command  []string
	Dir      string
	State    State
	ExitCode int
}

// Result describes how the child process exited.
type Result struct {
	ExitCode int
}

// RunError is an Rome failure carrying the shell-compatible exit code that the
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
	return "session error"
}

func (e *RunError) Unwrap() error {
	return e.Err
}

// ErrorCode extracts a CLI exit code from an Rome runtime error.
func ErrorCode(err error) int {
	var runErr *RunError
	if errors.As(err, &runErr) && runErr.Code > 0 {
		return runErr.Code
	}
	return 1
}

// ManagerOptions configure all sessions started by a Manager. Zero values use
// the documented defaults.
type ManagerOptions struct {
	HistoryBytes     int
	SubscriberBuffer int
	GracePeriod      time.Duration
}

// StartOptions configure the initial PTY window size.
type StartOptions struct {
	Rows uint16
	Cols uint16
}

type normalizedOptions struct {
	historyBytes     int
	subscriberBuffer int
	gracePeriod      time.Duration
}

// Session owns one child process and its PTY. All PTY output is read by one
// goroutine and published as immutable byte slices.
type Session struct {
	id      string
	command []string
	dir     string

	stateMu sync.RWMutex
	state   State
	result  Result
	runErr  error

	cmd    *exec.Cmd
	ptmx   *os.File
	input  io.Writer
	cancel context.CancelFunc
	done   chan struct{}

	inputMu sync.Mutex

	subscribersMu sync.Mutex
	history       *byteRing
	subscribers   map[uint64]*Subscription
	nextID        uint64
	closed        bool

	signalRequests chan os.Signal
	options        normalizedOptions
	closePTYOnce   sync.Once
}

func normalizeOptions(options ManagerOptions) (normalizedOptions, error) {
	if options.HistoryBytes < 0 {
		return normalizedOptions{}, fmt.Errorf("history size cannot be negative")
	}
	if options.SubscriberBuffer < 0 {
		return normalizedOptions{}, fmt.Errorf("subscriber buffer cannot be negative")
	}
	if options.GracePeriod < 0 {
		return normalizedOptions{}, fmt.Errorf("grace period cannot be negative")
	}

	normalized := normalizedOptions{
		historyBytes:     options.HistoryBytes,
		subscriberBuffer: options.SubscriberBuffer,
		gracePeriod:      options.GracePeriod,
	}
	if normalized.historyBytes == 0 {
		normalized.historyBytes = DefaultHistoryBytes
	}
	if normalized.subscriberBuffer == 0 {
		normalized.subscriberBuffer = DefaultSubscriberBuffer
	}
	if normalized.gracePeriod == 0 {
		normalized.gracePeriod = 2 * time.Second
	}
	return normalized, nil
}

func cloneStrings(values []string) []string {
	return append([]string(nil), values...)
}

func writeAll(dst io.Writer, data []byte) (int, error) {
	written := 0
	for written < len(data) {
		n, err := dst.Write(data[written:])
		written += n
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}
