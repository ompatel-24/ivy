//go:build !darwin && !linux

package terminal

import "context"

// Run reports the intentionally narrow platform scope of Ivy V0.
func (r Runner) Run(_ context.Context, _ []string) (Result, error) {
	return Result{}, &RunError{Code: 1, Message: "this Ivy milestone supports only macOS and Linux"}
}
