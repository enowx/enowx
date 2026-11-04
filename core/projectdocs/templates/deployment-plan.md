::progress

## Environments and secrets

_To fill: each environment and what it is for; then every variable by name with where it is stored. Never a secret's value in this document. Link [[Secret Handling#Inventory]] if there is one._

| Environment | Purpose | URL | Data |
|---|---|---|---|
| Development | | | Made up |
| Staging | | | |
| Production | | | Real |

| Variable name | Purpose | Secret | Environments | Stored in |
|---|---|---|---|---|
| | | Yes / No | | |

## CI/CD, domain and deploy

_To fill: what builds and checks each change, what triggers a deploy to each environment and who may do it, the steps of a deploy and how it avoids downtime, where migrations run in that order, the domains, DNS and certificates and who renews them._

```text
push -> checks -> build -> staging -> approve -> production -> health check
```

## Backup and restore

_To fill: what is backed up (database, files, configuration), how often, where it is kept, for how long, and the steps to restore it. An untested backup doesn't count: leave the to-do below unticked until a restore has really been done. Link [[Backup and Disaster Recovery]] if there is one._

- [ ] restore tested on (date):

## Rollback

_To fill: how a bad release is undone: the command or steps, how long it takes, what happens to a migration already run, who decides. Link [[Rollback Plan]] if there is one._

## Monthly cost and limit

_To fill: each paid thing with its monthly cost, the total, the limit that must not be passed and what alerts when it nears. Mark a price not read on the provider's page with {{Unverified}}. Link [[Cost Estimate]] if there is one._

| Item | Provider | Per month | Grows with |
|---|---|---|---|
| | | | |

Limit per month:

## Legal

_To fill: the terms of service, the privacy policy and the licenses of what is used, and whether each is done. Link [[Terms of Service and Privacy Policy]] and [[Privacy and Compliance#Status]] if they exist, or "Skipped: internal tool, no personal data"._

## Launch

_To fill: how it is announced, where its documentation is, how users get help and who answers. Link [[Launch Plan]] and [[User and API Documentation]] if they exist._

## Open questions

## Change history
