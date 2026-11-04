# Your data on every host

Notes, their groups, canvases, settings and the vault's encrypted data are the same on every host: this device, your servers, your other computers. Moving from the Mac to a VPS loses nothing.

## How

Each host keeps the data as files in its enowX folder (`~/.enowx-app`): `notes/`, `canvas/` (not a canvas's versions kept for undo, `canvas/<id>/history*`: those are each host's own), `vault/`, `settings/`, `catalog/` (each host's projects) and `sync/conflicts/`. Hosts don't reach each other, so the app does the syncing: this device's own daemon is the hub, and every other host the app reaches (the one shown, or one watched in the background) syncs with it when it connects, every 30 seconds, and right after a change on either side. A round where neither host's manifest changed since the last good one stops there: no base read or written, no list of conflicts or trash read. A host offline keeps its changes and sends them when it is back; a new host gets everything at its first sync.

A sync is three-way, file by file (`core/usersync`), against what the two hosts had at their last sync, kept in this device's `usersync/` folder:

- a file changed on one side goes to the other;
- changed on both: a note merges its header field by field (the later `updated` wins) and its body line by line, JSON lists merge item by item (by `id` or `name`), other text line by line, and a log (`vault/audit.jsonl`) keeps every line of both;
- changes to the same lines don't merge: the hub's version stays in place on both hosts and a conflict records both versions, until you pick one, or both (the other then becomes a copy, a note titled after its host), in **Settings → Sync** or with `enowx-app sync resolve`;
- a file deleted on one side is deleted on the other, unless the other changed it, which brings it back. Deleted files stay 30 days in each host's `trash/`, listed under **Recently deleted** to restore;
- at the first sync of two hosts, a note on both under different IDs (same title, same body) is kept once.

## Logs

`vault/audit.jsonl` only ever grows by a line, on each host. Merged as text, two hosts' new lines would conflict every time, each conflict holding the whole log twice and the second host's lines surviving only there. So it merges as a log (`usersync.AppendOnly`): every line of both, a line both have once, the same line several times on a host as many times as the host having it most, each host's lines in their order, and between hosts by the time they say; a clock that's off moves a host's lines against another's, never among its own. No line is changed, so it stays what it was: JSON lines without values, not encrypted, in a file only the user reads.

Conflicts on a log left from before, or made by an older app, need no one: a daemon asked for its manifest or its conflicts writes every version into the file, with what's there, then moves the conflict to its trash, kept the 30 days, not listed under **Recently deleted**. A host's writing a line and a sync replacing the file take the same lock.

The log's own limit still applies: past 800 KB, the host recording a use keeps the last 1000 lines. A merge doesn't apply it, so what one host trimmed can come back from another until it trims again, and only a host changing the log alone between two syncs carries its trim over.

Kept per host, never synced: proposals agents made (their sessions are there), versions kept for undo, and project files (see project sync).

## Protocol

`sync.manifest`, `sync.read` and `sync.write` (writes only when the file still has the hash expected), `sync.report` and `sync.status`, `sync.request` (asks the apps to sync, `sync.requested`), `sync.conflicts`, `sync.conflict`, `sync.resolve`, `sync.trash`, `sync.restore`, `sync.forget`, and `catalog.list`. A daemon without them (an older one) is skipped until it's updated.

Transfers are batched by their encoded JSON size, including base64 file data, within the protocol's 16 MiB frame limit. A single file that cannot fit returns a sync error without disconnecting the host. The batching uses the existing protocol and also works with older daemons; newer daemons return an RPC error if a reply grows past the limit after the manifest was read.

`sync.conflicts` names the conflicts and each version's host and size, without its data: however large the files, it's a few hundred bytes each. `sync.conflict` reads one version's data, when something wants to show it. No app ever read the data from the list, so an older app gets what it uses; an older daemon still sends it, which the client drops, and the app only asks after files changed.

## Command line

```
enowx-app sync                    # this host's synced data and its syncs
enowx-app sync now                # ask the app to sync now, and wait
enowx-app sync conflicts          # what didn't merge
enowx-app sync show ID [--version N]   # a version of a conflict
enowx-app sync resolve ID --keep HOST|N|both
enowx-app sync trash | restore ID
enowx-app sync forget --yes       # take the synced data off this host
```

The same are MCP tools: `sync_status`, `sync_now`, `sync_conflicts`, `sync_resolve`, `sync_trash`, `sync_restore`.

## Older apps

A canvas's versions stopped being synced in 0.6.0. An older app would take a host's not listing them for their deletion, and delete them on the other host, so a host on 0.6.0 or later gives it no manifest: its sync fails, saying to update the app, and nothing is changed. Update every device and host together.
