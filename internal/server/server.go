package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/ompatel-24/ivy/internal/protocol"
	"github.com/ompatel-24/ivy/internal/session"
)

const (
	readHeaderTimeout = 5 * time.Second
	idleTimeout       = 60 * time.Second
	writeTimeout      = 10 * time.Second
	maxHeaderBytes    = 16 * 1024
)

// Server exposes one authenticated Session over HTTP and WebSocket.
type Server struct {
	manager    *session.Manager
	sessionID  string
	credential Credential
	limiter    *failureLimiter
	web        fs.FS

	httpServer *http.Server
	ctx        context.Context
	cancel     context.CancelFunc

	connectionsMu sync.Mutex
	connections   map[*websocket.Conn]struct{}
	connectionsWG sync.WaitGroup
	closing       bool

	beforeWrite func(*websocket.Conn, websocket.MessageType, []byte)
}

func (s *Server) runBeforeWriteHook(connection *websocket.Conn, messageType websocket.MessageType, data []byte) {
	if s.beforeWrite != nil {
		s.beforeWrite(connection, messageType, data)
	}
}

func New(manager *session.Manager, sessionID string, credential Credential, web fs.FS) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	result := &Server{
		manager:     manager,
		sessionID:   sessionID,
		credential:  credential,
		limiter:     newFailureLimiter(),
		web:         web,
		ctx:         ctx,
		cancel:      cancel,
		connections: make(map[*websocket.Conn]struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", result.handleHealth)
	mux.HandleFunc("GET /s/{id}", result.handleSessionPage)
	mux.HandleFunc("GET /assets/{path...}", result.handleWebAsset)
	mux.HandleFunc("GET /api/v1/sessions/{id}", result.handleMetadata)
	mux.HandleFunc("GET /api/v1/sessions/{id}/ws", result.handleWebSocket)
	result.httpServer = &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if strings.HasPrefix(request.URL.Path, "/s/") || strings.HasPrefix(request.URL.Path, "/assets/") {
				setWebSecurityHeaders(w.Header())
			}
			mux.ServeHTTP(w, request)
		}),
		ReadHeaderTimeout: readHeaderTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          log.New(io.Discard, "", 0),
	}
	return result
}

func (s *Server) Serve(listener net.Listener) error {
	err := s.httpServer.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown stops accepting HTTP requests and gives active WebSockets until
// ctx expires to drain their final Session output and exit message.
func (s *Server) Shutdown(ctx context.Context) error {
	s.connectionsMu.Lock()
	s.closing = true
	s.connectionsMu.Unlock()

	httpErr := s.httpServer.Shutdown(ctx)
	if errors.Is(httpErr, context.DeadlineExceeded) || errors.Is(httpErr, context.Canceled) {
		_ = s.httpServer.Close()
		httpErr = nil
	}
	done := make(chan struct{})
	go func() {
		s.connectionsWG.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.cancel()
		return httpErr
	case <-ctx.Done():
		s.cancel()
		s.closeConnectionsNow()
		_ = s.httpServer.Close()
		return httpErr
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeHTTPJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleMetadata(w http.ResponseWriter, request *http.Request) {
	managed, ok := s.authorize(w, request, request.PathValue("id"), bearerToken(request.Header.Get("Authorization")))
	if !ok {
		return
	}
	writeHTTPJSON(w, http.StatusOK, sanitizedMetadata(managed))
}

func (s *Server) authorize(w http.ResponseWriter, request *http.Request, sessionID, token string) (*session.Session, bool) {
	if sessionID != s.sessionID {
		http.NotFound(w, request)
		return nil, false
	}
	managed, ok := s.manager.Get(sessionID)
	if !ok {
		http.NotFound(w, request)
		return nil, false
	}
	allowed, retryAfter := s.limiter.allowed(request.RemoteAddr)
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Round(time.Second)/time.Second))))
		http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
		return nil, false
	}
	if !s.credential.matches(token) {
		s.limiter.failed(request.RemoteAddr)
		w.Header().Set("WWW-Authenticate", `Bearer realm="ivy"`)
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return nil, false
	}
	s.limiter.succeeded(request.RemoteAddr)
	return managed, true
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func sanitizedMetadata(managed *session.Session) protocol.SessionMetadata {
	metadata := managed.Metadata()
	command := ""
	if len(metadata.Command) > 0 {
		command = filepath.Base(metadata.Command[0])
	}
	result := protocol.SessionMetadata{
		ID:        metadata.ID,
		Command:   command,
		Directory: metadata.Dir,
		State:     string(metadata.State),
	}
	if metadata.State == session.StateExited {
		exitCode := metadata.ExitCode
		result.ExitCode = &exitCode
	}
	return result
}

func writeHTTPJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (s *Server) trackConnection(connection *websocket.Conn) bool {
	s.connectionsMu.Lock()
	defer s.connectionsMu.Unlock()
	if s.closing {
		return false
	}
	s.connections[connection] = struct{}{}
	s.connectionsWG.Add(1)
	return true
}

func (s *Server) untrackConnection(connection *websocket.Conn) {
	s.connectionsMu.Lock()
	delete(s.connections, connection)
	s.connectionsMu.Unlock()
	s.connectionsWG.Done()
}

func (s *Server) closeConnectionsNow() {
	s.connectionsMu.Lock()
	connections := make([]*websocket.Conn, 0, len(s.connections))
	for connection := range s.connections {
		connections = append(connections, connection)
	}
	s.connectionsMu.Unlock()
	for _, connection := range connections {
		connection.CloseNow()
	}
}
