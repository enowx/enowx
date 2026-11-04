::progress

## Diagram

Draw how data moves, not how the code is called: people and outside systems as external entities, processes, data stores, and each flow labeled with the data it carries. Draw every trust boundary (browser to server, service to third party, one tenant to another). Source: the request handlers, the jobs and the code calling third parties.

_To fill: a canvas of kind diagram drawn from the schema and code, linked with docs diagram; no canvas yet._

## Flows

_To fill: each flow that crosses a trust boundary or carries personal data, with how it is protected. Classes come from [[Data Retention and Classification#Classification]] if there is one; threats go to [[Threat Model#Threats and handling]]._

| Flow | From | To | Data | Class | Crosses a boundary | Protected by |
|---|---|---|---|---|---|---|
| | | | | | Yes / No | |

## What the diagram leaves out

_To fill: what isn't drawn and where it is described (logs, backups, analytics often carry data too), and the date the diagram was last checked against the code. Mark what wasn't compared with {{Unverified}}._

## Open questions

## Change history
