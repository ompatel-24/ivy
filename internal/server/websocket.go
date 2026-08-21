package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"syscall"

	"github.com/coder/websocket"
	"github.com/ompatel-24/rome/internal/protocol"
	"github.com/ompatel-24/rome/internal/session"
)

type clientReadError struct {
	code    string
	message string
	err     error
}

func (e *clientReadError) Error() string {
	return e.err.Error()
}

func (s *Server) handleWebSocket(w http.ResponseWriter, request *http.Request) {
	protocols := requestedSubprotocols(request.Header.Values("Sec-WebSocket-Protocol"))
	if !containsProtocol(protocols, protocol.Subprotocol) {
		http.Error(w, "required WebSocket subprotocol rome.v1 was not offered", http.StatusBadRequest)
		return
	}
	token, ok := authenticationSubprotocol(protocols)
	if !ok {
		token = ""
	}
	managed, authorized := s.authorize(w, request, request.PathValue("id"), token)
	if !authorized {
		return
	}

	connection, err := websocket.Accept(w, request, &websocket.AcceptOptions{
		Subprotocols:    []string{protocol.Subprotocol},
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return
	}
	if !s.trackConnection(connection) {
		_ = connection.Close(websocket.StatusGoingAway, "server is shutting down")
		return
	}
	defer s.untrackConnection(connection)
	defer connection.CloseNow()
	connection.SetReadLimit(protocol.MaxInputBytes)

	s.serveWebSocket(connection, managed)
}

func (s *Server) serveWebSocket(connection *websocket.Conn, managed *session.Session) {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	subscription := managed.Subscribe()
	defer subscription.Close()

	if err := s.writeJSON(ctx, connection, protocol.NewHello(sanitizedMetadata(managed))); err != nil {
		return
	}
	if history := subscription.Initial(); len(history) > 0 {
		if err := s.writeWebSocket(ctx, connection, websocket.MessageBinary, history); err != nil {
			return
		}
	}

	readDone := make(chan error, 1)
	go func() {
		readDone <- readClient(ctx, connection, managed)
	}()

	for {
		select {
		case <-s.ctx.Done():
			_ = connection.Close(websocket.StatusGoingAway, "server is shutting down")
			return

		case readErr := <-readDone:
			if readErr == nil || isNormalWebSocketClose(readErr) {
				return
			}
			var clientErr *clientReadError
			if errors.As(readErr, &clientErr) {
				_ = s.writeJSON(ctx, connection, protocol.NewError(clientErr.code, clientErr.message))
				_ = connection.Close(websocket.StatusPolicyViolation, clientErr.message)
				return
			}
			_ = s.writeJSON(ctx, connection, protocol.NewError("input_failure", "session input failed"))
			_ = connection.Close(websocket.StatusInternalError, "session input failed")
			return

		case output, open := <-subscription.Output():
			if open {
				if err := s.writeWebSocket(ctx, connection, websocket.MessageBinary, output); err != nil {
					return
				}
				continue
			}
			if errors.Is(subscription.Err(), session.ErrSlowSubscriber) {
				_ = s.writeJSON(ctx, connection, protocol.NewError("slow_consumer", "client could not keep up with terminal output"))
				_ = connection.Close(websocket.StatusPolicyViolation, "slow consumer")
				return
			}
			result, waitErr := managed.Wait()
			if waitErr != nil {
				_ = s.writeJSON(ctx, connection, protocol.NewError("session_failure", "session ended with an Rome error"))
			}
			if err := s.writeJSON(ctx, connection, protocol.NewExit(result.ExitCode)); err != nil {
				return
			}
			_ = connection.Close(websocket.StatusNormalClosure, "session exited")
			return
		}
	}
}

func readClient(ctx context.Context, connection *websocket.Conn, managed *session.Session) error {
	for {
		messageType, data, err := connection.Read(ctx)
		if err != nil {
			return err
		}
		switch messageType {
		case websocket.MessageBinary:
			if len(data) > protocol.MaxInputBytes {
				return &clientReadError{code: "message_too_large", message: "input message is too large", err: errors.New("input message is too large")}
			}
			if _, err := managed.Write(data); err != nil {
				if errors.Is(err, session.ErrClosed) {
					return nil
				}
				return fmt.Errorf("write session input: %w", err)
			}

		case websocket.MessageText:
			resize, err := protocol.DecodeResize(data)
			if err != nil {
				return &clientReadError{code: "bad_message", message: "invalid control message", err: err}
			}
			if err := managed.Resize(uint16(resize.Cols), uint16(resize.Rows)); err != nil {
				if errors.Is(err, session.ErrClosed) {
					return nil
				}
				return fmt.Errorf("resize session: %w", err)
			}

		default:
			return &clientReadError{code: "bad_message", message: "unsupported WebSocket message type", err: errors.New("unsupported WebSocket message type")}
		}
	}
}

func requestedSubprotocols(values []string) []string {
	var result []string
	for _, value := range values {
		for _, candidate := range strings.Split(value, ",") {
			candidate = strings.TrimSpace(candidate)
			if candidate != "" {
				result = append(result, candidate)
			}
		}
	}
	return result
}

func containsProtocol(protocols []string, expected string) bool {
	for _, candidate := range protocols {
		if candidate == expected {
			return true
		}
	}
	return false
}

func authenticationSubprotocol(protocols []string) (string, bool) {
	var token string
	for _, candidate := range protocols {
		if !strings.HasPrefix(candidate, protocol.AuthPrefix) {
			continue
		}
		if token != "" {
			return "", false
		}
		token = strings.TrimPrefix(candidate, protocol.AuthPrefix)
		if token == "" {
			return "", false
		}
	}
	return token, token != ""
}

func (s *Server) writeJSON(ctx context.Context, connection *websocket.Conn, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.writeWebSocket(ctx, connection, websocket.MessageText, data)
}

func (s *Server) writeWebSocket(ctx context.Context, connection *websocket.Conn, messageType websocket.MessageType, data []byte) error {
	s.runBeforeWriteHook(connection, messageType, data)
	writeContext, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	return connection.Write(writeContext, messageType, data)
}

func isNormalWebSocketClose(err error) bool {
	status := websocket.CloseStatus(err)
	return status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway || status == websocket.StatusNoStatusRcvd || errors.Is(err, context.Canceled) || errors.Is(err, syscall.ECONNRESET)
}
