# Providers

A provider implements `omniroadmap-core`'s `provider.Provider` interface,
translating one tool's native records into canonical types. Adapters live
inside each tool's own SDK repo (the embedded-adapter pattern used by
elevenlabs-go/omnivoice and opik-go/integrations); importing the
`omniroadmap` package registers all of them by name.

## `aha` — live Aha! API

- **Repo**: [aha-go](https://github.com/grokify/aha-go), subpackage `omniroadmap/`
- **Config**: `*aha.Client` (credentials via `AHA_SUBDOMAIN` / `AHA_API_KEY`)
- **Kinds**: feature, epic, initiative; releases supported
- Richest status fidelity: Aha's workflow statuses carry a `Complete` flag
  and position, so `StatusCategory` normalization is reliable.
- List responses are meta-level (Aha's list endpoints return lightweight
  records); `GetItem` returns full fidelity including custom fields.
- Product-scoped operations (`ListReleases`, `ListStatuses`) require
  `WithProductID`.

## `aha-studio` — local Aha cache

- **Repo**: [aha-studio](https://github.com/grokify/aha-studio), subpackage `omniroadmap/`
- **Config**: `*sync.DB` (default cache at `~/.ahastudio/cache.db`)
- **Kinds**: feature, epic, initiative; releases supported
- Zero API traffic — reads whatever aha-studio has synced. aha-studio
  remains the Aha-native system of record; this provider generalizes it.
- Fidelity caveats: only status *names* are cached (category normalization
  is a name heuristic); custom fields exist only for detail-synced
  records; custom field *definitions* aren't available.
- Canonical IDs are prefixed `aha-studio:` — syncing both `aha` and
  `aha-studio` into one store produces parallel copies, not merged rows.

## `productboard` — ProductBoard API v2

- **Repo**: [productboard-go](https://github.com/grokify/productboard-go), subpackage `omniroadmap/`
- **Config**: `*productboard.Client` (`PRODUCTBOARD_API_TOKEN`)
- **Kinds**: feature (incl. subfeatures), objective; releases supported
- ProductBoard's status field values carry no completion/category signal,
  so `Status.Category` is deliberately left unset rather than guessed.
- `ListStatuses` and `ListCustomFieldDefinitions` are unsupported in the
  thin v0.1 SDK scope.

## `jpd` — Jira Product Discovery

- **Repo**: [go-atlassian](https://github.com/grokify/go-atlassian), subpackage `omniroadmap/`
- **Config**: `*jira.Client` (`JIRA_URL` / `JIRA_USER` / `JIRA_TOKEN`)
- **Kinds**: feature (JPD "Ideas" map to ItemKindFeature); no releases
- Scope is deliberately narrow: JPD Ideas are ordinary Jira issues,
  reachable via the standard public REST API. JPD's distinctive features —
  Views (prioritization matrices), Insights (weighted scoring), roadmap
  timelines — have **no officially supported public API** and are
  excluded.
- Requires `WithProjectKey` to scope to the JPD project; the idea issue
  type name is configurable via `WithIdeaIssueType` (default "Idea").
- Status normalization is authoritative: Jira's own status categories
  (`new`/`indeterminate`/`done`) map directly.

## Capabilities

Check what a provider actually supports before relying on an operation:

```go
caps := p.Capabilities()
if caps.SupportsReleases {
    releases, err := p.ListReleases(ctx, &omniroadmap.ListReleasesRequest{})
    ...
}
```

Unsupported operations return `omniroadmap.ErrUnsupportedOperation`
(checkable via `omniroadmap.IsUnsupportedOperation`).

## Writing a new adapter

See [omniroadmap-core's documentation](https://grokify.github.io/omniroadmap-core/)
for the interface contract, registration pattern, and the `providertest`
conformance suite every adapter should run.
