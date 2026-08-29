# COMPASS-RICE Prioritization — MoSCoW + normalized cross-profile ranking, scoring CLI, and DashForge dashboards — Roadmap

**Initiative:** `INIT-OMNIROADMAP-001`
**Repository:** `github.com/grokify/omniroadmap`
**Status:** Planned

> RMI IDs are stable and permanent. Commits implementing an item carry the trailer `Refs: RMI-<REPOSLUG>-<NNN>`. Phase status is derived from member RMIs — a phase is complete only when all its required RMIs are complete.

## Phase 1 — Framework surface: compass-rice catalog + prism-roadmap assessment IR

**Theme:** Make the frameworks consumable — export compass-rice's ProfileID→Normalizer catalog, and extend prism-roadmap's assessment IR with the COMPASS judgment record, resolution gate, and two-phase profile assignment.

- [ ] `RMI-COMPASSRICE-014` Export ProfileID→Normalizer catalog package (Get/IDs/Normalize/NormalizeJSON/Document); refactor cmd/rice to consume it
- [ ] `RMI-PRISMROADMAP-015` CompassAssessment type (evidence JSON, Normalized, categories, claims, human-review state) + Compass field on OpportunityAssessment
- [ ] `RMI-PRISMROADMAP-016` ResolveCompassRICE gate; ToRankInput compass-first resolution; RICEScoreResult.ProfileID provenance
  - Depends on: `RMI-PRISMROADMAP-015`
- [ ] `RMI-PRISMROADMAP-017` ProfileAssignment record: two-phase (LLM-proposed, PM-confirmed) primary investment thesis with status lifecycle
- [ ] `RMI-PRISMROADMAP-018` Assessment docs COMPASS-RICE section + v0.20.0 release docs

## Phase 2 — omniroadmap scoring runtime: ingest, store, review gate, compile gating

**Theme:** The two-phase scoring pipeline — LLM judge.Output ingestion with a claims-backed confidence integrity check, Dolt persistence, PM review gate, and compile-time exclusion of anything not yet confirmed.

- [ ] `RMI-OMNIROADMAP-012` compassbridge: judge.Output ingest with claims-backed confidence integrity check, cycle attach, profile assignment helpers
  - Depends on: `RMI-COMPASSRICE-014`, `RMI-PRISMROADMAP-015`, `RMI-PRISMROADMAP-017`
- [ ] `RMI-OMNIROADMAP-013` store: profile_assignments table + compass_profile_id projection; compass-first rice_score projection
  - Depends on: `RMI-PRISMROADMAP-016`, `RMI-PRISMROADMAP-017`
- [ ] `RMI-OMNIROADMAP-014` review EditProfile kind + compile two-phase gating (excluded until profile confirmed and assessment reviewed)
  - Depends on: `RMI-OMNIROADMAP-013`
- [ ] `RMI-OMNIROADMAP-015` render: COMPASS section (profile, band, method, raw-evidence echo) in opportunity report

## Phase 3 — Analytics: catalog datasets, query dispatch, DashForge dashboard pack

**Theme:** DashForge is the dashboard layer — expose assessments, assignments, and per-profile raw evidence as analytics datasets, and ship the curated DashboardIR pack (Splunk model: curated dashboards on a generic engine).

- [ ] `RMI-OMNIROADMAP-016` analyticscatalog: opportunity_assessments, profile_assignments, and per-profile raw-evidence datasets
  - Depends on: `RMI-OMNIROADMAP-013`
- [ ] `RMI-OMNIROADMAP-017` analyticsquery: dataset dispatch beyond items (assessments, assignments, compass_* evidence)
  - Depends on: `RMI-OMNIROADMAP-016`
- [ ] `RMI-OMNIROADMAP-018` analyticsdashboards: curated DashboardIR pack (compass-rice-all, moscow-compass-rice, compass-profile, portfolio-overview) + serve/export
  - Depends on: `RMI-OMNIROADMAP-017`

## Phase 4 — Scoring CLI and documentation

**Theme:** The write path for humans and agents — Cobra commands wrapping the scoring libraries, plus pipeline documentation.

- [ ] `RMI-OMNIROADMAP-019` CLI assess group: list/show/import (LLM judge.Output)/set (human-attested evidence)
  - Depends on: `RMI-OMNIROADMAP-012`
- [ ] `RMI-OMNIROADMAP-020` CLI profile and moscow groups: two-phase assignment lifecycle; evidence-backed MoSCoW get/set via review
  - Depends on: `RMI-OMNIROADMAP-014`
- [ ] `RMI-OMNIROADMAP-021` Docs: compass-rice pipeline guide, README, CLAUDE.md conventions; analytics export-dashboards command docs
