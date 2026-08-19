# Ivy 🌿

Take your terminal agents with you.

```console
$ ivy claude
$ ivy codex
$ ivy <anything>
```

Ivy is an open-source tool for running any interactive CLI inside a
pseudo-terminal. A transport-independent Session owns that process, keeps
bounded output history, and serves the local terminal plus an authenticated
mobile browser terminal over the local network.

## Current status

Milestone 4 adds a minimal mobile-first xterm.js client over Milestone 3's
opt-in authenticated local transport. Setting `IVY_LISTEN` now adds:

- a same-origin browser terminal for the active Session;
- raw binary terminal input/output and responsive PTY resizing;
- bounded-history replay and automatic reconnection after network loss; and
- per-tab token handoff without cookies, query strings, or persistent storage.

There is still no QR pairing, mobile helper-key row, TLS termination, E2EE, or
hosted relay. Networking is disabled unless explicitly enabled.

## Build

Ivy currently requires Go 1.23 or newer plus a Vite-compatible Node.js/npm
toolchain to build. Node.js is not needed when running the built artifacts.

```bash
make build          # produces dist/ivy and dist/web
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

### Mobile terminal

Enable the transport on one concrete loopback interface:

```bash
IVY_LISTEN=127.0.0.1:7654 ./dist/ivy bash
```

Port `0` requests an available ephemeral port. Ivy rejects wildcard listeners
such as `0.0.0.0`, `[::]`, and an empty host. A concrete LAN address can be used
for trusted-LAN development, but the milestone does not provide TLS and must
not be exposed to the internet.

When enabled, Ivy prints a URL such as
`http://127.0.0.1:7654/s/<id>#token=<token>`. Open it in a current browser, or
bind Ivy to your Mac's concrete LAN address and open the printed URL on a phone
using the same trusted Wi-Fi. The token fragment is not sent in HTTP requests;
the browser moves it into per-tab session storage and removes it from the
address bar.

The built binary finds mobile assets in `dist/web` beside it. Source-tree
development can override the location with `IVY_WEB_DIR=web/dist`.

## Development

```bash
make lint
make test
make build
make dev ARGS=bash IVY_LISTEN=127.0.0.1:7654
```

See [docs/manual-testing.md](docs/manual-testing.md) for the compatibility test
matrix and current verified results. See [docs/sessions.md](docs/sessions.md) for
the Session contract, [docs/protocol.md](docs/protocol.md) for the transport
protocol, [docs/mobile-client.md](docs/mobile-client.md) for the browser
lifecycle, and [SECURITY.md](SECURITY.md) for the security model.

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

- Secure QR pairing and mobile helper controls
- Embedded web assets and single-binary releases

Hosted relay work is intentionally deferred until the local PTY and session
layers are solid.

## Platforms

Milestone 4 targets macOS and Linux on amd64 and arm64. The browser client
targets current stable Safari and Chrome. Windows is not currently supported.

## License

MIT. See [LICENSE](LICENSE).
