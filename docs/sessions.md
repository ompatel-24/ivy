# Session contract

Milestone 2 moves PTY and child-process ownership into an internal,
transport-independent Session. The existing local terminal runner uses the same
Session operations that later HTTP and WebSocket transports will use. No Session
API is exported outside Ivy yet.

## Identity and metadata

Each Session receives a cryptographically random 128-bit ID encoded with
unpadded base64url. The ID is routing metadata only, not authentication or an
access token. Command arguments and the starting directory are copied when the
Session starts and returned as defensive snapshots.

The in-memory Manager can start, look up, and close Sessions. Completed Sessions
remain registered until Manager shutdown so their final state and bounded
history stay available.

## Input, output, and history

One goroutine reads the PTY. Every output chunk is appended to a 512 KiB
raw-byte ring and offered to all subscribers. Subscription is atomic with the
history snapshot, so a new subscriber receives the snapshot and then live bytes
without a gap.

Input writes share a mutex. A complete write from one source cannot interleave
with a concurrent write from another source. Resizes and process-group signals
are also routed through the Session.

Each subscriber has a bounded queue of 64 chunks. If that queue fills, the
subscriber is closed with a slow-consumer error; the child and other subscribers
continue running. History retains bytes rather than parsed terminal state, so a
wrapped snapshot can start midway through an ANSI sequence or UTF-8 character.

## Lifecycle

Session shutdown coordinates context cancellation, process waiting, PTY
closure, output draining, and subscriber closure. `SIGINT` is forwarded to the
child process group. `SIGTERM` and `SIGHUP` are forwarded and escalate to
`SIGKILL` after two seconds or a second termination request. Child exit codes
and signal-derived exit codes retain the existing CLI behavior.
