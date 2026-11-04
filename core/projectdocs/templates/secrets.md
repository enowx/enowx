::progress

:::callout warning Names only
This document lists secrets by name and place. Never write a value here, not even an old or a test one.
:::

## Inventory

_To fill: every API key, token, password, signing key and certificate, by its variable name. Same list as [[Deployment Plan#Environments and secrets]]._

| Variable name | What it opens | Used by | Environments | Stored in | Owner | Last rotated |
|---|---|---|---|---|---|---|
| | | | | | | |

## Where secrets live

_To fill: the store for each environment (secret manager, vault, CI secrets), how a running service gets them, and how a developer gets those they need locally. Nothing in the repository, images or logs._

## Who has access

_To fill: who and what can read each store, how access is granted and removed, and what happens when someone leaves._

## Rotation

_To fill: how often each kind is rotated, the steps, and whether it can be done with no downtime. Say which were actually rotated once._

- [ ] Rotation tried on one secret, on (date):

## Keeping them out

_To fill: what stops a secret reaching places it shouldn't: ignored files, a commit scanner, masked CI logs, log filters, error reports stripped._

## When one leaks

_To fill: the steps in order: revoke, replace, check what was reached with it (the provider's logs), tell who must be told, remove it from history, record the incident. Link [[Runbook and Incident Response#Playbooks]]._

## Open questions

## Change history
