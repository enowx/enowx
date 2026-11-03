# Protocol

The app and the daemon (and the command line) talk over one byte stream: the daemon's Unix socket (`~/.enowx-app/enowxd.sock`) on the same machine, or `enowxd bridge` over SSH for a server. App and daemon update independently, so the protocol keeps working between versions; this is what it promises (`proto/`).

## Frames

Every message is a frame:

```
+--------+--------+-----------+----------+
| length | type   | channel   | payload  |
| u32 BE | u8     | u32 BE    | length-5 |
+--------+--------+-----------+----------+
```

`length` counts the type, the channel and the payload; a payload is at most 16 MiB.

| Type | Value | Channel | Payload |
|---|---|---|---|
| hello | 1 | 0 | JSON `Hello` |
| request | 2 | 0 | JSON `Request` (`id`, `method`, `params`) |
| response | 3 | 0 | JSON `Response` (`id`, `result` or `error`) |
| event | 4 | 0 | JSON `Event` (`name`, `data`), pushed by the daemon |
| output | 5 | a stream | a terminal's output, from the daemon |
| resize | 6 | a stream | the daemon's screen size before the output drawn at it: u16 cols, u16 rows, big endian |
| input | 7 | a stream | typed bytes, to the daemon |
| tunnel-data | 8 | a tunnel | bytes of a tunnel (`tunnel.open`), both ways |
| tunnel-close | 9 | a tunnel | the tunnel ended, from either side |
| tunnel-ack | 10 | a tunnel | credit for bytes taken: u32, big endian |
| part | 11 | the message's number | a piece of a larger frame (below) |

Streams come from `session.attach`, tunnels from `tunnel.open`.

### Frames larger than a frame

A canvas's page with its files written in, or the picture of one, can pass 16 MiB. Between two sides that both said `"parts": true` in their `Hello`, such a frame goes as `part` frames of up to 8 MiB, each with its message's inner type (u8), inner channel, index and total (u32, big endian) before its data; a whole message is at most 256 MiB. Other frames go on between the parts, and parts of different messages may interleave. A visitor (the team listener) is sent parts but may not send them.

Too large for a connection is never a reason to close it: the frame is refused alone. A reply becomes an RPC error saying so, a request's `Call` returns `proto.ErrTooLarge`, a render or a video started by an event fails at once with that error, and any other event doesn't reach that client and is logged (`frame too large, refused`). Only a failed write or read of the socket ends a connection. An app or daemon from before parts simply never gets them: it sees those errors, and what it could read still goes as one frame.

## Handshake and versions

Each side sends a `Hello` first: `{"protocol": N, "agent": "enowx/0.1.0"}`. The version `N` is that of the framing and the handshake, which change only incompatibly. A connection speaks the lower of the two sides' versions (`proto.Negotiate`), which must be `proto.MinVersion` or later; the daemon answers an older app with an error, and the app tells the user to reconnect, which updates the daemon. Today both are 1.

## Within a version

Methods, events and fields are added without a new version, under these rules, which `go test ./proto` enforces against a record of the API (`proto/testdata/api.txt`: every method and event name, frame type, constant value, and JSON field with its type):

- Nothing is removed or renamed, and no field changes type. A method kept only for older apps stays answered.
- New request and result fields are optional (`omitempty`): an older side leaves them out, a newer one reads them as zero. Unknown fields are ignored.
- A daemon answers a method it doesn't know with an error containing `unknown method`; the app then does without (`client.IsUnknownMethod`), or tells the user the host's daemon is older (`client.Explain`), which reconnecting updates.
- Events a side doesn't know are ignored.
- Behaviors gated on a new daemon are found by asking (a new method, or a field of `daemon.info`), not by comparing version strings.

Adding to the API means recording it: `go test ./proto -run API -update`, and checking the diff of `testdata/api.txt` has only additions.

## Methods

The methods, by domain, are in `proto/*.go` (each file's constants say what they take and return) and are handled in `daemon/rpc_*.go`; `TestEveryMethodHasHandler` checks every one has a handler. `testdata/api.txt` lists them all.
