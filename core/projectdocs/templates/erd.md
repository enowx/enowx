::progress

## Diagram

Draw every table or collection from the real schema or the migrations, never from memory: its name, primary key and the columns that matter, each relation with its cardinality and the foreign key behind it. Before there is a schema, draw from [[Technical Spec#Data model and migrations]] and mark the diagram {{Unverified}}.

_To fill: a canvas of kind diagram drawn from the schema and code, linked with docs diagram; no canvas yet._

## Entities and relations

_To fill: every entity and relation, which must match the diagram row for row. When the schema changes, the table, the diagram and [[Database Schema and Data Dictionary#Tables]] change with it._

| Entity | Primary key | What it holds |
|---|---|---|
| | | |

| From | To | Cardinality | Foreign key | On delete |
|---|---|---|---|---|
| | | 1:1 / 1:N / N:M | | |

## Source

_To fill: where the schema lives (migrations folder, schema file, ORM models), and the command that shows it as it is now._

## What the diagram leaves out

_To fill: tables not drawn and why (audit, jobs, framework tables), and the date the diagram was last checked against the schema, with the last migration it includes. Mark what wasn't compared with {{Unverified}}._

## Open questions

## Change history
