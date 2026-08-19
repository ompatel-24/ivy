package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/ompatel-24/ivy/internal/version"
)

func TestRunCLIHelp(t *testing.T) {
	stdin := openDevNull(t)
	var stdout, stderr bytes.Buffer

	code := runCLI(context.Background(), []string{"help"}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runCLI() code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "ivy <command> [args...]") {
		t.Fatalf("help output missing usage: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("help stderr = %q, want empty", stderr.String())
	}
}

func TestRunCLIMissingCommand(t *testing.T) {
	stdin := openDevNull(t)
	var stdout, stderr bytes.Buffer

	code := runCLI(context.Background(), nil, stdin, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("runCLI() code = %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "ivy: missing command") {
		t.Fatalf("stderr = %q, want missing-command error", stderr.String())
	}
}

func TestRunCLIVersion(t *testing.T) {
	stdin := openDevNull(t)
	var stdout, stderr bytes.Buffer
	originalVersion := version.Value
	version.Value = "test-version"
	t.Cleanup(func() { version.Value = originalVersion })

	code := runCLI(context.Background(), []string{"version"}, stdin, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("runCLI() code = %d, want 0; stderr=%q", code, stderr.String())
	}
	if stdout.String() != "ivy test-version\n" {
		t.Fatalf("version output = %q, want %q", stdout.String(), "ivy test-version\\n")
	}
}

func TestRunCLICommandNotFound(t *testing.T) {
	t.Setenv("IVY_LISTEN", "")
	stdin := openDevNull(t)
	var stdout, stderr bytes.Buffer

	code := runCLI(context.Background(), []string{"ivy-test-command-that-does-not-exist"}, stdin, &stdout, &stderr)
	if code != 127 {
		t.Fatalf("runCLI() code = %d, want 127", code)
	}
	if !strings.Contains(stderr.String(), "ivy: command not found: ivy-test-command-that-does-not-exist") {
		t.Fatalf("stderr = %q, want clean not-found error", stderr.String())
	}
}

func TestRunCLIRejectsUnsafeListenAddressBeforeLaunchingCommand(t *testing.T) {
	t.Setenv("IVY_LISTEN", "0.0.0.0:7654")
	stdin := openDevNull(t)
	var stdout, stderr bytes.Buffer

	code := runCLI(context.Background(), []string{"ivy-command-that-must-not-launch"}, stdin, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("runCLI() code = %d, want 1; stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "wildcard hosts are not allowed") {
		t.Fatalf("stderr = %q, want safe listen-address error", stderr.String())
	}
	if strings.Contains(stderr.String(), "command not found") {
		t.Fatalf("command launch was attempted before listen validation: %q", stderr.String())
	}
}

func TestDebugEnabled(t *testing.T) {
	tests := map[string]bool{
		"":      false,
		"0":     false,
		"false": false,
		"NO":    false,
		"1":     true,
		"true":  true,
		"debug": true,
	}
	for value, want := range tests {
		if got := debugEnabled(value); got != want {
			t.Errorf("debugEnabled(%q) = %t, want %t", value, got, want)
		}
	}
}

func openDevNull(t *testing.T) *os.File {
	t.Helper()
	file, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}
