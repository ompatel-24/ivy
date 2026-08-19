# Ivy transport protocol v1

Ivy exposes an authenticated HTTP/WebSocket transport when
`IVY_LISTEN=<host:port>` is set. The protocol is agent-agnostic: terminal bytes
are never parsed for Claude, Codex, or any other program. Milestone 4 adds a
same-origin browser implementation without changing protocol version 1.

## Discovery and authentication

Ivy prints one URL to the controlling terminal before entering raw mode:

```text
http://127.0.0.1:7654/s/<id>#token=<token>
```

The Session ID is routing metadata, not authentication. The token is a random
256-bit base64url value. Its URL fragment is not included in HTTP requests.

Routes:

- `GET /s/{id}` serves the unauthenticated static client shell for a known Session.
- `GET /assets/*` serves exact built client assets without directory listings.
- `GET /health` is unauthenticated and returns only `{"status":"ok"}`.
- `GET /api/v1/sessions/{id}` requires `Authorization: Bearer <token>`.
- `GET /api/v1/sessions/{id}/ws` is the WebSocket endpoint.

The WebSocket client must offer both `ivy.v1` and `ivy.auth.<token>` in
`Sec-WebSocket-Protocol`; the server selects `ivy.v1`. Authentication tokens
must not be placed in query parameters. Cross-origin WebSockets are rejected;
future browser clients must be served from the same origin.

Authenticated metadata contains the Session ID, command basename, directory,
state, and an exit code only after the Session has exited. It never exposes the
environment or full child argument vector.

## WebSocket messages

Server text messages use JSON. The first message is `hello`:

```json
{"type":"hello","version":1,"session":{"id":"...","command":"bash","directory":"/work","state":"running","exitCode":null}}
```

The server then sends the atomic history snapshot, if non-empty, as one binary
message followed by live PTY chunks as binary messages. History is raw and
bounded, so its first byte may be in the middle of an ANSI escape sequence or
UTF-8 character.

Clients send terminal input as binary messages. A binary message is one
concurrency-safe Session write and may contain control bytes such as Ctrl+C
(`0x03`) or Ctrl+D (`0x04`). Binary client messages are limited to 64 KiB.

Clients resize the shared PTY with a text message no larger than 4 KiB:

```json
{"type":"resize","cols":100,"rows":30}
```

Fields are strict, dimensions must be between 1 and 65535, and the most recent
valid local or remote resize wins. Unknown or malformed controls receive an
`error` message and a WebSocket policy-violation close:

```json
{"type":"error","code":"bad_message","message":"invalid control message"}
```

After all PTY output is delivered, the server sends the child status and closes
normally:

```json
{"type":"exit","code":0}
```

A subscriber that exhausts its bounded Session queue receives the stable error
code `slow_consumer` and is disconnected without affecting the child or other
clients. Disconnecting and reconnecting creates a new subscription and replays
the current bounded history before live output. The browser resets its xterm
state before a replay so history is not duplicated on screen.

## Browser credential and reconnect lifecycle

The client reads the token from the fragment, stores it only in
`sessionStorage` under the Session ID, and removes the fragment from the address
bar. Reloading the same tab can reconnect; closing the tab removes the browser's
session storage. Ivy clears the stored token after child exit, authentication
failure, or an unknown Session. It never uses cookies or local storage.

Before opening a WebSocket, the client authenticates to the metadata endpoint.
Unexpected network failures retry after 250 ms, 500 ms, one second, two seconds,
and then every five seconds. Retries pause while the page is hidden or the
browser is offline and resume when it becomes visible and online. Input is
disabled unless the WebSocket has completed its `hello` exchange.
