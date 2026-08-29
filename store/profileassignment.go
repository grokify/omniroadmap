package store

import (
	"context"
	"fmt"

	"github.com/ProductBuildersHQ/compass-rice/rice"
	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/prism-roadmap/assessment"

	"github.com/grokify/omniroadmap/ent"
	entprofileassignment "github.com/grokify/omniroadmap/ent/profileassignment"
)

// SaveProfileAssignment upserts the current profile assignment for one
// opportunity spec. Call a.Validate() (required fields) before calling this
// — this store does not re-validate, matching this repo's existing division
// of labor between domain-type validation and store persistence. A re-save
// (e.g. compassbridge.ConfirmProfile's result) fully replaces the prior row.
func (s *DoltStore) SaveProfileAssignment(ctx context.Context, a assessment.ProfileAssignment) error {
	if err := a.Validate(); err != nil {
		return fmt.Errorf("store: invalid profile assignment: %w", err)
	}

	secondary := make([]string, len(a.Secondary))
	for i, p := range a.Secondary {
		secondary[i] = string(p)
	}
	evidenceIDs := a.EvidenceIDs
	if evidenceIDs == nil {
		evidenceIDs = []string{}
	}

	create := s.client.ProfileAssignment.Create().
		SetID(a.SpecID).
		SetProfileID(string(a.ProfileID)).
		SetSecondary(secondary).
		SetRationale(a.Rationale).
		SetProposedBy(a.ProposedBy).
		SetStatus(string(a.Status)).
		SetConfirmedBy(a.ConfirmedBy).
		SetEvidenceIds(evidenceIDs)

	if !a.ConfirmedAt.IsZero() {
		create = create.SetConfirmedAt(a.ConfirmedAt)
	}

	err := create.
		OnConflict().
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: upserting profile assignment for %s: %w", a.SpecID, err)
	}
	return nil
}

// GetProfileAssignment returns the current profile assignment for one
// opportunity spec, or omniroadmap.ErrNotFound if none exists.
func (s *DoltStore) GetProfileAssignment(ctx context.Context, specID string) (*assessment.ProfileAssignment, error) {
	row, err := s.client.ProfileAssignment.Query().
		Where(entprofileassignment.ID(specID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("store: profile assignment for %s: %w", specID, omniroadmap.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: querying profile assignment for %s: %w", specID, err)
	}
	a := profileAssignmentFromRow(row)
	return &a, nil
}

// ListProfileAssignments returns every current profile assignment, ordered
// by opportunity spec ID — the input omniroadmap's compile gating needs to
// check whether an opportunity's Compass.ProfileID has been PM-confirmed.
func (s *DoltStore) ListProfileAssignments(ctx context.Context) ([]assessment.ProfileAssignment, error) {
	rows, err := s.client.ProfileAssignment.Query().
		Order(ent.Asc(entprofileassignment.FieldID)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying profile assignments: %w", err)
	}
	out := make([]assessment.ProfileAssignment, len(rows))
	for i, row := range rows {
		out[i] = profileAssignmentFromRow(row)
	}
	return out, nil
}

// DeleteProfileAssignment removes the profile assignment for one opportunity
// spec. Deleting a nonexistent assignment is a no-op.
func (s *DoltStore) DeleteProfileAssignment(ctx context.Context, specID string) error {
	_, err := s.client.ProfileAssignment.Delete().
		Where(entprofileassignment.ID(specID)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: deleting profile assignment for %s: %w", specID, err)
	}
	return nil
}

func profileAssignmentFromRow(row *ent.ProfileAssignment) assessment.ProfileAssignment {
	secondary := make([]rice.Profile, len(row.Secondary))
	for i, s := range row.Secondary {
		secondary[i] = rice.Profile(s)
	}

	a := assessment.ProfileAssignment{
		SpecID:      row.ID,
		ProfileID:   rice.ProfileID(row.ProfileID),
		Secondary:   secondary,
		Rationale:   row.Rationale,
		ProposedBy:  row.ProposedBy,
		Status:      assessment.ProfileAssignmentStatus(row.Status),
		ConfirmedBy: row.ConfirmedBy,
		EvidenceIDs: row.EvidenceIds,
	}
	if row.ConfirmedAt != nil {
		a.ConfirmedAt = *row.ConfirmedAt
	}
	return a
}
