//go:build darwin || linux

package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestSessionHelperProcess(t *testing.T) {
	mode := os.Getenv("ROME_SESSION_TEST_HELPER")
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

	case "burst":
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		payload := strings.Repeat("x", 2048)
		for index := 0; index < 24; index++ {
			fmt.Fprintf(os.Stdout, "%02d:%s\n", index, payload)
			time.Sleep(5 * time.Millisecond)
		}
		fmt.Fprintln(os.Stdout, "BURST-DONE")

	case "resize":
		resizeSignals := make(chan os.Signal, 1)
		signal.Notify(resizeSignals, syscall.SIGWINCH)
		defer signal.Stop(resizeSignals)
		printSessionSize("READY")
		<-resizeSignals
		printSessionSize("RESIZED")

	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", mode)
		os.Exit(98)
	}
}

func TestSessionMetadataAndManagerLifecycle(t *testing.T) {
	manager := newTestManager(t, ManagerOptions{})
	argv := sessionHelperArgv(t, "stream")
	started, err := manager.Start(context.Background(), argv, StartOptions{Rows: 31, Cols: 91})
	if err != nil {
		t.Fatalf("Manager.Start() unexpected error: %v", err)
	}

	metadata := started.Metadata()
	decoded, err := base64.RawURLEncoding.DecodeString(metadata.ID)
	if err != nil || len(decoded) != 16 {
		t.Fatalf("session ID %q did not decode to 128 bits: bytes=%d err=%v", metadata.ID, len(decoded), err)
	}
	if metadata.State != StateRunning {
		t.Fatalf("Metadata().State = %q, want %q", metadata.State, StateRunning)
	}
	if strings.Join(metadata.Command, "\x00") != strings.Join(argv, "\x00") {
		t.Fatalf("Metadata().Command = %q, want %q", metadata.Command, argv)
	}
	metadata.Command[0] = "mutated"
	if started.Metadata().Command[0] != argv[0] {
		t.Fatal("Metadata() exposed mutable command storage")
	}
	if metadata.Dir == "" {
		t.Fatal("Metadata().Dir is empty")
	}
	if resolved, ok := manager.Get(metadata.ID); !ok || resolved != started {
		t.Fatalf("Manager.Get(%q) did not return the started session", metadata.ID)
	}

	if _, err := started.Write([]byte("exit\n")); err != nil {
		t.Fatalf("Session.Write(exit): %v", err)
	}
	result, err := waitSession(t, started)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("Session.Wait() = (%+v, %v), want exit 0", result, err)
	}
	exited := started.Metadata()
	if exited.State != StateExited || exited.ExitCode != 0 {
		t.Fatalf("exited metadata = %+v, want exited with code 0", exited)
	}

	closeContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.Close(closeContext); err != nil {
		t.Fatalf("Manager.Close(): %v", err)
	}
	if _, err := manager.Start(context.Background(), argv, StartOptions{}); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("Manager.Start() after Close error = %v, want ErrManagerClosed", err)
	}
}

func TestSessionIDsAreUnique(t *testing.T) {
	manager := newTestManager(t, ManagerOptions{})
	argv := sessionHelperArgv(t, "stream")
	first, err := manager.Start(context.Background(), argv, StartOptions{})
	if err != nil {
		t.Fatalf("start first session: %v", err)
	}
	second, err := manager.Start(context.Background(), argv, StartOptions{})
	if err != nil {
		t.Fatalf("start second session: %v", err)
	}
	if first.Metadata().ID == second.Metadata().ID {
		t.Fatalf("two sessions received duplicate ID %q", first.Metadata().ID)
	}
	closeManager(t, manager)
}

func TestSessionBroadcastsIdenticalOutput(t *testing.T) {
	manager := newTestManager(t, ManagerOptions{})
	started := startTestSession(t, manager, "stream")
	first := newCollector(started.Subscribe())
	second := newCollector(started.Subscribe())

	if _, err := started.Write([]byte("alpha\nexit\n")); err != nil {
		t.Fatalf("Session.Write(): %v", err)
	}
	result, err := waitSession(t, started)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("Session.Wait() = (%+v, %v), want exit 0", result, err)
	}
	first.wait(t)
	second.wait(t)
	if !bytes.Equal(first.bytes(), second.bytes()) {
		t.Fatalf("subscriber outputs differ:\nfirst=%q\nsecond=%q", first.bytes(), second.bytes())
	}
	if !bytes.Contains(first.bytes(), []byte("OUT:alpha")) || !bytes.Contains(first.bytes(), []byte("EXITING")) {
		t.Fatalf("subscriber output missing child data: %q", first.bytes())
	}
}

func TestLateSubscriberGetsHistoryThenGapFreeLiveOutput(t *testing.T) {
	manager := newTestManager(t, ManagerOptions{HistoryBytes: 4096})
	started := startTestSession(t, manager, "stream")
	early := newCollector(started.Subscribe())

	if _, err := started.Write([]byte("first\n")); err != nil {
		t.Fatalf("write first input: %v", err)
	}
	early.waitFor(t, "OUT:first")

	lateSubscription := started.Subscribe()
	late := newCollector(lateSubscription)
	if !bytes.Contains(lateSubscription.Initial(), []byte("OUT:first")) {
		t.Fatalf("late history = %q, want first output", lateSubscription.Initial())
	}
	if _, err := started.Write([]byte("second\nexit\n")); err != nil {
		t.Fatalf("write live input: %v", err)
	}
	if _, err := waitSession(t, started); err != nil {
		t.Fatalf("Session.Wait(): %v", err)
	}
	early.wait(t)
	late.wait(t)
	lateOutput := late.bytes()
	if bytes.Count(lateOutput, []byte("OUT:first")) != 1 {
		t.Fatalf("late output contains first response %d times, want once: %q", bytes.Count(lateOutput, []byte("OUT:first")), lateOutput)
	}
	if !bytes.Contains(lateOutput, []byte("OUT:second")) || !bytes.Contains(lateOutput, []byte("EXITING")) {
		t.Fatalf("late output has a history/live gap: %q", lateOutput)
	}
}

func TestDisconnectDoesNotTerminateSession(t *testing.T) {
	manager := newTestManager(t, ManagerOptions{})
	started := startTestSession(t, manager, "stream")
	subscription := started.Subscribe()
	subscription.Close()
	select {
	case <-started.Done():
		t.Fatal("closing a subscriber terminated the child process")
	case <-time.After(50 * time.Millisecond):
	}
	if _, err := started.Write([]byte("exit\n")); err != nil {
		t.Fatalf("session stopped accepting input after disconnect: %v", err)
	}
	if _, err := waitSession(t, started); err != nil {
		t.Fatalf("Session.Wait(): %v", err)
	}
}

func TestSlowSubscriberDoesNotStallHealthySubscriber(t *testing.T) {
	manager := newTestManager(t, ManagerOptions{HistoryBytes: 128, SubscriberBuffer: 4})
	started := startTestSession(t, manager, "burst")
	slow := started.Subscribe()
	healthy := newCollector(started.Subscribe())

	if _, err := started.Write([]byte("start\n")); err != nil {
		t.Fatalf("start burst: %v", err)
	}
	result, err := waitSession(t, started)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("Session.Wait() = (%+v, %v), want exit 0", result, err)
	}
	healthy.wait(t)
	if !errors.Is(slow.Err(), ErrSlowSubscriber) {
		t.Fatalf("slow subscriber error = %v, want ErrSlowSubscriber", slow.Err())
	}
	if healthy.subscription.Err() != nil {
		t.Fatalf("healthy subscriber error = %v", healthy.subscription.Err())
	}
	if !bytes.Contains(healthy.bytes(), []byte("BURST-DONE")) {
		t.Fatalf("healthy subscriber missed final output: %q", healthy.bytes())
	}
	if initial := started.Subscribe().Initial(); len(initial) > 128 {
		t.Fatalf("history length = %d, want <= 128", len(initial))
	}
}

func TestConcurrentWritesDoNotInterleave(t *testing.T) {
	writer := &yieldingWriter{}
	started := &Session{
		done:  make(chan struct{}),
		input: writer,
	}
	first := bytes.Repeat([]byte("A"), 256)
	second := bytes.Repeat([]byte("B"), 256)
	var wait sync.WaitGroup
	wait.Add(2)
	go func() {
		defer wait.Done()
		if _, err := started.Write(first); err != nil {
			t.Errorf("write first chunk: %v", err)
		}
	}()
	go func() {
		defer wait.Done()
		if _, err := started.Write(second); err != nil {
			t.Errorf("write second chunk: %v", err)
		}
	}()
	wait.Wait()
	got := writer.bytes()
	wantFirst := append(bytes.Clone(first), second...)
	wantSecond := append(bytes.Clone(second), first...)
	if !bytes.Equal(got, wantFirst) && !bytes.Equal(got, wantSecond) {
		t.Fatalf("concurrent writes interleaved: %q", got)
	}
}

func TestSessionResize(t *testing.T) {
	manager := newTestManager(t, ManagerOptions{})
	argv := sessionHelperArgv(t, "resize")
	started, err := manager.Start(context.Background(), argv, StartOptions{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatalf("Manager.Start(): %v", err)
	}
	collector := newCollector(started.Subscribe())
	collector.waitFor(t, "READY:80x24")
	if err := started.Resize(144, 52); err != nil {
		t.Fatalf("Session.Resize(): %v", err)
	}
	result, err := waitSession(t, started)
	if err != nil || result.ExitCode != 0 {
		t.Fatalf("Session.Wait() = (%+v, %v), want exit 0", result, err)
	}
	collector.wait(t)
	if !bytes.Contains(collector.bytes(), []byte("RESIZED:144x52")) {
		t.Fatalf("resize output = %q, want 144x52", collector.bytes())
	}
}

func TestManagerCloseStopsRunningSessions(t *testing.T) {
	manager := newTestManager(t, ManagerOptions{GracePeriod: 100 * time.Millisecond})
	started := startTestSession(t, manager, "stream")
	closeContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.Close(closeContext); err != nil {
		t.Fatalf("Manager.Close(): %v", err)
	}
	select {
	case <-started.Done():
	case <-time.After(time.Second):
		t.Fatal("running session remained alive after Manager.Close")
	}
}

func TestManagerOptionValidation(t *testing.T) {
	for _, options := range []ManagerOptions{
		{HistoryBytes: -1},
		{SubscriberBuffer: -1},
		{GracePeriod: -1},
	} {
		if _, err := NewManager(options); err == nil {
			t.Fatalf("NewManager(%+v) error = nil", options)
		}
	}
}

func TestResolveExecutableErrors(t *testing.T) {
	_, err := resolveExecutable("rome-session-test-command-that-does-not-exist")
	if err == nil {
		t.Fatal("resolveExecutable() error = nil")
	}
	if code := ErrorCode(err); code != 127 {
		t.Fatalf("ErrorCode() = %d, want 127", code)
	}
}

func printSessionSize(prefix string) {
	size, err := pty.GetsizeFull(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get size: %v\n", err)
		os.Exit(99)
	}
	fmt.Fprintf(os.Stdout, "%s:%dx%d\n", prefix, size.Cols, size.Rows)
}

func sessionHelperArgv(t *testing.T, mode string) []string {
	t.Helper()
	t.Setenv("ROME_SESSION_TEST_HELPER", mode)
	return []string{os.Args[0], "-test.run=^TestSessionHelperProcess$"}
}

func newTestManager(t *testing.T, options ManagerOptions) *Manager {
	t.Helper()
	manager, err := NewManager(options)
	if err != nil {
		t.Fatalf("NewManager(): %v", err)
	}
	return manager
}

func startTestSession(t *testing.T, manager *Manager, mode string) *Session {
	t.Helper()
	started, err := manager.Start(context.Background(), sessionHelperArgv(t, mode), StartOptions{})
	if err != nil {
		t.Fatalf("Manager.Start(): %v", err)
	}
	return started
}

func waitSession(t *testing.T, started *Session) (Result, error) {
	t.Helper()
	done := make(chan struct {
		result Result
		err    error
	}, 1)
	go func() {
		result, err := started.Wait()
		done <- struct {
			result Result
			err    error
		}{result: result, err: err}
	}()
	select {
	case outcome := <-done:
		return outcome.result, outcome.err
	case <-time.After(3 * time.Second):
		t.Fatal("Session.Wait() timed out")
		return Result{}, errors.New("unreachable")
	}
}

func closeManager(t *testing.T, manager *Manager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := manager.Close(ctx); err != nil {
		t.Fatalf("Manager.Close(): %v", err)
	}
}

type collector struct {
	subscription *Subscription
	mu           sync.Mutex
	buffer       bytes.Buffer
	done         chan struct{}
}

func newCollector(subscription *Subscription) *collector {
	collector := &collector{subscription: subscription, done: make(chan struct{})}
	collector.buffer.Write(subscription.Initial())
	go func() {
		defer close(collector.done)
		for output := range subscription.Output() {
			collector.mu.Lock()
			collector.buffer.Write(output)
			collector.mu.Unlock()
		}
	}()
	return collector
}

func (c *collector) bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return bytes.Clone(c.buffer.Bytes())
}

func (c *collector) waitFor(t *testing.T, substring string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if bytes.Contains(c.bytes(), []byte(substring)) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in %q", substring, c.bytes())
}

func (c *collector) wait(t *testing.T) {
	t.Helper()
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber collector did not close")
	}
}

type yieldingWriter struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (w *yieldingWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buffer.WriteByte(data[0])
	runtime.Gosched()
	return 1, nil
}

func (w *yieldingWriter) bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return bytes.Clone(w.buffer.Bytes())
}
