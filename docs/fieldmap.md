# Fieldmap: MoSCoW/Kano/RICE from custom fields

No PM tool exposes MoSCoW, Kano, or RICE as first-class API fields — but
many teams store them as **custom fields**, with tenant-specific keys and
value vocabularies. The `fieldmap` package captures that per-tenant
knowledge as a JSON config and applies it after fetch, populating the
canonical `Item.MoSCoW`, `Item.Kano`, and `Item.RICE` fields.

Fieldmap-derived values are **provider data**: they're refreshed (and
overwritten) on every sync, tracking the source custom fields. For
hand-authored prioritization that must survive re-syncs, use
[Augments](augment.md) instead — augment values win over fieldmap-derived
ones on read.

## Config format

```json
{
  "description": "Acme Aha workspace",
  "moscow": {
    "key": "priority_moscow",
    "values": {"P1": "must_have", "P2": "should_have", "P3": "could_have"}
  },
  "kano": {
    "key": "kano_category",
    "values": {"Table Stakes": "must-be"}
  },
  "reach":      {"key": "rice_reach"},
  "impact":     {"key": "rice_impact"},
  "confidence": {"key": "rice_confidence", "values": {"High": "1.0", "Medium": "0.8", "Low": "0.5"}},
  "effort":     {"key": "rice_effort"},
  "score":      {"key": "rice_score"}
}
```

Each rule names the custom-field `key` to read, plus an optional `values`
table normalizing the tenant's vocabulary (matched case-insensitively).
Omit a rule entirely when the tenant doesn't store that component.

## Normalization behavior

**MoSCoW** values pass through the `values` table first, then a built-in
normalization: lowercase, spaces/hyphens → underscores, apostrophes
stripped, common aliases recognized. All of these resolve without any
`values` config:

| Raw value | Result |
|---|---|
| `must_have`, `Must Have`, `must` | `must_have` |
| `should`, `Should Have` | `should_have` |
| `could-have`, `Could Have` | `could_have` |
| `Won't Have`, `wont` | `wont_have` |
| anything unrecognized | left unset |

**Kano** values normalize the same way (lowercase, spaces/underscores →
hyphens) onto prism-roadmap's category vocabulary, with common aliases:

| Raw value | Result |
|---|---|
| `must-be`, `must_be`, `Basic`, `Threshold`, `Expected` | `must-be` |
| `performance`, `One Dimensional`, `Linear` | `performance` |
| `attractive`, `Delighter`, `Excitement` | `attractive` |
| `indifferent` / `reverse` / `questionable` | as-is |
| anything unrecognized | left unset |

**RICE** components coerce to numbers: raw numeric custom fields, numeric
strings (`"5000"`), and raw-JSON values all work; the `values` table lets
label vocabularies map to numbers (`{"High": "1.0"}`). Impact/confidence
are multiplier-style floats (confidence also tolerates percentages —
`80` → `0.8` at export time).

Raw-JSON byte values (as returned by aha-go's typed client) are unwrapped
automatically.

## Applying

Via the CLI, per sync run:

```bash
omniroadmap sync --provider aha --fieldmap tenant-acme.fieldmap.json
```

Or in code:

```go
mapping, err := fieldmap.Load("tenant-acme.fieldmap.json")
fieldmap.Apply(items, mapping) // enriches items in place; nil mapping = no-op
```

Items whose mapped fields are absent are left untouched — imported items
without prioritization data stay honestly unprioritized. That's fine
downstream: prism-roadmap's `rmi.RoadmapItem` treats MoSCoW as optional
(v0.17.0+), so untriaged imports validate cleanly and can be prioritized
later in prism-roadmap's own tooling.

## Finding your tenant's field keys

For Aha, list custom field definitions to discover keys — via aha-go's
library (`client.ListCustomFieldDefinitions(ctx)`), aha-studio's
`list_custom_fields` MCP tool, or a live-API sync (`omniroadmap sync
--provider aha` stores definitions in the `custom_field_defs` table). Then
check a few records' `custom_fields` values to learn the value vocabulary
before writing the `values` tables.
