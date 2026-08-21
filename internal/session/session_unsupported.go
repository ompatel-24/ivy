//go:build !darwin && !linux

package session

import (
	"context"
	"errors"
	"os"
)

func start(_ context.Context, _ []string, _ StartOptions, _ normalizedOptions) (*Session, error) {
	return nil, &RunError{Code: 1, Message: "this Rome milestone supports only macOS and Linux"}
}

func (s *Session) Write(_ []byte) (int, error) {
	return 0, ErrClosed
}

func (s *Session) Resize(_, _ uint16) error {
	return ErrClosed
}

func (s *Session) Signal(_ os.Signal) error {
	return ErrClosed
}

func (s *Session) Close() error {
	if s == nil {
		return nil
	}
	return errors.New("this Rome milestone supports only macOS and Linux")
}
