//go:build darwin || linux

package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/ompatel-24/rome/internal/pairing"
	"github.com/ompatel-24/rome/internal/protocol"
	"github.com/ompatel-24/rome/internal/server"
	"github.com/ompatel-24/rome/internal/terminal"
)

func TestAppHelperProcess(t *testing.T) {
	if os.Getenv("ROME_APP_TEST_HELPER") == "" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "exit" {
			fmt.Fprintln(os.Stdout, "APP-EXITING")
			return
		}
		fmt.Fprintf(os.Stdout, "APP-OUT:%s\n", line)
	}
}

func TestRunnerWithoutListenAddressStaysQuiet(t *testing.T) {
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	var stdout, stderr lockedBuffer
	runner := Runner{
		WebRoot:  t.TempDir(),
		Terminal: terminal.Runner{Stdin: stdin, Stdout: &stdout, Stderr: &stderr},
		formatPairing: func(string, pairing.Options) (string, error) {
			t.Fatal("network-disabled Runner attempted to format pairing output")
			return "", nil
		},
	}

	result, err := runner.Run(context.Background(), []string{"/usr/bin/true"})
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("Runner.Run() = (%+v, %v)", result, err)
	}
	if stderr.String() != "" {
		t.Fatalf("network-disabled stderr = %q, want empty", stderr.String())
	}
}

func TestRunnerFormatsPairingBeforeTerminalOutput(t *testing.T) {
	t.Setenv("ROME_APP_TEST_HELPER", "1")
	t.Setenv("TERM", "xterm-256color")
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdinReader.Close()
	if _, err := io.WriteString(stdinWriter, "exit\n"); err != nil {
		t.Fatal(err)
	}
	_ = stdinWriter.Close()

	var combined lockedBuffer
	var receivedOptions pairing.Options
	runner := Runner{
		ListenAddress: "127.0.0.1:0",
		Terminal: terminal.Runner{
			Stdin:       stdinReader,
			Stdout:      &combined,
			Stderr:      &combined,
			GracePeriod: 100 * time.Millisecond,
		},
		pairingIsTTY: func(io.Writer) bool { return true },
		formatPairing: func(connectionURL string, options pairing.Options) (string, error) {
			receivedOptions = options
			if !strings.Contains(connectionURL, "/s/") || !strings.Contains(connectionURL, "#token=") {
				t.Fatalf("pairing URL = %q", connectionURL)
			}
			return "PAIRING-BEFORE-RAW\n", nil
		},
	}

	result, runErr := runner.Run(context.Background(), []string{os.Args[0], "-test.run=^TestAppHelperProcess$"})
	if runErr != nil || result.ExitCode != 0 {
		t.Fatalf("Runner.Run() = (%+v, %v)", result, runErr)
	}
	output := combined.String()
	pairingIndex := strings.Index(output, "PAIRING-BEFORE-RAW")
	childIndex := strings.Index(output, "APP-EXITING")
	if pairingIndex < 0 || childIndex < 0 || pairingIndex > childIndex {
		t.Fatalf("pairing output did not precede terminal history: %q", output)
	}
	if !receivedOptions.Interactive || receivedOptions.Columns != 80 || receivedOptions.Term != "xterm-256color" {
		t.Fatalf("pairing options = %+v", receivedOptions)
	}
}

func TestRunnerStopsChildWhenPairingGenerationFails(t *testing.T) {
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	var stdout, stderr lockedBuffer
	runner := Runner{
		ListenAddress: "127.0.0.1:0",
		Terminal: terminal.Runner{
			Stdin:       stdin,
			Stdout:      &stdout,
			Stderr:      &stderr,
			GracePeriod: 100 * time.Millisecond,
		},
		pairingIsTTY: func(io.Writer) bool { return true },
		formatPairing: func(string, pairing.Options) (string, error) {
			return "", errors.New("injected QR failure")
		},
	}

	started := time.Now()
	_, runErr := runner.Run(context.Background(), []string{"/bin/sh", "-c", "sleep 30"})
	if runErr == nil || terminal.ErrorCode(runErr) != 1 || !strings.Contains(runErr.Error(), "failed to create pairing QR code") {
		t.Fatalf("Runner.Run() error = %v", runErr)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("Runner.Run() took %s after pairing failure", elapsed)
	}
	if stdout.String() != "" || stderr.String() != "" {
		t.Fatalf("pairing failure produced output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunnerTransportEndToEnd(t *testing.T) {
	t.Setenv("ROME_APP_TEST_HELPER", "1")
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdinReader.Close()
	defer stdinWriter.Close()
	var stdout, stderr lockedBuffer
	runner := Runner{
		ListenAddress: "127.0.0.1:0",
		Terminal: terminal.Runner{
			Stdin:       stdinReader,
			Stdout:      &stdout,
			Stderr:      &stderr,
			GracePeriod: 100 * time.Millisecond,
		},
	}

	done := make(chan terminalOutcome, 1)
	go func() {
		result, runErr := runner.Run(context.Background(), []string{os.Args[0], "-test.run=^TestAppHelperProcess$"})
		done <- terminalOutcome{result: result, err: runErr}
	}()

	connectionURL := waitForConnectionURL(t, &stderr)
	parsed, err := url.Parse(connectionURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", connectionURL, err)
	}
	if parsed.RawQuery != "" || !strings.HasPrefix(parsed.Fragment, "token=") {
		t.Fatalf("connection URL placed token outside fragment: %q", connectionURL)
	}
	token := strings.TrimPrefix(parsed.Fragment, "token=")
	parsed.Fragment = ""
	sessionID := strings.TrimPrefix(parsed.Path, "/s/")
	if sessionID == parsed.Path || sessionID == "" {
		t.Fatalf("connection URL path = %q, want /s/<id>", parsed.Path)
	}

	healthURL := parsed.Scheme + "://" + parsed.Host + "/health"
	response, err := http.Get(healthURL)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /health status = %d", response.StatusCode)
	}
	response, err = http.Get(parsed.String())
	if err != nil {
		t.Fatalf("GET Session page: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET Session page status = %d", response.StatusCode)
	}

	wsURL := "ws://" + parsed.Host + "/api/v1/sessions/" + sessionID + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	connection, response, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{protocol.Subprotocol, protocol.AuthPrefix + token},
	})
	cancel()
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatalf("websocket.Dial(): %v", err)
	}
	defer connection.CloseNow()

	messageType, data := appReadWS(t, connection)
	if messageType != websocket.MessageText {
		t.Fatalf("hello message type = %v", messageType)
	}
	var hello protocol.Hello
	if err := json.Unmarshal(data, &hello); err != nil || hello.Type != "hello" {
		t.Fatalf("hello = %q, error=%v", data, err)
	}
	appWriteWS(t, connection, websocket.MessageBinary, []byte("exit\n"))

	var sawOutput, sawExit bool
	for !sawExit {
		messageType, data = appReadWS(t, connection)
		if messageType == websocket.MessageBinary {
			sawOutput = sawOutput || bytes.Contains(data, []byte("APP-EXITING"))
			continue
		}
		var exit protocol.Exit
		if err := json.Unmarshal(data, &exit); err == nil && exit.Type == "exit" {
			sawExit = true
		}
	}
	if !sawOutput {
		t.Fatalf("WebSocket did not receive helper output; local output=%q", stdout.String())
	}
	_ = connection.Close(websocket.StatusNormalClosure, "session complete")

	select {
	case outcome := <-done:
		if outcome.err != nil || outcome.result.ExitCode != 0 {
			t.Fatalf("Runner.Run() = (%+v, %v)", outcome.result, outcome.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Runner.Run() did not finish")
	}
	if lines := strings.Count(strings.TrimSpace(stderr.String()), "\n") + 1; lines != 1 {
		t.Fatalf("transport printed %d stderr lines: %q", lines, stderr.String())
	}
}

func TestRunnerStopsSessionWhenTransportFailsUnexpectedly(t *testing.T) {
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	var stdout, stderr lockedBuffer
	runner := Runner{
		ListenAddress: "127.0.0.1:0",
		WebRoot:       testWebRoot(t),
		Terminal: terminal.Runner{
			Stdin:       stdin,
			Stdout:      &stdout,
			Stderr:      &stderr,
			GracePeriod: 100 * time.Millisecond,
		},
		serveTransport: func(_ *server.Server, _ net.Listener) error {
			return errors.New("injected transport failure")
		},
	}

	started := time.Now()
	_, runErr := runner.Run(context.Background(), []string{"/bin/sh", "-c", "sleep 30"})
	if runErr == nil || terminal.ErrorCode(runErr) != 1 || !strings.Contains(runErr.Error(), "local transport failed") {
		t.Fatalf("Runner.Run() error = %v, want transport failure with exit code 1", runErr)
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("Runner.Run() took %s after transport failure", elapsed)
	}
	if !strings.Contains(stderr.String(), "rome: transport http://") {
		t.Fatalf("transport failed before the child was launched: stderr=%q", stderr.String())
	}
}

func TestRunnerRejectsMissingWebAssetsBeforeLaunchingCommand(t *testing.T) {
	stdin, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	var stdout, stderr lockedBuffer
	webRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte(`<script src="/assets/missing.js"></script>`), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := Runner{
		ListenAddress: "127.0.0.1:0",
		WebRoot:       webRoot,
		Terminal:      terminal.Runner{Stdin: stdin, Stdout: &stdout, Stderr: &stderr},
	}

	_, runErr := runner.Run(context.Background(), []string{"rome-command-that-must-not-launch"})
	if runErr == nil || terminal.ErrorCode(runErr) != 1 || !strings.Contains(runErr.Error(), "failed to load mobile client") {
		t.Fatalf("Runner.Run() error = %v, want mobile-client asset failure", runErr)
	}
	if strings.Contains(runErr.Error(), "command not found") || stderr.String() != "" {
		t.Fatalf("child launch or banner occurred before asset validation: error=%v stderr=%q", runErr, stderr.String())
	}
}

func testWebRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte(`<!doctype html><title>Rome test</title><script src="/assets/app.js"></script>`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("console.log('rome')"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func waitForConnectionURL(t *testing.T, buffer *lockedBuffer) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		output := buffer.String()
		if newline := strings.IndexByte(output, '\n'); newline >= 0 {
			line := output[:newline]
			const prefix = "rome: transport "
			if !strings.HasPrefix(line, prefix) {
				t.Fatalf("transport banner = %q", line)
			}
			return strings.TrimPrefix(line, prefix)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for transport banner; stderr=%q", buffer.String())
	return ""
}

func appReadWS(t *testing.T, connection *websocket.Conn) (websocket.MessageType, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	messageType, data, err := connection.Read(ctx)
	if err != nil {
		t.Fatalf("WebSocket read: %v", err)
	}
	return messageType, data
}

func appWriteWS(t *testing.T, connection *websocket.Conn, messageType websocket.MessageType, data []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := connection.Write(ctx, messageType, data); err != nil {
		t.Fatalf("WebSocket write: %v", err)
	}
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}
