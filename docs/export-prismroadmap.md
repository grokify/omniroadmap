# prism-roadmap Export

The `export/prismroadmap` package converts canonical omniroadmap items
into [prism-roadmap](https://github.com/grokify/prism-roadmap) types, so
PM data ingested from Aha/ProductBoard/JPD can feed prism-roadmap's
prioritization tooling, `splan` CLI, MCP server, and visualization
pipeline.

```go
import "github.com/grokify/omniroadmap/export/prismroadmap"

set, err := prismroadmap.ToRoadmapItemSet(items)
// set is an *rmi.RoadmapItemSet; every converted item passes
// prism-roadmap's own Validate().
```

## Field mapping

| Canonical (`provider.Item`) | prism-roadmap (`rmi.RoadmapItem`) |
|---|---|
| `ID` | `ID` (keeps the `provider:sourceID` provenance prefix) |
| `Name` / `Description` | `Name` / `Description` |
| `MoSCoW` | `MoSCoW` — optional as of prism-roadmap v0.17.0; unset stays unset |
| `RICE` (raw floats) | `RICE *prioritization.RICEScore` — see below |
| `Status.Category` | `Status`: todo→planned, in_progress→in_progress, done→completed, canceled→cancelled; unset→planned |
| `StartDate` / `DueDate` | ISO date strings |
| `Progress` | `Progress *int` |
| `Owner` | `Owner` (name, falling back to email) |
| `Tags` | `Tags` |
| `SourceRef` + `SourceURL` | `Notes` (`Source: SE-2 (https://...)`) — provenance survives |

### RICE translation

Canonical RICE components are raw numbers; prism-roadmap's `RICEScore`
uses enum levels for impact/confidence. The converter maps to the
**nearest level by multiplier**:

| Component | Mapping |
|---|---|
| Reach | `int` passthrough |
| Impact | nearest of massive=3.0, high=2.0, medium=1.0, low=0.5, minimal=0.25 |
| Confidence | nearest of high=1.0, medium=0.8, low=0.5 (percentages tolerated: 80 → 0.8) |
| Effort | passthrough |
| Score | source value if provided, else `Calculate()` when all components are present |

## Error handling

`ToRoadmapItemSet` skips items that can't convert (missing ID/Name) and
reports them in a joined error alongside the successfully converted set —
one bad record doesn't abort an export.

## Unprioritized items

Items without MoSCoW/RICE (nothing mapped by the
[fieldmap](fieldmap.md), or the tenant doesn't store prioritization)
export as valid, unprioritized `RoadmapItem`s. They can then be triaged
inside prism-roadmap's own tooling (`splan rmi update --moscow ...`, the
MCP `update_rmi` tool) — the import pipeline never fabricates priorities.
