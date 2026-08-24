// Package materialize writes a compiled, reviewed
// assessment.ReportDataset's ranking back onto each cited
// OpportunityAssessment row's rank projection columns, and marks the
// dataset "final" — the last step in the compile → review → materialize
// workflow (prism-roadmap PRD FR14 / RMI-OMNIROADMAP-006).
//
// This is the promised follow-up to RMI-OMNIROADMAP-001, which
// deliberately left opportunity_rank_calculated/final unset on
// OpportunityAssessment: ranking is inherently cross-opportunity and could
// not be derived when a single assessment was saved. Now that a full-corpus
// compile (compile.Compile) has produced a dataset and it has been
// reviewed (review.Apply, then a fresh compile), this writes the result
// back so a query against one assessment can find its current rank
// directly, without needing the whole dataset.
package materialize

import (
	"context"
	"fmt"

	"github.com/grokify/prism-roadmap/assessment"
)

// StatusFinal marks a dataset as reviewed and rank-materialized.
const StatusFinal = "final"

// Store is the persistence surface Materialize needs.
type Store interface {
	SetOpportunityRank(ctx context.Context, assessmentID string, calculated, final int) error
	SetReportDatasetStatus(ctx context.Context, id, status string) error
}

// Materialize writes dataset's ranking back onto each cited
// OpportunityAssessment row, then marks datasetID StatusFinal.
//
// Excluded opportunities (RankedOpportunity.Excluded != "") are skipped —
// they have no CalculatedRank/FinalRank to write. Each assessment's rank is
// set independently (not wrapped in one cross-call transaction, since Store
// is an interface over persistence this package doesn't own the
// transaction boundary for); a failure partway through leaves some
// assessments updated and some not, but SetOpportunityRank is idempotent,
// so re-running Materialize after fixing the cause is always safe.
func Materialize(ctx context.Context, s Store, datasetID string, dataset assessment.ReportDataset) error {
	for _, r := range dataset.Ranking {
		if r.Excluded != "" {
			continue
		}
		if err := s.SetOpportunityRank(ctx, r.AssessmentID, r.CalculatedRank, r.FinalRank); err != nil {
			return fmt.Errorf("materialize: setting rank for %s: %w", r.AssessmentID, err)
		}
	}

	if err := s.SetReportDatasetStatus(ctx, datasetID, StatusFinal); err != nil {
		return fmt.Errorf("materialize: marking dataset %s final: %w", datasetID, err)
	}
	return nil
}
