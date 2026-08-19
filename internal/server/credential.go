package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
)

const tokenBytes = 32

// Credential stores only a digest of a session token.
type Credential struct {
	digest [sha256.Size]byte
}

// NewCredential creates a 256-bit base64url token and its non-reversible
// in-memory credential.
func NewCredential() (string, Credential, error) {
	raw := make([]byte, tokenBytes)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", Credential{}, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, credentialForToken(token), nil
}

func credentialForToken(token string) Credential {
	return Credential{digest: sha256.Sum256([]byte(token))}
}

func (c Credential) matches(candidate string) bool {
	digest := sha256.Sum256([]byte(candidate))
	return subtle.ConstantTimeCompare(c.digest[:], digest[:]) == 1
}
