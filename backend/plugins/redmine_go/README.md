<!--
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

# Redmine Go

The Redmine Go plugin collects issues, journals, relations, time entries and
changesets from [Redmine](https://www.redmine.org/) through its core REST API
and maps them to DevLake's ticket domain, so Redmine feeds the issue metrics
and the incident side of DORA.

It needs only an API key: **no Redmine plugin is required**. It works out of
the box on a Redmine with default data, and a scope config maps your own
trackers, statuses and custom fields. It collects by polling on the blueprint
schedule; it does not use webhooks.

The plugin has its own `_raw_redmine_go_*` and `_tool_redmine_go_*` tables, so
it can coexist with the Python `redmine` plugin.

## Requirements

- **Redmine 7.x** (tested on 7.0.2) with the REST web service enabled
  (Administration → Settings → API). Older versions are untested.
- **An API key** (My account → API access key), sent in the
  `X-Redmine-API-Key` header and stored encrypted. The plugin only reads.
- **An administrator key is recommended, not required.** Redmine lists custom
  field definitions to administrators only. With any other key the mapping
  form offers the custom fields and values seen in collected issues instead
  (run one collection first), enumeration values stay raw ids, and collection
  completes with one warning.

## Connection and scope

Create a connection with:

- **Endpoint URL**: the Redmine base URL without a trailing slash, for example
  `https://redmine.example.org`, or `https://host/redmine` for a sub-URI.
- **API Key**: see above. **Test Connection** names the account and says
  whether it is an administrator.
- **Proxy** (optional) and **Custom Rate Limit** (requests per hour, default
  `36000`).

The scope is a Redmine **project**, chosen from a project tree that includes
subprojects; each becomes a DevLake board. Closed projects show with a
`[closed]` suffix; archived ones are not served by Redmine's API.
**A parent project's issues exclude its subprojects' issues**: select each
subproject you want, and nothing is counted twice.

## Domain mapping

| Redmine | DevLake domain |
| --- | --- |
| Project | `boards` |
| Issue | `issues`, `board_issues`, `issue_assignees` |
| Severity, component, environment class | `issues.severity`, `issues.component`, `issue_custom_array_fields` (`environment_class`) |
| Journal detail | `issue_changelogs` (one row per detail) |
| Journal note | `issue_comments` |
| Relation | `issue_relationships` |
| Time entry | `issue_worklogs` |
| Changeset | `issue_commits` |
| Version | `issues.fix_versions`; `sprints`, `board_sprints`, `sprint_issues` with *Versions as sprints* |
| User named on an issue | `accounts` (no e-mail stored) |

`issues.original_type` and `original_status` keep the Redmine tracker and
status names, next to the mapped standard values.

## Mapping

The scope config holds ordered **type rules**, **status mappings**,
**severity**, **environment** and **component** mappings, and the toggles.
References carry the Redmine id and name; the id wins, the name is the
fallback if the id no longer exists.

On a default Redmine, with no scope config at all, the built-in fallbacks
apply. They use only language-independent facts:

| Item | Default |
| --- | --- |
| Type | English default tracker names, case-insensitive: `Bug` → `BUG`, `Feature` → `REQUIREMENT`, `Support` → `TASK`. Any other tracker: its name upper-cased. |
| Status | `DONE` if Redmine says closed; `TODO` if it is a tracker's default status; else `IN_PROGRESS`. |
| Component | The issue category. |

`Support` is a `TASK`, not an `INCIDENT`: a support request is no evidence of
a failed change. Redmine's default data is localized, so on a non-English
install the tracker names do not match and fall back to their upper-cased
name; add a type rule per tracker. The UI opens a new scope config pre-filled
with this effective mapping.

- **Type rules**: the first rule that matches sets `issues.type`. A rule has a
  tracker, an optional custom-field condition (`in` / `notIn` a list of raw
  values) and a type (`REQUIREMENT`, `BUG`, `INCIDENT`, `TASK`, `SUBTASK`).
  Put a conditional rule above the plain rule of the same tracker.
- **Status**: each status maps to `TODO`, `IN_PROGRESS`, `DONE` or `OTHER`.
  `resolution_date` is the start of the issue's current uninterrupted run of
  `DONE`-mapped statuses, falling back to `closed_on`.
- **Severity** and **environment class**: custom fields tried in order, with a
  value map. Severity lands in `issues.severity`, and so in DORA
  `incidents.severity`. Environment classes are `PRODUCTION`, `STAGING`,
  `TESTING` and `DEVELOPMENT`.
- **Component**: issue category, a custom field (labels joined by commas), or
  none.

Example, production bugs and critical support tickets as incidents:

```json
{
  "name": "Incidents",
  "typeRules": [
    {"tracker": {"id": 1, "name": "Bug"},
     "when": {"field": {"id": 21, "name": "Discovery Environment"}, "op": "in",
              "values": [{"value": "PROD"}]},
     "type": "INCIDENT"},
    {"tracker": {"id": 1, "name": "Bug"}, "type": "BUG"},
    {"tracker": {"id": 12, "name": "Support Ticket"},
     "when": {"field": {"id": 43, "name": "Ticket Severity"}, "op": "in",
              "values": [{"value": "1", "label": "Critical"}]},
     "type": "INCIDENT"},
    {"tracker": {"id": 12, "name": "Support Ticket"}, "type": "TASK"}
  ],
  "statusMappings": [
    {"status": {"id": 15, "name": "Awaiting Client Sign-off"}, "std": "OTHER"}
  ]
}
```

A condition matches the value exactly as Redmine stores it: the option text of
a list field, the item id of an enumeration. The API rejects an invalid
mapping with one `400` that lists every problem.
`GET /plugins/redmine_go/connections/:connectionId/scope-config-defaults`
returns the built-in mapping resolved against your Redmine.

## DORA

Every issue whose mapped type is `INCIDENT` becomes a DORA incident, with its
severity, creation date and resolution date: they feed change failure rate
and time to restore. By default none is, so add type rules as above.

Incidents that no change caused, such as an external outage, are kept out of
change failure rate by an ordinary type rule, for example a field
`Caused by change = No` mapped to `BUG` above the incident rule. The cost:
those issues also leave time to restore.

## Toggles and environment flags

| Toggle | Default | Effect | Flag |
| --- | --- | --- | --- |
| `reconcileSweep` | on | Re-reads each project's full issue list with relations; deletes vanished relations and deleted issues | `REDMINE_GO_RECONCILE_SWEEP` |
| `collectTimeEntries` | on | Time entries to `issue_worklogs` | `REDMINE_GO_COLLECT_TIME_ENTRIES` |
| `collectChangesets` | on | Changesets to `issue_commits` | `REDMINE_GO_COLLECT_CHANGESETS` |
| `collectComments` | on | Journal notes to `issue_comments` | `REDMINE_GO_COLLECT_COMMENTS` |
| `includePrivateNotes` | off | Include private notes | `REDMINE_GO_INCLUDE_PRIVATE_NOTES` |
| `versionsAsSprints` | off | Emit versions as sprints | `REDMINE_GO_VERSIONS_AS_SPRINTS` |

An unset toggle uses its default. A flag can only switch a behaviour **off**
for every scope config: `false` (or `0`, `f`) wins over the scope config; an
unset flag or any other value has no effect, and `true` never forces a
behaviour on.

## Performance and sizing

Measured on one host: Redmine 7.0.2 with a single Puma process, Postgres 18, and
a 50 000-issue project with 229 000 journals, 362 000 field changes, 15 000
time entries and 2 900 relations; the connection's request budget was raised
to 108 000 an hour so Redmine, not the limiter, set the pace.

| Run | Wall time | Where it goes |
| --- | --- | --- |
| First run, 50 000 issues | 36 min | Collection 34 min: issue details 28 min (one request per issue), issue list 3 min, sweep 3 min, time entries 8 s. Extraction and conversion 2.3 min. |
| Incremental, 100 issues edited | 4.6 min | The sweep, 3 min; the rest is time entries, the 100 details and converting the project again. |

- **Requests.** A first run sends about N/100 (issue list) + N (details) +
  N/100 (sweep) + T/100 (time entries) requests, for N issues and T time
  entries. A later run sends N/100 + T/100 plus one per edited issue, because
  the sweep and the time entries are always read in full.
- **Rate limit.** At the default 36 000 an hour (10 a second) the details of
  50 000 issues take 83 minutes. That Redmine answered about 30 detail
  requests a second before queueing: an interactive request stayed at about
  40 ms while DevLake sent 10 or 20 a second, and its 95th percentile rose to
  about 400-650 ms at 30 a second. The right limit depends on the cost of a
  request on your Redmine, so the default stays conservative; raise it for a
  large first collection when Redmine is quiet, and lower it if users notice.
- **Cutting the incremental cost.** Switching `reconcileSweep` off saves the N/100
  sweep requests per run, at the price of missing removed relations and deleted
  issues.
- **Memory.** DevLake's container peaked at 315-347 MiB (docker stats) over a
  first run of 50 000 issues, and at 443 MiB in an earlier run over 14 000:
  collectors, extractors and convertors work in batches of 500 rows, so the
  peak does not follow the size of the project.
- **Disk.** Per 50 000 issues of this shape: raw tables 130 MB, plugin tool
  tables 170 MB, domain tables 190 MB. Raw tables are never compacted: each
  run adds about 2 KB per re-collected issue.
- **Progress.** Subtasks report progress against a counted total, so
  `_devlake_subtasks.finished_records` of a finished extractor or convertor
  shows the last percent written, not the final count; the task log has the
  record count.

## Limits

- Adding or removing a relation does not change an issue's `updated_on`, and
  Redmine sends no event, hence the reconciliation sweep on every run.
- Redmine ignores the `updated_on` filter on time entries, so a project's time
  entries are pulled in full on every run.
- Incremental runs collect issues updated since the last successful start
  minus one hour, to absorb clock skew; rows are replaced, not duplicated.
- Private notes are excluded unless `includePrivateNotes` is on.
- A 403 or 404 on a single issue is skipped with a warning.
- The proxy route the config UI uses forwards only an allowlist of Redmine
  metadata paths (projects, trackers, issue statuses, enumerations, custom
  fields, versions, issue categories) and answers anything else with 403:
  `users/current.json` would return the connection's own API key.
- Redmine versions older than 7 are untested.

## Troubleshooting

| Symptom | Cause and fix |
| --- | --- |
| Test fails with 401 | Wrong or revoked key. Copy it again from My account → API access key. |
| Test fails with 403, or the endpoint "did not answer with a Redmine user" | The REST web service is disabled (Administration → Settings → API), the account is locked, or the URL is not the Redmine root. |
| Test fails with 404 | Wrong endpoint; a Redmine under a sub-URI needs it, for example `https://host/redmine`. |
| The form says custom-field lists come from collected data | The key is not an administrator. Use an administrator key, or collect once and reopen the scope config. |
| A custom field is missing | It does not apply to the tracker or project, the key cannot see it, or (non-administrator key) no collected issue carries a value yet. A deleted field shows as stale and never matches. |
| Issues of type `FEHLER` or `CHANGE REQUEST` | No built-in mapping for that tracker; add a type rule. |
| No incidents in DORA | No type rule produces `INCIDENT`. |
| Subproject issues are missing | Select the subproject itself; a parent excludes them. |

See the [Redmine Go plugin page](https://devlake.apache.org/docs/Plugins/redmine_go)
on the DevLake website for the full guide.
