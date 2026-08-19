//go:build darwin || linux

package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

const inputPollIntervalMilliseconds = 50

// pumpInput copies local terminal bytes while remaining cancellable even when
// no key is being pressed. A plain io.Copy goroutine can remain blocked in a
// terminal read after the child exits.
func pumpInput(ctx context.Context, dst io.Writer, src *os.File) error {
	pollDescriptors := []unix.PollFd{{
		Fd:     int32(src.Fd()),
		Events: unix.POLLIN | unix.POLLHUP | unix.POLLERR,
	}}
	buffer := make([]byte, 32*1024)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ready, err := unix.Poll(pollDescriptors, inputPollIntervalMilliseconds)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			return fmt.Errorf("poll input: %w", err)
		}
		if ready == 0 {
			continue
		}

		revents := pollDescriptors[0].Revents
		if revents&unix.POLLNVAL != 0 {
			return os.ErrClosed
		}
		if revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR) == 0 {
			continue
		}

		read, readErr := src.Read(buffer)
		if read > 0 {
			if _, writeErr := writeAll(dst, buffer[:read]); writeErr != nil {
				return fmt.Errorf("write PTY input: %w", writeErr)
			}
		}
		if readErr != nil {
			return readErr
		}
		if read == 0 && revents&unix.POLLHUP != 0 {
			return io.EOF
		}
	}
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
