# Analytics & Dashboards

omniroadmap's dashboard surface is entirely [DashForge](https://github.com/plexusone/dashforge)
DashboardIR — the Splunk model: a generic analytics engine plus an
application-specific "app" of curated dashboards, rather than a
hand-rolled UI. omniroadmap's job is to be a first-class analytics
source (a catalog of queryable datasets, plus read-only GuardSQL query
execution over them) and to ship that curated dashboard pack.

## The catalog

`analyticscatalog.BuildFromStore` describes every dataset a dashboard can
query:

| Dataset | Source | Notes |
|---|---|---|
| `items` / `initiatives` / `features` / `epics` | Synced provider items | Standard + per-provider custom fields, discovered dynamically |
| `opportunity_assessments` | Current-cycle assessments | Compass-first RICE resolution (matches `ToRankInput`'s precedence), PM-review state, calculated/final rank |
| `profile_assignments` | Two-phase profile lifecycle | `spec_id`, `profile_id`, `status`, `proposed_by`, `confirmed_by`, `rationale` |
| `compass_<profile>` | One per COMPASS-RICE profile present, e.g. `compass_customer_b2b_v1` | Normalized columns (`reach`, `impact`, `confidence`, `effort_pd`, `method`, `score`) beside the profile's raw evidence fields, flattened from `EvidenceJSON` — the human-validation view: compare a normalized score directly against what produced it |

Every dataset field carries `Count`/`Coverage` stats computed from real
rows, not just a static schema declaration — a dashboard builder can see
at a glance how populated a field actually is.

Fetch it directly:

```bash
curl http://127.0.0.1:13317/api/analytics/catalog | jq .
```

## Query execution

`analyticsquery.Execute` runs a read-only [GuardSQL](https://github.com/grokify/guardsql)
query against whichever dataset the query names — the same dispatch the
catalog describes, field-for-field. `compass_<profile>` datasets support
full GuardSQL, including `GROUP BY`/aggregates (`COUNT`, `SUM`, `AVG`,
`MIN`, `MAX`):

```sql
SELECT spec_id, title, compass_profile_id, score
FROM opportunity_assessments
WHERE computable = true
ORDER BY score DESC
LIMIT 200
```

```sql
SELECT provider, kind, status, COUNT(*) AS count
FROM items
GROUP BY provider, kind, status
```

Note: `omniroadmap ui`'s own `/api/query` endpoint (the local table
browser) is a separate, hand-rolled query engine over items only — it
predates `analyticsquery` and doesn't share its implementation. The
`analyticsquery` package is what a DashForge server calls (via
`dashforgeconnector.Provider.Query`) to execute a `SavedQuestion` against
this catalog.

## The curated dashboard pack

`analyticsdashboards.Build` assembles four dashboards (`compass-rice-all`,
`moscow-compass-rice`, one `compass-profile-<slug>` per profile present,
and `portfolio-overview`) — see [COMPASS-RICE Prioritization](compass-rice.md#visualizing-scores-dashforge-dashboards)
for what each shows. Every widget's `SavedQuestion` is plain GuardSQL
against a catalog dataset; no dashboard here ever references a field the
catalog doesn't declare.

Two ways to get the pack:

- **Live**, served by `omniroadmap ui`:

  ```bash
  curl http://127.0.0.1:13317/api/analytics/dashboards | jq .
  ```

- **Exported**, for import into a standalone dashforge-server:

  ```bash
  omniroadmap analytics export-dashboards ./dashboards
  # writes ./dashboards/dashboards.json — {"dashboards": [...], "questions": [...]}
  ```

## Registering omniroadmap as a DashForge connector

`cmd/omniroadmap-server` composes DashForge's own server CLI
(`dashforge/cmd/dashforge-server`) with the omniroadmap connector
(`dashforgeconnector`), per DashForge's engine/connector split (the engine
core ships no connectors; application binaries compose engine + connector):

```bash
export OMNIROADMAP_DSN='root:@tcp(127.0.0.1:13307)/omniroadmap'
go run ./cmd/omniroadmap-server serve --address 127.0.0.1:13319

# Register the source once (persists in .dashforge/ or the metadata DB):
curl -X POST http://127.0.0.1:13319/api/v1/analytics/sources \
  -H 'Content-Type: application/json' \
  -d '{"id":"omniroadmap","name":"OmniRoadmap","connector":"omniroadmap",
       "dsnRef":"env://OMNIROADMAP_DSN","enabled":true}'
```

From there, import an exported `dashboards.json` (or author new dashboards
in the DashForge builder against the `omniroadmap` catalog directly) to
get the full curated pack running against a real DashForge deployment
instead of the local `omniroadmap ui`.
