package store

import (
	"context"
	"fmt"
	"time"

	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/omniroadmap-core/provider"

	"github.com/grokify/omniroadmap/augment"
	"github.com/grokify/omniroadmap/ent"
	"github.com/grokify/omniroadmap/ent/itemaugment"
)

// SetItemAugment upserts locally-authored data for one item, replacing any
// existing augment for the same (provider, source_ref) wholesale. Sync
// never touches augments, so they survive provider re-syncs.
func (s *DoltStore) SetItemAugment(ctx context.Context, aug augment.ItemAugment) error {
	if aug.Provider == "" || aug.SourceRef == "" {
		return fmt.Errorf("store: augment requires provider and source_ref: %w",
			omniroadmap.ErrInvalidConfiguration)
	}
	if aug.UpdatedAt.IsZero() {
		aug.UpdatedAt = time.Now()
	}

	rice := map[string]float64{}
	if aug.RICE != nil {
		rice = riceToMap(aug.RICE)
	}
	okrRefs := aug.OKRRefs
	if okrRefs == nil {
		okrRefs = []string{}
	}
	metadata := aug.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	// Every column is set explicitly (empty values included) so the
	// on-conflict update fully replaces the prior augment.
	err := s.client.ItemAugment.Create().
		SetID(aug.Key()).
		SetProvider(aug.Provider).
		SetSourceRef(aug.SourceRef).
		SetMoscow(aug.MoSCoW).
		SetKano(aug.Kano).
		SetRice(rice).
		SetOkrRefs(okrRefs).
		SetNotes(aug.Notes).
		SetMetadata(normalizeMap(metadata)).
		SetUpdatedAt(aug.UpdatedAt).
		OnConflict().
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: upserting augment %s: %w", aug.Key(), err)
	}
	return nil
}

// GetItemAugment returns the augment for (provider, source_ref), or
// omniroadmap.ErrNotFound when none exists.
func (s *DoltStore) GetItemAugment(ctx context.Context, providerName, sourceRef string) (*augment.ItemAugment, error) {
	row, err := s.client.ItemAugment.Query().
		Where(itemaugment.Provider(providerName), itemaugment.SourceRef(sourceRef)).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, fmt.Errorf("store: augment %s:%s: %w", providerName, sourceRef, omniroadmap.ErrNotFound)
	}
	if err != nil {
		return nil, fmt.Errorf("store: querying augment %s:%s: %w", providerName, sourceRef, err)
	}
	aug := augmentFromRow(row)
	return &aug, nil
}

// ListItemAugments returns all augments, optionally filtered to one
// provider (empty providerName = all), ordered by provider then ref.
func (s *DoltStore) ListItemAugments(ctx context.Context, providerName string) ([]augment.ItemAugment, error) {
	q := s.client.ItemAugment.Query()
	if providerName != "" {
		q = q.Where(itemaugment.Provider(providerName))
	}
	rows, err := q.
		Order(ent.Asc(itemaugment.FieldProvider), ent.Asc(itemaugment.FieldSourceRef)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying augments: %w", err)
	}
	augs := make([]augment.ItemAugment, len(rows))
	for i, row := range rows {
		augs[i] = augmentFromRow(row)
	}
	return augs, nil
}

// DeleteItemAugment removes the augment for (provider, source_ref).
// Deleting a nonexistent augment is a no-op.
func (s *DoltStore) DeleteItemAugment(ctx context.Context, providerName, sourceRef string) error {
	_, err := s.client.ItemAugment.Delete().
		Where(itemaugment.Provider(providerName), itemaugment.SourceRef(sourceRef)).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: deleting augment %s:%s: %w", providerName, sourceRef, err)
	}
	return nil
}

// augmentFromRow converts an ent row back to the canonical augment type.
func augmentFromRow(row *ent.ItemAugment) augment.ItemAugment {
	return augment.ItemAugment{
		Provider:  row.Provider,
		SourceRef: row.SourceRef,
		MoSCoW:    row.Moscow,
		Kano:      row.Kano,
		RICE:      riceFromMap(row.Rice),
		OKRRefs:   row.OkrRefs,
		Notes:     row.Notes,
		Metadata:  row.Metadata,
		UpdatedAt: row.UpdatedAt,
	}
}

// riceFromMap is the inverse of riceToMap: nil when no components are set.
func riceFromMap(m map[string]float64) *provider.RICE {
	if len(m) == 0 {
		return nil
	}
	rice := provider.RICE{}
	if v, ok := m["reach"]; ok {
		rice.Reach = &v
	}
	if v, ok := m["impact"]; ok {
		rice.Impact = &v
	}
	if v, ok := m["confidence"]; ok {
		rice.Confidence = &v
	}
	if v, ok := m["effort"]; ok {
		rice.Effort = &v
	}
	if v, ok := m["score"]; ok {
		rice.Score = &v
	}
	return &rice
}
