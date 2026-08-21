//go:build !darwin && !linux

package terminal

import (
	"context"

	"github.com/ompatel-24/rome/internal/session"
)

// Run reports the intentionally narrow platform scope of Rome V0.
func (r Runner) Run(_ context.Context, _ []string) (Result, error) {
	return Result{}, &RunError{Code: 1, Message: "this Rome milestone supports only macOS and Linux"}
}

func (r Runner) InitialSize() (session.StartOptions, error) {
	return session.StartOptions{}, &RunError{Code: 1, Message: "this Rome milestone supports only macOS and Linux"}
}

func (r Runner) Attach(_ context.Context, _ *session.Session) (Result, error) {
	return Result{}, &RunError{Code: 1, Message: "this Rome milestone supports only macOS and Linux"}
}
