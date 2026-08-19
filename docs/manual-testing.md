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
| `ivy bash` | Verified | macOS arm64, 2026-08-19; Milestone 2 built binary |
| `ivy python` | Unavailable | No `python` or `python3` in the current PATH |
| `ivy vim` | Unavailable | Not installed in the current PATH |
| `ivy top` | Unavailable | Not installed in the current PATH |
| `ivy claude` | Unavailable | Not installed in the current PATH |
| `ivy codex` | Verified | macOS arm64, 2026-08-19; full-screen UI launched through the built binary |

## Session checks

These behaviors are covered by race-enabled automated integration tests because
Milestone 2 does not yet expose a network or subscriber CLI:

- [x] Two subscribers receive identical live output
- [x] A late subscriber receives history followed by gap-free live output
- [x] Subscriber disconnect does not terminate the child
- [x] A slow subscriber is disconnected without stalling a healthy subscriber
- [x] Concurrent writes reach the PTY as complete, non-interleaved writes
- [x] Manager shutdown closes all registered sessions

## Future mobile checks

- [ ] Phone connects to the existing process
- [ ] Phone disconnect does not stop the child
- [ ] Phone reconnect resumes the same session
- [ ] Local and phone input both reach the PTY
- [ ] Local and phone clients receive the same output
