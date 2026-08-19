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
| `ivy bash` | Verified | macOS arm64, 2026-08-18; `go run ./cmd/ivy bash` and built binary |
| `ivy python` | Unavailable | No `python` or `python3` in the current PATH |
| `ivy vim` | Unavailable | Not installed in the current PATH |
| `ivy top` | Unavailable | Not installed in the current PATH |
| `ivy claude` | Unavailable | Not installed in the current PATH |
| `ivy codex` | Unavailable | Not installed in the current PATH |

## Future mobile checks

- [ ] Phone connects to the existing process
- [ ] Phone disconnect does not stop the child
- [ ] Phone reconnect resumes the same session
- [ ] Local and phone input both reach the PTY
- [ ] Local and phone clients receive the same output
