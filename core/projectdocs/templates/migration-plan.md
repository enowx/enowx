::progress

## Source and target

_To fill: what data moves, from where to where, how much of it, and how much may be lost or wrong (usually none). Link [[ERD#Entities and relations]]. Connections by variable name, never their values. If there is nothing to move, "Skipped: no existing data" and go to the seed data._

## Mapping

_To fill: each source field to its target field, with the conversion, the default for missing values, and what happens to data that doesn't fit._

| Source | Target | Conversion | When missing or invalid |
|---|---|---|---|
| | | | |

## Seed data

_To fill: what a new environment needs to start (roles, plans, settings, a first admin), where the seed lives and whether it is safe to run twice. Test data never reaches production. A first admin's password is generated or set by the owner, never written here._

## Schema migrations

_To fill: the tool, how a migration is written, reviewed and applied, whether old code keeps working during it, and how long the biggest table locks._

## Steps

_To fill: the order of the move with who runs each step and how long it took in rehearsal: backup, freeze or dual write, copy, verify, switch, unfreeze._

- [ ] Backup taken and its restore tried first: [[Backup and Disaster Recovery#Restore test]]
- [ ] Rehearsed on a copy of real data, on (date):
- [ ] Moved in production, on (date):

## Verification

_To fill: how the result is proved right: row counts, sums, a sample read by hand, the main flows run. Say who signs it off._

## Way back

_To fill: until when the move can be undone and how, what happens to data written since, and who decides._

## Open questions

## Change history
