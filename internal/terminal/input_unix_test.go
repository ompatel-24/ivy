//go:build darwin || linux

package terminal

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

func TestPumpInputCancellation(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	t.Cleanup(func() {
		_ = reader.Close()
		_ = writer.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- pumpInput(ctx, io.Discard, reader)
	}()

	cancel()
	select {
	case pumpErr := <-done:
		if !errors.Is(pumpErr, context.Canceled) {
			t.Fatalf("pumpInput() error = %v, want context.Canceled", pumpErr)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("pumpInput() did not stop after cancellation")
	}
}

func TestWriteAllHandlesShortWrites(t *testing.T) {
	writer := &shortWriter{limit: 2}
	written, err := writeAll(writer, []byte("abcdef"))
	if err != nil {
		t.Fatalf("writeAll() unexpected error: %v", err)
	}
	if written != 6 || writer.value != "abcdef" {
		t.Fatalf("writeAll() = (%d, %q), want (6, %q)", written, writer.value, "abcdef")
	}
}

type shortWriter struct {
	limit int
	value string
}

func (w *shortWriter) Write(data []byte) (int, error) {
	if len(data) > w.limit {
		data = data[:w.limit]
	}
	w.value += string(data)
	return len(data), nil
}
