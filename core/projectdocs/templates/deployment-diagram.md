::progress

## Diagram

Draw what really runs in production, from the infrastructure code, compose files and the provider's console: every server or managed service with its region and size, what runs on each, the networks and firewalls between them, the domains and load balancers in front, and where backups go. Draw other environments only where they differ.

_To fill: a canvas of kind diagram drawn from the schema and code, linked with docs diagram; no canvas yet._

## Infrastructure

_To fill: each thing drawn with its provider, region, size and who can reach it. Environments come from [[Deployment Plan#Environments and secrets]]; costs go to [[Cost Estimate#Monthly costs]]. No addresses or credentials that give access._

| Resource | Provider and region | Size | Runs | Reachable from | Defined in |
|---|---|---|---|---|---|
| | | | | | |

## Single points of failure

_To fill: what stops the service when it fails, what happens then and what is accepted for now. Recovery goes in [[Backup and Disaster Recovery#Disaster scenarios]]._

## What the diagram leaves out

_To fill: what isn't drawn and where it is described (DNS details, CI runners, developers' access), and the date the diagram was last checked against the running system. Mark what wasn't compared with {{Unverified}}._

## Open questions

## Change history
