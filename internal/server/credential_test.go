package server

import (
	"encoding/base64"
	"testing"
)

func TestNewCredential(t *testing.T) {
	token, credential, err := NewCredential()
	if err != nil {
		t.Fatalf("NewCredential(): %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != tokenBytes {
		t.Fatalf("token decoded to %d bytes with error %v, want %d bytes", len(raw), err, tokenBytes)
	}
	if !credential.matches(token) {
		t.Fatal("credential did not match its generated token")
	}
	if credential.matches(token + "wrong") {
		t.Fatal("credential matched an invalid token")
	}
}

func TestCredentialsAreUnique(t *testing.T) {
	first, _, err := NewCredential()
	if err != nil {
		t.Fatalf("first NewCredential(): %v", err)
	}
	second, _, err := NewCredential()
	if err != nil {
		t.Fatalf("second NewCredential(): %v", err)
	}
	if first == second {
		t.Fatal("two generated credentials used the same token")
	}
}
