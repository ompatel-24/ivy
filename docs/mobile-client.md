# Mobile browser client

Milestone 6 embeds the small vanilla TypeScript and xterm.js client into the
Rome executable. It is a terminal surface, not an agent-specific UI: all input
and output remain raw PTY bytes.

## Build and run

```bash
make build
ROME_LISTEN=127.0.0.1:7654 ./dist/rome bash
```

`make build` produces one self-contained `dist/rome`; no adjacent asset
directory is required. `make web` regenerates the committed production bundle,
and source development can use:

```bash
make dev ARGS=bash ROME_LISTEN=127.0.0.1:7654
```

A plain `go build` embeds the committed bundle without invoking Node.js.
`ROME_WEB_DIR` can explicitly replace the embedded client for development or
failure testing; Rome validates the override before launching the child.

For a phone on the same trusted Wi-Fi, replace loopback with the Mac's concrete
LAN address. Never use a wildcard address. Open the printed `/s/<id>#token=...`
URL manually or scan the QR code rendered by Rome. The QR contains the same
session-long bearer token as the URL and should not be shared or photographed.

## Connection behavior

The page authenticates to Session metadata, connects with the two required
WebSocket subprotocols, and writes raw binary output to xterm.js. Keyboard input
is UTF-8 encoded into binary frames. FitAddon and viewport observers send the
most recent terminal dimensions to the shared PTY.

The terminal accepts input only while its status is **Live**. A temporary
disconnect triggers bounded-backoff retries, and a successful reconnect resets
the terminal before replaying Rome's bounded history. Exit and protocol errors
disable input permanently for that page.

The bottom helper area provides one-tap Ctrl+C, Ctrl+D, Esc, Tab, and arrow
keys. Controls use the existing binary terminal input path, remain disabled
until the Session is Live, and preserve terminal focus after a tap. On narrow
phones the eight controls wrap into two rows so every target remains at least
44 pixels high and wide.

## Current limitations

- The transport is plaintext HTTP/WS and is only suitable for loopback or a
  trusted private LAN.
- The raw history snapshot can begin within an ANSI or UTF-8 sequence.
- Current xterm.js mobile support has limitations around touch selection,
  scrolling, predictive keyboards, and clipboard behavior.
- Closing the Rome process ends the page and transport; there is no hosted relay.
