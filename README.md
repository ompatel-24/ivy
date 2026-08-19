# Ivy 🌿

Take your terminal agents with you.

```console
$ ivy claude
$ ivy codex
$ ivy <anything>
```

Ivy is an open-source tool for running any interactive CLI inside a
pseudo-terminal. The PTY is the product's foundation: it lets Ivy preserve the
same process while local and, in later milestones, mobile clients interact with
it.

## Current status

Milestone 1 is a local, agent-agnostic PTY wrapper. It preserves interactive
terminal behavior, raw keyboard input, ANSI output, terminal resizing, signals,
and child exit codes. Mobile access, sessions, WebSockets, pairing, and relays
are not implemented yet.

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
matrix and current verified results.

## Roadmap

- Session abstraction, output subscribers, and bounded history
- Local HTTP and WebSocket transport
- Mobile xterm.js client
- Secure QR pairing and authentication
- Embedded web assets and single-binary releases

Hosted relay work is intentionally deferred until the local PTY and session
layers are solid.

## Platforms

Milestone 1 targets macOS and Linux on amd64 and arm64. Windows is not currently
supported.

## License

MIT. See [LICENSE](LICENSE).
