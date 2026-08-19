//go:build darwin || linux

package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"
	"github.com/creack/pty"
	"github.com/ompatel-24/ivy/internal/protocol"
	"github.com/ompatel-24/ivy/internal/session"
)

const testToken = "test-token-that-is-never-written-by-the-server"

func TestServerHelperProcess(t *testing.T) {
	mode := os.Getenv("IVY_SERVER_TEST_HELPER")
	if mode == "" {
		return
	}

	switch mode {
	case "stream":
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "exit" {
				fmt.Fprintln(os.Stdout, "EXITING")
				return
			}
			fmt.Fprintf(os.Stdout, "OUT:%s\n", line)
		}

	case "resize":
		resizeSignals := make(chan os.Signal, 1)
		signal.Notify(resizeSignals, syscall.SIGWINCH)
		defer signal.Stop(resizeSignals)
		printServerTestSize("READY")
		<-resizeSignals
		printServerTestSize("RESIZED")

	case "burst":
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		payload := strings.Repeat("x", 16*1024)
		for index := 0; index < 256; index++ {
			fmt.Fprintf(os.Stdout, "%03d:%s\n", index, payload)
			time.Sleep(20 * time.Millisecond)
		}
		fmt.Fprintln(os.Stdout, "BURST-DONE")

	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", mode)
		os.Exit(98)
	}
}

func TestHTTPRoutesAndAuthentication(t *testing.T) {
	transport := startTestTransport(t, "stream", session.ManagerOptions{})

	response := httpRequest(t, http.MethodGet, transport.baseURL+"/health", "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /health status = %d", response.StatusCode)
	}
	healthBody := readBody(t, response)
	if bytes.Contains(healthBody, []byte(transport.token)) || bytes.Contains(healthBody, []byte(transport.id)) {
		t.Fatalf("health response leaked session data: %q", healthBody)
	}

	metadataURL := transport.metadataURL()
	response = httpRequest(t, http.MethodGet, metadataURL, "")
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated metadata status = %d, want 401", response.StatusCode)
	}
	_ = readBody(t, response)

	response = httpRequest(t, http.MethodGet, metadataURL, "wrong-token")
	wrongBody := readBody(t, response)
	if response.StatusCode != http.StatusUnauthorized || bytes.Contains(wrongBody, []byte(transport.token)) {
		t.Fatalf("invalid-token response = (%d, %q)", response.StatusCode, wrongBody)
	}

	response = httpRequest(t, http.MethodGet, metadataURL, transport.token)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("authenticated metadata status = %d", response.StatusCode)
	}
	if response.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("metadata endpoint unexpectedly enabled CORS")
	}
	var metadata protocol.SessionMetadata
	decodeBody(t, response, &metadata)
	if metadata.ID != transport.id || metadata.Command != filepath.Base(os.Args[0]) || metadata.State != string(session.StateRunning) {
		t.Fatalf("metadata = %+v", metadata)
	}
	if metadata.ExitCode != nil {
		t.Fatalf("running metadata exit code = %v, want nil", *metadata.ExitCode)
	}

	response = httpRequest(t, http.MethodGet, transport.baseURL+"/api/v1/sessions/not-a-session", transport.token)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown session status = %d, want 404", response.StatusCode)
	}
	_ = readBody(t, response)

	request, err := http.NewRequest(http.MethodPost, transport.baseURL+"/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST /health: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /health status = %d, want 405", response.StatusCode)
	}
}

func TestWebClientRoutesAndSecurityHeaders(t *testing.T) {
	transport := startTestTransport(t, "stream", session.ManagerOptions{})

	response := httpRequest(t, http.MethodGet, transport.baseURL+"/s/"+transport.id, "")
	body := readBody(t, response)
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte("Ivy test client")) {
		t.Fatalf("session page = (%d, %q)", response.StatusCode, body)
	}
	for _, header := range []string{"Content-Security-Policy", "Referrer-Policy", "X-Content-Type-Options", "X-Frame-Options"} {
		if response.Header.Get(header) == "" {
			t.Fatalf("session page missing %s", header)
		}
	}
	if response.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("session page unexpectedly enabled CORS")
	}

	response = httpRequest(t, http.MethodGet, transport.baseURL+"/assets/app.js", "")
	asset := readBody(t, response)
	if response.StatusCode != http.StatusOK || !bytes.Equal(asset, []byte("console.log('ivy')")) {
		t.Fatalf("web asset = (%d, %q)", response.StatusCode, asset)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "javascript") {
		t.Fatalf("asset Content-Type = %q", contentType)
	}

	response = httpRequest(t, http.MethodGet, transport.baseURL+"/s/AAAAAAAAAAAAAAAAAAAAAA", "")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown Session page status = %d", response.StatusCode)
	}
	if body := readBody(t, response); !bytes.Contains(body, []byte("Ivy test client")) {
		t.Fatalf("unknown Session page did not render the safe client shell: %q", body)
	}

	response = httpRequest(t, http.MethodGet, transport.baseURL+"/assets/missing.js", "")
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing asset status = %d", response.StatusCode)
	}
	if response.Header.Get("Content-Security-Policy") == "" || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("missing asset response omitted web security headers")
	}
	_ = readBody(t, response)

	response = httpRequest(t, http.MethodGet, transport.baseURL+"/assets/%2e%2e/index.html", "")
	if response.StatusCode == http.StatusOK {
		t.Fatal("asset traversal unexpectedly succeeded")
	}
	_ = readBody(t, response)

	response = httpRequest(t, http.MethodPost, transport.baseURL+"/s/"+transport.id, "")
	if response.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST Session page status = %d, want 405", response.StatusCode)
	}
	if response.Header.Get("Content-Security-Policy") == "" || response.Header.Get("X-Frame-Options") != "DENY" {
		t.Fatal("method error response omitted web security headers")
	}
	_ = readBody(t, response)
}

func TestHTTPAuthenticationRateLimit(t *testing.T) {
	transport := startTestTransport(t, "stream", session.ManagerOptions{})
	for attempt := 0; attempt < maxAuthenticationFailures; attempt++ {
		response := httpRequest(t, http.MethodGet, transport.metadataURL(), "wrong-token")
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatalf("attempt %d status = %d, want 401", attempt+1, response.StatusCode)
		}
		_ = readBody(t, response)
	}
	response := httpRequest(t, http.MethodGet, transport.metadataURL(), "wrong-token")
	defer response.Body.Close()
	if response.StatusCode != http.StatusTooManyRequests || response.Header.Get("Retry-After") == "" {
		t.Fatalf("rate-limited response = %d Retry-After=%q", response.StatusCode, response.Header.Get("Retry-After"))
	}
}

func TestWebSocketAuthenticationAndOrigin(t *testing.T) {
	transport := startTestTransport(t, "stream", session.ManagerOptions{})

	assertDialStatus(t, transport, []string{protocol.Subprotocol, protocol.AuthPrefix + "wrong"}, nil, http.StatusUnauthorized)
	assertDialStatus(t, transport, []string{protocol.AuthPrefix + transport.token}, nil, http.StatusBadRequest)
	assertDialStatus(t, transport, transport.protocols(), http.Header{"Origin": []string{"https://evil.example"}}, http.StatusForbidden)

	connection := transport.dial(t)
	defer connection.CloseNow()
	if connection.Subprotocol() != protocol.Subprotocol {
		t.Fatalf("negotiated subprotocol = %q", connection.Subprotocol())
	}
	hello := readHello(t, connection)
	if hello.Version != protocol.Version || hello.Session.ID != transport.id {
		t.Fatalf("hello = %+v", hello)
	}
}

func TestWebSocketInputOutputAndExit(t *testing.T) {
	transport := startTestTransport(t, "stream", session.ManagerOptions{})
	connection := transport.dial(t)
	defer connection.CloseNow()
	_ = readHello(t, connection)

	writeWS(t, connection, websocket.MessageBinary, []byte("alpha\nexit\n"))
	output, exit := readUntilExit(t, connection)
	if !bytes.Contains(output, []byte("OUT:alpha")) || !bytes.Contains(output, []byte("EXITING")) {
		t.Fatalf("terminal output = %q", output)
	}
	if exit.Code != 0 {
		t.Fatalf("exit code = %d, want 0", exit.Code)
	}
}

func TestTwoWebSocketsAndDisconnect(t *testing.T) {
	transport := startTestTransport(t, "stream", session.ManagerOptions{})
	first := transport.dial(t)
	second := transport.dial(t)
	defer second.CloseNow()
	_ = readHello(t, first)
	_ = readHello(t, second)

	writeWS(t, first, websocket.MessageBinary, []byte("shared\n"))
	firstOutput := readUntilContains(t, first, "OUT:shared")
	secondOutput := readUntilContains(t, second, "OUT:shared")
	if !bytes.Equal(firstOutput, secondOutput) {
		t.Fatalf("subscriber output differs:\nfirst=%q\nsecond=%q", firstOutput, secondOutput)
	}
	if err := first.Close(websocket.StatusNormalClosure, "disconnect"); err != nil {
		t.Fatalf("close first WebSocket: %v", err)
	}
	select {
	case <-transport.managed.Done():
		t.Fatal("WebSocket disconnect terminated the child")
	case <-time.After(50 * time.Millisecond):
	}

	writeWS(t, second, websocket.MessageBinary, []byte("after\nexit\n"))
	output, _ := readUntilExit(t, second)
	if !bytes.Contains(output, []byte("OUT:after")) {
		t.Fatalf("remaining client output = %q", output)
	}
}

func TestWebSocketReconnectGetsHistoryThenLiveOutput(t *testing.T) {
	transport := startTestTransport(t, "stream", session.ManagerOptions{HistoryBytes: 4096})
	first := transport.dial(t)
	_ = readHello(t, first)
	writeWS(t, first, websocket.MessageBinary, []byte("first\n"))
	_ = readUntilContains(t, first, "OUT:first")
	_ = first.Close(websocket.StatusNormalClosure, "reconnect")

	second := transport.dial(t)
	defer second.CloseNow()
	_ = readHello(t, second)
	historyType, history := readWS(t, second)
	if historyType != websocket.MessageBinary || !bytes.Contains(history, []byte("OUT:first")) {
		t.Fatalf("reconnect history = (%v, %q)", historyType, history)
	}
	writeWS(t, second, websocket.MessageBinary, []byte("second\nexit\n"))
	output, _ := readUntilExit(t, second)
	if !bytes.Contains(output, []byte("OUT:second")) {
		t.Fatalf("reconnect live output = %q", output)
	}
}

func TestWebSocketResize(t *testing.T) {
	transport := startTestTransport(t, "resize", session.ManagerOptions{})
	connection := transport.dial(t)
	defer connection.CloseNow()
	_ = readHello(t, connection)
	if output := readUntilContains(t, connection, "READY:80x24"); !bytes.Contains(output, []byte("READY:80x24")) {
		t.Fatalf("initial size output = %q", output)
	}
	writeWS(t, connection, websocket.MessageText, []byte(`{"type":"resize","cols":144,"rows":52}`))
	output, exit := readUntilExit(t, connection)
	if !bytes.Contains(output, []byte("RESIZED:144x52")) || exit.Code != 0 {
		t.Fatalf("resize output = %q, exit=%+v", output, exit)
	}
}

func TestWebSocketRejectsMalformedControlMessage(t *testing.T) {
	transport := startTestTransport(t, "stream", session.ManagerOptions{})
	connection := transport.dial(t)
	defer connection.CloseNow()
	_ = readHello(t, connection)
	writeWS(t, connection, websocket.MessageText, []byte(`{"type":"unknown"}`))

	messageType, data := readWS(t, connection)
	if messageType != websocket.MessageText {
		t.Fatalf("error message type = %v, want text", messageType)
	}
	var message protocol.Error
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatalf("decode error control: %v", err)
	}
	if message.Type != "error" || message.Code != "bad_message" {
		t.Fatalf("error control = %+v", message)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _, err := connection.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusPolicyViolation {
		t.Fatalf("close error = %v, status=%v", err, websocket.CloseStatus(err))
	}
	select {
	case <-transport.managed.Done():
		t.Fatal("malformed client message terminated the child")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestWebSocketRejectsOversizedInputWithoutStoppingSession(t *testing.T) {
	transport := startTestTransport(t, "stream", session.ManagerOptions{})
	connection := transport.dial(t)
	defer connection.CloseNow()
	_ = readHello(t, connection)
	writeWS(t, connection, websocket.MessageBinary, bytes.Repeat([]byte("x"), protocol.MaxInputBytes+1))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _, err := connection.Read(ctx)
	if websocket.CloseStatus(err) != websocket.StatusMessageTooBig {
		t.Fatalf("oversized input close = %v, status=%v", err, websocket.CloseStatus(err))
	}
	select {
	case <-transport.managed.Done():
		t.Fatal("oversized client input terminated the child")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestServerShutdownIsBoundedWithActiveWebSocket(t *testing.T) {
	transport := startTestTransport(t, "stream", session.ManagerOptions{})
	connection := transport.dial(t)
	defer connection.CloseNow()
	_ = readHello(t, connection)

	started := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	err := transport.server.Shutdown(ctx)
	cancel()
	if err != nil {
		t.Fatalf("Server.Shutdown(): %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Server.Shutdown() took %s, want bounded shutdown", elapsed)
	}
	select {
	case <-transport.managed.Done():
		t.Fatal("server shutdown terminated the child")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSlowWebSocketDoesNotStallHealthyClient(t *testing.T) {
	blocked := make(chan struct{})
	var hookMu sync.Mutex
	var slowServerConnection *websocket.Conn
	transport := startTestTransportWithHook(t, "burst", session.ManagerOptions{HistoryBytes: 128, SubscriberBuffer: 64}, func(transport *Server) {
		transport.beforeWrite = func(connection *websocket.Conn, messageType websocket.MessageType, _ []byte) {
			hookMu.Lock()
			if slowServerConnection == nil {
				slowServerConnection = connection
			}
			isSlow := connection == slowServerConnection
			hookMu.Unlock()
			if isSlow && messageType == websocket.MessageBinary {
				<-blocked
			}
		}
	})

	slow := transport.dial(t)
	defer slow.CloseNow()
	_ = readHello(t, slow)

	healthy := transport.dial(t)
	defer healthy.CloseNow()
	_ = readHello(t, healthy)
	writeWS(t, healthy, websocket.MessageBinary, []byte("start\n"))
	healthyOutput, healthyExit := readUntilExit(t, healthy)
	if healthyExit.Code != 0 || !bytes.Contains(healthyOutput, []byte("BURST-DONE")) {
		t.Fatalf("healthy client stalled or missed output: bytes=%d exit=%+v", len(healthyOutput), healthyExit)
	}
	close(blocked)

	for attempts := 0; attempts < 200; attempts++ {
		messageType, data := readWS(t, slow)
		if messageType != websocket.MessageText {
			continue
		}
		var message protocol.Error
		if err := json.Unmarshal(data, &message); err == nil && message.Type == "error" && message.Code == "slow_consumer" {
			return
		}
	}
	t.Fatal("slow WebSocket was not explicitly disconnected as a slow consumer")
}

type testTransport struct {
	manager   *session.Manager
	managed   *session.Session
	server    *Server
	listener  net.Listener
	serveDone chan error
	baseURL   string
	id        string
	token     string
}

func startTestTransport(t *testing.T, mode string, options session.ManagerOptions) *testTransport {
	t.Helper()
	return startTestTransportWithHook(t, mode, options, nil)
}

func startTestTransportWithHook(t *testing.T, mode string, options session.ManagerOptions, beforeServe func(*Server)) *testTransport {
	t.Helper()
	t.Setenv("IVY_SERVER_TEST_HELPER", mode)
	if options.GracePeriod == 0 {
		options.GracePeriod = 100 * time.Millisecond
	}
	manager, err := session.NewManager(options)
	if err != nil {
		t.Fatalf("session.NewManager(): %v", err)
	}
	managed, err := manager.Start(context.Background(), []string{os.Args[0], "-test.run=^TestServerHelperProcess$", "super-secret-argument"}, session.StartOptions{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatalf("Manager.Start(): %v", err)
	}
	listener, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen(): %v", err)
	}
	transport := New(manager, managed.Metadata().ID, credentialForToken(testToken), testWebAssets())
	if beforeServe != nil {
		beforeServe(transport)
	}
	serveDone := make(chan error, 1)
	go func() { serveDone <- transport.Serve(listener) }()

	result := &testTransport{
		manager:   manager,
		managed:   managed,
		server:    transport,
		listener:  listener,
		serveDone: serveDone,
		baseURL:   "http://" + listener.Addr().String(),
		id:        managed.Metadata().ID,
		token:     testToken,
	}
	t.Cleanup(func() {
		closeContext, closeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = manager.Close(closeContext)
		closeCancel()
		shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
		_ = transport.Shutdown(shutdownContext)
		shutdownCancel()
		_ = listener.Close()
		select {
		case <-serveDone:
		case <-time.After(time.Second):
			t.Error("transport server did not stop")
		}
	})
	return result
}

func testWebAssets() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte(`<!doctype html><title>Ivy test client</title><script src="/assets/app.js"></script>`)},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('ivy')")},
	}
}

func (t *testTransport) metadataURL() string {
	return t.baseURL + "/api/v1/sessions/" + t.id
}

func (t *testTransport) websocketURL() string {
	return "ws" + strings.TrimPrefix(t.metadataURL(), "http") + "/ws"
}

func (t *testTransport) protocols() []string {
	return []string{protocol.Subprotocol, protocol.AuthPrefix + t.token}
}

func (t *testTransport) dial(testingT *testing.T) *websocket.Conn {
	testingT.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	connection, response, err := websocket.Dial(ctx, t.websocketURL(), &websocket.DialOptions{Subprotocols: t.protocols()})
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		testingT.Fatalf("websocket.Dial(): %v", err)
	}
	return connection
}

func assertDialStatus(t *testing.T, transport *testTransport, protocols []string, headers http.Header, want int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	connection, response, err := websocket.Dial(ctx, transport.websocketURL(), &websocket.DialOptions{Subprotocols: protocols, HTTPHeader: headers})
	if connection != nil {
		connection.CloseNow()
	}
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err == nil || response == nil || response.StatusCode != want {
		t.Fatalf("websocket.Dial() = response %v, error %v, want HTTP %d", responseStatus(response), err, want)
	}
}

func responseStatus(response *http.Response) any {
	if response == nil {
		return nil
	}
	return response.StatusCode
}

func httpRequest(t *testing.T, method, target, token string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, target, nil)
	if err != nil {
		t.Fatalf("http.NewRequest(): %v", err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("HTTP request: %v", err)
	}
	return response
}

func readBody(t *testing.T, response *http.Response) []byte {
	t.Helper()
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return data
}

func decodeBody(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func readHello(t *testing.T, connection *websocket.Conn) protocol.Hello {
	t.Helper()
	messageType, data := readWS(t, connection)
	if messageType != websocket.MessageText {
		t.Fatalf("hello message type = %v, want text", messageType)
	}
	var hello protocol.Hello
	if err := json.Unmarshal(data, &hello); err != nil {
		t.Fatalf("decode hello: %v", err)
	}
	if hello.Type != "hello" {
		t.Fatalf("first control message = %+v, want hello", hello)
	}
	return hello
}

func readUntilContains(t *testing.T, connection *websocket.Conn, substring string) []byte {
	t.Helper()
	var output bytes.Buffer
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		messageType, data := readWS(t, connection)
		if messageType == websocket.MessageBinary {
			output.Write(data)
			if bytes.Contains(output.Bytes(), []byte(substring)) {
				return output.Bytes()
			}
		}
	}
	t.Fatalf("timed out waiting for %q in %q", substring, output.Bytes())
	return nil
}

func readUntilExit(t *testing.T, connection *websocket.Conn) ([]byte, protocol.Exit) {
	t.Helper()
	var output bytes.Buffer
	for attempts := 0; attempts < 10000; attempts++ {
		messageType, data := readWS(t, connection)
		if messageType == websocket.MessageBinary {
			output.Write(data)
			continue
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			t.Fatalf("decode control envelope: %v", err)
		}
		if envelope.Type == "error" {
			continue
		}
		if envelope.Type == "exit" {
			var exit protocol.Exit
			if err := json.Unmarshal(data, &exit); err != nil {
				t.Fatalf("decode exit: %v", err)
			}
			return output.Bytes(), exit
		}
	}
	t.Fatal("did not receive exit control message")
	return nil, protocol.Exit{}
}

func readWS(t *testing.T, connection *websocket.Conn) (websocket.MessageType, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	messageType, data, err := connection.Read(ctx)
	if err != nil {
		t.Fatalf("WebSocket read: %v", err)
	}
	return messageType, data
}

func writeWS(t *testing.T, connection *websocket.Conn, messageType websocket.MessageType, data []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := connection.Write(ctx, messageType, data); err != nil {
		t.Fatalf("WebSocket write: %v", err)
	}
}

func printServerTestSize(prefix string) {
	size, err := pty.GetsizeFull(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get size: %v\n", err)
		os.Exit(99)
	}
	fmt.Fprintf(os.Stdout, "%s:%dx%d\n", prefix, size.Cols, size.Rows)
}
