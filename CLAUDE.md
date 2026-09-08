# CLAUDE.md — omniroadmap

Repo-specific guidance for Claude Code. Also read the org-level
`~/go/src/github.com/grokify/.github/CLAUDE.md` for grokify-wide conventions.

## What this repo is

The batteries-included entry point for the omniroadmap ecosystem: a
Dolt-backed canonical store, provider-agnostic sync engine, per-tenant
custom-field prioritization mapping (`fieldmap`), locally-authored data
that survives re-syncs (`augment`), export into
[prism-roadmap](https://github.com/grokify/prism-roadmap), and the
`omniroadmap` CLI. It bundles the provider adapters that live inside each
tool's own SDK repo (`aha-go/omniroadmap`, `aha-studio/omniroadmap`,
`productboard-go/omniroadmap`, `go-atlassian/omniroadmap`) via blank
imports.

The core contract (the `Provider` interface, canonical types, registry,
`providertest` conformance) lives in
[`omniroadmap-core`](https://github.com/grokify/omniroadmap-core) — kept
free of Ent/Dolt so the three SDK repos that `require` it don't inherit
those deps. Dolt/store/visualization concerns belong here, never in core.

## `fieldmap` vs. `augment` — don't conflate these

Both populate `Item.MoSCoW`/`Item.Kano`/`Item.RICE`, but they have
opposite lifecycles and that distinction is load-bearing:

- **`fieldmap`** runs *during sync*, reading provider custom fields
  (`Item.CustomFields`). Its output is provider data — a re-sync
  recomputes and overwrites it, same as any other synced column.
- **`augment`** is hand-authored, stored in a separate `item_augments`
  table keyed by `(provider, source_ref)`, and the sync engine never
  writes to it. Reads overlay it on top of synced items, winning over
  fieldmap-derived values.

If you're adding a new prioritization-style field, decide up front which
category it is. A field a source system exposes (even indirectly, via a
custom field) belongs in fieldmap; a field only a human decides (e.g. "we
chose to override this to must-have despite what Aha's custom field
says") belongs in augment.

## COMPASS-RICE prioritization — compass-only, two-phase, no silent drops

See [`docs/compass-rice.md`](docs/compass-rice.md) for the full pipeline.
Load-bearing conventions when touching this code:

- **`compile.gateRankInput` never falls back to the legacy ladder RICE.**
  prism-roadmap's own `OpportunityAssessment.ToRankInput` falls back to
  `ComputeRICE` when `Compass` is nil, for other library consumers.
  omniroadmap's own pipeline deliberately does not: an opportunity without
  a `Compass` assessment, or one whose profile isn't PM-confirmed, is
  uncomputable with an explicit reason, never legacy-scored. The two Reach
  regimes (0–100 banded vs. 0..1 fraction) are numerically incompatible in
  one ranked list.
- **`compassbridge.Ingest`'s confidence integrity check is not optional.**
  It compares the Confidence tier compass-rice's Normalizer derives from
  the evidence struct's self-reported source counts against the tier
  `provenance.Confidence` derives independently from the judge's actual
  verified claims, and rejects the assessment if the former exceeds the
  latter. Don't bypass this for the LLM-judge path (`assess import`); the
  human-entered path (`assess set` / `compassbridge.IngestHumanEvidence`)
  is the only place evidence is trusted without it, since there are no
  claims to cross-check.
- **A cycle-producing command must carry forward every other judgment
  field it isn't changing.** `assessment.OpportunityAssessment.NextCycle`
  itself only carries `Opportunity`/`Title`/`Cycle` forward by design —
  every other field starts blank. `compassbridge.NextCycleWithCompass` and
  the CLI's `carryForwardNextCycle` (`cmd/omniroadmap/moscow_commands.go`)
  both explicitly re-copy MoSCoW answers, Dimensions, Contributions, and
  Capabilities before overriding just the one field that command changed.
  A new cycle-producing command that skips this will silently drop
  unrelated judgment data — this was a real bug caught during
  RMI-OMNIROADMAP-020, not a hypothetical one.
- **Profile selection is two-phase and never self-confirmed.**
  `assessment.ProfileAssignment.Status` moves `proposed` → `confirmed`/
  `rejected` only through `compassbridge.ConfirmProfile`/`RejectProfile`,
  which both reject a transition attempted from any status other than
  `proposed`. Route all profile assignment writes through
  `review.Apply(review.Edit{Kind: review.EditProfile, ...})`, not a direct
  `store.SaveProfileAssignment` call, so the edit stays auditable.

## Ent schema changes

`ent/schema/*.go` is hand-written; everything else under `ent/` is
generated — never hand-edit it. After changing a schema file:

```bash
go generate ./ent
go build ./...   # regenerated code must compile before anything else
```

The `sql/upsert` feature flag is enabled (`ent/generate.go`) because sync
is upsert-heavy — every `store.Upsert*` call uses
`.OnConflict().UpdateNewValues()`. When adding a field to `Item` or
`ItemAugment`, update in lockstep:

1. `ent/schema/{item,itemaugment}.go` — the column.
2. `store/upsert.go` (`upsertItem`) and/or `store/augment.go`
   (`SetItemAugment`) — set it explicitly, including the zero value, so
   upsert/replace semantics stay correct (an omitted `Set*` call on
   create leaves the column at its Ent default, not "unchanged").
3. `store/query.go` (`itemFromRow`) / `store/augment.go`
   (`augmentFromRow`) — the reverse conversion for reads.
4. `omniroadmap-core/provider/types.go` if it's a new canonical field —
   adapters must NOT populate MoSCoW/Kano/RICE (no PM tool exposes them
   as first-class API fields); only fieldmap/augment do.

## Store configuration — port collisions

This store isn't the only Dolt-backed tool on a dev machine —
[visionstudio](https://github.com/ProductBuildersHQ/visionstudio) runs
its own `dolt sql-server` on port 13306. omniroadmap defaults to 13307
and additionally supports `~/.omniroadmap/config.json`
(`store.Config`/`store.LoadConfig`/`Resolve`) so a third tool, or a
changed default, doesn't require passing `--port`/`--dsn` on every
command. Resolution order: CLI flag > `OMNIROADMAP_*` env var > config
file > built-in default. See `store/config.go` and `docs/cli.md`'s
`omniroadmap config` section.

visionstudio and omniroadmap may merge in the future (per user direction,
not currently planned) — keep the config file shape close to
visionstudio's `pkg/cliconfig` (`dsn`/`defaults`-style JSON) so a merge
doesn't require a migration.

## Testing conventions

- The Dolt integration tests (`store/doltstore_test.go`,
  `store/opportunityspec_test.go`) are opt-in, not opt-out: they skip by
  default — even when the `dolt` binary happens to be on PATH — unless
  `OMNIROADMAP_TEST_DOLT` is set, so a plain `go test ./...` is a fast,
  hermetic check of the Go library code and never depends on a live Dolt
  server. Run them explicitly with
  `OMNIROADMAP_TEST_DOLT=1 go test ./store/...`. Don't add
  dolt-dependent assertions outside that file's `startDoltServer` gate.
- The sync engine (`sync/sync.go`) is tested against an in-memory fake
  `Store`, not Dolt — keep it that way; Dolt-specific behavior (upsert,
  commit-on-dirty) is `store` package's test responsibility.
- Copy-first discipline for manual/E2E testing: never point `omniroadmap
  sync --provider aha-studio` at the live `~/.ahastudio/cache.db` when
  experimenting — copy it first (`--cache-db /path/to/copy.db`).
