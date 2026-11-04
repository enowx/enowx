::progress

## Roles

_To fill: each role, who holds it, how it is given and taken away, and what a new user starts as. Start from [[PRD#Roles and permissions]]._

| Role | Who | Given by | Default |
|---|---|---|---|
| | | | Yes / No |

## Matrix

_To fill: every resource and action against every role: Allow, Deny, or Own (only their own records). Deny is the default for anything not listed. Add a row for each admin and billing action._

| Resource | Action | Anonymous | Role A | Role B | Admin |
|---|---|---|---|---|---|
| | Read | Deny | | | |
| | Create | Deny | | | |
| | Change | Deny | | | |
| | Delete | Deny | | | |

## Ownership and tenancy

_To fill: what "own" means here (user, team, organization), how one tenant is kept from another's data, and how that is enforced at the query level._

## Where it is enforced

_To fill: the single place in the code each check happens, on the server and never only in the UI, and what a refused caller gets back. Link [[API Spec#Permissions]]._

## Audit

_To fill: which actions are logged with who, what and when (role changes, admin reads, deletes), how long, and who can read that log._

## How it is tested

_To fill: how the matrix is tested, a test for each Deny that matters most. Link [[Test Plan#Levels of testing]]. Mark rows not tested with {{Unverified}}._

## Open questions

## Change history
