# Your data on every host

Notes, their groups, canvases, settings and the vault's encrypted data are the same on every host: this device, your servers, your other computers. Moving from the Mac to a VPS loses nothing.

## How

Each host keeps the data as files in its enowX folder (`~/.enowx-app`): `notes/`, `canvas/` (not a canvas's versions kept for undo, `canvas/<id>/history*`: those are each host's own), `vault/`, `settings/`, `catalog/` (each host's projects) and `sync/conflicts/`. Hosts don't reach each other, so the app does the syncing: this device's own daemon is the hub, and every other host the app reaches (the one shown, or one watched in the background) syncs with it when it connects, every 30 seconds, and right after a change on either side. A host offline keeps its changes and sends them when it is back; a new host gets everything at its first sync.

A sync is three-way, file by file (`core/usersync`), against what the two hosts had at their last sync, kept in this device's `usersync/` folder:

- a file changed on one side goes to the other;
- changed on both: a note merges its header field by field (the later `updated` wins) and its body line by line, JSON lists merge item by item (by `id` or `name`), other text line by line;
- changes to the same lines don't merge: the hub's version stays in place on both hosts and a conflict records both versions, until you pick one, or both (the other then becomes a copy, a note titled after its host), in **Settings → Sync** or with `enowx-app sync resolve`;
- a file deleted on one side is deleted on the other, unless the other changed it, which brings it back. Deleted files stay 30 days in each host's `trash/`, listed under **Recently deleted** to restore;
- at the first sync of two hosts, a note on both under different IDs (same title, same body) is kept once.

Kept per host, never synced: proposals agents made (their sessions are there), versions kept for undo, and project files (see project sync).

## Protocol

`sync.manifest`, `sync.read` and `sync.write` (writes only when the file still has the hash expected), `sync.report` and `sync.status`, `sync.request` (asks the apps to sync, `sync.requested`), `sync.conflicts`, `sync.resolve`, `sync.trash`, `sync.restore`, `sync.forget`, and `catalog.list`. A daemon without them (an older one) is skipped until it's updated.

Transfers are batched by their encoded JSON size, including base64 file data, within the protocol's 16 MiB frame limit. A single file that cannot fit returns a sync error without disconnecting the host. The batching uses the existing protocol and also works with older daemons; newer daemons return an RPC error if a reply grows past the limit after the manifest was read.

## Command line

```
enowx-app sync                    # this host's synced data and its syncs
enowx-app sync now                # ask the app to sync now, and wait
enowx-app sync conflicts          # what didn't merge
enowx-app sync resolve ID --keep HOST|N|both
enowx-app sync trash | restore ID
enowx-app sync forget --yes       # take the synced data off this host
```

The same are MCP tools: `sync_status`, `sync_now`, `sync_conflicts`, `sync_resolve`, `sync_trash`, `sync_restore`.

## Older apps

A canvas's versions stopped being synced in 0.6.0. An older app would take a host's not listing them for their deletion, and delete them on the other host, so a host on 0.6.0 or later gives it no manifest: its sync fails, saying to update the app, and nothing is changed. Update every device and host together.
