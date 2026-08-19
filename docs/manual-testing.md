# Manual compatibility testing

Do not mark a command as compatible until every applicable local check has been
performed. Mobile checks begin in the mobile-client milestone.

## Local checks

- [x] Child sees an interactive TTY
- [x] ANSI colors render correctly
- [x] Arrow keys and command history work
- [x] Tab and backspace work
- [x] Ctrl+C reaches the foreground child process
- [x] Ctrl+D produces terminal EOF behavior
- [x] Resizing the local window updates `stty size`
- [x] Child exit restores the local terminal
- [x] Child exit code is preserved

## Program matrix

| Program | Status | Notes |
| --- | --- | --- |
| `ivy bash` | Verified | macOS arm64, 2026-08-19; Milestone 3 built binary with local-only and network-enabled paths |
| `ivy python` | Unavailable | No `python` or `python3` in the current PATH |
| `ivy vim` | Unavailable | Not installed in the current PATH |
| `ivy top` | Unavailable | Not installed in the current PATH |
| `ivy claude` | Unavailable | Not installed in the current PATH |
| `ivy codex` | Verified | macOS arm64, 2026-08-19; full-screen UI launched through the built binary |

## Session checks

These Milestone 2 behaviors remain covered by race-enabled automated integration
tests:

- [x] Two subscribers receive identical live output
- [x] A late subscriber receives history followed by gap-free live output
- [x] Subscriber disconnect does not terminate the child
- [x] A slow subscriber is disconnected without stalling a healthy subscriber
- [x] Concurrent writes reach the PTY as complete, non-interleaved writes
- [x] Manager shutdown closes all registered sessions

## Local transport checks

These behaviors are covered by race-enabled HTTP/WebSocket integration tests:

- [x] Networking remains disabled and quiet without `IVY_LISTEN`
- [x] Unsafe wildcard listeners fail before the child launches
- [x] Health exposes no Session data
- [x] Metadata and WebSockets reject missing or invalid tokens
- [x] Cross-origin WebSockets are rejected
- [x] Binary phone-style input reaches the same PTY
- [x] History is replayed before gap-free live output on reconnect
- [x] Two WebSockets receive the same output
- [x] Remote resize updates the PTY
- [x] Disconnect and malformed input do not terminate the child
- [x] Slow clients are disconnected without stalling healthy clients
- [x] Child exit is delivered after terminal output
- [x] Active WebSockets are closed within a bounded server shutdown

Manual loopback smoke test:

```bash
IVY_LISTEN=127.0.0.1:7654 ./dist/ivy bash
curl http://127.0.0.1:7654/health
```

Copy the printed Session ID and fragment token into a separate shell to query
the authenticated metadata endpoint. Do not paste real Session tokens into bug
reports or committed files.

## Future mobile checks

Milestone 3 has no browser terminal, so these remain intentionally unverified.

- [ ] Phone connects to the existing process
- [ ] Phone disconnect does not stop the child
- [ ] Phone reconnect resumes the same session
- [ ] Local and phone input both reach the PTY
- [ ] Local and phone clients receive the same output
