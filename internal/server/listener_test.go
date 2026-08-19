package server

import (
	"net/url"
	"strings"
	"testing"
)

func TestListenRejectsUnsafeAddresses(t *testing.T) {
	for _, address := range []string{
		"",
		":7654",
		"0.0.0.0:7654",
		"[::]:7654",
		"127.0.0.1",
		"127.0.0.1:http",
		"127.0.0.1:-1",
		"127.0.0.1:65536",
	} {
		t.Run(strings.ReplaceAll(address, "/", "_"), func(t *testing.T) {
			listener, err := Listen(address)
			if err == nil {
				_ = listener.Close()
				t.Fatalf("Listen(%q) unexpectedly succeeded", address)
			}
		})
	}
}

func TestListenAndConnectionURL(t *testing.T) {
	listener, err := Listen("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen(): %v", err)
	}
	defer listener.Close()

	connectionURL, err := ConnectionURL(listener.Addr(), "session-id", "secret-token")
	if err != nil {
		t.Fatalf("ConnectionURL(): %v", err)
	}
	parsed, err := url.Parse(connectionURL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", connectionURL, err)
	}
	if parsed.Scheme != "http" || parsed.Path != "/api/v1/sessions/session-id" || parsed.Fragment != "token=secret-token" {
		t.Fatalf("ConnectionURL() = %q", connectionURL)
	}
	if parsed.Hostname() != "127.0.0.1" || parsed.Port() == "" || parsed.Port() == "0" {
		t.Fatalf("ConnectionURL() host = %q, want bound loopback port", parsed.Host)
	}
	if strings.Contains(parsed.RawQuery, "secret-token") {
		t.Fatal("connection URL placed token in the HTTP query")
	}
}
