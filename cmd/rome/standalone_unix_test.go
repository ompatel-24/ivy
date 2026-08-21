//go:build darwin || linux

package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/ompatel-24/rome/internal/protocol"
)

var standaloneAssetPattern = regexp.MustCompile(`(?:src|href)=["'](/assets/[^"'?#]+)["']`)

func TestStandaloneBinaryServesEmbeddedClient(t *testing.T) {
	if testing.Short() {
		t.Skip("standalone binary smoke test builds Rome")
	}

	packageDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("locate standalone test package: %v", err)
	}
	repositoryRoot := filepath.Clean(filepath.Join(packageDirectory, "..", ".."))
	distribution := t.TempDir()
	binary := filepath.Join(distribution, "rome")
	build := exec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/rome")
	build.Dir = repositoryRoot
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build standalone Rome: %v\n%s", err, output)
	}
	entries, err := os.ReadDir(distribution)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "rome" {
		t.Fatalf("standalone distribution entries = %v, want only rome", entryNames(entries))
	}

	command := exec.Command(binary, "/bin/sh")
	command.Dir = t.TempDir()
	command.Env = append(os.Environ(), "ROME_LISTEN=127.0.0.1:0", "ROME_WEB_DIR=", "TERM=dumb")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	var stdout bytes.Buffer
	command.Stdout = &stdout
	stderr, err := command.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start standalone Rome: %v", err)
	}
	defer stopStandaloneProcess(command)

	connectionURL := readStandaloneURL(t, stderr)
	parsed, err := url.Parse(connectionURL)
	if err != nil {
		t.Fatalf("parse connection URL: %v", err)
	}
	token := strings.TrimPrefix(parsed.Fragment, "token=")
	if token == parsed.Fragment || token == "" {
		t.Fatalf("connection URL has no fragment token: %q", connectionURL)
	}
	parsed.Fragment = ""
	sessionID := strings.TrimPrefix(parsed.Path, "/s/")
	if sessionID == parsed.Path || sessionID == "" {
		t.Fatalf("connection URL has invalid Session path: %q", parsed.Path)
	}

	page := standaloneGET(t, parsed.String(), "")
	references := standaloneAssetPattern.FindAllSubmatch(page, -1)
	if len(references) == 0 {
		t.Fatalf("embedded page contains no asset references: %q", page)
	}
	assetURL := parsed.Scheme + "://" + parsed.Host + string(references[0][1])
	if asset := standaloneGET(t, assetURL, ""); len(asset) == 0 {
		t.Fatal("embedded asset response is empty")
	}
	metadataURL := parsed.Scheme + "://" + parsed.Host + "/api/v1/sessions/" + sessionID
	metadata := standaloneGET(t, metadataURL, token)
	if !bytes.Contains(metadata, []byte(`"id":"`+sessionID+`"`)) {
		t.Fatalf("authenticated metadata = %q", metadata)
	}

	wsURL := "ws://" + parsed.Host + "/api/v1/sessions/" + sessionID + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	connection, response, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		Subprotocols: []string{protocol.Subprotocol, protocol.AuthPrefix + token},
	})
	cancel()
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatalf("connect to standalone WebSocket: %v", err)
	}
	defer connection.CloseNow()

	messageType, data := standaloneReadWS(t, connection)
	if messageType != websocket.MessageText {
		t.Fatalf("first WebSocket message type = %v, want text", messageType)
	}
	var hello protocol.Hello
	if err := json.Unmarshal(data, &hello); err != nil || hello.Type != "hello" {
		t.Fatalf("hello = %q, error=%v", data, err)
	}
	standaloneWriteWS(t, connection, []byte("printf 'ROME-STANDALONE\\n'\nexit\n"))

	var sawOutput, sawExit bool
	for !sawExit {
		messageType, data = standaloneReadWS(t, connection)
		if messageType == websocket.MessageBinary {
			sawOutput = sawOutput || bytes.Contains(data, []byte("ROME-STANDALONE"))
			continue
		}
		var exit protocol.Exit
		if err := json.Unmarshal(data, &exit); err == nil && exit.Type == "exit" {
			sawExit = true
			if exit.Code != 0 {
				t.Fatalf("standalone Session exit code = %d", exit.Code)
			}
		}
	}
	if !sawOutput {
		t.Fatalf("standalone WebSocket did not receive PTY output; local output=%q", stdout.String())
	}
	_ = connection.Close(websocket.StatusNormalClosure, "test complete")

	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("standalone Rome exit: %v", err)
		}
		command = nil
	case <-time.After(5 * time.Second):
		t.Fatal("standalone Rome did not exit")
	}
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name()
	}
	return names
}

func readStandaloneURL(t *testing.T, reader io.Reader) string {
	t.Helper()
	result := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(reader)
		if scanner.Scan() {
			result <- scanner.Text()
			return
		}
		result <- ""
	}()
	select {
	case line := <-result:
		const prefix = "rome: transport "
		if !strings.HasPrefix(line, prefix) {
			t.Fatalf("standalone transport output = %q", line)
		}
		return strings.TrimPrefix(line, prefix)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for standalone connection URL")
		return ""
	}
}

func standaloneGET(t *testing.T, address, token string) []byte {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, address, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", request.URL.Path, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, body=%q", request.URL.Path, response.StatusCode, body)
	}
	return body
}

func standaloneReadWS(t *testing.T, connection *websocket.Conn) (websocket.MessageType, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	messageType, data, err := connection.Read(ctx)
	if err != nil {
		t.Fatalf("standalone WebSocket read: %v", err)
	}
	return messageType, data
}

func standaloneWriteWS(t *testing.T, connection *websocket.Conn, data []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := connection.Write(ctx, websocket.MessageBinary, data); err != nil {
		t.Fatalf("standalone WebSocket write: %v", err)
	}
}

func stopStandaloneProcess(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = command.Process.Kill()
}
