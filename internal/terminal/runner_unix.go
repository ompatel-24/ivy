//go:build darwin || linux

package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/ompatel-24/rome/internal/session"
	"golang.org/x/term"
)

// Run starts a Session and attaches the local terminal to it. Application
// orchestration can instead call InitialSize and Attach around an externally
// managed Session.
func (r Runner) Run(ctx context.Context, argv []string) (result Result, returnErr error) {
	if len(argv) == 0 {
		return Result{}, &RunError{Code: 2, Message: "missing command"}
	}
	startOptions, err := r.InitialSize()
	if err != nil {
		return Result{}, err
	}

	manager, err := session.NewManager(session.ManagerOptions{GracePeriod: r.GracePeriod})
	if err != nil {
		return Result{}, &RunError{Code: 1, Message: fmt.Sprintf("failed to create session manager: %v", err), Err: err}
	}
	defer func() {
		closeContext, cancel := context.WithTimeout(context.Background(), managerCloseTimeout(r.GracePeriod))
		defer cancel()
		if closeErr := manager.Close(closeContext); closeErr != nil && returnErr == nil {
			returnErr = &RunError{Code: 1, Message: fmt.Sprintf("failed to close session manager: %v", closeErr), Err: closeErr}
		}
	}()

	managed, err := manager.Start(ctx, argv, startOptions)
	if err != nil {
		return Result{}, err
	}
	return r.Attach(ctx, managed)
}

// InitialSize returns the local terminal dimensions or the 80x24 fallback used
// for non-terminal input.
func (r Runner) InitialSize() (session.StartOptions, error) {
	if r.Stdin == nil {
		return session.StartOptions{}, &RunError{Code: 1, Message: "standard input is unavailable"}
	}

	stdinIsTerminal := term.IsTerminal(int(r.Stdin.Fd()))
	rows, cols := uint16(defaultRows), uint16(defaultCols)
	if stdinIsTerminal {
		if size, err := pty.GetsizeFull(r.Stdin); err == nil {
			rows, cols = size.Rows, size.Cols
		} else {
			r.debugf("could not read local terminal size; using %dx%d: %v", defaultCols, defaultRows, err)
		}
	}
	return session.StartOptions{Rows: rows, Cols: cols}, nil
}

// Attach connects the local terminal to an existing Session.
func (r Runner) Attach(ctx context.Context, managed *session.Session) (result Result, returnErr error) {
	if managed == nil {
		return Result{}, &RunError{Code: 1, Message: "session is unavailable"}
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

	stdinIsTerminal := term.IsTerminal(int(r.Stdin.Fd()))

	var oldState *term.State
	if stdinIsTerminal {
		var err error
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

	subscription := managed.Subscribe()
	defer subscription.Close()

	signalCh, stopSignals := r.signalChannel()
	defer stopSignals()

	inputCtx, cancelInput := context.WithCancel(ctx)
	defer cancelInput()
	inputDone := make(chan error, 1)
	go func() {
		inputErr := pumpInput(inputCtx, managed, r.Stdin)
		if errors.Is(inputErr, io.EOF) {
			if !stdinIsTerminal {
				if _, writeErr := managed.Write([]byte{4}); writeErr != nil && !isExpectedCloseError(writeErr) {
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

	outputDone := make(chan error, 1)
	go func() {
		if initial := subscription.Initial(); len(initial) > 0 {
			if _, writeErr := writeAll(r.Stdout, initial); writeErr != nil {
				outputDone <- fmt.Errorf("write terminal history: %w", writeErr)
				return
			}
		}
		for output := range subscription.Output() {
			if _, writeErr := writeAll(r.Stdout, output); writeErr != nil {
				outputDone <- fmt.Errorf("write terminal output: %w", writeErr)
				return
			}
		}
		outputDone <- subscription.Err()
	}()

	var (
		sessionEnded bool
		outputEnded  bool
		inputEnded   bool
		romeErr      error
		contextDone  = ctx.Done()
	)

	for !sessionEnded || !outputEnded {
		select {
		case <-contextDone:
			contextDone = nil

		case receivedSignal, ok := <-signalCh:
			if !ok {
				signalCh = nil
				continue
			}
			sig, ok := receivedSignal.(syscall.Signal)
			if !ok {
				continue
			}
			if sig == syscall.SIGWINCH {
				if stdinIsTerminal && !sessionEnded {
					if resizeErr := inheritTerminalSize(r.Stdin, managed); resizeErr != nil && !isExpectedCloseError(resizeErr) {
						r.debugf("failed to resize session PTY: %v", resizeErr)
					}
				}
				continue
			}
			if !sessionEnded {
				if signalErr := managed.Signal(sig); signalErr != nil && !errors.Is(signalErr, session.ErrClosed) {
					r.debugf("failed to forward %s: %v", sig, signalErr)
				}
			}

		case inputErr := <-inputDone:
			inputEnded = true
			inputDone = nil
			if inputErr != nil && !errors.Is(inputErr, context.Canceled) && !isExpectedCloseError(inputErr) {
				if romeErr == nil {
					romeErr = &RunError{Code: 1, Message: fmt.Sprintf("failed to read terminal input: %v", inputErr), Err: inputErr}
				}
				if !sessionEnded {
					_ = managed.Signal(syscall.SIGHUP)
				}
			}

		case outputErr := <-outputDone:
			outputEnded = true
			outputDone = nil
			if outputErr != nil && romeErr == nil {
				romeErr = &RunError{Code: 1, Message: fmt.Sprintf("failed to stream terminal output: %v", outputErr), Err: outputErr}
				if !sessionEnded {
					_ = managed.Signal(syscall.SIGHUP)
				}
			}

		case <-managed.Done():
			sessionEnded = true
			cancelInput()
		}
	}

	cancelInput()
	if !inputEnded {
		select {
		case inputErr := <-inputDone:
			if inputErr != nil && !errors.Is(inputErr, context.Canceled) && !isExpectedCloseError(inputErr) && romeErr == nil {
				romeErr = &RunError{Code: 1, Message: fmt.Sprintf("failed to stop terminal input: %v", inputErr), Err: inputErr}
			}
		case <-time.After(250 * time.Millisecond):
			if romeErr == nil {
				romeErr = &RunError{Code: 1, Message: "failed to stop terminal input"}
			}
		}
	}

	result, sessionErr := managed.Wait()
	if romeErr != nil {
		return Result{}, romeErr
	}
	if sessionErr != nil {
		return Result{}, sessionErr
	}
	return result, nil
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

type terminalResizer interface {
	Resize(cols, rows uint16) error
}

func inheritTerminalSize(source *os.File, target terminalResizer) error {
	size, err := pty.GetsizeFull(source)
	if err != nil {
		return err
	}
	return target.Resize(size.Cols, size.Rows)
}

func isExpectedCloseError(err error) bool {
	return errors.Is(err, os.ErrClosed) || errors.Is(err, syscall.EBADF) || errors.Is(err, syscall.EIO) || errors.Is(err, session.ErrClosed)
}

func managerCloseTimeout(gracePeriod time.Duration) time.Duration {
	if gracePeriod <= 0 {
		gracePeriod = 2 * time.Second
	}
	return gracePeriod + time.Second
}
