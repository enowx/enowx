# Guarded terminal input

MyGo v0.6.2 has separate `Text()` and `Send()` operations. A permission/menu
redraw can complete after the text sample but before Enter is enqueued. Its
public API has no operation that covers both. This directory adds one method,
`Terminal.SendChecked(data, allow) bool`, inside the terminal package.

The addition is in `guarded_input.go.txt`, with tests in
`guarded_input_test.go.txt`. They are Go source stored as embedded resources so
they are compiled only in the dependency's package. No existing dependency file
is rewritten. No entire dependency fork is checked in.

`SendChecked` holds `Terminal.mu`, then `inputQueue.mu`, through a pure screen
predicate and FIFO insertion. `Terminal.write`/`Feed` use the same terminal
mutex; terminal replies already use this lock order. The queue consumer releases
the queue lock before PTY I/O or completion callbacks. Data is copied, and a
closed terminal/queue refuses input. The predicate must not block or reenter
the terminal. The session holds manager -> input -> session locks before this
call, so known hook/state/program/manual-input transitions cannot invalidate
the guard between validation and enqueue. Notifications run after unlocking.

Success means **enqueued**. It does not mean the PTY consumed Enter, nor that a
native permission was accepted. Output that reaches the terminal after enqueue
is later in this ordering. An unsafe screen that appeared and disappeared
before inspection is detected only if the session observed it during settling;
this API does not promise a historical classification of every raw redraw.

## Reproducible use

Use the shell entrypoint from the repository root with Go 1.27.3+ (Git Bash
on Windows):

```sh
sh scripts/terminalpatch.sh -- test ./core/session ./daemon
sh scripts/terminalpatch.sh -- test github.com/egoist/mygo/plugins/terminal -run '^TestEnowxCheckedInput' -count=10
sh scripts/terminalpatch.sh -- vet ./core/session ./daemon
```

`terminalpatch.sh` asks the selected Go toolchain for GOHOSTOS/GOHOSTARCH and
compiles the preparer for that host in a private temporary directory. It runs
the preparer with the original environment, so an inherited GOOS/GOARCH still
selects the child's build target. The bootstrap executable is removed when the
child finishes. `HOST_GO` selects an underlying Go executable, including a path
containing spaces, rather than another wrapper. Its ordinary GOTOOLCHAIN
selection still applies. Preparation and child Go commands use that same
executable, without recursive bootstrap.

`make` targets, the daemon/package scripts and applicable CI/release commands
use this entrypoint. Direct `go test/build` of packages importing session requires
the prepared modfile; otherwise the missing `SendChecked` method fails compilation.
If overriding Make's `GO`, retain the wrapper or supply its prepared modfile.

Go forbids overlays beneath `GOMODCACHE`. The tool therefore stages the pinned
module (about 16 MiB) in a unique temporary directory, adds these two files, and
selects it through a private modfile and `GOFLAGS`. Child Go commands inherit
that modfile; the wrapper removes its own temporary directory after the child
exits. The module cache and repository `go.mod`/`go.sum` are untouched. To retain
a fixture or cross compile with a host bootstrap:

```sh
flags=$(sh scripts/terminalpatch.sh -out "/absolute/new fixture/go.mod" -print-go-flags)
GOFLAGS="$flags" go test ./core/session ./daemon
```

The output directory must be new; preparation refuses to overlay an existing
module copy. Use `TMPDIR` (Unix) or `TMP`/`TEMP` (Windows) to select the staging
drive. `TMPDIR` selects the shell bootstrap directory. `-print-go-flags` emits
the existing GOFLAGS plus a quoted modfile token using the preparer's native
path spelling. This also handles Git Bash's argument-path conversion on Windows;
putting a shell-only `/c/...` path inside GOFLAGS would not convert it for Go.
An existing unrelated `-modfile` is refused by the wrapper.

The tool requires un-replaced MyGo v0.6.2 and verifies SHA-256 of the terminal
owner (`terminal.go`):
`7902afddc644b21ffcbc0d567a144b430ef579c8dd538f80499c99beb412738d`.
On a MyGo upgrade, review the mutex/queue layout, adopt a supported upstream
operation if available, and remove this patch and bootstrap when obsolete.

## Deterministic boundary test

`TestEnowxCheckedInputRedrawBoundary` pauses the readiness predicate with channels.
With no PTY, watcher, UI or queue-consumer goroutine, `TryLock` directly proves
whether a raw redraw can acquire the terminal mutex at that boundary. If it
can, the test completes an unsafe `Feed` before allowing enqueue and fails on
the queued Enter. With the patch, that lock remains held, Enter is enqueued
before the redraw can complete, and a later redraw-first attempt is refused.
No delay or timeout is used to manufacture the interleaving.

For fail-before reproduction, in a fresh **private fixture copy only**, replace
the addition's implementation with the old split sequence:

```go
func (t *Terminal) SendChecked(data []byte, allow func(string) bool) bool {
    if !allow(t.Text()) { return false }
    t.Send(data)
    return true
}
```

Run the exact boundary test above. This is an instrumented extraction of the
old session's `Text`/predicate/`Send` sequence, not a native permission fixture.
The queue test separately checks FIFO order, copy ownership and refusal after
closure. Existing daemon-protocol and native fixture tests cover the delivery
ledger, interrupted retries, manual drafts and lost responses.
