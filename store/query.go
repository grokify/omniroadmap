package store

import (
	"context"
	"fmt"

	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/omniroadmap-core/provider"

	"github.com/grokify/omniroadmap/augment"
	"github.com/grokify/omniroadmap/ent"
	"github.com/grokify/omniroadmap/ent/item"
)

// ItemFilter narrows a ListItems call. Zero value = every item.
type ItemFilter struct {
	Provider string
	Kinds    []provider.ItemKind

	// WithoutAugments returns raw synced provider data, skipping the
	// augment overlay.
	WithoutAugments bool
}

// ListItems reads canonical items back from the store. By default,
// locally-authored augments (MoSCoW, Kano, RICE, OKR refs, notes) are
// overlaid, winning over what sync stored.
func (s *DoltStore) ListItems(ctx context.Context, f ItemFilter) ([]provider.Item, error) {
	q := s.client.Item.Query()
	if f.Provider != "" {
		q = q.Where(item.Provider(f.Provider))
	}
	if len(f.Kinds) > 0 {
		kinds := make([]string, len(f.Kinds))
		for i, k := range f.Kinds {
			kinds[i] = string(k)
		}
		q = q.Where(item.KindIn(kinds...))
	}
	rows, err := q.
		Order(ent.Asc(item.FieldProvider), ent.Asc(item.FieldKind), ent.Asc(item.FieldSourceRef)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying items: %w", err)
	}

	items := make([]provider.Item, len(rows))
	for i, row := range rows {
		items[i] = itemFromRow(row)
	}

	if !f.WithoutAugments && len(items) > 0 {
		augs, err := s.ListItemAugments(ctx, f.Provider)
		if err != nil {
			return nil, err
		}
		augment.Apply(items, augs)
	}
	return items, nil
}

// GetItem reads one item by its source reference (e.g. "MYPROJ-123"),
// source ID, or canonical ID, with augments applied. Returns
// omniroadmap.ErrNotFound when no item matches.
func (s *DoltStore) GetItem(ctx context.Context, providerName, ref string) (*provider.Item, error) {
	q := s.client.Item.Query().
		Where(item.Or(item.SourceRef(ref), item.SourceID(ref), item.ID(ref)))
	if providerName != "" {
		q = q.Where(item.Provider(providerName))
	}
	rows, err := q.Limit(2).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying item %s: %w", ref, err)
	}
	switch len(rows) {
	case 0:
		return nil, fmt.Errorf("store: item %s: %w", ref, omniroadmap.ErrNotFound)
	case 1:
		// found
	default:
		return nil, fmt.Errorf("store: item ref %q is ambiguous across providers; pass a provider name", ref)
	}

	converted := itemFromRow(rows[0])
	aug, err := s.GetItemAugment(ctx, converted.Provider, augmentRefForItem(&converted))
	switch {
	case err == nil:
		items := []provider.Item{converted}
		augment.Apply(items, []augment.ItemAugment{*aug})
		converted = items[0]
	case !omniroadmap.IsNotFound(err):
		return nil, err
	}
	return &converted, nil
}

// augmentRefForItem returns the key augments use for an item: the source
// ref when present, else the source ID.
func augmentRefForItem(it *provider.Item) string {
	if it.SourceRef != "" {
		return it.SourceRef
	}
	return it.SourceID
}

// itemFromRow converts an ent row back to the canonical item (the inverse
// of upsertItem).
func itemFromRow(row *ent.Item) provider.Item {
	it := provider.Item{
		ID:           row.ID,
		Provider:     row.Provider,
		SourceID:     row.SourceID,
		SourceRef:    row.SourceRef,
		SourceURL:    row.SourceURL,
		WorkspaceRef: row.WorkspaceRef,

		Kind:        provider.ItemKind(row.Kind),
		Name:        row.Name,
		Description: row.Description,

		Progress:  row.Progress,
		ParentID:  row.ParentID,
		ReleaseID: row.ReleaseID,

		StartDate: row.StartDate,
		DueDate:   row.DueDate,
		CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,

		MoSCoW: row.Moscow,
		Kano:   row.Kano,
		RICE:   riceFromMap(row.Rice),

		Tags:     row.Tags,
		Metadata: row.Metadata,
	}

	if row.StatusID != "" || row.StatusName != "" || row.StatusCategory != "" || row.StatusComplete {
		it.Status = &provider.Status{
			ID:       row.StatusID,
			Name:     row.StatusName,
			Category: provider.StatusCategory(row.StatusCategory),
			Complete: row.StatusComplete,
		}
	}
	if row.OwnerID != "" || row.OwnerName != "" || row.OwnerEmail != "" {
		it.Owner = &provider.Person{
			ID:    row.OwnerID,
			Name:  row.OwnerName,
			Email: row.OwnerEmail,
		}
	}
	if len(row.CustomFields) > 0 {
		fields := make([]provider.CustomField, len(row.CustomFields))
		for i, f := range row.CustomFields {
			fields[i] = provider.CustomField{
				Key:   stringAt(f, "key"),
				Name:  stringAt(f, "name"),
				Value: f["value"],
				Type:  stringAt(f, "type"),
			}
		}
		it.CustomFields = fields
	}
	return it
}

// stringAt reads a string value from a JSON-decoded map, "" when absent or
// non-string.
func stringAt(m map[string]any, key string) string {
	s, ok := m[key].(string)
	if !ok {
		return ""
	}
	return s
}
