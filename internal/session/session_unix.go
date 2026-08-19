//go:build darwin || linux

package session

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/creack/pty"
)

const (
	outputBufferBytes  = 32 * 1024
	outputDrainTimeout = 500 * time.Millisecond
)

func start(ctx context.Context, argv []string, startOptions StartOptions, options normalizedOptions) (*Session, error) {
	if len(argv) == 0 {
		return nil, &RunError{Code: 2, Message: "missing command"}
	}
	if err := ctx.Err(); err != nil {
		return nil, &RunError{Code: 1, Message: fmt.Sprintf("cannot start session: %v", err), Err: err}
	}

	path, err := resolveExecutable(argv[0])
	if err != nil {
		return nil, err
	}
	id, err := newID()
	if err != nil {
		return nil, &RunError{Code: 1, Message: fmt.Sprintf("failed to generate session ID: %v", err), Err: err}
	}
	dir, err := os.Getwd()
	if err != nil {
		return nil, &RunError{Code: 1, Message: fmt.Sprintf("failed to read working directory: %v", err), Err: err}
	}

	rows, cols := startOptions.Rows, startOptions.Cols
	if rows == 0 {
		rows = defaultRows
	}
	if cols == 0 {
		cols = defaultCols
	}
	command := cloneStrings(argv)
	cmd := &exec.Cmd{Path: path, Args: command}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	if err != nil {
		return nil, startError(argv[0], err)
	}

	sessionContext, cancel := context.WithCancel(ctx)
	started := &Session{
		id:             id,
		command:        command,
		dir:            dir,
		state:          StateRunning,
		cmd:            cmd,
		ptmx:           ptmx,
		input:          ptmx,
		cancel:         cancel,
		done:           make(chan struct{}),
		history:        newByteRing(options.historyBytes),
		subscribers:    make(map[uint64]*Subscription),
		signalRequests: make(chan os.Signal, 8),
		options:        options,
	}
	go started.supervise(sessionContext)
	return started, nil
}

// Write sends one complete input chunk to the PTY without interleaving it with
// concurrent input sources.
func (s *Session) Write(data []byte) (int, error) {
	if isDone(s.done) {
		return 0, ErrClosed
	}
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	if isDone(s.done) {
		return 0, ErrClosed
	}
	written, err := writeAll(s.input, data)
	if err != nil {
		return written, fmt.Errorf("write session input: %w", err)
	}
	return written, nil
}

// Resize changes the child PTY dimensions.
func (s *Session) Resize(cols, rows uint16) error {
	if cols == 0 || rows == 0 {
		return errors.New("terminal dimensions must be greater than zero")
	}
	if isDone(s.done) {
		return ErrClosed
	}
	s.inputMu.Lock()
	defer s.inputMu.Unlock()
	if isDone(s.done) {
		return ErrClosed
	}
	if err := pty.Setsize(s.ptmx, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
		if isExpectedCloseError(err) {
			return ErrClosed
		}
		return fmt.Errorf("resize session PTY: %w", err)
	}
	return nil
}

// Signal forwards a Unix signal to the child's process group. SIGTERM and
// SIGHUP begin graceful shutdown; a second termination request forces SIGKILL.
func (s *Session) Signal(received os.Signal) error {
	if _, ok := received.(syscall.Signal); !ok {
		return fmt.Errorf("unsupported signal %v", received)
	}
	select {
	case <-s.done:
		return ErrClosed
	case s.signalRequests <- received:
		return nil
	}
}

// Close gracefully terminates the child and waits for output to drain.
func (s *Session) Close() error {
	if !isDone(s.done) {
		if err := s.Signal(syscall.SIGHUP); err != nil && !errors.Is(err, ErrClosed) {
			return err
		}
	}
	_, err := s.Wait()
	return err
}

func (s *Session) supervise(ctx context.Context) {
	outputDone := make(chan error, 1)
	go func() {
		outputDone <- s.readOutput()
	}()
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- s.cmd.Wait()
	}()

	var (
		childExited   bool
		outputEnded   bool
		waitErr       error
		runErr        error
		contextDone   = ctx.Done()
		shutdownTimer *time.Timer
		drainTimer    *time.Timer
		shutdownCh    <-chan time.Time
		drainCh       <-chan time.Time
		shuttingDown  bool
	)

	stopTimer := func(timer *time.Timer) {
		if timer != nil && !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	defer func() {
		stopTimer(shutdownTimer)
		stopTimer(drainTimer)
	}()

	requestShutdown := func(sig syscall.Signal) {
		if sig == syscall.SIGKILL || shuttingDown {
			_ = signalProcessGroup(s.cmd.Process.Pid, syscall.SIGKILL)
			shuttingDown = true
			return
		}
		shuttingDown = true
		if err := signalProcessGroup(s.cmd.Process.Pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) && runErr == nil {
			runErr = &RunError{Code: 1, Message: fmt.Sprintf("failed to signal child process: %v", err), Err: err}
		}
		shutdownTimer = time.NewTimer(s.options.gracePeriod)
		shutdownCh = shutdownTimer.C
	}

	for !childExited || !outputEnded {
		select {
		case <-contextDone:
			contextDone = nil
			if !childExited {
				requestShutdown(syscall.SIGTERM)
			}

		case requested := <-s.signalRequests:
			sig := requested.(syscall.Signal)
			if childExited {
				continue
			}
			switch sig {
			case syscall.SIGTERM, syscall.SIGHUP, syscall.SIGKILL:
				requestShutdown(sig)
			default:
				if err := signalProcessGroup(s.cmd.Process.Pid, sig); err != nil && !errors.Is(err, syscall.ESRCH) && runErr == nil {
					runErr = &RunError{Code: 1, Message: fmt.Sprintf("failed to signal child process: %v", err), Err: err}
				}
			}

		case outputErr := <-outputDone:
			outputEnded = true
			outputDone = nil
			if outputErr != nil && !isExpectedPTYReadError(outputErr) {
				if runErr == nil {
					runErr = &RunError{Code: 1, Message: fmt.Sprintf("failed to read PTY output: %v", outputErr), Err: outputErr}
				}
				if !childExited {
					requestShutdown(syscall.SIGHUP)
				}
			}

		case waitErr = <-waitDone:
			childExited = true
			waitDone = nil
			stopTimer(shutdownTimer)
			shutdownCh = nil
			if !outputEnded {
				drainTimer = time.NewTimer(outputDrainTimeout)
				drainCh = drainTimer.C
			}

		case <-shutdownCh:
			shutdownCh = nil
			if !childExited {
				_ = signalProcessGroup(s.cmd.Process.Pid, syscall.SIGKILL)
			}

		case <-drainCh:
			drainCh = nil
			if !outputEnded {
				s.closePTY()
			}
		}
	}

	s.cancel()
	s.closePTY()
	s.finish(Result{ExitCode: childExitCode(waitErr)}, runErr)
}

func (s *Session) readOutput() error {
	buffer := make([]byte, outputBufferBytes)
	for {
		read, err := s.ptmx.Read(buffer)
		if read > 0 {
			s.publish(bytes.Clone(buffer[:read]))
		}
		if err != nil {
			return err
		}
	}
}

func (s *Session) closePTY() {
	s.closePTYOnce.Do(func() {
		s.inputMu.Lock()
		defer s.inputMu.Unlock()
		_ = s.ptmx.Close()
	})
}

func newID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func resolveExecutable(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err == nil || errors.Is(err, exec.ErrDot) {
		return path, nil
	}

	label := commandLabel(name)
	if errors.Is(err, fs.ErrPermission) {
		return "", &RunError{Code: 126, Message: fmt.Sprintf("cannot execute command: %s: permission denied", label), Err: err}
	}
	return "", &RunError{Code: 127, Message: fmt.Sprintf("command not found: %s", label), Err: err}
}

func startError(name string, err error) error {
	code := 1
	message := fmt.Sprintf("failed to create PTY: %v", err)
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.ENOEXEC) {
		code = 126
		message = fmt.Sprintf("cannot execute command: %s: %v", commandLabel(name), err)
	}
	return &RunError{Code: code, Message: message, Err: err}
}

func commandLabel(name string) string {
	if strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return strconv.Quote(name)
	}
	return name
}

func childExitCode(waitErr error) int {
	if waitErr == nil {
		return 0
	}

	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		return 1
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		if code := exitErr.ExitCode(); code >= 0 {
			return code
		}
		return 1
	}
	if status.Signaled() {
		return 128 + int(status.Signal())
	}
	return status.ExitStatus()
}

func signalProcessGroup(pid int, sig syscall.Signal) error {
	return syscall.Kill(-pid, sig)
}

func isExpectedPTYReadError(err error) bool {
	return err == nil || errors.Is(err, io.EOF) || errors.Is(err, syscall.EIO) || isExpectedCloseError(err)
}

func isExpectedCloseError(err error) bool {
	return errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EBADF) || errors.Is(err, syscall.EIO)
}
