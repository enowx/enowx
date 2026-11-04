::progress

## Classification

_To fill: the classes used here and how each must be handled. Change the suggestion below to fit; {{Assumption}} until the owner confirms it._

| Class | Means | Examples | Handling |
|---|---|---|---|
| Public | Anyone may see it | | |
| Internal | Staff only | | |
| Personal | Identifies a person | | Encrypted at rest, access logged |
| Restricted | Harm if leaked: payment, health, ID numbers | | Stored only if needed, masked everywhere |

## Inventory

_To fill: each kind of data, wherever it is: databases, files, logs, backups, analytics, third parties. Tables come from [[Database Schema and Data Dictionary#Sensitive columns]]._

| Data | Class | Where | Why it is kept | Kept for | Then |
|---|---|---|---|---|---|
| | | | | | Deleted / Anonymized |

## Deletion

_To fill: what actually deletes data when its time comes (a job, by hand), how often, how it reaches backups and third parties, and how anyone knows it ran._

## Access

_To fill: who may read each class, how that access is granted and removed, and whether reads are logged. Roles come from [[Roles and Permissions Matrix#Matrix]]._

## Requests from users

_To fill: how a person's request to see, export or delete their data is carried out, step by step, and how long it may take. Rights and laws are in [[Privacy and Compliance#People's rights]]._

## Open questions

## Change history
