//go:build darwin || linux

package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/creack/pty"
	"golang.org/x/term"
)

const outputDrainTimeout = 500 * time.Millisecond

type copyResult struct {
	err error
}

// Run launches argv in a new PTY and connects it to the configured local
// streams. It returns child exit statuses as results and reserves errors for
// failures in Ivy itself.
func (r Runner) Run(ctx context.Context, argv []string) (result Result, returnErr error) {
	if len(argv) == 0 {
		return Result{}, &RunError{Code: 2, Message: "missing command"}
	}
	if r.Stdin == nil {
		return Result{}, &RunError{Code: 1, Message: "standard input is unavailable"}
	}
	if r.Stdout == nil {
		r.Stdout = io.Discard
	}
	if r.Stderr == nil {
		r.Stderr = io.Discard
	}

	path, err := resolveExecutable(argv[0])
	if err != nil {
		return Result{}, err
	}

	stdinIsTerminal := term.IsTerminal(int(r.Stdin.Fd()))
	windowSize := &pty.Winsize{Rows: defaultRows, Cols: defaultCols}
	if stdinIsTerminal {
		if size, sizeErr := pty.GetsizeFull(r.Stdin); sizeErr == nil {
			windowSize = size
		} else {
			r.debugf("could not read local terminal size; using %dx%d: %v", defaultCols, defaultRows, sizeErr)
		}
	}

	var oldState *term.State
	if stdinIsTerminal {
		oldState, err = term.MakeRaw(int(r.Stdin.Fd()))
		if err != nil {
			return Result{}, &RunError{Code: 1, Message: fmt.Sprintf("failed to enter raw mode: %v", err), Err: err}
		}
		defer func() {
			if restoreErr := term.Restore(int(r.Stdin.Fd()), oldState); restoreErr != nil {
				if returnErr == nil {
					returnErr = &RunError{Code: 1, Message: fmt.Sprintf("failed to restore terminal: %v", restoreErr), Err: restoreErr}
				} else {
					r.debugf("failed to restore terminal after another error: %v", restoreErr)
				}
			}
		}()
	}

	cmd := &exec.Cmd{Path: path, Args: append([]string(nil), argv...)}
	ptmx, err := pty.StartWithSize(cmd, windowSize)
	if err != nil {
		return Result{}, startError(argv[0], err)
	}
	defer func() {
		if closeErr := ptmx.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			r.debugf("failed to close PTY: %v", closeErr)
		}
	}()

	signalCh, stopSignals := r.signalChannel()
	defer stopSignals()

	inputCtx, cancelInput := context.WithCancel(ctx)
	defer cancelInput()

	inputDone := make(chan error, 1)
	go func() {
		inputErr := pumpInput(inputCtx, ptmx, r.Stdin)
		if errors.Is(inputErr, io.EOF) {
			if !stdinIsTerminal {
				if _, writeErr := ptmx.Write([]byte{4}); writeErr != nil && !isExpectedCloseError(writeErr) {
					inputErr = fmt.Errorf("send end of input: %w", writeErr)
				} else {
					inputErr = nil
				}
			} else {
				inputErr = nil
			}
		}
		inputDone <- inputErr
	}()

	outputDone := make(chan copyResult, 1)
	go func() {
		_, outputErr := io.Copy(r.Stdout, ptmx)
		outputDone <- copyResult{err: outputErr}
	}()

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
	}()

	var (
		childExited   bool
		outputEnded   bool
		inputEnded    bool
		waitErr       error
		ivyErr        error
		shutdownTimer *time.Timer
		drainTimer    *time.Timer
		contextDone   = ctx.Done()
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
		if shuttingDown {
			r.debugf("second shutdown request; killing child process group")
			_ = signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
			return
		}
		shuttingDown = true
		if signalErr := signalProcessGroup(cmd.Process.Pid, sig); signalErr != nil && !errors.Is(signalErr, syscall.ESRCH) {
			r.debugf("failed to forward %s: %v", sig, signalErr)
		}
		shutdownTimer = time.NewTimer(r.gracePeriod())
		shutdownCh = shutdownTimer.C
	}

	for !childExited || !outputEnded {
		select {
		case <-contextDone:
			contextDone = nil
			if !childExited {
				requestShutdown(syscall.SIGTERM)
			}

		case receivedSignal, ok := <-signalCh:
			if !ok {
				signalCh = nil
				continue
			}
			sig, ok := receivedSignal.(syscall.Signal)
			if !ok {
				continue
			}
			switch sig {
			case syscall.SIGWINCH:
				if stdinIsTerminal && !childExited {
					if resizeErr := copyTerminalSize(r.Stdin, ptmx); resizeErr != nil && !isExpectedCloseError(resizeErr) {
						r.debugf("failed to resize child PTY: %v", resizeErr)
					}
				}
			case syscall.SIGINT:
				if !childExited {
					if signalErr := signalProcessGroup(cmd.Process.Pid, sig); signalErr != nil && !errors.Is(signalErr, syscall.ESRCH) {
						r.debugf("failed to forward SIGINT: %v", signalErr)
					}
				}
			case syscall.SIGTERM, syscall.SIGHUP:
				if !childExited {
					requestShutdown(sig)
				}
			}

		case inputErr := <-inputDone:
			inputEnded = true
			inputDone = nil
			if inputErr != nil && !errors.Is(inputErr, context.Canceled) && !isExpectedCloseError(inputErr) {
				if ivyErr == nil {
					ivyErr = &RunError{Code: 1, Message: fmt.Sprintf("failed to read terminal input: %v", inputErr), Err: inputErr}
				}
				if !childExited {
					requestShutdown(syscall.SIGHUP)
				}
			}

		case output := <-outputDone:
			outputEnded = true
			outputDone = nil
			if output.err != nil && !isExpectedPTYReadError(output.err) {
				if ivyErr == nil {
					ivyErr = &RunError{Code: 1, Message: fmt.Sprintf("failed to read PTY output: %v", output.err), Err: output.err}
				}
				if !childExited {
					requestShutdown(syscall.SIGHUP)
				}
			}

		case waitErr = <-waitDone:
			childExited = true
			waitDone = nil
			cancelInput()
			stopTimer(shutdownTimer)
			shutdownCh = nil
			if !outputEnded {
				drainTimer = time.NewTimer(outputDrainTimeout)
				drainCh = drainTimer.C
			}

		case <-shutdownCh:
			shutdownCh = nil
			if !childExited {
				r.debugf("child did not exit within %s; killing process group", r.gracePeriod())
				_ = signalProcessGroup(cmd.Process.Pid, syscall.SIGKILL)
			}

		case <-drainCh:
			drainCh = nil
			if !outputEnded {
				r.debugf("closing PTY after output drain timeout")
				_ = ptmx.Close()
			}
		}
	}

	cancelInput()
	if !inputEnded {
		select {
		case inputErr := <-inputDone:
			if inputErr != nil && !errors.Is(inputErr, context.Canceled) && !isExpectedCloseError(inputErr) && ivyErr == nil {
				ivyErr = &RunError{Code: 1, Message: fmt.Sprintf("failed to stop terminal input: %v", inputErr), Err: inputErr}
			}
		case <-time.After(250 * time.Millisecond):
			if ivyErr == nil {
				ivyErr = &RunError{Code: 1, Message: "failed to stop terminal input"}
			}
		}
	}

	if ivyErr != nil {
		return Result{}, ivyErr
	}
	return Result{ExitCode: childExitCode(waitErr)}, nil
}

func (r Runner) signalChannel() (<-chan os.Signal, func()) {
	if r.Signals != nil {
		return r.Signals, func() {}
	}

	signals := make(chan os.Signal, 8)
	signal.Notify(signals, syscall.SIGWINCH, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	return signals, func() {
		signal.Stop(signals)
		close(signals)
	}
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

func copyTerminalSize(source, target *os.File) error {
	return pty.InheritSize(source, target)
}

func isExpectedPTYReadError(err error) bool {
	return err == nil || errors.Is(err, syscall.EIO) || isExpectedCloseError(err)
}

func isExpectedCloseError(err error) bool {
	return errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EBADF) || errors.Is(err, syscall.EIO)
}
