::progress

## Error format

_To fill: the one shape every error has, the list of codes with their meaning and HTTP status, and which are safe to retry. The message never carries stack traces or secrets. The form below is a suggestion, {{Assumption}} until confirmed._

```json
{
  "error": {
    "code": "not_found",
    "message": "No such thing",
    "request_id": "req_123",
    "details": []
  }
}
```

| Code | HTTP status | Means | Safe to retry |
|---|---|---|---|
| | | | Yes / No |

## Versions

_To fill: how a version is named and asked for (path, header), what counts as breaking, how long an old version stays after a new one, and how callers are told._

## Rate limits

_To fill: each limit with who it counts (key, user, address), the window and the number, the response when it is hit, and how a client learns what is left._

| Applies to | Counted by | Limit | Window | When exceeded |
|---|---|---|---|---|
| | | | | 429 with Retry-After |

## Pagination, filters and dates

_To fill: how lists page, filter and sort, the largest page, how dates and money are written, and the naming of fields._

## Idempotency and retries

_To fill: which writes take an idempotency key, how long a key is kept, and what a client should retry and how long it should wait. Or "Skipped: read-only API"._

## Open questions

## Change history
