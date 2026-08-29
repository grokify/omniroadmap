package store

import (
	"context"
	"fmt"

	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/prism-roadmap/assessment"

	"github.com/grokify/omniroadmap/ent"
	entassessment "github.com/grokify/omniroadmap/ent/opportunityassessment"
)

// kanoDimensionID / mihDimensionID identify the two built-in portfolio
// dimensions (github.com/grokify/prism-roadmap/assessment.KanoDimension,
// MarketInvestmentHorizonDimension) whose resolved category gets its own
// indexed column here. Custom dimensions live in normalized tables
// (RMI-OMNIROADMAP-002), not as ad hoc columns on this table.
const (
	kanoDimensionID = "kano"
	mihDimensionID  = "market-investment-horizon"
)

// assessmentProjection holds the indexed columns derived from an
// assessment.OpportunityAssessment's canonical fields — computed here so
// they can never drift out of sync with the canonical record they're
// projected from.
type assessmentProjection struct {
	moscowClass      string
	riceScore        *float64
	riceComputable   bool
	kanoCategory     string
	mihCategory      string
	compassProfileID string
}

// projectAssessment derives rice_score/rice_computable compass-first,
// matching assessment.OpportunityAssessment.ToRankInput's own precedence
// (RMI-PRISMROADMAP-016): a Compass assessment's ResolveCompassRICE result
// wins over the legacy ladder RICE whenever both are present, since the two
// use incompatible Reach scales and mixing them in one column would make
// rice_score meaningless for cross-opportunity sorting.
func projectAssessment(a assessment.OpportunityAssessment) assessmentProjection {
	proj := assessmentProjection{
		moscowClass: a.MoSCoW().String(),
	}

	switch {
	case a.Compass != nil:
		proj.compassProfileID = string(a.Compass.ProfileID)
		result := assessment.ResolveCompassRICE(a.Compass)
		proj.riceComputable = result.Computable
		if result.Computable {
			score := result.Score
			proj.riceScore = &score
		}
	case a.RICE != nil:
		result := assessment.ComputeRICE(*a.RICE)
		proj.riceComputable = result.Computable
		if result.Computable {
			score := result.Score
			proj.riceScore = &score
		}
	}

	if d := a.DimensionAssignment(kanoDimensionID); d != nil {
		if ids := d.SelectedOptionIDs(); len(ids) > 0 {
			proj.kanoCategory = ids[0]
		}
	}
	if d := a.DimensionAssignment(mihDimensionID); d != nil {
		if ids := d.SelectedOptionIDs(); len(ids) > 0 {
			proj.mihCategory = ids[0]
		}
	}

	return proj
}

// SaveOpportunityAssessment upserts a as its own row, and — when
// a.Cycle.SupersedesID is set — atomically flips the row it supersedes to
// Current=false in the same transaction. This enforces "at most one
// current row per opportunity" without requiring the caller to remember a
// second call: prism-roadmap's OpportunityAssessment.NextCycle leaves
// calling MarkSuperseded to "the caller" by design, and this store is that
// caller.
//
// Indexed projection columns (moscow_class, rice_score, rice_computable,
// kano_category, mih_category) are computed from a's canonical fields at
// save time — every column is set explicitly, including empty/zero values,
// so a re-save of the same ID (e.g. re-running a judge pass mid-cycle)
// fully replaces the prior projection rather than leaving stale values
// behind (prism-roadmap TRD: "omniroadmap relational fields = indexed/
// materialized projection"). opportunity_rank_calculated/final are left
// untouched here — ranking is inherently cross-opportunity and is
// populated separately by rank materialization across the full corpus
// (RMI-OMNIROADMAP-006).
func (s *DoltStore) SaveOpportunityAssessment(ctx context.Context, a assessment.OpportunityAssessment) error {
	if err := a.Validate(); err != nil {
		return fmt.Errorf("store: invalid opportunity assessment: %w", err)
	}

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("store: starting transaction: %w", err)
	}

	if a.Cycle.SupersedesID != "" {
		if _, err := tx.OpportunityAssessment.Update().
			Where(entassessment.ID(a.Cycle.SupersedesID)).
			SetCurrent(false).
			Save(ctx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: superseding %s: %w", a.Cycle.SupersedesID, err)
		}
	}

	proj := projectAssessment(a)

	err = tx.OpportunityAssessment.Create().
		SetID(a.ID).
		SetOpportunitySpecID(a.Opportunity.SpecID).
		SetRmiID(a.Opportunity.RMIID).
		SetTitle(a.Title).
		SetCycleNumber(a.Cycle.Number).
		SetAssessedAt(a.Cycle.AssessedAt).
		SetSupersedesID(a.Cycle.SupersedesID).
		SetCurrent(a.Cycle.Current).
		SetMoscowClass(proj.moscowClass).
		SetNillableRiceScore(proj.riceScore).
		SetRiceComputable(proj.riceComputable).
		SetKanoCategory(proj.kanoCategory).
		SetMihCategory(proj.mihCategory).
		SetCompassProfileID(proj.compassProfileID).
		SetCanonical(a).
		OnConflict().
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: upserting assessment %s: %w", a.ID, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: committing assessment %s: %w", a.ID, err)
	}
	return nil
}

// GetOpportunityAssessment returns one assessment cycle by ID, or
// omniroadmap.ErrNotFound when none exists.
func (s *DoltStore) GetOpportunityAssessment(ctx context.Context, id string) (*assessment.OpportunityAssessment, error) {
	row, err := s.client.OpportunityAssessment.Query().
		Where(entassessment.ID(id)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("store: opportunity assessment %s: %w", id, omniroadmap.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: querying opportunity assessment %s: %w", id, err)
	}
	return &row.Canonical, nil
}

// ListOpportunityAssessments returns every cycle recorded for one
// opportunity (by OpportunitySpecID), ordered oldest cycle first.
func (s *DoltStore) ListOpportunityAssessments(ctx context.Context, opportunitySpecID string) ([]assessment.OpportunityAssessment, error) {
	rows, err := s.client.OpportunityAssessment.Query().
		Where(entassessment.OpportunitySpecID(opportunitySpecID)).
		Order(ent.Asc(entassessment.FieldCycleNumber)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying assessments for %s: %w", opportunitySpecID, err)
	}
	out := make([]assessment.OpportunityAssessment, len(rows))
	for i, row := range rows {
		out[i] = row.Canonical
	}
	return out, nil
}

// ListCurrentOpportunityAssessments returns the current cycle for every
// opportunity that has one — the corpus a compile run
// (RMI-OMNIROADMAP-004) operates over.
func (s *DoltStore) ListCurrentOpportunityAssessments(ctx context.Context) ([]assessment.OpportunityAssessment, error) {
	rows, err := s.client.OpportunityAssessment.Query().
		Where(entassessment.Current(true)).
		Order(ent.Asc(entassessment.FieldOpportunitySpecID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying current assessments: %w", err)
	}
	out := make([]assessment.OpportunityAssessment, len(rows))
	for i, row := range rows {
		out[i] = row.Canonical
	}
	return out, nil
}

// DeleteOpportunityAssessment removes one assessment cycle by ID. Deleting
// a nonexistent assessment is a no-op.
func (s *DoltStore) DeleteOpportunityAssessment(ctx context.Context, id string) error {
	_, err := s.client.OpportunityAssessment.Delete().
		Where(entassessment.ID(id)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: deleting opportunity assessment %s: %w", id, err)
	}
	return nil
}

// SetOpportunityRank updates only the opportunity_rank_calculated/final
// projection columns for one assessment — never touches canonical or any
// other column. This is the promised follow-up to SaveOpportunityAssessment
// (RMI-OMNIROADMAP-001), which deliberately left these two columns unset:
// ranking is inherently cross-opportunity and cannot be derived from a
// single assessment in isolation. Rank materialization
// (RMI-OMNIROADMAP-006) calls this once a compiled ReportDataset has been
// reviewed and approved.
func (s *DoltStore) SetOpportunityRank(ctx context.Context, assessmentID string, calculated, final int) error {
	n, err := s.client.OpportunityAssessment.Update().
		Where(entassessment.ID(assessmentID)).
		SetOpportunityRankCalculated(calculated).
		SetOpportunityRankFinal(final).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("store: setting rank for assessment %s: %w", assessmentID, err)
	}
	if n == 0 {
		return fmt.Errorf("store: assessment %s: %w", assessmentID, omniroadmap.ErrNotFound)
	}
	return nil
}
