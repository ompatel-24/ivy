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

Milestone 7 packages Ivy as versioned, verifiable release archives and a
Homebrew formula. The mobile-first xterm.js client remains embedded inside the
Ivy binary.
Setting `IVY_LISTEN` adds:

- a same-origin browser terminal for the active Session;
- a scannable authenticated Session URL in the local terminal;
- one-tap Ctrl+C, Ctrl+D, Esc, Tab, and arrow controls;
- raw binary terminal input/output and responsive PTY resizing;
- bounded-history replay and automatic reconnection after network loss; and
- per-tab token handoff without cookies, query strings, or persistent storage.

The production distribution is one self-contained executable. There is still
no TLS termination, E2EE, or hosted relay. Networking is disabled unless
explicitly enabled.

## Install

On macOS or Linux with Homebrew:

```bash
brew install ompatel-24/tap/ivy
ivy version
```

GitHub releases contain archives for macOS and Linux on amd64 and arm64. Each
archive contains only `ivy`, `README.md`, and `LICENSE`. To install an archive,
download the one for your platform, verify it, and place `ivy` somewhere on
your `PATH`:

```bash
shasum -a 256 -c ivy_0.1.0_checksums.txt
gh attestation verify ivy_0.1.0_darwin_arm64.tar.gz --repo ompatel-24/ivy
tar -xzf ivy_0.1.0_darwin_arm64.tar.gz
install ivy /usr/local/bin/ivy
```

Run `gh attestation verify` for the checksum manifest and every archive you
download. Checksums detect corruption; GitHub provenance verifies that an
artifact was built by Ivy's release workflow.

## Build

Ivy requires Go 1.23 or newer. `make build` also requires a Vite-compatible
Node.js/npm toolchain so it can verify and regenerate the committed browser
bundle. Plain `go build` uses the committed bundle without Node.js, and Node.js
is never needed to run Ivy.

```bash
make build          # produces the self-contained dist/ivy
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

When enabled in an interactive terminal, Ivy prints a QR code plus a URL such
as `http://127.0.0.1:7654/s/<id>#token=<token>`. Scan the QR code, open the URL
directly, or bind Ivy to your Mac's concrete LAN address and scan it from a
phone using the same trusted Wi-Fi. Narrow, dumb, and non-interactive terminals
receive only the one-line URL fallback. The token fragment is not sent in HTTP
requests; the browser moves it into per-tab session storage and removes it from
the address bar.

The QR code contains the same session-long bearer token as the printed URL. Do
not share screenshots of it. Public Wi-Fi may isolate clients from one another;
Ivy's future hosted relay will address that reachability problem.

Mobile assets are embedded in every Go build. `IVY_WEB_DIR` remains an explicit
development/testing override; when set, Ivy requires that directory to contain
a valid production client and fails before launching the child if it does not.

## Development

```bash
make lint
make test
make build
make release-check
make release-snapshot
make dev ARGS=bash IVY_LISTEN=127.0.0.1:7654
```

See [docs/manual-testing.md](docs/manual-testing.md) for the compatibility test
matrix and current verified results. See [docs/sessions.md](docs/sessions.md) for
the Session contract, [docs/protocol.md](docs/protocol.md) for the transport
protocol, [docs/mobile-client.md](docs/mobile-client.md) for the browser
lifecycle, and [SECURITY.md](SECURITY.md) for the security model.
Maintainers should also read [docs/releasing.md](docs/releasing.md) before
creating a version tag.

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

- TLS/WSS and hosted relay connectivity
- End-to-end encryption
- Apple signing and additional package formats

Windows support, casks, automatic version selection, and hosted relays remain
deferred.

## Platforms

Milestone 7 publishes macOS and Linux archives for amd64 and arm64. The browser
client targets current stable Safari and Chrome. Windows is not currently
supported.

## License

MIT. See [LICENSE](LICENSE).
