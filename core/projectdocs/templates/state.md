::progress

## Entities with a status

_To fill: each entity whose status matters (order, payment, subscription), the field that holds it and where that field is written in the code. Link [[ERD#Entities and relations]]._

| Entity | Status field | Values | Written in |
|---|---|---|---|
| | | | |

## Diagram

Draw one state machine per entity from its status field and the code that changes it: every value as a state, every transition with its event and condition, the start and end states. A value the code can set but no path reaches is a bug to report, not to draw around.

_To fill: a canvas of kind diagram drawn from the schema and code, linked with docs diagram; no canvas yet._

## Transitions

_To fill: every allowed transition. Anything not listed must be refused by the code; say where it is refused. Link [[Test Cases#Cases]] for those tested._

| Entity | From | Event | Condition | To | Side effects |
|---|---|---|---|---|---|
| | | | | | |

## What the diagram leaves out

_To fill: entities and transitions not drawn and where they are described (manual fixes by support, data repairs), and the date the diagram was last checked against the code. Mark what wasn't compared with {{Unverified}}._

## Open questions

## Change history
