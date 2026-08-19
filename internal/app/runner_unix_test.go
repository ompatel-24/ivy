//go:build darwin || linux

package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/ompatel-24/ivy/internal/protocol"
	"github.com/ompatel-24/ivy/internal/server"
	"github.com/ompatel-24/ivy/internal/terminal"
)

func TestAppHelperProcess(t *testing.T) {
	if os.Getenv("IVY_APP_TEST_HELPER") == "" {
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
	runner := Runner{Terminal: terminal.Runner{Stdin: stdin, Stdout: &stdout, Stderr: &stderr}}

	result, err := runner.Run(context.Background(), []string{"/usr/bin/true"})
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("Runner.Run() = (%+v, %v)", result, err)
	}
	if stderr.String() != "" {
		t.Fatalf("network-disabled stderr = %q, want empty", stderr.String())
	}
}

func TestRunnerTransportEndToEnd(t *testing.T) {
	t.Setenv("IVY_APP_TEST_HELPER", "1")
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

	healthURL := parsed.Scheme + "://" + parsed.Host + "/health"
	response, err := http.Get(healthURL)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /health status = %d", response.StatusCode)
	}

	wsURL := "ws://" + parsed.Host + parsed.Path + "/ws"
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
	if !strings.Contains(stderr.String(), "ivy: transport http://") {
		t.Fatalf("transport failed before the child was launched: stderr=%q", stderr.String())
	}
}

func waitForConnectionURL(t *testing.T, buffer *lockedBuffer) string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		output := buffer.String()
		if newline := strings.IndexByte(output, '\n'); newline >= 0 {
			line := output[:newline]
			const prefix = "ivy: transport "
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
