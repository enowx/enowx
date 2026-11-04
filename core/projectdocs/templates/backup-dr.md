::progress

:::callout warning A backup counts once it was restored
Until a restore has really been done, the backup is {{Unverified}} and this document isn't done.
:::

## Targets

_To fill: how much data may be lost (RPO) and how long the service may be down (RTO), for each part. Ask what an hour of each costs. Mark a target nobody agreed to with {{Assumption}}._

| Part | Data lost at most (RPO) | Down at most (RTO) | Why |
|---|---|---|---|
| | | | |

## What is backed up

_To fill: everything that can't be rebuilt: databases, uploaded files, configuration, secrets' store, DNS. Keep at least one copy in another place and another account from production. Encryption keys by name and where they are stored, kept apart from the backups._

| What | How | How often | Where kept | Kept for | Encrypted with (key's name) |
|---|---|---|---|---|---|
| | | | | | |

## Restore steps

_To fill: the exact steps to restore each part, someone other than the author able to follow them, and how the result is checked (counts, sign-in, the main flow)._

## Restore test

_To fill: how often a real restore to an empty environment is tried and by whom, and how long it took against the target. Add a line for each test; leave it unticked until it has been done._

- [ ] restore tested on (date):

## Disaster scenarios

_To fill: for each one, what is lost, the steps back, how long it takes and who decides: a server dies, the database is corrupted or deleted by mistake, the region or provider is gone, the account is lost or locked, ransomware reaches the backups, the only person who knows is away._

| Scenario | What is lost | Steps back | Time | Who decides |
|---|---|---|---|---|
| | | | | |

## Monitoring of backups

_To fill: what alerts when a backup fails or stops, and who checks its size and age. Link [[Monitoring, Logs, SLOs and Alerts#Uptime and outside checks]]._

## Open questions

## Change history
