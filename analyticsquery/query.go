// Package analyticsquery executes read-only GuardSQL analytics queries over
// OmniRoadmap's canonical item model.
package analyticsquery

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/grokify/guardsql"
	"github.com/grokify/omniroadmap-core/provider"
	"github.com/grokify/omniroadmap/store"
	"github.com/plexusone/dashforge/dashboardir"
)

// Execute reads canonical items from the store and executes a read-only
// GuardSQL query against them.
func Execute(ctx context.Context, s *store.DoltStore, req dashboardir.AnalyticsQueryRequest) (dashboardir.AnalyticsQueryResult, error) {
	items, err := s.ListItems(ctx, store.ItemFilter{})
	if err != nil {
		return dashboardir.AnalyticsQueryResult{}, err
	}
	return ExecuteItems(items, req)
}

// ExecuteItems executes a read-only GuardSQL query against canonical items.
func ExecuteItems(items []provider.Item, req dashboardir.AnalyticsQueryRequest) (dashboardir.AnalyticsQueryResult, error) {
	input := strings.TrimSpace(req.Query)
	if input == "" {
		return dashboardir.AnalyticsQueryResult{}, fmt.Errorf("query is required")
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 500
	}
	start := time.Now()
	schema := querySchema(items, limit)
	q, issues := guardsql.Lint(input, guardsql.LintConfig{
		Schema:       schema,
		AllowedOps:   []guardsql.Operation{guardsql.OperationRead},
		MaxDepth:     8,
		MaxNodes:     80,
		MaxInValues:  100,
		RequireLimit: false,
	})
	if len(issues) > 0 {
		return dashboardir.AnalyticsQueryResult{}, fmt.Errorf("invalid query: %s", issues[0].Message)
	}
	evalQuery := *q
	if len(q.Prioritize) > 0 {
		evalQuery.OrderBy = nil
		evalQuery.Limit = -1
	}
	entityFilter, err := entityKind(q.From)
	if err != nil {
		return dashboardir.AnalyticsQueryResult{}, err
	}
	rows := make([]guardsql.Row, 0, len(items))
	for i := range items {
		if entityFilter != "" && string(items[i].Kind) != entityFilter {
			continue
		}
		rows = append(rows, queryRow(&items[i]))
	}
	filtered, err := guardsql.Eval(&evalQuery, rows)
	if err != nil {
		return dashboardir.AnalyticsQueryResult{}, err
	}
	if len(q.Prioritize) > 0 {
		filtered, err = applyPrioritization(filtered, q.Prioritize)
		if err != nil {
			return dashboardir.AnalyticsQueryResult{}, err
		}
		if q.Limit >= 0 && len(filtered) > q.Limit {
			filtered = filtered[:q.Limit]
		}
	}
	columns := resultColumns(q, filtered)
	outRows := make([]map[string]any, 0, len(filtered))
	for _, row := range filtered {
		next := make(map[string]any, len(row))
		for _, col := range columns {
			next[col.Name] = row[col.Name]
		}
		outRows = append(outRows, next)
	}
	return dashboardir.AnalyticsQueryResult{
		Columns:       columns,
		Rows:          outRows,
		RowCount:      len(outRows),
		ExecutionTime: time.Since(start).Milliseconds(),
	}, nil
}

func resultColumns(q *guardsql.Query, rows []guardsql.Row) []dashboardir.AnalyticsQueryColumn {
	if q != nil && len(q.Select) > 0 && !(len(q.Select) == 1 && q.Select[0].Star) {
		out := make([]dashboardir.AnalyticsQueryColumn, 0, len(q.Select))
		for _, item := range q.Select {
			if item.Star {
				continue
			}
			name := guardsql.OutputName(item)
			out = append(out, dashboardir.AnalyticsQueryColumn{Name: name})
		}
		return out
	}
	names := map[string]struct{}{}
	for _, row := range rows {
		for name := range row {
			names[name] = struct{}{}
		}
	}
	out := make([]dashboardir.AnalyticsQueryColumn, 0, len(names))
	for name := range names {
		out = append(out, dashboardir.AnalyticsQueryColumn{Name: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func entityKind(entity string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(entity)) {
	case "items":
		return "", nil
	case "initiatives":
		return "initiative", nil
	case "features":
		return "feature", nil
	case "epics":
		return "epic", nil
	default:
		return "", fmt.Errorf("unknown entity %q", entity)
	}
}

func querySchema(items []provider.Item, maxLimit int) guardsql.Schema {
	fields := map[string]guardsql.Field{}
	add := func(name string, typ guardsql.FieldType, sortable bool) {
		fields[name] = guardsql.Field{Name: name, Type: typ, Selectable: true, Filterable: true, Sortable: sortable}
	}
	add("id", guardsql.FieldString, true)
	add("provider", guardsql.FieldString, true)
	add("source_id", guardsql.FieldString, true)
	add("source_ref", guardsql.FieldString, true)
	add("source_url", guardsql.FieldString, true)
	add("workspace_ref", guardsql.FieldString, true)
	add("kind", guardsql.FieldString, true)
	add("name", guardsql.FieldString, true)
	add("status", guardsql.FieldString, true)
	add("status_category", guardsql.FieldString, true)
	add("owner", guardsql.FieldString, true)
	add("release_id", guardsql.FieldString, true)
	add("moscow", guardsql.FieldString, true)
	add("moscow_rank", guardsql.FieldNumber, true)
	add("kano", guardsql.FieldString, true)
	add("progress", guardsql.FieldNumber, true)
	add("rice_score", guardsql.FieldNumber, true)
	add("due_date", guardsql.FieldString, true)
	add("updated_at", guardsql.FieldString, true)
	for _, it := range items {
		for _, field := range it.CustomFields {
			queryField := customQueryField(field.Key)
			if queryField == "" {
				continue
			}
			fields[queryField] = guardsql.Field{Name: queryField, Type: inferFieldType(normalizeCustomValue(field.Value)), Selectable: true, Filterable: true, Sortable: true}
		}
	}
	return guardsql.Schema{
		MaxLimit: maxLimit,
		Entities: map[string]guardsql.Entity{
			"items":       {Name: "items", Fields: fields},
			"initiatives": {Name: "initiatives", Fields: fields},
			"features":    {Name: "features", Fields: fields},
			"epics":       {Name: "epics", Fields: fields},
		},
	}
}

func queryRow(it *provider.Item) guardsql.Row {
	row := guardsql.Row{
		"id":            it.ID,
		"provider":      it.Provider,
		"source_id":     it.SourceID,
		"source_ref":    it.SourceRef,
		"source_url":    it.SourceURL,
		"workspace_ref": it.WorkspaceRef,
		"kind":          string(it.Kind),
		"name":          it.Name,
		"release_id":    it.ReleaseID,
		"moscow":        it.MoSCoW,
		"moscow_rank":   moscowRankValue(it.MoSCoW),
		"kano":          it.Kano,
		"progress":      numericOrNil(it.Progress),
		"rice_score":    riceScore(it.RICE),
		"due_date":      timeString(it.DueDate),
		"updated_at":    timeString(it.UpdatedAt),
	}
	if it.Status != nil {
		row["status"] = it.Status.Name
		row["status_category"] = string(it.Status.Category)
	}
	if it.Owner != nil {
		row["owner"] = firstNonEmpty(it.Owner.Name, it.Owner.Email)
	}
	for _, field := range it.CustomFields {
		queryField := customQueryField(field.Key)
		if queryField != "" {
			row[queryField] = queryValue(normalizeCustomValue(field.Value))
		}
	}
	return row
}

func applyPrioritization(rows []guardsql.Row, strategies []string) ([]guardsql.Row, error) {
	for _, strategy := range strategies {
		switch strategy {
		case "moscow_rice":
			rows = excludeMoscowWontHave(rows)
			sort.SliceStable(rows, func(i, j int) bool {
				ir := moscowRankValue(rows[i]["moscow"])
				jr := moscowRankValue(rows[j]["moscow"])
				if ir != jr {
					return ir < jr
				}
				irice, _ := asFloat64(rows[i]["rice_score"])
				jrice, _ := asFloat64(rows[j]["rice_score"])
				if irice != jrice {
					return irice > jrice
				}
				return fmt.Sprint(rows[i]["updated_at"]) > fmt.Sprint(rows[j]["updated_at"])
			})
		default:
			return nil, fmt.Errorf("unknown prioritization strategy %q", strategy)
		}
	}
	return rows, nil
}

func excludeMoscowWontHave(rows []guardsql.Row) []guardsql.Row {
	out := rows[:0]
	for _, row := range rows {
		if moscowRankValue(row["moscow"]) == 99 {
			continue
		}
		out = append(out, row)
	}
	return out
}

func moscowRankValue(value any) float64 {
	switch strings.ToLower(strings.TrimSpace(fmt.Sprint(value))) {
	case "must_have", "must have", "must-have", "must":
		return 1
	case "should_have", "should have", "should-have", "should":
		return 2
	case "could_have", "could have", "could-have", "could":
		return 3
	case "wont_have", "won't_have", "wont have", "won't have", "wont-have", "will_not_have", "wont":
		return 99
	default:
		return 50
	}
}

func asFloat64(value any) (float64, bool) {
	switch v := value.(type) {
	case nil:
		return 0, false
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func numericOrNil(v *float64) any {
	if v == nil {
		return nil
	}
	return *v
}

func riceScore(r *provider.RICE) any {
	if r == nil {
		return nil
	}
	if r.Score != nil {
		return *r.Score
	}
	if r.Reach == nil || r.Impact == nil || r.Confidence == nil || r.Effort == nil || *r.Effort == 0 {
		return nil
	}
	return (*r.Reach * *r.Impact * *r.Confidence) / *r.Effort
}

func timeString(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format("2006-01-02")
}

func queryValue(v any) any {
	v = normalizeCustomValue(v)
	switch n := v.(type) {
	case float64, float32, int, int64, bool:
		return n
	case json.Number:
		if f, err := n.Float64(); err == nil {
			return f
		}
	}
	return valueString(v)
}

func inferFieldType(v any) guardsql.FieldType {
	v = normalizeCustomValue(v)
	switch v.(type) {
	case float64, float32, int, int64, json.Number:
		return guardsql.FieldNumber
	case bool:
		return guardsql.FieldBool
	default:
		return guardsql.FieldString
	}
}

func customQueryField(key string) string {
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

func valueString(v any) string {
	v = normalizeCustomValue(v)
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(t), 'f', -1, 32)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case json.Number:
		return t.String()
	default:
		data, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(data)
	}
}

func normalizeCustomValue(v any) any {
	switch t := v.(type) {
	case []byte:
		var out any
		if err := json.Unmarshal(t, &out); err == nil {
			return out
		}
	case string:
		decoded, err := base64.StdEncoding.DecodeString(t)
		if err != nil || !json.Valid(decoded) {
			return t
		}
		var out any
		if err := json.Unmarshal(decoded, &out); err == nil {
			return out
		}
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
