package store

import (
	"context"
	"fmt"

	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/prism-roadmap/canvas"

	"github.com/grokify/omniroadmap/ent"
	entopportunityspec "github.com/grokify/omniroadmap/ent/opportunityspec"
)

// SaveOpportunitySpec upserts spec by its Metadata.ID. Every column is set
// explicitly, including empty/zero values, so a re-save fully replaces the
// prior record rather than leaving stale values behind (same discipline as
// SaveOpportunityAssessment/SaveEvidence).
func (s *DoltStore) SaveOpportunitySpec(ctx context.Context, spec canvas.OpportunitySpec) error {
	if spec.Metadata.ID == "" {
		return fmt.Errorf("store: opportunity spec: metadata.id is required")
	}

	err := s.client.OpportunitySpec.Create().
		SetID(spec.Metadata.ID).
		SetTitle(spec.Metadata.Title).
		SetCanonical(spec).
		OnConflictColumns(entopportunityspec.FieldID).
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: upserting opportunity spec %s: %w", spec.Metadata.ID, err)
	}
	return nil
}

// GetOpportunitySpec returns one spec by ID, or omniroadmap.ErrNotFound when
// none exists.
func (s *DoltStore) GetOpportunitySpec(ctx context.Context, id string) (*canvas.OpportunitySpec, error) {
	row, err := s.client.OpportunitySpec.Query().
		Where(entopportunityspec.ID(id)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("store: opportunity spec %s: %w", id, omniroadmap.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: querying opportunity spec %s: %w", id, err)
	}
	return &row.Canonical, nil
}
