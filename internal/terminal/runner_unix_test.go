//go:build darwin || linux

package terminal

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"golang.org/x/term"
)

func TestPTYHelperProcess(t *testing.T) {
	mode := os.Getenv("ROME_TEST_HELPER")
	if mode == "" {
		return
	}

	switch mode {
	case "echo":
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Fprintln(os.Stderr, "stdin is not a terminal")
			os.Exit(91)
		}
		fmt.Fprint(os.Stdout, "\x1b[31mREADY\x1b[0m\n")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			fmt.Fprintf(os.Stderr, "read input: %v\n", err)
			os.Exit(92)
		}
		fmt.Fprintf(os.Stdout, "ECHO:%s", line)
		os.Exit(7)

	case "size":
		printSize("SIZE")

	case "environment":
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "get working directory: %v\n", err)
			os.Exit(96)
		}
		fmt.Fprintf(os.Stdout, "ARGV0:%s\nCWD:%s\nENV:%s\n", os.Args[0], cwd, os.Getenv("ROME_TEST_VALUE"))

	case "eof":
		data := make([]byte, 1)
		read, err := os.Stdin.Read(data)
		if err == io.EOF {
			fmt.Fprintln(os.Stdout, "EOF:0")
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "read end-of-input marker: %v\n", err)
			os.Exit(97)
		}
		fmt.Fprintf(os.Stdout, "EOT:%d:%d\n", read, data[0])

	case "resize":
		resizeSignals := make(chan os.Signal, 1)
		signal.Notify(resizeSignals, syscall.SIGWINCH)
		defer signal.Stop(resizeSignals)
		printSize("READY")
		<-resizeSignals
		printSize("RESIZED")

	case "sigint":
		interrupts := make(chan os.Signal, 1)
		signal.Notify(interrupts, syscall.SIGINT)
		defer signal.Stop(interrupts)
		fmt.Fprintln(os.Stdout, "READY")
		received := <-interrupts
		fmt.Fprintf(os.Stdout, "SIGNAL:%s\n", received)

	case "ignore-term":
		signal.Ignore(syscall.SIGTERM, syscall.SIGHUP)
		fmt.Fprintln(os.Stdout, "READY")
		for {
			time.Sleep(time.Second)
		}

	case "self-term":
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
		time.Sleep(time.Second)
		os.Exit(93)

	default:
		fmt.Fprintf(os.Stderr, "unknown helper mode %q\n", mode)
		os.Exit(94)
	}
}

func TestRunnerPTYInputOutputAndExit(t *testing.T) {
	stdin := pipeInput(t, "hello rome\n")
	var output lockedBuffer
	runner := Runner{Stdin: stdin, Stdout: &output, Stderr: io.Discard}

	result, err := runner.Run(context.Background(), helperArgv(t, "echo"))
	if err != nil {
		t.Fatalf("Runner.Run() unexpected error: %v; output=%q", err, output.String())
	}
	if result.ExitCode != 7 {
		t.Fatalf("Runner.Run() exit code = %d, want 7", result.ExitCode)
	}
	if !strings.Contains(output.String(), "\x1b[31mREADY\x1b[0m") {
		t.Fatalf("PTY output did not preserve ANSI bytes: %q", output.String())
	}
	if !strings.Contains(output.String(), "ECHO:hello rome") {
		t.Fatalf("child did not receive PTY input: %q", output.String())
	}
}

func TestRunnerMapsSignalExit(t *testing.T) {
	stdin := devNull(t)
	runner := Runner{Stdin: stdin, Stdout: io.Discard, Stderr: io.Discard}

	result, err := runner.Run(context.Background(), helperArgv(t, "self-term"))
	if err != nil {
		t.Fatalf("Runner.Run() unexpected error: %v", err)
	}
	if result.ExitCode != 128+int(syscall.SIGTERM) {
		t.Fatalf("Runner.Run() exit code = %d, want %d", result.ExitCode, 128+int(syscall.SIGTERM))
	}
}

func TestRunnerDefaultSizeAndPipedEOF(t *testing.T) {
	var sizeOutput lockedBuffer
	runner := Runner{Stdin: devNull(t), Stdout: &sizeOutput, Stderr: io.Discard}
	result, err := runner.Run(context.Background(), helperArgv(t, "size"))
	if err != nil {
		t.Fatalf("Runner.Run(size) unexpected error: %v; output=%q", err, sizeOutput.String())
	}
	if result.ExitCode != 0 || !strings.Contains(sizeOutput.String(), "SIZE:80x24") {
		t.Fatalf("default-size result = (%d, %q), want exit 0 and 80x24", result.ExitCode, sizeOutput.String())
	}

	var eofOutput lockedBuffer
	runner.Stdin = pipeInput(t, "")
	runner.Stdout = &eofOutput
	result, err = runner.Run(context.Background(), helperArgv(t, "eof"))
	if err != nil {
		t.Fatalf("Runner.Run(eof) unexpected error: %v; output=%q", err, eofOutput.String())
	}
	gotEndOfInput := strings.Contains(eofOutput.String(), "EOF:0") || strings.Contains(eofOutput.String(), "EOT:1:4")
	if result.ExitCode != 0 || !gotEndOfInput {
		t.Fatalf("end-of-input result = (%d, %q), want exit 0 and canonical EOF or EOT byte 4", result.ExitCode, eofOutput.String())
	}
}

func TestRunnerPreservesArgvEnvironmentAndDirectory(t *testing.T) {
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd(): %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(originalDirectory); chdirErr != nil {
			t.Errorf("restore working directory: %v", chdirErr)
		}
	})
	temporaryDirectory := t.TempDir()
	resolvedDirectory, err := filepath.EvalSymlinks(temporaryDirectory)
	if err != nil {
		t.Fatalf("filepath.EvalSymlinks(%q): %v", temporaryDirectory, err)
	}
	if err := os.Chdir(temporaryDirectory); err != nil {
		t.Fatalf("os.Chdir(%q): %v", temporaryDirectory, err)
	}
	t.Setenv("ROME_TEST_VALUE", "preserved")
	argv := helperArgv(t, "environment")

	var output lockedBuffer
	runner := Runner{Stdin: devNull(t), Stdout: &output, Stderr: io.Discard}
	result, err := runner.Run(context.Background(), argv)
	if err != nil {
		t.Fatalf("Runner.Run() unexpected error: %v; output=%q", err, output.String())
	}
	if result.ExitCode != 0 {
		t.Fatalf("Runner.Run() exit code = %d, want 0", result.ExitCode)
	}
	for _, expected := range []string{
		"ARGV0:" + argv[0],
		"CWD:" + resolvedDirectory,
		"ENV:preserved",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("output %q does not contain %q", output.String(), expected)
		}
	}
}

func TestRunnerInitialSizeAndTerminalRestore(t *testing.T) {
	sourcePTY, sourceTTY := openPTYPair(t)
	setPTYSize(t, sourcePTY, 123, 45)
	before, err := term.GetState(int(sourceTTY.Fd()))
	if err != nil {
		t.Fatalf("term.GetState(before): %v", err)
	}

	var output lockedBuffer
	runner := Runner{Stdin: sourceTTY, Stdout: &output, Stderr: io.Discard}
	result, err := runner.Run(context.Background(), helperArgv(t, "size"))
	if err != nil {
		t.Fatalf("Runner.Run() unexpected error: %v; output=%q", err, output.String())
	}
	if result.ExitCode != 0 {
		t.Fatalf("Runner.Run() exit code = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(output.String(), "SIZE:123x45") {
		t.Fatalf("child size output = %q, want 123x45", output.String())
	}

	after, err := term.GetState(int(sourceTTY.Fd()))
	if err != nil {
		t.Fatalf("term.GetState(after): %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("Runner.Run() did not restore the local terminal state")
	}
}

func TestRunnerRestoresTerminalAfterStartFailure(t *testing.T) {
	sourcePTY, sourceTTY := openPTYPair(t)
	setPTYSize(t, sourcePTY, 80, 24)
	before, err := term.GetState(int(sourceTTY.Fd()))
	if err != nil {
		t.Fatalf("term.GetState(before): %v", err)
	}

	invalidExecutable := filepath.Join(t.TempDir(), "invalid-executable")
	if err := os.WriteFile(invalidExecutable, []byte("not an executable format\n"), 0o755); err != nil {
		t.Fatalf("write invalid executable: %v", err)
	}

	runner := Runner{Stdin: sourceTTY, Stdout: io.Discard, Stderr: io.Discard}
	_, runErr := runner.Run(context.Background(), []string{invalidExecutable})
	if runErr == nil {
		t.Fatal("Runner.Run() error = nil, want executable format error")
	}
	if code := ErrorCode(runErr); code != 126 {
		t.Fatalf("ErrorCode() = %d, want 126; error=%v", code, runErr)
	}

	after, err := term.GetState(int(sourceTTY.Fd()))
	if err != nil {
		t.Fatalf("term.GetState(after): %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("Runner.Run() did not restore terminal state after start failure")
	}
}

func TestCopyTerminalSize(t *testing.T) {
	sourcePTY, sourceTTY := openPTYPair(t)
	setPTYSize(t, sourcePTY, 111, 37)
	target := &recordingResizer{}

	if err := inheritTerminalSize(sourceTTY, target); err != nil {
		t.Fatalf("inheritTerminalSize(): %v", err)
	}
	if target.cols != 111 || target.rows != 37 {
		t.Fatalf("target size = %dx%d, want 111x37", target.cols, target.rows)
	}
}

func TestRunnerPropagatesResize(t *testing.T) {
	sourcePTY, sourceTTY := openPTYPair(t)
	setPTYSize(t, sourcePTY, 80, 24)
	signals := make(chan os.Signal, 4)
	var output lockedBuffer
	runner := Runner{Stdin: sourceTTY, Stdout: &output, Stderr: io.Discard, Signals: signals}
	argv := helperArgv(t, "resize")

	done := make(chan runOutcome, 1)
	go func() {
		result, err := runner.Run(context.Background(), argv)
		done <- runOutcome{result: result, err: err}
	}()

	waitForOutput(t, &output, "READY:80x24")
	setPTYSize(t, sourcePTY, 132, 51)
	signals <- syscall.SIGWINCH

	outcome := waitForRun(t, done)
	if outcome.err != nil {
		t.Fatalf("Runner.Run() unexpected error: %v; output=%q", outcome.err, output.String())
	}
	if outcome.result.ExitCode != 0 {
		t.Fatalf("Runner.Run() exit code = %d, want 0", outcome.result.ExitCode)
	}
	if !strings.Contains(output.String(), "RESIZED:132x51") {
		t.Fatalf("resize output = %q, want 132x51", output.String())
	}
}

func TestRunnerForwardsExternalSIGINT(t *testing.T) {
	signals := make(chan os.Signal, 4)
	var output lockedBuffer
	runner := Runner{
		Stdin:   devNull(t),
		Stdout:  &output,
		Stderr:  io.Discard,
		Signals: signals,
	}
	argv := helperArgv(t, "sigint")

	done := make(chan runOutcome, 1)
	go func() {
		result, err := runner.Run(context.Background(), argv)
		done <- runOutcome{result: result, err: err}
	}()

	waitForOutput(t, &output, "READY")
	signals <- syscall.SIGINT

	outcome := waitForRun(t, done)
	if outcome.err != nil {
		t.Fatalf("Runner.Run() unexpected error: %v; output=%q", outcome.err, output.String())
	}
	if !strings.Contains(output.String(), "SIGNAL:interrupt") {
		t.Fatalf("signal output = %q, want forwarded SIGINT", output.String())
	}
}

func TestRunnerEscalatesIgnoredTermination(t *testing.T) {
	signals := make(chan os.Signal, 4)
	var output lockedBuffer
	runner := Runner{
		Stdin:       devNull(t),
		Stdout:      &output,
		Stderr:      io.Discard,
		Signals:     signals,
		GracePeriod: 75 * time.Millisecond,
	}
	argv := helperArgv(t, "ignore-term")

	done := make(chan runOutcome, 1)
	go func() {
		result, err := runner.Run(context.Background(), argv)
		done <- runOutcome{result: result, err: err}
	}()

	waitForOutput(t, &output, "READY")
	signals <- syscall.SIGTERM

	outcome := waitForRun(t, done)
	if outcome.err != nil {
		t.Fatalf("Runner.Run() unexpected error: %v; output=%q", outcome.err, output.String())
	}
	if outcome.result.ExitCode != 128+int(syscall.SIGKILL) {
		t.Fatalf("Runner.Run() exit code = %d, want %d", outcome.result.ExitCode, 128+int(syscall.SIGKILL))
	}
}

func printSize(prefix string) {
	size, err := pty.GetsizeFull(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "get size: %v\n", err)
		os.Exit(95)
	}
	fmt.Fprintf(os.Stdout, "%s:%dx%d\n", prefix, size.Cols, size.Rows)
}

func helperArgv(t *testing.T, mode string) []string {
	t.Helper()
	t.Setenv("ROME_TEST_HELPER", mode)
	return []string{os.Args[0], "-test.run=^TestPTYHelperProcess$"}
}

func pipeInput(t *testing.T, data string) *os.File {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe(): %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	go func() {
		_, _ = io.WriteString(writer, data)
		_ = writer.Close()
	}()
	return reader
}

func devNull(t *testing.T) *os.File {
	t.Helper()
	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func openPTYPair(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open(): %v", err)
	}
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = tty.Close()
	})
	return ptmx, tty
}

func setPTYSize(t *testing.T, terminal *os.File, cols, rows uint16) {
	t.Helper()
	if err := pty.Setsize(terminal, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
		t.Fatalf("pty.Setsize(%dx%d): %v", cols, rows, err)
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

func waitForOutput(t *testing.T, output *lockedBuffer, substring string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), substring) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in output %q", substring, output.String())
}

type runOutcome struct {
	result Result
	err    error
}

type recordingResizer struct {
	cols uint16
	rows uint16
}

func (r *recordingResizer) Resize(cols, rows uint16) error {
	r.cols = cols
	r.rows = rows
	return nil
}

func waitForRun(t *testing.T, done <-chan runOutcome) runOutcome {
	t.Helper()
	select {
	case outcome := <-done:
		return outcome
	case <-time.After(3 * time.Second):
		t.Fatal("Runner.Run() did not return")
		return runOutcome{}
	}
}
