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
| Time entry logged on an issue | `issue_worklogs` (entries logged on the project itself are skipped) |
| Changeset | `issue_commits` (`commit_sha` is the revision; needs the Cross-domain entity) |
| Version | `issues.fix_versions`; `sprints`, `board_sprints`, `sprint_issues` with *Versions as sprints* |
| User seen as author, assignee, journal user, time-entry user or committer | `accounts` (`full_name` only: no login or e-mail stored) |

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
  tracker, a custom-field condition (`in` / `notIn` a list of raw values), or
  both, and a type (`REQUIREMENT`, `BUG`, `INCIDENT`, `TASK`, `SUBTASK`). A
  rule without a tracker applies to every tracker, and `notIn` also holds when
  the field is empty. An issue that no rule matches takes the built-in
  fallback. Put a conditional rule above the plain rule of the same tracker.
- **Status**: each status maps to `TODO`, `IN_PROGRESS`, `DONE` or `OTHER`.
  `resolution_date` is set only while the issue's status maps to `DONE`. It is
  the start of the issue's current uninterrupted run of `DONE`-mapped
  statuses, read from the journals' status changes. When they cannot place
  it, it is `closed_on`; for an issue whose status never changed and that has
  no `closed_on`, it is `created_on`. `lead_time_minutes` is the whole minutes
  from `created_on` to `resolution_date`.
- **Severity** and **environment class**: custom fields tried in order, with a
  value map; the first field with a value decides. Severity is that field's
  first value, translated by the map, else by its label, else left raw, and
  lands in `issues.severity` and so in DORA `incidents.severity`. Each mapped
  environment value adds one class and an unmapped value adds none; the
  classes are `PRODUCTION`, `STAGING`, `TESTING` and `DEVELOPMENT`.
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
mapping with one `400` (`invalid scope config: …`) that lists every rule
violation; an unknown key or a value of the wrong JSON type is rejected first,
on its own.
`GET /plugins/redmine_go/connections/:connectionId/scope-config-defaults`
returns the built-in mapping resolved against your Redmine.

## DORA

Every issue whose mapped type is `INCIDENT` becomes a DORA incident, with its
severity, creation date and resolution date: they feed change failure rate
and time to restore. With the built-in fallbacks none is, unless a tracker is
named `Incident` (its upper-cased name is `INCIDENT`). Add type rules as
above. DORA reads the issues of the boards in a DevLake project, so put the
Redmine connection's projects into one.

Incidents that no change caused, such as an external outage, are kept out of
change failure rate by an ordinary type rule, for example a field
`Caused by change = No` mapped to `BUG` above the incident rule. The cost:
those issues also leave time to restore.

## Toggles and environment flags

| Toggle | Default | Effect | Flag |
| --- | --- | --- | --- |
| `reconcileSweep` | on | Re-reads each project's whole issue list with relations; deletes vanished relations and issues deleted in Redmine or moved to another project. A full run with no time range re-reads every relation anyway and skips it | `REDMINE_GO_RECONCILE_SWEEP` |
| `collectTimeEntries` | on | Time entries to `issue_worklogs` | `REDMINE_GO_COLLECT_TIME_ENTRIES` |
| `collectChangesets` | on | Changesets to `issue_commits` (issue details still request them) | `REDMINE_GO_COLLECT_CHANGESETS` |
| `collectComments` | on | Journal notes to `issue_comments` (journals are still stored for the changelogs) | `REDMINE_GO_COLLECT_COMMENTS` |
| `includePrivateNotes` | off | Include private notes | `REDMINE_GO_INCLUDE_PRIVATE_NOTES` |
| `versionsAsSprints` | off | Emit versions as sprints | `REDMINE_GO_VERSIONS_AS_SPRINTS` |

An unset toggle uses its default. A flag can only switch a behaviour **off**
for every scope config: `false` (or `0`, `f`) wins over the scope config; an
unset flag or any other value has no effect, and `true` never forces a
behaviour on.

Toggles and flags act through the blueprint's plan, which leaves a switched-off
subtask out. A hand-made `POST /pipelines` without a `subtasks` list runs every
subtask; only `includePrivateNotes` (read when notes are converted) and its
flag still apply there.

## Performance and sizing

Measured on one host: Redmine 7.0.2 with a single Puma process, Postgres 18, and
a 50 000-issue project with 229 000 journals, 362 000 field changes, 15 000
time entries and 2 900 relations; the connection's request budget was raised
to 108 000 an hour so Redmine, not the limiter, set the pace.

| Run | Wall time | Where it goes |
| --- | --- | --- |
| First run, no time range, 50 000 issues | 33 min | Collection 31 min: issue details 28 min (one request per issue), issue list 3 min, time entries 8 s. No sweep. Extraction and conversion 2.2 min. |
| First run with a time range, 50 000 issues | 36 min | The same, plus the sweep, 3 min. |
| Incremental, 100 issues edited | 4.4-4.6 min | The sweep, 3 min; the rest is time entries, the 100 details and converting the project again. |

A full refresh with no time range skips the sweep as the first run does; the
tests cover it, but it was not timed.

- **Requests.** A run sends, for a project of N issues and T time entries:
  6 metadata requests; W/100 issue-list pages and W detail requests, where W
  is every issue on a run with no time range, the issues updated since the time
  range on a full run with one, and the issues updated since the last start
  (minus an hour) on an incremental run; T/100 time-entry pages, always read in
  full; and the sweep, N/100 pages plus one request per issue it finds
  missing. A full run with no time range sends no sweep request. The first run
  above sent 500 + 50 000 + 152 requests for issues, details and time entries;
  its incremental run sent 1 + 100 + 500 (sweep) + 152.
- **Rate limit.** At the default 36 000 an hour (10 a second) the details of
  50 000 issues take 83 minutes. That Redmine answered about 30 detail
  requests a second before queueing: an interactive request stayed at about
  40 ms while DevLake sent 10 or 20 a second, and its 95th percentile rose to
  about 400-650 ms at 30 a second. The right limit depends on the cost of a
  request on your Redmine, so the default stays conservative; raise it for a
  large first collection when Redmine is quiet, and lower it if users notice.
- **Cutting the incremental cost.** Switching `reconcileSweep` off saves the
  N/100 sweep requests of each incremental run (3 min at 50 000 issues), at
  the price of missing removed relations and issues deleted or moved away,
  until a full refresh with no time range.
- **Memory.** DevLake's container peaked at 341-353 MiB (docker stats) over
  first runs of 50 000 issues, and at 443 MiB in an earlier run over 14 000:
  extractors and convertors save in batches of 500 rows and collectors page by
  100, so the peak does not follow the size of the project.
- **Disk.** Per 50 000 issues of this shape: raw tables 130 MB, plugin tool
  tables 170 MB, domain tables 190 MB. The sweep's raw table adds about 31 MB
  (indexes included) and each sweep replaces it. Raw tables are never
  compacted: each run adds about 2 KB per re-collected issue.
- **Progress.** Subtasks report progress against a counted total, so
  `_devlake_subtasks.finished_records` of a finished extractor or convertor
  shows the last percent written, not the final count; the task log has the
  record count.

## Limits

- Adding or removing a relation does not change an issue's `updated_on`, and
  Redmine sends no event, hence the reconciliation sweep on every incremental
  run and every run bounded by a time range. A full run with no time range
  re-reads every issue's relations and skips the sweep.
- A full run collects the issues updated on or after the blueprint's time
  range. DevLake gives a new project a range of six months back, so clear it to
  collect every issue. A later full sync without a range reuses the stored one.
- Redmine ignores the `updated_on` filter on time entries, so a project's time
  entries are pulled in full on every run.
- Incremental runs collect issues updated since the last successful start
  minus one hour, to absorb clock skew; rows are replaced, not duplicated.
- Private notes are excluded unless `includePrivateNotes` is on.
- A 403 or 404 on one issue's detail is skipped with a warning. An issue that
  the sweep no longer lists and that Redmine answers with 403 or 404 is removed
  from DevLake. A 403 or 404 on the issue list or the sweep fails the run; on a
  project's time entries, versions or categories it skips that list with a
  warning.
- The proxy route the config UI uses forwards only an allowlist of Redmine
  metadata paths (projects, trackers, issue statuses, enumerations, custom
  fields, versions, issue categories) and answers anything else with 403:
  `users/current.json` would return the connection's own API key.
- Redmine versions older than 7 are untested.

## Troubleshooting

| Symptom | Cause and fix |
| --- | --- |
| Test fails: `the Redmine server rejected the API key (401)` | Wrong or revoked key. Copy it again from My account → API access key. |
| Test fails: `the Redmine server answered 403: the REST web service is disabled …`, or `the endpoint did not answer with a Redmine user` | The REST web service is disabled (Administration → Settings → API), the account is locked, or the URL is not the Redmine root. |
| Test fails: `not found (404), check the endpoint …` | Wrong endpoint; a Redmine under a sub-URI needs it, for example `https://host/redmine`. |
| Test fails: `cannot reach Redmine at …` | DevLake cannot reach the host: check the URL, DNS, firewall and the proxy field. |
| The form says custom-field lists come from collected data | The key is not an administrator. Use an administrator key, or collect once and reopen the scope config. |
| A custom field is missing | It does not apply to the tracker or project, the key cannot see it, or (non-administrator key) no collected issue carries a value yet. A deleted field shows as stale; it then matches only a field of the same name, if one exists. |
| Issues of type `FEHLER` or `CHANGE REQUEST` | No built-in mapping for that tracker; add a type rule. |
| No incidents in DORA | No type rule produces `INCIDENT`, or the board is not in a DevLake project. |
| Subproject issues are missing | Select the subproject itself; a parent excludes them. |

See the [Redmine Go plugin page](https://devlake.apache.org/docs/Plugins/redmine_go)
on the DevLake website for the full guide.
