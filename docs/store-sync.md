# Store & Sync

## The Dolt store

The canonical store is [Dolt](https://www.dolthub.com/) — a
MySQL-compatible, version-controlled database — accessed through
[Ent](https://entgo.io/) over the MySQL wire protocol. The wiring follows
the pattern established by visionstudio: a local `dolt sql-server`
subprocess (launched automatically when none is reachable), and Dolt
commits (`CALL DOLT_ADD`/`DOLT_COMMIT`) wrapping each sync run so the
store's history is itself versioned.

Defaults:

| Setting | Value |
|---|---|
| DSN | `root:@tcp(127.0.0.1:13307)/omniroadmap` |
| Data directory | `~/.omniroadmap` |
| Port | 13307 (distinct from visionstudio's 13306) |

Port/DSN/data-dir are configurable via `~/.omniroadmap/config.json` (or
`OMNIROADMAP_PORT`/`OMNIROADMAP_DSN`/`OMNIROADMAP_DATA_DIR`, or CLI flags)
— see [`omniroadmap config`](cli.md#omniroadmap-config). This matters
when running multiple Dolt-backed tools on one machine: each needs its
own port so their `dolt sql-server` processes don't collide.

### Testing the store

The `store` package's Dolt integration tests are opt-in, not opt-out:
they skip by default — even with the `dolt` binary on PATH — so a plain
`go test ./...` stays a fast, hermetic check of the Go library code and
never depends on a live Dolt server. Run them explicitly with:

```bash
OMNIROADMAP_TEST_DOLT=1 go test ./store/...
```

### Tables

| Table | Contents |
|---|---|
| `items` | Canonical items: provenance (`provider`, `source_id`, `source_ref`, `source_url`), kind, name/description, flattened status (`status_name`, `status_category`, ...), dates, owner, `moscow`/`kano`, `rice` (JSON), `tags`/`custom_fields`/`metadata` (JSON) |
| `releases` | Canonical releases with the same provenance columns |
| `item_augments` | Locally-authored data keyed by (provider, source ref): `moscow`, `kano`, `rice`, `okr_refs`, `notes`. Sync never writes here — see [Augments](augment.md) |
| `custom_field_defs` | Custom field definitions per provider (unique on provider+key) |
| `sync_meta` | Last sync time + record count per (provider, kind) |

Item IDs are canonical `"<provider>:<source id>"` strings, so re-syncing
upserts in place (Ent's `sql/upsert` feature, `ON DUPLICATE KEY UPDATE`).
`source_ref` carries the source system's human-facing reference (e.g. an
Aha reference number like `MYPROJ-123`) and is what augments key on.

Re-syncing **overwrites every provider-derived column** on an item —
including `moscow`/`kano`/`rice` values the fieldmap derived from provider
custom fields. Local judgments that must survive re-syncs belong in the
`item_augments` table instead ([Augments](augment.md)); reads overlay them
back on top.

## The sync engine

`sync.Run(ctx, store, provider, opts)` drives one provider into the store:

1. For each supported (or requested) item kind: paginate `ListItems`,
   apply the [fieldmap](fieldmap.md), upsert the batch, record sync
   metadata.
2. `ListReleases` / `ListCustomFieldDefinitions` when the provider's
   `Capabilities` advertise them (a provider that has per-record custom
   fields but can't list definitions — the aha-studio case — is
   tolerated).
3. One Dolt commit for the whole run.

Pagination is defensive: providers differ (some honor `Page`/`PerPage`,
some return everything on the first call), so paging stops when a page is
empty, short, or contributes no new IDs — a provider that ignores paging
entirely cannot cause an infinite loop or duplicates.

```go
report, err := sync.Run(ctx, doltStore, p, sync.Options{
    Kinds:    []provider.ItemKind{provider.ItemKindFeature},
    Fieldmap: mapping,
    PerPage:  100,
})
// report.ItemCounts, report.ReleaseCount, report.CustomFieldDefCount
```

The engine writes through a small `sync.Store` interface —
`*store.DoltStore` implements it, and tests run against an in-memory fake,
so the engine itself needs no Dolt to test.

## Programmatic store access

`store.DoltStore` reads items back as canonical types, with the
[augment](augment.md) overlay applied by default:

```go
s, _ := store.New(store.DefaultDSN())

// Merged view (synced data + augments); filter by provider/kind:
items, err := s.ListItems(ctx, store.ItemFilter{Provider: "aha-studio"})

// One item by source ref (e.g. "MYPROJ-123"), source ID, or canonical ID:
it, err := s.GetItem(ctx, "aha-studio", "MYPROJ-123")

// Raw synced data, no overlay:
raw, err := s.ListItems(ctx, store.ItemFilter{WithoutAugments: true})
```

`store.DoltStore.Client()` exposes the underlying Ent client for queries
beyond that:

```go
rows, err := s.Client().Item.Query().
    Where(item.Provider("aha-studio"), item.Moscow("must_have")).
    All(ctx)
```

Because the store is Dolt, you also get versioned history for free:
`dolt log`, `dolt diff`, and branches all work on `~/.omniroadmap`'s
database — every sync run is a commit.
