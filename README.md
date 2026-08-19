# Ivy 🌿

Take your terminal agents with you.

```console
$ ivy claude
$ ivy codex
$ ivy <anything>
```

Ivy is an open-source tool for running any interactive CLI inside a
pseudo-terminal. A transport-independent Session now owns that process, keeps
bounded output history, and can serve multiple output subscribers. The local
terminal is the first client; network and mobile clients arrive in later
milestones.

## Current status

Milestone 2 adds the in-memory Session layer without changing the CLI. It
preserves Milestone 1's interactive terminal behavior, raw keyboard input, ANSI
output, terminal resizing, signals, and child exit codes while adding:

- concurrency-safe input and lifecycle management;
- multiple independent output subscribers;
- 512 KiB of bounded raw-byte output history; and
- immutable command and directory metadata managed under random session IDs.

There is still no HTTP listener, WebSocket transport, phone interface, pairing,
authentication, or hosted relay.

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

## Development

```bash
make lint
make test
make build
```

See [docs/manual-testing.md](docs/manual-testing.md) for the compatibility test
matrix and current verified results. See [docs/sessions.md](docs/sessions.md) for
the Session contract and its current limits.

## Session safety and limits

A Session ID is a random 128-bit base64url routing identifier. It is not a
credential and must not be treated as authentication. Future network transports
must authenticate and authorize access separately.

Each subscriber has a 64-chunk queue. Ivy disconnects a subscriber that cannot
keep up instead of allowing it to stall the PTY or other subscribers. Output
history stores the newest 512 KiB exactly as received; because it is bounded raw
data, a snapshot may begin in the middle of an ANSI escape sequence or UTF-8
character.

## Roadmap

- Local HTTP and WebSocket transport
- Mobile xterm.js client
- Secure QR pairing and authentication
- Embedded web assets and single-binary releases

Hosted relay work is intentionally deferred until the local PTY and session
layers are solid.

## Platforms

Milestone 2 targets macOS and Linux on amd64 and arm64. Windows is not currently
supported.

## License

MIT. See [LICENSE](LICENSE).
