::progress

## Services

_To fill: every outside service, payment included. Credentials by variable name and where each is stored, never their values: see [[Secret Handling#Inventory]]. Mark a price or a limit not read on the service's own page with {{Unverified}}._

| Service | What for | Credential (variable name) | Stored in | Sandbox | Limits and price | Owner of the account |
|---|---|---|---|---|---|---|
| | | | | Yes / No | | |

## When a service fails

_To fill: for each service, what users see when it is slow or down, the timeout and retries, whether work is queued, and how it catches up._

| Service | Timeout | Retries | What users see | Catching up |
|---|---|---|---|---|
| | | | | |

## Webhooks received

_To fill: each webhook with how its signature is checked, how a repeat delivery is ignored (idempotency key), the order assumed, and how a missed one is found and fetched again. Or "Skipped: none"._

| From | Event | URL | Signature check | Idempotency key | If missed |
|---|---|---|---|---|---|
| | | | | | |

## Webhooks sent

_To fill: each webhook this project sends, how its receiver verifies it, and the retry schedule. Payloads go in [[API Spec#Contracts]]. Or "Skipped: none"._

## Payment

_To fill: the provider and flow, what is stored here (never full card numbers), how a payment's state is settled against the provider's, refunds and disputes, and how a price change reaches customers. Link [[Sequence Diagrams#Flows covered]]. Or "Skipped: no payment"._

## Leaving a service

_To fill: for the services hardest to replace, how the data gets out and what would replace it._

## Open questions

## Change history
