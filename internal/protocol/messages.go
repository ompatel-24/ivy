// Package protocol defines Rome's versioned WebSocket control messages.
// Terminal input and output remain raw binary WebSocket messages.
package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	Version         = 1
	Subprotocol     = "rome.v1"
	AuthPrefix      = "rome.auth."
	MaxControlBytes = 4 * 1024
	MaxInputBytes   = 64 * 1024
)

// SessionMetadata is the sanitized session information exposed to an
// authenticated transport client.
type SessionMetadata struct {
	ID        string `json:"id"`
	Command   string `json:"command"`
	Directory string `json:"directory"`
	State     string `json:"state"`
	ExitCode  *int   `json:"exitCode"`
}

type Hello struct {
	Type    string          `json:"type"`
	Version int             `json:"version"`
	Session SessionMetadata `json:"session"`
}

func NewHello(metadata SessionMetadata) Hello {
	return Hello{Type: "hello", Version: Version, Session: metadata}
}

type Exit struct {
	Type string `json:"type"`
	Code int    `json:"code"`
}

func NewExit(code int) Exit {
	return Exit{Type: "exit", Code: code}
}

type Error struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewError(code, message string) Error {
	return Error{Type: "error", Code: code, Message: message}
}

type Resize struct {
	Type string `json:"type"`
	Cols int    `json:"cols"`
	Rows int    `json:"rows"`
}

// DecodeResize strictly decodes the only client text message supported in
// protocol version 1.
func DecodeResize(data []byte) (Resize, error) {
	if len(data) > MaxControlBytes {
		return Resize{}, fmt.Errorf("control message exceeds %d bytes", MaxControlBytes)
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var resize Resize
	if err := decoder.Decode(&resize); err != nil {
		return Resize{}, fmt.Errorf("decode control message: %w", err)
	}
	if err := requireJSONEnd(decoder); err != nil {
		return Resize{}, err
	}
	if resize.Type != "resize" {
		return Resize{}, fmt.Errorf("unsupported control message type %q", resize.Type)
	}
	if resize.Cols < 1 || resize.Cols > 65535 || resize.Rows < 1 || resize.Rows > 65535 {
		return Resize{}, errors.New("resize dimensions must be between 1 and 65535")
	}
	return resize, nil
}

func requireJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("control message contains multiple JSON values")
	}
	return fmt.Errorf("decode trailing control data: %w", err)
}
