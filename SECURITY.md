# Security model

Rome transports terminal input and output and therefore grants the equivalent of
interactive access to the child process. Treat every Session credential as a
shell-access secret.

## Local transport boundary

Networking is off by default. `ROME_LISTEN` must name one concrete loopback or
LAN interface; Rome rejects empty and wildcard hosts rather than silently
binding to every interface. The listener is created before the child starts so
an unsafe or unavailable address fails closed.

Each network-enabled Session receives a cryptographically random 256-bit token.
Only its SHA-256 digest is retained by the server and comparisons are constant
time. Session IDs remain non-authenticating routing metadata. Failed
authentication is limited to ten attempts per remote IP per minute with a
bounded in-memory table.

The token is intentionally displayed once to the controlling local terminal in
a URL fragment. Fragments are not sent in HTTP requests. Rome never puts the
token in a query string, request log, debug output, analytics, or HTTP response.
Metadata requests use a Bearer header; browser-compatible WebSockets use the
`rome.auth.<token>` subprotocol alongside `rome.v1`.

The Milestone 4 browser stores the token in per-tab `sessionStorage`, removes it
from the visible URL, and clears it after exit, authentication failure, or an
unknown Session. It does not use cookies or persistent local storage. Closing
the browser tab ends the storage lifetime, subject to the browser's normal tab
restoration behavior.

Milestone 5 renders that same authenticated URL as a QR code in the controlling
terminal. The QR is a visual representation of the session-long bearer token,
not a one-time exchange. Anyone who scans or photographs it while the Session
is running can control the terminal, so treat terminal scrollback and QR
screenshots as credentials. Rome retains only the token digest and does not log
the QR payload.

WebSockets enforce same-origin checks, HTTP responses do not enable CORS, input
sizes are bounded, and authenticated metadata omits the environment and full
argument vector. A client disconnect or protocol violation never terminates the
child Session.

The static client is served only for the current routing ID. Production assets
come from Rome's embedded, build-verified filesystem. `ROME_WEB_DIR` can replace
that filesystem explicitly for development, so users of the override are
responsible for trusting its contents. In either mode, only exact files are
served: directory listings and traversal are not supported. Browser responses
use a restrictive content security policy, deny framing, suppress referrers,
and disable unused device permissions.

## Limitations

The local transport uses plaintext HTTP and WebSocket connections. Loopback use
is the safest mode. A concrete LAN address is intended only for a trusted
private network: anyone able to observe that traffic may read terminal contents
or capture the token. Do not expose the listener to the public internet,
forward its port, or place it behind an untrusted proxy.

Some public and café Wi-Fi networks isolate connected devices, so a phone may
be unable to reach the computer even when both appear to use the same network.
QR pairing does not change network reachability and must not be treated as a
relay or tunnel.

TLS/WSS, hosted relay, and end-to-end encryption are not implemented yet. Those
features must preserve the rule that credentials are never server-logged and
must use standard cryptographic primitives rather than custom encryption.

## Release supply chain

Official releases contain SHA-256 checksums and GitHub provenance attestations
for every platform archive and the checksum manifest. Verify both before
installing a downloaded binary:

```bash
shasum -a 256 -c rome_0.2.0_checksums.txt
gh attestation verify rome_0.2.0_darwin_arm64.tar.gz --repo ompatel-24/rome
```

The release workflow uses the repository-scoped `GITHUB_TOKEN` only for Rome's
GitHub release. Homebrew formula publishing uses a separate fine-grained token
restricted to `ompatel-24/homebrew-tap` with Contents read/write access. It is
passed only to the formula publisher and must never appear in repository files,
logs, issues, or support output.

Rome's macOS binaries are currently unsigned. The project publishes a Homebrew
formula rather than a cask to avoid presenting an unsigned app bundle as a
signed distribution. Apple signing and additional packaging are deferred.
