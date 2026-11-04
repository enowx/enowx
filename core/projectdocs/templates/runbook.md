::progress

## Service at a glance

_To fill: what runs where, how to reach dashboards and logs (links, no credentials), how to tell it is healthy, and which outside services it depends on. Link [[Deployment and Infrastructure Diagram#Infrastructure]]._

## Who is on call

_To fill: who answers and when, how they are reached, who is next when they don't answer, and who may talk to users._

## Severity and response

_To fill: what each level means here and how fast it gets answered._

| Level | Means | Answer within | Who is told |
|---|---|---|---|
| SEV-1 | Down, or data at risk | | |
| SEV-2 | A main feature broken | | |
| SEV-3 | Degraded, a workaround exists | | |

## During an incident

_To fill: the steps in order: acknowledge, name one person in charge, stop the bleeding before finding the cause (roll back first: [[Rollback Plan#Steps]]), update users on a schedule, keep a timeline. Where is the status page?_

## Playbooks

_To fill: one playbook for each alert in [[Monitoring, Logs, SLOs and Alerts#Alerts]] and each thing that has gone wrong, as a copy of the form below. Commands exact enough for someone woken at night. Credentials by name, as in [[Secret Handling#Where secrets live]]. Mark a playbook never tried with {{Unverified}}._

### Symptom: what is seen

- **Check:**
- **Likely cause:**
- **Fix:**
- **Confirm it worked:**
- **If not, go to:**

## After an incident

_To fill: within how many days a review is written and by whom. It records the timeline, the cause and what will change, with no blame, and each action becomes a task._

## Incident log

_To fill: each incident with its date, level, how long it lasted, its cause and a link to its review, or "Skipped: none yet"._

| Date | Level | Duration | Cause | Review |
|---|---|---|---|---|
| | | | | |

## Open questions

## Change history
