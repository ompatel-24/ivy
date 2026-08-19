// Package app coordinates Ivy's local terminal, Session, and optional network
// transport lifecycles.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/ompatel-24/ivy/internal/server"
	"github.com/ompatel-24/ivy/internal/session"
	"github.com/ompatel-24/ivy/internal/terminal"
)

const cleanupTimeout = 3 * time.Second

type Runner struct {
	Terminal      terminal.Runner
	ListenAddress string
	WebRoot       string

	serveTransport func(*server.Server, net.Listener) error
}

type terminalOutcome struct {
	result session.Result
	err    error
}

func (r Runner) Run(ctx context.Context, argv []string) (session.Result, error) {
	if r.ListenAddress == "" {
		return r.Terminal.Run(ctx, argv)
	}

	listener, err := server.Listen(r.ListenAddress)
	if err != nil {
		return session.Result{}, runError("failed to start local transport: %v", err)
	}
	defer listener.Close()

	webAssets, err := resolveWebAssets(r.WebRoot)
	if err != nil {
		return session.Result{}, runError("failed to load mobile client: %v", err)
	}
	if err := server.ValidateWebAssets(webAssets); err != nil {
		return session.Result{}, runError("failed to load mobile client: %v", err)
	}

	token, credential, err := server.NewCredential()
	if err != nil {
		return session.Result{}, runError("failed to create transport credential: %v", err)
	}
	startOptions, err := r.Terminal.InitialSize()
	if err != nil {
		return session.Result{}, err
	}
	manager, err := session.NewManager(session.ManagerOptions{GracePeriod: r.Terminal.GracePeriod})
	if err != nil {
		return session.Result{}, runError("failed to create session manager: %v", err)
	}

	runContext, cancel := context.WithCancel(ctx)
	defer cancel()
	managed, err := manager.Start(runContext, argv, startOptions)
	if err != nil {
		closeManager(manager)
		return session.Result{}, err
	}

	transport := server.New(manager, managed.Metadata().ID, credential, webAssets)
	connectionURL, err := server.ConnectionURL(listener.Addr(), managed.Metadata().ID, token)
	if err != nil {
		cancel()
		closeManager(manager)
		return session.Result{}, runError("failed to create transport URL: %v", err)
	}

	serveDone := make(chan error, 1)
	go func() {
		if r.serveTransport != nil {
			serveDone <- r.serveTransport(transport, listener)
			return
		}
		serveDone <- transport.Serve(listener)
	}()

	stderr := r.Terminal.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	fmt.Fprintf(stderr, "ivy: transport %s\n", connectionURL)

	terminalDone := make(chan terminalOutcome, 1)
	go func() {
		result, terminalErr := r.Terminal.Attach(runContext, managed)
		terminalDone <- terminalOutcome{result: result, err: terminalErr}
	}()

	var (
		outcome    terminalOutcome
		primaryErr error
	)
	select {
	case outcome = <-terminalDone:
	case serveErr := <-serveDone:
		if serveErr == nil {
			serveErr = errors.New("transport server stopped unexpectedly")
		}
		primaryErr = runError("local transport failed: %v", serveErr)
		cancel()
		outcome = <-terminalDone
	case <-ctx.Done():
		cancel()
		outcome = <-terminalDone
	}

	cancel()
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), cleanupTimeout)
	shutdownErr := transport.Shutdown(shutdownContext)
	shutdownCancel()
	managerErr := closeManager(manager)

	if primaryErr != nil {
		return session.Result{}, primaryErr
	}
	if outcome.err != nil {
		return session.Result{}, outcome.err
	}
	if shutdownErr != nil {
		return session.Result{}, runError("failed to stop local transport: %v", shutdownErr)
	}
	if managerErr != nil {
		return session.Result{}, runError("failed to close session manager: %v", managerErr)
	}
	return outcome.result, nil
}

func closeManager(manager *session.Manager) error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()
	return manager.Close(ctx)
}

func runError(format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	return &session.RunError{Code: 1, Message: message, Err: errors.New(message)}
}
