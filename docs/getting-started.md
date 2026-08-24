# Getting Started

## Install

```bash
# CLI
go install github.com/grokify/omniroadmap/cmd/omniroadmap@latest

# Library
go get github.com/grokify/omniroadmap
```

The canonical store uses [Dolt](https://docs.dolthub.com/introduction/installation)
over the MySQL wire protocol — install the `dolt` binary. The CLI starts a
local `dolt sql-server` automatically when none is reachable (data lives in
`~/.omniroadmap` by default, port 13307).

Running another Dolt-backed tool locally (e.g.
[visionstudio](https://github.com/ProductBuildersHQ/visionstudio) on port
13306)? Confirm there's no collision, or set a different port, with:

```bash
omniroadmap config          # shows the resolved DSN/port/data-dir
omniroadmap config set-port 13309
```

See [CLI Reference](cli.md#omniroadmap-config) for the full resolution
order (flag > env var > config file > default).

## First sync

The fastest zero-credential path is the `aha-studio` provider, which reads
[aha-studio](https://github.com/grokify/aha-studio)'s local SQLite cache —
no Aha API traffic at all:

```bash
omniroadmap sync --provider aha-studio
omniroadmap status
```

```
aha-studio     epic            152 records  last sync 2026-08-13 07:15:10
aha-studio     feature        6980 records  last sync 2026-08-13 07:15:08
aha-studio     initiative      626 records  last sync 2026-08-13 07:15:15
```

Now layer a local judgment on top — it survives every future re-sync,
even though a re-sync will overwrite everything else about this item:

```bash
omniroadmap augment set --provider aha-studio --ref MYPROJ-123 --moscow must_have
omniroadmap item get MYPROJ-123   # merged view: synced data + your augment
```

See [Augments](augment.md) for the full picture of what's overwritten by
sync vs. what's yours to keep.

For live-API providers, set credentials via environment variables first:

```bash
# Aha!
export AHA_SUBDOMAIN=mycompany AHA_API_KEY=...
omniroadmap sync --provider aha

# ProductBoard
export PRODUCTBOARD_API_TOKEN=...
omniroadmap sync --provider productboard

# Jira Product Discovery
export JIRA_URL=https://mycompany.atlassian.net JIRA_USER=... JIRA_TOKEN=...
omniroadmap sync --provider jpd
```

## Library usage

```go
import (
    "github.com/grokify/omniroadmap"
    "github.com/grokify/omniroadmap/fieldmap"
    "github.com/grokify/omniroadmap/export/prismroadmap"
)

// Construct a provider by registry name (blank imports in the omniroadmap
// package register all bundled adapters).
p, err := omniroadmap.NewProvider("aha", ahaClient)
if err != nil {
    return err
}
defer p.Close()

resp, err := p.ListItems(ctx, &omniroadmap.ListItemsRequest{
    Kinds: []omniroadmap.ItemKind{omniroadmap.ItemKindFeature},
})

// Optional: enrich with tenant-specific MoSCoW/RICE custom-field mapping.
mapping, _ := fieldmap.Load("tenant.fieldmap.json")
fieldmap.Apply(resp.Items, mapping)

// Optional: export to prism-roadmap.
set, err := prismroadmap.ToRoadmapItemSet(resp.Items)
```

Or drive the full pipeline programmatically with the sync engine — see
[Store & Sync](store-sync.md).
