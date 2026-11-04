::progress

## Overview

_To fill: who uses this API, its base URLs by environment, how a caller authenticates (the method and its header, never a real key), and the content types. Errors, versions and limits follow [[Error Format, API Versions and Rate Limits#Error format]]._

## Contracts

_To fill: every endpoint and event, in the two blocks below or linked to the files in the repository they are generated from. A contract is the truth: code and documents follow it. Remove the block that doesn't apply, with a line "Skipped: no events" or "Skipped: no HTTP API"._

### HTTP (OpenAPI)

```yaml
openapi: 3.1.0
info:
  title: PROJECT API
  version: 0.1.0
servers:
  - url: https://api.example.com/v1
security:
  - bearerAuth: []
paths:
  /things:
    get:
      summary: List things
      operationId: listThings
      parameters:
        - name: cursor
          in: query
          schema: { type: string }
      responses:
        "200":
          description: A page of things
          content:
            application/json:
              schema:
                type: object
                required: [items]
                properties:
                  items:
                    type: array
                    items: { $ref: "#/components/schemas/Thing" }
                  next_cursor: { type: string }
        "401": { $ref: "#/components/responses/Error" }
components:
  securitySchemes:
    bearerAuth: { type: http, scheme: bearer }
  schemas:
    Thing:
      type: object
      required: [id]
      properties:
        id: { type: string }
  responses:
    Error:
      description: See the error format
```

### Events (AsyncAPI)

```yaml
asyncapi: 3.0.0
info:
  title: PROJECT events
  version: 0.1.0
channels:
  thingCreated:
    address: things.created
    messages:
      thingCreated:
        payload:
          type: object
          required: [id, occurred_at]
          properties:
            id: { type: string }
            occurred_at: { type: string, format: date-time }
operations:
  publishThingCreated:
    action: send
    channel: { $ref: "#/channels/thingCreated" }
```

## Permissions

_To fill: which role may call each endpoint or receive each event, from [[Roles and Permissions Matrix#Matrix]], and what a caller without permission gets back._

## Examples

_To fill: one full request and response for each main flow, with made-up data and a placeholder token. Mark a contract no one has run yet with {{Unverified}}._

## Open questions

## Change history
