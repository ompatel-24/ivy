# Ivy 🌿

Take your terminal agents with you.

```console
$ ivy claude
$ ivy codex
$ ivy <anything>
```

Ivy is an open-source tool for running any interactive CLI inside a
pseudo-terminal. A transport-independent Session owns that process, keeps
bounded output history, and can serve the local terminal plus authenticated
HTTP/WebSocket clients. A mobile browser client arrives in a later milestone.

## Current status

Milestone 3 adds an opt-in authenticated local transport without changing the
ordinary `ivy <command>` experience. Setting `IVY_LISTEN` adds:

- health and sanitized Session metadata endpoints;
- raw binary terminal input and output over WebSockets;
- versioned JSON controls for hello, resize, errors, and process exit;
- random 256-bit per-Session credentials and authentication throttling; and
- bounded, coordinated HTTP/WebSocket shutdown.

There is still no phone interface, QR pairing, TLS termination, E2EE, or hosted
relay. Networking is disabled unless explicitly enabled.

## Build

Ivy currently requires Go 1.23 or newer to build.

```bash
make build
./dist/ivy bash
```

For development:

```bash
go run ./cmd/ivy bash
```

## Usage

```text
ivy <command> [args...]
ivy -- <command> [args...]
ivy help
ivy version
```

Ivy executes the supplied argument vector directly. It does not invoke a shell,
parse agent output, or contain Claude-, Codex-, or other agent-specific paths.
Use `--` to launch a command named `help` or `version`.

### Local transport

Enable the transport on one concrete loopback interface:

```bash
IVY_LISTEN=127.0.0.1:7654 ./dist/ivy bash
```

Port `0` requests an available ephemeral port. Ivy rejects wildcard listeners
such as `0.0.0.0`, `[::]`, and an empty host. A concrete LAN address can be used
for trusted-LAN development, but the milestone does not provide TLS and must
not be exposed to the internet.

When enabled, Ivy prints one local connection URL whose `#token=` fragment is
not sent in HTTP requests. The token authenticates metadata requests and
WebSocket connections; the Session ID alone never grants access.

## Development

```bash
make lint
make test
make build
```

See [docs/manual-testing.md](docs/manual-testing.md) for the compatibility test
matrix and current verified results. See [docs/sessions.md](docs/sessions.md) for
the Session contract, [docs/protocol.md](docs/protocol.md) for the transport
protocol, and [SECURITY.md](SECURITY.md) for the security model.

## Session safety and limits

A Session ID is a random 128-bit base64url routing identifier. It is not a
credential and must not be treated as authentication. Network clients use a
distinct per-Session token.

Each subscriber has a 64-chunk queue. Ivy disconnects a subscriber that cannot
keep up instead of allowing it to stall the PTY or other subscribers. Output
history stores the newest 512 KiB exactly as received; because it is bounded raw
data, a snapshot may begin in the middle of an ANSI escape sequence or UTF-8
character.

## Roadmap

- Mobile xterm.js client
- Secure QR pairing and authentication
- Embedded web assets and single-binary releases

Hosted relay work is intentionally deferred until the local PTY and session
layers are solid.

## Platforms

Milestone 3 targets macOS and Linux on amd64 and arm64. Windows is not currently
supported.

## License

MIT. See [LICENSE](LICENSE).
