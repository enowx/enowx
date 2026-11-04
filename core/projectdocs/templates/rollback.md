::progress

## When to roll back

_To fill: what triggers it (an error rate above a number, a main flow broken, data being written wrong), who decides, and the time to try a fix before going back. Roll back first and find the cause after. Link [[Monitoring, Logs, SLOs and Alerts#Alerts]]._

## Steps

_To fill: the exact commands or clicks for each part, how long they take, and how to confirm it worked. Name where old versions are kept and how many._

| Part | How it goes back | Time | Confirmed by |
|---|---|---|---|
| App | | | |
| Worker | | | |
| Mobile or desktop client | | | |

## Migrations and data

_To fill: how a release with a schema change goes back: changes written to be safe with the old code, what can't be undone, what happens to data written since the release, and when the only way is restoring [[Backup and Disaster Recovery#Restore steps]]._

## Flags and gradual release

_To fill: what can be turned off without a release, how a release reaches a share of users first, and who can flip them. Or "Skipped: all or nothing, because"._

## Third parties and side effects

_To fill: what a rollback doesn't undo: emails sent, payments taken, webhooks, a cache, an API contract others started using. What is done about each?_

## Tried

_To fill: when a rollback was last really done and how long it took. A plan nobody ran stays {{Unverified}}._

- [ ] Rollback tried on staging, on (date):

## Telling people

_To fill: who is told, where, and in what words. Add it to [[Runbook and Incident Response#Incident log]] and fix forward with a test._

## Open questions

## Change history
