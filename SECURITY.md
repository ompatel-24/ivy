# Security model

Ivy transports terminal input and output and therefore grants the equivalent of
interactive access to the child process. Treat every Session credential as a
shell-access secret.

## Milestone 3 boundary

Networking is off by default. `IVY_LISTEN` must name one concrete loopback or
LAN interface; Ivy rejects empty and wildcard hosts rather than silently
binding to every interface. The listener is created before the child starts so
an unsafe or unavailable address fails closed.

Each network-enabled Session receives a cryptographically random 256-bit token.
Only its SHA-256 digest is retained by the server and comparisons are constant
time. Session IDs remain non-authenticating routing metadata. Failed
authentication is limited to ten attempts per remote IP per minute with a
bounded in-memory table.

The token is intentionally displayed once to the controlling local terminal in
a URL fragment. Fragments are not sent in HTTP requests. Ivy never puts the
token in a query string, request log, debug output, analytics, or HTTP response.
Metadata requests use a Bearer header; browser-compatible WebSockets use the
`ivy.auth.<token>` subprotocol alongside `ivy.v1`.

WebSockets enforce same-origin checks, HTTP responses do not enable CORS, input
sizes are bounded, and authenticated metadata omits the environment and full
argument vector. A client disconnect or protocol violation never terminates the
child Session.

## Limitations

Milestone 3 uses plaintext HTTP and WebSocket connections. Loopback use is the
safest mode. A concrete LAN address is intended only for a trusted private
network: anyone able to observe that traffic may read terminal contents or
capture the token. Do not expose the listener to the public internet, forward
its port, or place it behind an untrusted proxy.

TLS/WSS, QR pairing, the mobile client, hosted relay, and end-to-end encryption
are not implemented yet. Those features must preserve the rule that credentials
are never server-logged and must use standard cryptographic primitives rather
than custom encryption.
