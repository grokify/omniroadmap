// Package analyticscatalog exposes OmniRoadmap's queryable entities and fields
// in UIForge's neutral analytics catalog shape.
package analyticscatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/grokify/omniroadmap-core/provider"
	"github.com/grokify/omniroadmap/store"
	"github.com/plexusone/dashforge/dashboardir"
)

// BuildFromStore reads the OmniRoadmap store and returns a UIForge analytics
// catalog. This is the main integration point for a generic UIForge server
// pointed at an OmniRoadmap database.
func BuildFromStore(ctx context.Context, s *store.DoltStore) (dashboardir.AnalyticsCatalog, error) {
	items, err := s.ListItems(ctx, store.ItemFilter{})
	if err != nil {
		return dashboardir.AnalyticsCatalog{}, err
	}
	return Build(items), nil
}

// Build creates a UIForge analytics catalog from canonical OmniRoadmap items.
func Build(items []provider.Item) dashboardir.AnalyticsCatalog {
	return dashboardir.AnalyticsCatalog{
		ID:          "omniroadmap",
		Name:        "OmniRoadmap",
		Description: "Roadmap, product-management, prioritization, and custom-field analytics.",
		Sources: []dashboardir.AnalyticsSource{
			{
				ID:          "omniroadmap",
				Name:        "OmniRoadmap",
				Type:        dashboardir.AnalyticsSourceTypeApplication,
				Description: "Canonical OmniRoadmap store with provider, standard, derived, and custom fields.",
				Datasets: []dashboardir.AnalyticsDataset{
					datasetFor(items, "items", "Items", ""),
					datasetFor(items, "initiatives", "Initiatives", string(provider.ItemKindInitiative)),
					datasetFor(items, "features", "Features", string(provider.ItemKindFeature)),
					datasetFor(items, "epics", "Epics", string(provider.ItemKindEpic)),
				},
			},
		},
	}
}

func datasetFor(items []provider.Item, id, name, kind string) dashboardir.AnalyticsDataset {
	filtered := make([]provider.Item, 0, len(items))
	for _, item := range items {
		if kind == "" || string(item.Kind) == kind {
			filtered = append(filtered, item)
		}
	}
	return dashboardir.AnalyticsDataset{
		ID:        id,
		Name:      name,
		QueryName: id,
		Fields:    fieldsFor(filtered),
	}
}

func fieldsFor(items []provider.Item) []dashboardir.AnalyticsField {
	fields := standardFields(len(items))
	custom := newCustomStats(len(items))
	for i := range items {
		custom.add(&items[i])
	}
	fields = append(fields, custom.fields()...)
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].Source != fields[j].Source {
			return fields[i].Source < fields[j].Source
		}
		return fields[i].QueryName < fields[j].QueryName
	})
	return fields
}

func standardFields(total int) []dashboardir.AnalyticsField {
	defs := []struct {
		queryName, name, typ, source, role string
		nullable                           bool
	}{
		{"id", "ID", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, false},
		{"provider", "Provider", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, false},
		{"source_id", "Source ID", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, false},
		{"source_ref", "Source Ref", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, true},
		{"source_url", "Source URL", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleLink, true},
		{"workspace_ref", "Workspace Ref", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, true},
		{"kind", "Kind", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, false},
		{"name", "Name", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, true},
		{"status", "Status", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, true},
		{"status_category", "Status Category", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, true},
		{"owner", "Owner", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, true},
		{"release_id", "Release ID", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, true},
		{"moscow", "MoSCoW", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, true},
		{"kano", "Kano", dashboardir.AnalyticsFieldTypeString, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleDimension, true},
		{"progress", "Progress", dashboardir.AnalyticsFieldTypeNumber, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleMeasure, true},
		{"due_date", "Due Date", dashboardir.AnalyticsFieldTypeDate, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleTime, true},
		{"updated_at", "Updated At", dashboardir.AnalyticsFieldTypeDate, dashboardir.AnalyticsFieldSourceStandard, dashboardir.AnalyticsFieldRoleTime, true},
		{"moscow_rank", "MoSCoW Rank", dashboardir.AnalyticsFieldTypeNumber, dashboardir.AnalyticsFieldSourceDerived, dashboardir.AnalyticsFieldRoleMeasure, false},
		{"rice_score", "RICE Score", dashboardir.AnalyticsFieldTypeNumber, dashboardir.AnalyticsFieldSourceDerived, dashboardir.AnalyticsFieldRoleMeasure, true},
	}
	out := make([]dashboardir.AnalyticsField, 0, len(defs))
	for _, def := range defs {
		out = append(out, dashboardir.AnalyticsField{
			ID:         def.queryName,
			Name:       def.name,
			QueryName:  def.queryName,
			Type:       def.typ,
			Source:     def.source,
			Role:       def.role,
			Selectable: true,
			Filterable: true,
			Sortable:   true,
			Nullable:   def.nullable,
			Count:      total,
			Coverage:   1,
		})
	}
	return out
}

type customStats struct {
	total  int
	byName map[string]*dashboardir.AnalyticsField
	values map[string]map[string]struct{}
}

func newCustomStats(total int) *customStats {
	return &customStats{
		total:  total,
		byName: map[string]*dashboardir.AnalyticsField{},
		values: map[string]map[string]struct{}{},
	}
}

func (s *customStats) add(item *provider.Item) {
	seen := map[string]struct{}{}
	for _, field := range item.CustomFields {
		queryName := customQueryName(field.Key)
		if queryName == "" {
			continue
		}
		stat := s.byName[queryName]
		if stat == nil {
			stat = &dashboardir.AnalyticsField{
				ID:         queryName,
				Name:       firstNonEmpty(field.Name, field.Key),
				QueryName:  queryName,
				Type:       firstNonEmpty(field.Type, inferType(field.Value)),
				Source:     dashboardir.AnalyticsFieldSourceCustom,
				Role:       dashboardir.AnalyticsFieldRoleDimension,
				Selectable: true,
				Filterable: true,
				Sortable:   true,
				Nullable:   true,
			}
			s.byName[queryName] = stat
			s.values[queryName] = map[string]struct{}{}
		}
		if stat.Name == "" {
			stat.Name = firstNonEmpty(field.Name, field.Key)
		}
		if stat.Type == "" {
			stat.Type = firstNonEmpty(field.Type, inferType(field.Value))
		}
		if _, ok := seen[queryName]; !ok {
			stat.Count++
			seen[queryName] = struct{}{}
		}
		if value := sampleString(field.Value); value != "" && len(s.values[queryName]) < 8 {
			s.values[queryName][value] = struct{}{}
		}
	}
}

func (s *customStats) fields() []dashboardir.AnalyticsField {
	out := make([]dashboardir.AnalyticsField, 0, len(s.byName))
	for queryName, stat := range s.byName {
		next := *stat
		if s.total > 0 {
			next.Coverage = float64(next.Count) / float64(s.total)
		}
		for value := range s.values[queryName] {
			next.SampleValues = append(next.SampleValues, value)
		}
		sort.Strings(next.SampleValues)
		out = append(out, next)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].QueryName < out[j].QueryName
	})
	return out
}

func customQueryName(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range strings.ToLower(key) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '_' || r == '-' || r == ' ' || r == '.':
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return ""
	}
	return "custom." + out
}

func inferType(value any) string {
	value = normalizeValue(value)
	switch value.(type) {
	case float64, float32, int, int64, json.Number:
		return dashboardir.AnalyticsFieldTypeNumber
	case bool:
		return dashboardir.AnalyticsFieldTypeBool
	case []any, map[string]any:
		return dashboardir.AnalyticsFieldTypeJSON
	default:
		return dashboardir.AnalyticsFieldTypeString
	}
}

func normalizeValue(value any) any {
	if raw, ok := value.([]byte); ok {
		var decoded any
		if err := json.Unmarshal(raw, &decoded); err == nil {
			return decoded
		}
		return string(raw)
	}
	return value
}

func sampleString(value any) string {
	value = normalizeValue(value)
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return strings.TrimSpace(fmt.Sprint(v))
		}
		return strings.TrimSpace(string(data))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
