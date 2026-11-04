::progress

## What matters most

_To fill: what must never break (money, sign-in, data loss) and where a bug is cheapest to live with, so tests go where the risk is. Link [[PRD#Features and priorities]]._

## Levels of testing

_To fill: what each level covers here, with what tool, where it runs and how fast it must stay. Name real flows, not "the main ones". Or a level's row saying "Skipped: why"._

| Level | Covers | Tool | Runs on | Target time |
|---|---|---|---|---|
| Unit | | | | |
| Integration | | | | |
| End-to-end | | | | |

## Test data and environments

_To fill: where tests run, how their data is made and cleaned, what is faked and what is real (a real database beats a mock), and how third parties are stood in for. No production data or production keys._

## Acceptance

_To fill: how each acceptance criterion in [[User Stories and Acceptance Criteria#Acceptance criteria]] is proved, and who ticks it. Hand-run cases go in [[Test Cases#Cases]]._

## Other kinds

_To fill: what else is tested and when, or "Skipped: why" for each: permissions from [[Roles and Permissions Matrix#How it is tested]], load from [[Performance and Load Targets#Load tests]], accessibility from [[Accessibility#How it is checked]], upgrades and migrations._

## In CI

_To fill: which tests run on every change, which nightly, what blocks a merge, and how a flaky test is dealt with._

## When a release may ship

_To fill: the conditions: tests green, no open bug of what severity, what was tried by hand. Link [[Definition of Done#A release is done when]]._

## Open questions

## Change history
