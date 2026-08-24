package store

import (
	"context"
	"fmt"
	"time"

	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/prism-roadmap/assessment"

	"github.com/grokify/omniroadmap/ent"
	entevidence "github.com/grokify/omniroadmap/ent/evidence"
)

// SaveEvidence upserts an evidence record. Projection columns (system,
// sensitivity, capturedBy, capturedAt, sourceURI, verified) are computed
// from e's canonical fields at save time — every column is set explicitly,
// including empty/zero values, so a re-save fully replaces the prior
// projection (same discipline as SaveOpportunityAssessment).
func (s *DoltStore) SaveEvidence(ctx context.Context, e assessment.Evidence) error {
	if err := e.Validate(); err != nil {
		return fmt.Errorf("store: invalid evidence: %w", err)
	}

	create := s.client.Evidence.Create().
		SetID(e.ID).
		SetSystem(string(e.System)).
		SetSensitivity(string(e.Sensitivity)).
		SetCapturedBy(e.CapturedBy).
		SetSourceURI(e.SourceURI()).
		SetVerified(e.IsVerified()).
		SetCanonical(e)

	if capturedAt := e.CapturedAtTime(); capturedAt != nil {
		create = create.SetCapturedAt(*capturedAt)
	}

	err := create.
		OnConflict().
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: upserting evidence %s: %w", e.ID, err)
	}
	return nil
}

// GetEvidence returns one evidence record by ID, or omniroadmap.ErrNotFound.
func (s *DoltStore) GetEvidence(ctx context.Context, id string) (*assessment.Evidence, error) {
	row, err := s.client.Evidence.Query().
		Where(entevidence.ID(id)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("store: evidence %s: %w", id, omniroadmap.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: querying evidence %s: %w", id, err)
	}
	return &row.Canonical, nil
}

// ListEvidence returns every evidence record, ordered by ID.
func (s *DoltStore) ListEvidence(ctx context.Context) ([]assessment.Evidence, error) {
	rows, err := s.client.Evidence.Query().
		Order(ent.Asc(entevidence.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying evidence: %w", err)
	}
	out := make([]assessment.Evidence, len(rows))
	for i, row := range rows {
		out[i] = row.Canonical
	}
	return out, nil
}

// DeleteEvidence removes one evidence record by ID. Deleting a nonexistent
// record is a no-op.
func (s *DoltStore) DeleteEvidence(ctx context.Context, id string) error {
	_, err := s.client.Evidence.Delete().
		Where(entevidence.ID(id)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: deleting evidence %s: %w", id, err)
	}
	return nil
}

// allEvidenceReferences walks every assessment cycle (not just current
// ones — a superseded cycle still cited real evidence at the time and
// should remain auditable) and extracts every EvidenceRef via
// assessment.OpportunityAssessment.EvidenceReferences.
func (s *DoltStore) allEvidenceReferences(ctx context.Context) ([]assessment.EvidenceRef, error) {
	rows, err := s.client.OpportunityAssessment.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying assessments for evidence references: %w", err)
	}
	var refs []assessment.EvidenceRef
	for _, row := range rows {
		refs = append(refs, row.Canonical.EvidenceReferences()...)
	}
	return refs, nil
}

// EvidenceCitations returns every assessment/question reference citing the
// given evidence ID, across every assessment cycle — the reverse lookup
// prism-roadmap PRD FR5 asks for ("which assessments cite this source").
func (s *DoltStore) EvidenceCitations(ctx context.Context, evidenceID string) ([]assessment.EvidenceRef, error) {
	refs, err := s.allEvidenceReferences(ctx)
	if err != nil {
		return nil, err
	}
	return assessment.NewEvidenceIndex(refs).CitedBy(evidenceID), nil
}

// StaleEvidence returns every evidence record whose capture time has
// exceeded its system's DefaultValidityWindow as of now (prism-roadmap PRD
// FR11).
func (s *DoltStore) StaleEvidence(ctx context.Context, now time.Time) ([]assessment.Evidence, error) {
	all, err := s.ListEvidence(ctx)
	if err != nil {
		return nil, err
	}
	var stale []assessment.Evidence
	for _, e := range all {
		window := assessment.DefaultValidityWindow(e.System)
		if e.IsStale(now, window) {
			stale = append(stale, e)
		}
	}
	return stale, nil
}

// StaleEvidenceCitations finds every stale evidence record and the
// assessment/question references that cite it, keyed by evidence ID — the
// input a staleness sweep needs to visibly degrade those assessments'
// Confidence display rather than silently trusting stale support
// (prism-roadmap PRD FR11).
func (s *DoltStore) StaleEvidenceCitations(ctx context.Context, now time.Time) (map[string][]assessment.EvidenceRef, error) {
	stale, err := s.StaleEvidence(ctx, now)
	if err != nil {
		return nil, err
	}
	if len(stale) == 0 {
		return nil, nil
	}

	refs, err := s.allEvidenceReferences(ctx)
	if err != nil {
		return nil, err
	}
	index := assessment.NewEvidenceIndex(refs)

	out := make(map[string][]assessment.EvidenceRef, len(stale))
	for _, e := range stale {
		if cited := index.CitedBy(e.ID); len(cited) > 0 {
			out[e.ID] = cited
		}
	}
	return out, nil
}
