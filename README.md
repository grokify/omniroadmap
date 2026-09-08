# OmniRoadmap

[![Go CI][go-ci-svg]][go-ci-url]
[![Go Lint][go-lint-svg]][go-lint-url]
[![Go SAST][go-sast-svg]][go-sast-url]
[![Docs][docs-godoc-svg]][docs-godoc-url]
[![DevGuide][docs-mkdoc-svg]][docs-mkdoc-url]
[![Visualization][viz-svg]][viz-url]
[![License][license-svg]][license-url]

 [go-ci-svg]: https://github.com/grokify/omniroadmap/actions/workflows/go-ci.yaml/badge.svg?branch=main
 [go-ci-url]: https://github.com/grokify/omniroadmap/actions/workflows/go-ci.yaml
 [go-lint-svg]: https://github.com/grokify/omniroadmap/actions/workflows/go-lint.yaml/badge.svg?branch=main
 [go-lint-url]: https://github.com/grokify/omniroadmap/actions/workflows/go-lint.yaml
 [go-sast-svg]: https://github.com/grokify/omniroadmap/actions/workflows/go-sast-codeql.yaml/badge.svg?branch=main
 [go-sast-url]: https://github.com/grokify/omniroadmap/actions/workflows/go-sast-codeql.yaml
 [docs-godoc-svg]: https://pkg.go.dev/badge/github.com/grokify/omniroadmap
 [docs-godoc-url]: https://pkg.go.dev/github.com/grokify/omniroadmap
 [docs-mkdoc-svg]: https://img.shields.io/badge/Go-dev%20guide-blue.svg
 [docs-mkdoc-url]: https://grokify.github.io/omniroadmap
 [viz-svg]: https://img.shields.io/badge/visualizaton-Go-blue.svg
 [viz-url]: https://mango-dune-07a8b7110.1.azurestaticapps.net/?repo=grokify%2Fomniroadmap
 [loc-svg]: https://tokei.rs/b1/github/grokify/omniroadmap
 [repo-url]: https://github.com/grokify/omniroadmap
 [license-svg]: https://img.shields.io/badge/license-MIT-blue.svg
 [license-url]: https://github.com/grokify/omniroadmap/blob/main/LICENSE

Batteries-included entry point for the omniroadmap ecosystem: a common,
tool-agnostic representation of roadmap/product-management data (features,
epics, initiatives, releases, objectives) with pluggable providers, a
Dolt-backed canonical store, per-tenant prioritization mapping, and export
into [prism-roadmap](https://github.com/grokify/prism-roadmap).

The core contract (the `Provider` interface, canonical types, registry,
and conformance tests) lives in
[`omniroadmap-core`](https://github.com/grokify/omniroadmap-core); provider
adapters live inside each tool's SDK repo following the
elevenlabs-go/opik-go embedded-adapter pattern.

## Providers

| Name | Source | Config | Notes |
|---|---|---|---|
| `aha` | Aha! REST/GraphQL API | `*aha.Client` ([aha-go](https://github.com/grokify/aha-go)) | Live API; richest status fidelity |
| `aha-studio` | aha-studio's local SQLite cache | `*sync.DB` ([aha-studio](https://github.com/grokify/aha-studio)) | No Aha API traffic; custom fields for detail-synced records |
| `productboard` | ProductBoard REST API v2 | `*productboard.Client` ([productboard-go](https://github.com/grokify/productboard-go)) | Features/subfeatures, releases, objectives |
| `jpd` | Jira Product Discovery | `*jira.Client` ([go-atlassian](https://github.com/grokify/go-atlassian)) | Ideas-as-issues only (Views/Insights have no public API) |

## Packages

| Package | Purpose |
|---|---|
| `omniroadmap` (root) | Type aliases re-exporting the core API + blank-import registration of all bundled providers — `NewProvider("aha", client)` works by name |
| `fieldmap` | Per-tenant custom-field → prioritization mapping: some Aha workspaces store MoSCoW, Kano, and RICE as custom fields; a JSON `Mapping` config normalizes them onto `Item.MoSCoW` / `Item.Kano` / `Item.RICE` |
| `augment` | Locally-authored data layered on top of synced items — MoSCoW, Kano, RICE, OKR refs, notes — keyed by source reference (e.g. `MYPROJ-123`); never touched by sync, so it survives every re-sync |
| `store` | Dolt-backed canonical store (Ent over the MySQL wire protocol, launching a local `dolt sql-server` when needed, with Dolt commits wrapping sync runs) |
| `sync` | Provider-agnostic sync engine: paginate any provider → fieldmap enrichment → upsert into the store → sync metadata → Dolt commit |
| `export/prismroadmap` | Converts canonical Items into prism-roadmap types (`rmi.RoadmapItemSet`, validated by prism-roadmap itself), feeding its prioritization tooling and visualization pipeline |
| `compassbridge` | Turns a [compass-rice](https://github.com/ProductBuildersHQ/compass-rice) judge output (or human-entered evidence) into a prism-roadmap `CompassAssessment`, with a claims-backed confidence integrity check; implements the two-phase profile assignment lifecycle |
| `compile` | Assembles a portfolio-wide `ReportDataset` from the assessment corpus — compass-first RICE resolution, MoSCoW+score ranking, the two-phase gate (see [COMPASS-RICE Prioritization](https://grokify.github.io/omniroadmap/compass-rice/)) |
| `review` | The PM review gate: structured, auditable edits (rank overrides, new assessment cycles, profile assignment changes) that flow back into the assessment IR |
| `materialize` | Writes a reviewed `ReportDataset`'s ranking back onto the assessment corpus and marks it final |
| `analyticscatalog` / `analyticsquery` / `analyticsdashboards` | omniroadmap as a [DashForge](https://github.com/plexusone/dashforge) analytics source: catalog datasets, GuardSQL query execution, and a curated dashboard pack (see [Analytics & Dashboards](https://grokify.github.io/omniroadmap/analytics/)) |
| `cmd/omniroadmap` | CLI: `sync`, `db init`, `status`, `augment set/get/list/rm`, `item get`, `assess list/show/import/set`, `profile list/propose/confirm/reject`, `moscow get/set`, `analytics export-dashboards`, `ui` |

## Quick start (CLI)

```bash
go install github.com/grokify/omniroadmap/cmd/omniroadmap@latest

# Sync from aha-studio's local cache (no Aha API traffic; a local
# dolt sql-server is started automatically if none is running):
omniroadmap sync --provider aha-studio

# Or from the live Aha API:
export AHA_SUBDOMAIN=mycompany AHA_API_KEY=...
omniroadmap sync --provider aha --fieldmap tenant-acme.fieldmap.json

# Report what's synced:
omniroadmap status

# Layer local judgments on top — these survive every re-sync:
omniroadmap augment set --provider aha-studio --ref MYPROJ-123 \
  --moscow must_have --kano performance --effort 2 --okr OKR-2026-Q3-01

# Read the merged view (synced Aha data + your augments):
omniroadmap item get MYPROJ-123
```

Requires the [`dolt`](https://docs.dolthub.com/introduction/installation)
binary for the canonical store. Provider credentials come from environment
variables — see `omniroadmap sync --help`.

## Quick start (library)

```go
import (
    "github.com/grokify/omniroadmap"
    "github.com/grokify/omniroadmap/fieldmap"
    "github.com/grokify/omniroadmap/export/prismroadmap"
)

p, err := omniroadmap.NewProvider("aha", ahaClient)
resp, err := p.ListItems(ctx, &omniroadmap.ListItemsRequest{})

// Per-tenant prioritization mapping (Aha custom fields -> MoSCoW/RICE)
mapping, _ := fieldmap.Load("tenant-acme.fieldmap.json")
fieldmap.Apply(resp.Items, mapping)

// Export into prism-roadmap's RoadmapItemSet
set, err := prismroadmap.ToRoadmapItemSet(resp.Items)
```

A tenant fieldmap config:

```json
{
  "description": "Acme Aha workspace",
  "moscow": {"key": "priority_moscow", "values": {"P1": "must_have", "P2": "should_have"}},
  "kano": {"key": "kano_category", "values": {"Table Stakes": "must-be"}},
  "reach": {"key": "rice_reach"},
  "impact": {"key": "rice_impact"},
  "confidence": {"key": "rice_confidence", "values": {"High": "1.0", "Medium": "0.8", "Low": "0.5"}},
  "effort": {"key": "rice_effort"}
}
```

MoSCoW and Kano normalization handle common display vocabularies out of
the box ("Must Have", "should", "could-have", "Won't Have"; "Basic",
"Delighter", "One Dimensional"); the `values` table covers anything
tenant-specific. Items without mapped prioritization stay honestly
unprioritized — prism-roadmap's `rmi.RoadmapItem` treats MoSCoW as
optional (v0.17.0+), so imported-but-untriaged items validate cleanly.

## Sync semantics: provider data vs. augments

Items in the store are keyed by canonical ID and carry the source system's
reference (`Item.SourceRef`, e.g. an Aha reference number like
`MYPROJ-123`). A re-sync **overwrites provider data wholesale** — names,
statuses, dates, custom fields, and any prioritization *derived from*
provider data via fieldmap. That's the point: the items table always
mirrors the source.

Locally-authored judgments — MoSCoW, Kano, RICE overrides, OKR links,
notes — live in a separate **augments** table keyed by
`(provider, source ref)`. Sync never touches it, so augments survive every
re-sync. Reads (`store.ListItems`/`GetItem`, `omniroadmap item get`)
overlay augments onto items, with augment values winning over synced ones;
pass `WithoutAugments` (or inspect `augment get`) to see either layer on
its own.

## COMPASS-RICE prioritization

Ranking is **compass-only**: an opportunity's score counts only once a
human has confirmed its primary COMPASS-RICE investment thesis and an
LLM judge (or a human directly) has entered evidence-backed scoring for
it. Six domain-specific profiles normalize onto the same canonical
Reach/Impact/Confidence/Effort shape, so scores stay comparable across a
portfolio that mixes customer features, platform investments, and risk
mitigations — something one Reach fraction can't do honestly.

```bash
omniroadmap profile propose --spec-id OPP-42 --profile customer/b2b/v1 \
  --rationale "primarily a retention play" --by claude-session-9
omniroadmap profile confirm --spec-id OPP-42 --by pm@example.com
omniroadmap assess import judge-output.json
omniroadmap assess list --status computable
```

Dashboards are entirely [DashForge](https://github.com/plexusone/dashforge)
DashboardIR — `omniroadmap ui` serves a curated pack live
(`/api/analytics/dashboards`), or export it for a standalone
dashforge-server with `omniroadmap analytics export-dashboards ./dashboards`.

See [COMPASS-RICE Prioritization](https://grokify.github.io/omniroadmap/compass-rice/)
and [Analytics & Dashboards](https://grokify.github.io/omniroadmap/analytics/)
for the full pipeline.

## Store configuration (avoiding port collisions)

The Dolt store defaults to port 13307 and `~/.omniroadmap` — distinct
from [visionstudio](https://github.com/ProductBuildersHQ/visionstudio)'s
13306, so both can run side by side today. If you add another Dolt-backed
tool, or just want a different port, set it once in
`~/.omniroadmap/config.json` rather than passing `--port`/`--dsn` on
every command:

```bash
omniroadmap config set-port 13309   # writes ~/.omniroadmap/config.json
omniroadmap config                  # shows the resolved DSN/port/data-dir
                                     # and where each came from
```

Resolution order: `--dsn`/`--port`/`--data-dir` flags >
`OMNIROADMAP_DSN`/`OMNIROADMAP_PORT`/`OMNIROADMAP_DATA_DIR` env vars >
`~/.omniroadmap/config.json` > built-in defaults. See
[CLI Reference](https://grokify.github.io/omniroadmap/cli/#omniroadmap-config).

## Architecture

```
 Aha! API ──── aha-go/omniroadmap ────────┐
 Aha cache ─── aha-studio/omniroadmap ────┤    ┌──────────┐    ┌──────────────┐
 ProductBoard ─ productboard-go/omniroadmap ──►│ sync +   │───►│ Dolt store   │
 JPD ────────── go-atlassian/omniroadmap ─┘    │ fieldmap │    │ (Ent, MySQL  │
                                               └──────────┘    │  dialect)    │
        provider.Provider (omniroadmap-core)                   └──────┬───────┘
                                                                      │
                                                      export/prismroadmap
                                                                      │
                                                                      ▼
                                                    prism-roadmap RoadmapItemSet
                                                    (RMI tooling, MCP, viewers)
```

## Roadmap

- Embedded web UI for exploring/visualizing the generalized entities
  (visionstudio-style React/Vite SPA embedded in the Go binary)
- Embedded-Dolt mode (`dolt_embedded` build tag) — no external `dolt` binary
- Kano-aware export once prism-roadmap's RMI grows a Kano field

## Development

```bash
go build ./...
go vet ./...
golangci-lint run ./...
go test ./...
```

The `store` package's Dolt integration tests are opt-in: they skip by
default — even with the `dolt` binary on PATH — so plain `go test ./...`
stays a fast, hermetic check of the Go library code. Run them explicitly
with:

```bash
OMNIROADMAP_TEST_DOLT=1 go test ./store/...
```

Documentation lives in `docs/` (MkDocs):

```bash
pip install mkdocs-material mkdocs-minify-plugin
mkdocs serve
```
