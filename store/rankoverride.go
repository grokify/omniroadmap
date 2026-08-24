package store

import (
	"context"
	"fmt"

	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/prism-roadmap/assessment"

	"github.com/grokify/omniroadmap/ent"
	entrankoverride "github.com/grokify/omniroadmap/ent/rankoverride"
)

// SaveRankOverride upserts a governance override for one assessment. Call
// o.Validate() (required fields) before calling this — this store does not
// re-validate, matching this repo's existing division of labor between
// domain-type validation and store persistence.
func (s *DoltStore) SaveRankOverride(ctx context.Context, o assessment.RankOverride) error {
	if err := o.Validate(); err != nil {
		return fmt.Errorf("store: invalid rank override: %w", err)
	}

	evidenceIDs := o.EvidenceIDs
	if evidenceIDs == nil {
		evidenceIDs = []string{}
	}

	err := s.client.RankOverride.Create().
		SetID(o.AssessmentID).
		SetFinalRank(o.FinalRank).
		SetRationale(o.Rationale).
		SetApprovedBy(o.ApprovedBy).
		SetEvidenceIds(evidenceIDs).
		OnConflict().
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: upserting rank override for %s: %w", o.AssessmentID, err)
	}
	return nil
}

// GetRankOverride returns the active override for one assessment, or
// omniroadmap.ErrNotFound if none exists.
func (s *DoltStore) GetRankOverride(ctx context.Context, assessmentID string) (*assessment.RankOverride, error) {
	row, err := s.client.RankOverride.Query().
		Where(entrankoverride.ID(assessmentID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("store: rank override for %s: %w", assessmentID, omniroadmap.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: querying rank override for %s: %w", assessmentID, err)
	}
	o := rankOverrideFromRow(row)
	return &o, nil
}

// ListRankOverrides returns every active override, ordered by assessment
// ID — the input assessment.ApplyOverrides needs for a compile run.
func (s *DoltStore) ListRankOverrides(ctx context.Context) ([]assessment.RankOverride, error) {
	rows, err := s.client.RankOverride.Query().
		Order(ent.Asc(entrankoverride.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying rank overrides: %w", err)
	}
	out := make([]assessment.RankOverride, len(rows))
	for i, row := range rows {
		out[i] = rankOverrideFromRow(row)
	}
	return out, nil
}

// DeleteRankOverride removes the override for one assessment, reverting it
// to its calculated rank on the next compile. Deleting a nonexistent
// override is a no-op.
func (s *DoltStore) DeleteRankOverride(ctx context.Context, assessmentID string) error {
	_, err := s.client.RankOverride.Delete().
		Where(entrankoverride.ID(assessmentID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: deleting rank override for %s: %w", assessmentID, err)
	}
	return nil
}

func rankOverrideFromRow(row *ent.RankOverride) assessment.RankOverride {
	return assessment.RankOverride{
		AssessmentID: row.ID,
		FinalRank:    row.FinalRank,
		Rationale:    row.Rationale,
		ApprovedBy:   row.ApprovedBy,
		EvidenceIDs:  row.EvidenceIds,
	}
}
