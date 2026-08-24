# Augments (data that survives re-syncs)

Synced provider data and your own judgments have different lifecycles. A
re-sync must be free to **overwrite provider data wholesale** — names,
statuses, dates, custom fields, and prioritization derived from provider
custom fields via [fieldmap](fieldmap.md) — so the store always mirrors
the source. But locally-authored prioritization and planning data must
*not* be lost when that happens.

The `augment` package and `item_augments` table solve this: augments are
stored separately from synced items, keyed by the source system's stable
reference — e.g. an Aha reference number like `MYPROJ-123` — and the sync
engine never touches them. Reads overlay augments onto items, with augment
values winning.

| Layer | Written by | On re-sync |
|---|---|---|
| `items` table | `omniroadmap sync` (provider data + fieldmap) | Overwritten |
| `item_augments` table | You (`augment set`, `store.SetItemAugment`) | Untouched |

## What an augment holds

| Field | Meaning |
|---|---|
| `moscow` | `must_have` \| `should_have` \| `could_have` \| `wont_have` |
| `kano` | `must-be` \| `performance` \| `attractive` \| `indifferent` \| `reverse` \| `questionable` |
| `rice` | RICE components (reach, impact, confidence, effort, score). Set components override same-name synced/fieldmap components; unset ones shine through |
| `okr_refs` | Links to OKR documents/objectives (e.g. prism-roadmap objective IDs) |
| `notes` | Free text |
| `metadata` | Anything else; surfaces on the item's `Metadata` prefixed `augment.` |

## CLI

```bash
# Set fields (merges with any existing augment for that ref):
omniroadmap augment set --provider aha-studio --ref MYPROJ-123 \
  --moscow must_have --kano performance --effort 2 \
  --okr OKR-2026-Q3-01 --notes "exec ask"

# Inspect the augment layer alone:
omniroadmap augment get --provider aha-studio --ref MYPROJ-123
omniroadmap augment list

# Read the merged item (synced data + augment overlay):
omniroadmap item get MYPROJ-123

# Clear one field / remove the whole augment:
omniroadmap augment set --provider aha-studio --ref MYPROJ-123 --moscow ""
omniroadmap augment rm  --provider aha-studio --ref MYPROJ-123
```

`augment set` and `augment rm` each finish with a Dolt commit, so the
augment history is versioned alongside sync history (`dolt log`).

## Library

```go
import (
    "github.com/grokify/omniroadmap/augment"
    "github.com/grokify/omniroadmap/store"
)

s, _ := store.New(store.DefaultDSN())

// Write local judgments:
err := s.SetItemAugment(ctx, augment.ItemAugment{
    Provider:  "aha-studio",
    SourceRef: "MYPROJ-123",
    MoSCoW:    "must_have",
    Kano:      "performance",
})

// Reads overlay augments by default:
item, err := s.GetItem(ctx, "aha-studio", "MYPROJ-123")

// Raw synced data, no overlay:
items, err := s.ListItems(ctx, store.ItemFilter{
    Provider:        "aha-studio",
    WithoutAugments: true,
})
```

`augment.Apply(items, augs)` is also usable standalone against items from
any source (a live provider, an export pipeline) — matching is by
`(Provider, SourceRef)` with a fallback to `SourceID` for providers
without human-facing refs (e.g. ProductBoard UUIDs).

## Precedence

For any one field on a read item:

1. **Augment** value, when set — hand-authored wins.
2. **Fieldmap-derived** value stored at sync time (from provider custom
   fields).
3. Unset.

RICE merges per component: an augment that sets only `effort` overrides
effort while fieldmap-derived `reach` shines through.

## Fields with no canonical Item slot

`okr_refs`, `notes`, and `metadata` have no first-class field on the
canonical `Item`; after `Apply` they surface in `Item.Metadata` under
`augment.okr_refs`, `augment.notes`, and `augment.<key>`.

Note: [prism-roadmap export](export-prismroadmap.md) currently carries
MoSCoW and RICE into `rmi.RoadmapItem`; Kano has no RMI field yet and
stays on the canonical Item.
