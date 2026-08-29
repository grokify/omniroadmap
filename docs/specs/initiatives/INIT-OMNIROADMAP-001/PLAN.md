# PLAN — COMPASS-RICE Prioritization — MoSCoW + normalized cross-profile ranking, scoring CLI, and DashForge dashboards

## Approach

Three repos in strict dependency order, each released (push → CI → tag) before the next pins it:

1. **compass-rice → v0.4.0** (Phase 1, RMI-COMPASSRICE-014): extract the unexported `cmd/rice` catalog into an exported `catalog` package; refactor `cmd/rice` to consume it; update `docs/integration.md`.
2. **prism-roadmap → v0.20.0** (Phase 1, RMI-PRISMROADMAP-015..018): `CompassAssessment` + `Compass` field, `ResolveCompassRICE`, compass-first `ToRankInput`, `RICEScoreResult.ProfileID`, `ProfileAssignment`, docs.
3. **omniroadmap** (Phases 2–4): `compassbridge` ingest, store schema + projections, review/compile gating, render section, analytics datasets + query dispatch + curated DashForge dashboard pack, Cobra CLI groups (`assess`, `profile`, `moscow`, `analytics export-dashboards`), docs.

## Sequencing rationale

- The exported catalog must exist before `compassbridge.Ingest` can resolve a stored `ProfileID` to a normalizer (RMI-OMNIROADMAP-012 requires RMI-COMPASSRICE-014).
- The IR types and resolution gate must exist before omniroadmap's store projections and compile gating can reference them (RMI-OMNIROADMAP-013 requires RMI-PRISMROADMAP-016/017).
- Analytics datasets depend on the store surface (016 ← 013); the dashboard pack depends on datasets + query dispatch (018 ← 017 ← 016).
- CLI last: thin wrappers over already-tested libraries (019 ← 012, 020 ← 014).

## Key risks and mitigations

- **compass-rice API churn** (three releases, two renames in its first days): pin exact versions; `EvidenceJSON` + versioned `ProfileID` keep stored assessments reproducible across profile revisions.
- **Mixed score regimes** (legacy 0..1 vs compass 0–100): compile-level exclusion rather than rescaling; exclusion reasons keep un-migrated opportunities visible.
- **Importer/judge quality**: confidence integrity check + `NeedsHumanReview` routing prevent plausible-but-unverified scores from ranking.
- **Ent schema churn**: follow the repo's documented lockstep (schema → upsert → query conversion → `go generate ./ent`); Dolt round-trip covered by opt-in integration tests.

## Dependencies

- `github.com/ProductBuildersHQ/compass-rice` v0.4.0 (new: catalog package)
- `github.com/grokify/prism-roadmap` v0.20.0 (new: COMPASS IR)
- `github.com/plexusone/structured-evaluation` (claims/rubric — already a dependency everywhere it's needed)
- `github.com/plexusone/dashforge` (dashboardir — already a dependency of omniroadmap)

## Working agreements

- Local `replace` directives only during development; removed and pinned to real tags before any push (pre-push checklist).
- Commits carry `Refs: RMI-<REPOSLUG>-<NNN>` trailers; review and execution proceed by phase.
- No pushes or tags without explicit request; tags only after remote CI passes.
