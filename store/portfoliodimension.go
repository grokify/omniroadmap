package store

import (
	"context"
	"fmt"

	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/prism-roadmap/assessment"

	"github.com/grokify/omniroadmap/ent"
	entdimensionoption "github.com/grokify/omniroadmap/ent/dimensionoption"
	entportfoliodimension "github.com/grokify/omniroadmap/ent/portfoliodimension"
)

// dimensionRowID and dimensionOptionRowID are the composite string keys
// used across this file — "<dimensionId>:<version>" and
// "<dimensionRowID>:<optionId>" respectively.
func dimensionRowID(dimensionID, version string) string {
	return dimensionID + ":" + version
}

func dimensionOptionRowID(dimensionRowID, optionID string) string {
	return dimensionRowID + ":" + optionID
}

// RegisterDimension upserts a dimension definition and its options.
// Registering (or updating) a custom portfolio dimension is exactly this
// call — an INSERT into two tables, never a schema migration
// (prism-roadmap PRD FR4).
func (s *DoltStore) RegisterDimension(ctx context.Context, def assessment.DimensionDefinition, builtIn bool) error {
	if err := def.Validate(); err != nil {
		return fmt.Errorf("store: invalid dimension definition: %w", err)
	}

	rowID := dimensionRowID(def.ID, def.Version)

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("store: starting transaction: %w", err)
	}

	err = tx.PortfolioDimension.Create().
		SetID(rowID).
		SetDimensionID(def.ID).
		SetVersion(def.Version).
		SetName(def.Name).
		SetKind(string(def.Kind)).
		SetBuiltIn(builtIn).
		OnConflict().
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("store: upserting dimension %s: %w", rowID, err)
	}

	for i, opt := range def.Options {
		err = tx.DimensionOption.Create().
			SetID(dimensionOptionRowID(rowID, opt.ID)).
			SetDimensionRowID(rowID).
			SetOptionID(opt.ID).
			SetLabel(opt.Label).
			SetQuestions(opt.Questions).
			SetSequence(i).
			OnConflict().
			UpdateNewValues().
			Exec(ctx)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("store: upserting dimension option %s: %w", opt.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: committing dimension %s: %w", rowID, err)
	}
	return nil
}

// GetDimension returns a dimension definition, with its options in
// definition order, by ID+version — or omniroadmap.ErrNotFound.
func (s *DoltStore) GetDimension(ctx context.Context, dimensionID, version string) (*assessment.DimensionDefinition, error) {
	rowID := dimensionRowID(dimensionID, version)

	row, err := s.client.PortfolioDimension.Query().
		Where(entportfoliodimension.ID(rowID)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("store: dimension %s: %w", rowID, omniroadmap.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: querying dimension %s: %w", rowID, err)
	}

	options, err := s.dimensionOptions(ctx, rowID)
	if err != nil {
		return nil, err
	}

	return &assessment.DimensionDefinition{
		ID:      row.DimensionID,
		Name:    row.Name,
		Version: row.Version,
		Kind:    assessment.DimensionKind(row.Kind),
		Options: options,
	}, nil
}

// dimensionOptions returns a dimension row's options, in definition order.
func (s *DoltStore) dimensionOptions(ctx context.Context, rowID string) ([]assessment.DimensionOption, error) {
	rows, err := s.client.DimensionOption.Query().
		Where(entdimensionoption.DimensionRowID(rowID)).
		Order(ent.Asc(entdimensionoption.FieldSequence)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying options for dimension %s: %w", rowID, err)
	}
	options := make([]assessment.DimensionOption, len(rows))
	for i, opt := range rows {
		options[i] = assessment.DimensionOption{
			ID:        opt.OptionID,
			Label:     opt.Label,
			Questions: opt.Questions,
		}
	}
	return options, nil
}

// ListDimensions returns every registered dimension definition (every
// version of every dimension ID), ordered by dimension ID then version.
func (s *DoltStore) ListDimensions(ctx context.Context) ([]assessment.DimensionDefinition, error) {
	rows, err := s.client.PortfolioDimension.Query().
		Order(ent.Asc(entportfoliodimension.FieldDimensionID), ent.Asc(entportfoliodimension.FieldVersion)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying dimensions: %w", err)
	}
	defs := make([]assessment.DimensionDefinition, len(rows))
	for i, row := range rows {
		options, err := s.dimensionOptions(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		defs[i] = assessment.DimensionDefinition{
			ID:      row.DimensionID,
			Name:    row.Name,
			Version: row.Version,
			Kind:    assessment.DimensionKind(row.Kind),
			Options: options,
		}
	}
	return defs, nil
}

// ListCustomDimensions returns only non-built-in dimension definitions —
// the ones an organization registered itself.
func (s *DoltStore) ListCustomDimensions(ctx context.Context) ([]assessment.DimensionDefinition, error) {
	rows, err := s.client.PortfolioDimension.Query().
		Where(entportfoliodimension.BuiltIn(false)).
		Order(ent.Asc(entportfoliodimension.FieldDimensionID), ent.Asc(entportfoliodimension.FieldVersion)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying custom dimensions: %w", err)
	}
	defs := make([]assessment.DimensionDefinition, len(rows))
	for i, row := range rows {
		options, err := s.dimensionOptions(ctx, row.ID)
		if err != nil {
			return nil, err
		}
		defs[i] = assessment.DimensionDefinition{
			ID:      row.DimensionID,
			Name:    row.Name,
			Version: row.Version,
			Kind:    assessment.DimensionKind(row.Kind),
			Options: options,
		}
	}
	return defs, nil
}

// EnsureBuiltinDimensions registers Kano and Market Investment Horizon
// (github.com/grokify/prism-roadmap/assessment.KanoDimension,
// MarketInvestmentHorizonDimension) if not already present. Idempotent —
// safe to call on every startup. Deliberately not wired into Migrate: that
// method's job is schema migration, not seed data, and conflating the two
// would make a seed-data change look like a schema change in its diff.
func (s *DoltStore) EnsureBuiltinDimensions(ctx context.Context) error {
	if err := s.RegisterDimension(ctx, *assessment.KanoDimension(), true); err != nil {
		return fmt.Errorf("store: registering built-in kano dimension: %w", err)
	}
	if err := s.RegisterDimension(ctx, *assessment.MarketInvestmentHorizonDimension(), true); err != nil {
		return fmt.Errorf("store: registering built-in market-investment-horizon dimension: %w", err)
	}
	return nil
}
