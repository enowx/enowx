::progress

## Expected load

_To fill: users now and in a year, the busiest hour, the size of the data, and what makes a peak (a sale, a morning). Mark an estimate with {{Assumption}} and say what it is based on._

## Targets

_To fill: a number for each thing users would notice, measured at a stated percentile and load. Start from [[PRD#Non-functional requirements]]. An SLO built on these goes in [[Monitoring, Logs, SLOs and Alerts#SLOs]]._

| What | Measure | Target | At load | Measured today |
|---|---|---|---|---|
| Page load | Largest Contentful Paint, p75 | | | {{Unverified}} |
| API call | Latency, p95 | | | {{Unverified}} |
| Error rate | Share of requests | | | {{Unverified}} |
| Background job | Time to finish, p95 | | | {{Unverified}} |

## Load tests

_To fill: the tool, the scenarios (real flows, in real proportions), the environment and how close it is to production, and when they run. Never against production without saying so first._

| Date | Version | Scenario | Load | Result | Limit found |
|---|---|---|---|---|---|
| | | | | | |

## Limits and what breaks first

_To fill: the known ceiling of each part (connections, disk, third-party quotas), what fails first, and what users see when it does._

## Scaling

_To fill: what is done when load grows, in order: indexes, caching, bigger server, more servers, with roughly what each costs. Link [[Cost Estimate#When it grows]]._

## Open questions

## Change history
