package store

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/grokify/omniroadmap-core/provider"

	"github.com/grokify/omniroadmap/ent"
	"github.com/grokify/omniroadmap/ent/syncmeta"
)

// UpsertItems inserts or updates canonical items by ID.
func (s *DoltStore) UpsertItems(ctx context.Context, items []provider.Item) error {
	for i := range items {
		if err := s.upsertItem(ctx, &items[i]); err != nil {
			return fmt.Errorf("store: upserting item %s: %w", items[i].ID, err)
		}
	}
	return nil
}

func (s *DoltStore) upsertItem(ctx context.Context, item *provider.Item) error {
	create := s.client.Item.Create().
		SetID(item.ID).
		SetProvider(item.Provider).
		SetSourceID(item.SourceID).
		SetSourceRef(item.SourceRef).
		SetSourceURL(item.SourceURL).
		SetWorkspaceRef(item.WorkspaceRef).
		SetKind(string(item.Kind)).
		SetName(item.Name).
		SetDescription(item.Description).
		SetParentID(item.ParentID).
		SetReleaseID(item.ReleaseID).
		SetNillableStartDate(item.StartDate).
		SetNillableDueDate(item.DueDate).
		SetNillableCreatedAt(item.CreatedAt).
		SetNillableUpdatedAt(item.UpdatedAt).
		SetNillableProgress(item.Progress).
		SetMoscow(item.MoSCoW).
		SetKano(item.Kano).
		SetSyncedAt(time.Now())

	if item.Status != nil {
		create.
			SetStatusID(item.Status.ID).
			SetStatusName(item.Status.Name).
			SetStatusCategory(string(item.Status.Category)).
			SetStatusComplete(item.Status.Complete)
	}
	if item.Owner != nil {
		create.
			SetOwnerID(item.Owner.ID).
			SetOwnerName(item.Owner.Name).
			SetOwnerEmail(item.Owner.Email)
	}
	if item.RICE != nil {
		create.SetRice(riceToMap(item.RICE))
	}
	if len(item.Tags) > 0 {
		create.SetTags(item.Tags)
	}
	if len(item.CustomFields) > 0 {
		create.SetCustomFields(customFieldsToMaps(item.CustomFields))
	}
	if len(item.Metadata) > 0 {
		create.SetMetadata(normalizeMap(item.Metadata))
	}

	return create.OnConflict().UpdateNewValues().Exec(ctx)
}

// UpsertReleases inserts or updates canonical releases by ID.
func (s *DoltStore) UpsertReleases(ctx context.Context, releases []provider.Release) error {
	for i := range releases {
		if err := s.upsertRelease(ctx, &releases[i]); err != nil {
			return fmt.Errorf("store: upserting release %s: %w", releases[i].ID, err)
		}
	}
	return nil
}

func (s *DoltStore) upsertRelease(ctx context.Context, rel *provider.Release) error {
	create := s.client.Release.Create().
		SetID(rel.ID).
		SetProvider(rel.Provider).
		SetSourceID(rel.SourceID).
		SetSourceRef(rel.SourceRef).
		SetSourceURL(rel.SourceURL).
		SetName(rel.Name).
		SetNillableStartDate(rel.StartDate).
		SetNillableReleaseDate(rel.ReleaseDate).
		SetReleased(rel.Released).
		SetNillableProgress(rel.Progress).
		SetSyncedAt(time.Now())

	if rel.Status != nil {
		create.
			SetStatusID(rel.Status.ID).
			SetStatusName(rel.Status.Name).
			SetStatusCategory(string(rel.Status.Category))
	}
	if len(rel.Metadata) > 0 {
		create.SetMetadata(normalizeMap(rel.Metadata))
	}

	return create.OnConflict().UpdateNewValues().Exec(ctx)
}

// UpsertCustomFieldDefinitions inserts or updates custom field definitions
// for a provider.
func (s *DoltStore) UpsertCustomFieldDefinitions(ctx context.Context, providerName string, defs []provider.CustomFieldDefinition) error {
	for _, def := range defs {
		kinds := make([]string, len(def.AppliesToKinds))
		for i, k := range def.AppliesToKinds {
			kinds[i] = string(k)
		}
		create := s.client.CustomFieldDef.Create().
			SetID(providerName + ":" + def.Key).
			SetProvider(providerName).
			SetKey(def.Key).
			SetName(def.Name).
			SetFieldType(def.Type).
			SetSyncedAt(time.Now())
		if len(kinds) > 0 {
			create.SetKinds(kinds)
		}
		if len(def.Metadata) > 0 {
			create.SetMetadata(normalizeMap(def.Metadata))
		}
		if err := create.OnConflict().UpdateNewValues().Exec(ctx); err != nil {
			return fmt.Errorf("store: upserting custom field def %s: %w", def.Key, err)
		}
	}
	return nil
}

// SetSyncMeta records the last sync time and record count for a
// (provider, kind) pair.
func (s *DoltStore) SetSyncMeta(ctx context.Context, providerName, kind string, lastSync time.Time, count int) error {
	err := s.client.SyncMeta.Create().
		SetProvider(providerName).
		SetKind(kind).
		SetLastSync(lastSync).
		SetRecordCount(count).
		OnConflict().
		UpdateNewValues().
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("store: setting sync meta %s/%s: %w", providerName, kind, err)
	}
	return nil
}

// SyncMetaEntry is one (provider, kind) sync record.
type SyncMetaEntry struct {
	Provider    string
	Kind        string
	LastSync    time.Time
	RecordCount int
}

// GetSyncMeta returns all sync metadata entries, ordered by provider then
// kind.
func (s *DoltStore) GetSyncMeta(ctx context.Context) ([]SyncMetaEntry, error) {
	rows, err := s.client.SyncMeta.Query().
		Order(ent.Asc(syncmeta.FieldProvider), ent.Asc(syncmeta.FieldKind)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("store: querying sync meta: %w", err)
	}
	entries := make([]SyncMetaEntry, len(rows))
	for i, r := range rows {
		entries[i] = SyncMetaEntry{
			Provider:    r.Provider,
			Kind:        r.Kind,
			LastSync:    r.LastSync,
			RecordCount: r.RecordCount,
		}
	}
	return entries, nil
}

// riceToMap flattens a provider.RICE into the stored map shape, keeping
// only set components.
func riceToMap(r *provider.RICE) map[string]float64 {
	m := map[string]float64{}
	if r.Reach != nil {
		m["reach"] = *r.Reach
	}
	if r.Impact != nil {
		m["impact"] = *r.Impact
	}
	if r.Confidence != nil {
		m["confidence"] = *r.Confidence
	}
	if r.Effort != nil {
		m["effort"] = *r.Effort
	}
	if r.Score != nil {
		m["score"] = *r.Score
	}
	return m
}

// customFieldsToMaps converts custom field values to the stored JSON array
// shape, normalizing raw-JSON byte values (e.g. aha-go's jx.Raw) into
// native Go values — encoding/json would otherwise base64-encode them.
func customFieldsToMaps(fields []provider.CustomField) []map[string]any {
	out := make([]map[string]any, len(fields))
	for i, f := range fields {
		out[i] = map[string]any{
			"key":   f.Key,
			"name":  f.Name,
			"value": unwrapRawJSON(f.Value),
			"type":  f.Type,
		}
	}
	return out
}

// normalizeMap normalizes raw-JSON byte values inside a metadata map.
func normalizeMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = unwrapRawJSON(v)
	}
	return out
}

// unwrapRawJSON converts byte-slice values holding unparsed JSON (e.g.
// aha-go's jx.Raw, which has no MarshalJSON and would base64-encode) into
// native Go values. Non-byte-slice values pass through unchanged.
func unwrapRawJSON(raw any) any {
	rv := reflect.ValueOf(raw)
	if rv.Kind() != reflect.Slice || rv.Type().Elem().Kind() != reflect.Uint8 {
		return raw
	}
	var v any
	if err := json.Unmarshal(rv.Bytes(), &v); err != nil {
		return raw
	}
	return v
}
