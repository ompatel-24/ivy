package server

import (
	"fmt"
	"testing"
	"time"
)

func TestFailureLimiter(t *testing.T) {
	limiter := newFailureLimiter()
	now := time.Unix(1000, 0)
	limiter.now = func() time.Time { return now }
	remote := "192.0.2.1:1234"

	for attempt := 0; attempt < maxAuthenticationFailures; attempt++ {
		if allowed, _ := limiter.allowed(remote); !allowed {
			t.Fatalf("attempt %d was blocked too early", attempt+1)
		}
		limiter.failed(remote)
	}
	if allowed, retry := limiter.allowed(remote); allowed || retry <= 0 {
		t.Fatalf("allowed() = (%t, %s), want blocked with retry", allowed, retry)
	}

	now = now.Add(authenticationWindow)
	if allowed, _ := limiter.allowed(remote); !allowed {
		t.Fatal("expired authentication window remained blocked")
	}
	limiter.failed(remote)
	limiter.succeeded(remote)
	if allowed, _ := limiter.allowed(remote); !allowed {
		t.Fatal("successful authentication did not clear failures")
	}
}

func TestFailureLimiterIsBounded(t *testing.T) {
	limiter := newFailureLimiter()
	limiter.now = func() time.Time { return time.Unix(1000, 0) }
	for index := 0; index < maxLimiterEntries+50; index++ {
		limiter.failed(fmt.Sprintf("192.0.2.%d:1234", index))
	}
	if len(limiter.entries) > maxLimiterEntries {
		t.Fatalf("limiter retained %d entries, want <= %d", len(limiter.entries), maxLimiterEntries)
	}
}

func TestRemoteIP(t *testing.T) {
	if got := remoteIP("[2001:db8::1]:7654"); got != "2001:db8::1" {
		t.Fatalf("remoteIP() = %q", got)
	}
	if got := remoteIP("not-an-address"); got != "not-an-address" {
		t.Fatalf("remoteIP() fallback = %q", got)
	}
}
