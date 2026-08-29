# TRD — COMPASS-RICE Prioritization — MoSCoW + normalized cross-profile ranking, scoring CLI, and DashForge dashboards

## Architecture

### Layering across repos

| Layer | Repo | Owns |
|---|---|---|
| Methodology | `ProductBuildersHQ/compass-rice` (→ v0.4.0) | Profiles, evidence models, normalizers, judge contract, provenance-derived Confidence, **new: exported `catalog` package** (ProfileID → {Normalizer, evidence unmarshal}) |
| Assessment IR | `grokify/prism-roadmap` (→ v0.20.0) | `CompassAssessment` on `OpportunityAssessment`, `ResolveCompassRICE` gate, `ProfileAssignment` two-phase record, ranking (`RankingPolicy` unchanged) |
| Runtime | `grokify/omniroadmap` | Ingest (`compassbridge`), Dolt persistence, PM review gate, compile gating, analytics datasets, DashForge dashboard pack, Cobra CLI |

compass-rice's integration contract ("compass-rice defines the methodology; the consumer executes it") is preserved: no band thresholds or rubric content are duplicated outside compass-rice.

## Technical decisions

- **D1 — IR home**: COMPASS types live in prism-roadmap's `assessment` package (which takes a compass-rice dependency), so `ToRankInput`, store projections, and report rendering share one resolution point.
- **D2 — Score resolution**: `ToRankInput` prefers `Compass` when present; the legacy ladder path is otherwise unchanged (API compatibility). Compass-first *purity* is enforced in omniroadmap's `compile`: no Compass, unconfirmed assignment, or assignment/assessment profile mismatch → `Computable: false` with an explicit reason. Never rescale or mix the legacy 0..1 Reach regime with compass 0–100.
- **D3 — Two-phase gating**: `ProfileAssignment` (spec-scoped, survives cycles, like `RankOverride`) carries `proposed`/`confirmed`/`rejected` status; `CompassAssessment.NeedsHumanReview` (from `judge.Output.NeedsHumanReview()`) blocks scoring until a cycle carries `HumanReview`. PM acceptance is a new assessment cycle (cycle immutability), not a mutation.
- **D4 — Confidence integrity**: at ingest, `provenance.Confidence(output.Claims)` must equal `Normalized.Confidence` (derived from the evidence struct's verified-source counts). Mismatch → rejected with `RepairPrompts()`. A judge cannot claim more confidence than its verified claims support.
- **D5 — Storage**: existing hybrid pattern — canonical typed-JSON column + indexed projections. New `profile_assignments` table; `compass_profile_id` projection on `opportunity_assessments`; `rice_score`/`rice_computable` projections derive compass-first, matching `ToRankInput`.
- **D6 — Dashboards**: DashForge is the dashboard layer. omniroadmap is an "application" analytics source (catalog + read-only GuardSQL over canonical rows, in-memory — no raw SQL exposure). Curated `dashboardir.Dashboard`/`SavedQuestion` definitions are built in Go, served at `/api/analytics/dashboards`, and exportable for import into dashforge-server. Per-profile datasets flatten the profile's raw evidence fields into columns (all rows in one profile share an evidence schema), enabling raw-vs-normalized validation side by side.
- **D7 — CLI**: thin Cobra wrappers over library funcs only; all logic unit-testable without the CLI.

## Core types (sketch)

```go
// prism-roadmap assessment package
type CompassAssessment struct {
    ProfileID    rice.ProfileID
    EvidenceJSON json.RawMessage        // source of truth; re-normalizable via catalog
    Normalized   rice.Normalized        // deterministic stage-3 result
    Categories   []rubric.CategoryResult
    Claims       []*claims.Claim
    NeedsHumanReview bool
    HumanReview  *CompassHumanReview    // ReviewedBy, ReviewedAt, Note
}

func ResolveCompassRICE(c *CompassAssessment) RICEScoreResult // the single gate

type ProfileAssignment struct {
    SpecID, Rationale, ProposedBy, ConfirmedBy string
    ProfileID rice.ProfileID
    Secondary []rice.Profile              // context only, never scored
    Status    ProfileAssignmentStatus     // proposed | confirmed | rejected
    ConfirmedAt time.Time
    EvidenceIDs []string
}
```

## Package layout (omniroadmap)

```
compassbridge/        # Ingest(judge.Output), NewCycleWithCompass, Propose/ConfirmProfile
store/                # profileassignment.go + compass projections (ent lockstep)
review/               # EditProfile kind
compile/              # two-phase gating before Rank
render/               # COMPASS section via compass-rice render.Markdown
analyticscatalog/     # opportunity_assessments, profile_assignments, compass_<profile> datasets
analyticsquery/       # dataset dispatch beyond items
analyticsdashboards/  # curated DashboardIR pack + export
cmd/omniroadmap/      # assess, profile, moscow, analytics export-dashboards
```

## Testing strategy

- Library-first unit tests against fake stores (existing repo convention); Dolt round-trip only in opt-in `OMNIROADMAP_TEST_DOLT=1` tests.
- End-to-end unit test: propose → ingest customer/b2b fixture → confirm → compile → assert compass-scored rank; assert exclusion reasons for unconfirmed/unreviewed/un-assessed; assert `NeedsHumanReview` blocks until a `HumanReview` cycle lands.
- Dashboard pack validated against the DashboardIR schema and cross-checked to reference only datasets/fields the catalog exposes.

## Out of scope (technical)

- Fixing the godolt TCP-readiness race (tracked separately, RMI-OMNIROADMAP-010 territory).
- dashforge-server changes — omniroadmap only produces catalog/query/dashboard documents it already understands.
