# Mobile browser client

Milestone 5 provides a small vanilla TypeScript and xterm.js client for the
local authenticated transport. It is a terminal surface, not an agent-specific
UI: all input and output remain raw PTY bytes.

## Build and run

```bash
make build
IVY_LISTEN=127.0.0.1:7654 ./dist/ivy bash
```

`make build` produces `dist/ivy` and its required `dist/web` directory. The
frontend is not embedded until Milestone 6. `make web` builds only `web/dist`,
and source development can use:

```bash
make dev ARGS=bash IVY_LISTEN=127.0.0.1:7654
```

For a phone on the same trusted Wi-Fi, replace loopback with the Mac's concrete
LAN address. Never use a wildcard address. Open the printed `/s/<id>#token=...`
URL manually or scan the QR code rendered by Ivy. The QR contains the same
session-long bearer token as the URL and should not be shared or photographed.

## Connection behavior

The page authenticates to Session metadata, connects with the two required
WebSocket subprotocols, and writes raw binary output to xterm.js. Keyboard input
is UTF-8 encoded into binary frames. FitAddon and viewport observers send the
most recent terminal dimensions to the shared PTY.

The terminal accepts input only while its status is **Live**. A temporary
disconnect triggers bounded-backoff retries, and a successful reconnect resets
the terminal before replaying Ivy's bounded history. Exit and protocol errors
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
- Closing the Ivy process ends the page and transport; there is no hosted relay.
