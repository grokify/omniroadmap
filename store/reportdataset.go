package store

import (
	"context"
	"fmt"

	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/prism-roadmap/assessment"

	"github.com/grokify/omniroadmap/ent"
	entreportdataset "github.com/grokify/omniroadmap/ent/reportdataset"
)

const (
	// ReportDatasetStatusDraft marks a just-compiled dataset awaiting PM
	// review.
	ReportDatasetStatusDraft = "draft"

	// ReportDatasetStatusFinal marks a dataset that has been reviewed and
	// rank-materialized (RMI-OMNIROADMAP-006).
	ReportDatasetStatusFinal = "final"
)

// SaveReportDataset persists one compile run under id and status. Every
// compile produces a NEW id (never reused) — datasets are never mutated in
// place, mirroring OpportunityAssessment's own cycle-history discipline.
func (s *DoltStore) SaveReportDataset(ctx context.Context, id string, dataset assessment.ReportDataset, status string) error {
	if err := dataset.Validate(); err != nil {
		return fmt.Errorf("store: invalid report dataset: %w", err)
	}

	err := s.client.ReportDataset.Create().
		SetID(id).
		SetGeneratedAt(dataset.GeneratedAt).
		SetRankingPolicyID(dataset.RankingPolicyID).
		SetRankingPolicyVersion(dataset.RankingPolicyVersion).
		SetStatus(status).
		SetCanonical(dataset).
		OnConflict().
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: upserting report dataset %s: %w", id, err)
	}
	return nil
}

// GetReportDataset returns one compile run by ID, or omniroadmap.ErrNotFound.
func (s *DoltStore) GetReportDataset(ctx context.Context, id string) (*assessment.ReportDataset, error) {
	row, err := s.client.ReportDataset.Query().
		Where(entreportdataset.ID(id)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("store: report dataset %s: %w", id, omniroadmap.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: querying report dataset %s: %w", id, err)
	}
	return &row.Canonical, nil
}

// GetLatestReportDataset returns the most recently generated dataset (by
// GeneratedAt), or (nil, nil) if none has ever been compiled — a deliberate
// non-error "nothing yet" signal, since a first compile has nothing to
// diff deltas against and that's an entirely normal state, not a fault.
func (s *DoltStore) GetLatestReportDataset(ctx context.Context) (*assessment.ReportDataset, error) {
	row, err := s.client.ReportDataset.Query().
		Order(ent.Desc(entreportdataset.FieldGeneratedAt)).
		First(ctx)
	if ent.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: querying latest report dataset: %w", err)
	}
	return &row.Canonical, nil
}

// ListReportDatasets returns every compile run, newest first.
func (s *DoltStore) ListReportDatasets(ctx context.Context) ([]assessment.ReportDataset, error) {
	rows, err := s.client.ReportDataset.Query().
		Order(ent.Desc(entreportdataset.FieldGeneratedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying report datasets: %w", err)
	}
	out := make([]assessment.ReportDataset, len(rows))
	for i, row := range rows {
		out[i] = row.Canonical
	}
	return out, nil
}

// SetReportDatasetStatus updates one compile run's review status (e.g.
// draft -> final once PM review and rank materialization complete).
func (s *DoltStore) SetReportDatasetStatus(ctx context.Context, id, status string) error {
	n, err := s.client.ReportDataset.Update().
		Where(entreportdataset.ID(id)).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("store: updating report dataset %s status: %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("store: report dataset %s: %w", id, omniroadmap.ErrNotFound)
	}
	return nil
}
