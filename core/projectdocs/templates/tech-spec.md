::progress

## Scope

_To fill: what this spec covers and what it doesn't, in a few lines. Link [[PRD#Features and priorities]] rather than repeating the features._

## Stack and reasons

_To fill: each part of the stack with why it was picked and what it was picked over. A choice with lasting weight also gets an entry in [[Architecture Decision Records#Decisions]]. Mark a suggestion the owner hasn't confirmed with {{Assumption}}._

| Part | Choice | Why | Instead of |
|---|---|---|---|
| Language and runtime | | | |
| Framework | | | |
| Database | | | |
| Hosting | | | |

## Architecture

_To fill: the main parts, what each owns, and how they talk (calls, events, queues, files). Say where state lives and what happens when each part is down. Link [[Architecture Diagram (C4)#Diagram]] when one is drawn._

## Data model and migrations

_To fill: the main entities and what changes them, how schema changes are written and applied, and how a release with a migration stays safe while old code still runs. Link [[ERD#Entities and relations]] rather than repeating it._

## API

_To fill: the style (REST, GraphQL, RPC, events), who calls it and how it is versioned. The contracts themselves go in [[API Spec#Contracts]], or "Skipped: no API"._

## Authentication and authorization

_To fill: how people and services sign in (method, provider, session or token, its lifetime and how it ends), and where access is decided in the code. Roles come from [[PRD#Roles and permissions]]._

## Third-party integrations

_To fill: each outside service, payment included, with what it does for the project and what happens when it is down. Details go in [[Third-party Integrations and Webhooks#Services]], or "Skipped: none"._

## Structure and conventions

_To fill: the folder layout with what lives where, the code standards and linters, naming, how errors are handled, and the rules agents and people must follow in this repository._

```text
src/
```

## Security

_To fill: the main attacks on this project and how each is handled: input, authentication, access, secrets, dependencies. Link [[Threat Model#Threats and handling]] if there is one. Secrets by variable name only, as in [[Deployment Plan#Environments and secrets]]._

## Observability

_To fill: what is logged and at what level (never secrets or personal data), which metrics, and which alerts wake someone. Link [[Monitoring, Logs, SLOs and Alerts]] if there is one._

## Design system

_To fill: the colors, type and components this UI uses, and the library behind them if any. Link [[Design System]] if there is one, or "Skipped: no UI"._

## Performance and limits

_To fill: the numbers this design must meet and what it can't do (largest file, most users, slowest acceptable call), or link [[Performance and Load Targets#Targets]]._

## Open questions

## Change history
