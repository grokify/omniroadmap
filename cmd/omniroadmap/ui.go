package main

import (
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/grokify/guardsql"
	"github.com/grokify/omniroadmap-core/provider"
	"github.com/spf13/cobra"

	"github.com/grokify/omniroadmap/analyticscatalog"
	"github.com/grokify/omniroadmap/analyticsdashboards"
	"github.com/grokify/omniroadmap/store"
)

//go:embed ui_static/index.html
var uiIndexHTML string

type uiItem struct {
	ID             string         `json:"id"`
	Provider       string         `json:"provider"`
	SourceID       string         `json:"source_id"`
	SourceRef      string         `json:"source_ref,omitempty"`
	SourceURL      string         `json:"source_url,omitempty"`
	WorkspaceRef   string         `json:"workspace_ref,omitempty"`
	Kind           string         `json:"kind"`
	Name           string         `json:"name"`
	Status         string         `json:"status,omitempty"`
	StatusCategory string         `json:"status_category,omitempty"`
	Owner          string         `json:"owner,omitempty"`
	ReleaseID      string         `json:"release_id,omitempty"`
	MoSCoW         string         `json:"moscow,omitempty"`
	MoSCoWRank     float64        `json:"moscow_rank,omitempty"`
	Kano           string         `json:"kano,omitempty"`
	RICE           *uiRICE        `json:"rice,omitempty"`
	CustomFields   map[string]any `json:"custom_fields,omitempty"`
	Progress       *float64       `json:"progress,omitempty"`
	DueDate        *time.Time     `json:"due_date,omitempty"`
	UpdatedAt      *time.Time     `json:"updated_at,omitempty"`
}

type uiRICE struct {
	Reach      *float64 `json:"reach,omitempty"`
	Impact     *float64 `json:"impact,omitempty"`
	Confidence *float64 `json:"confidence,omitempty"`
	Effort     *float64 `json:"effort,omitempty"`
	Score      *float64 `json:"score,omitempty"`
}

type uiSyncMeta struct {
	Provider    string `json:"provider"`
	Kind        string `json:"kind"`
	RecordCount int    `json:"record_count"`
	LastSync    string `json:"last_sync"`
}

type uiProviderSummary struct {
	Provider string         `json:"provider"`
	Total    int            `json:"total"`
	ByKind   map[string]int `json:"by_kind"`
}

type uiSummary struct {
	TotalItems       int                 `json:"total_items"`
	TotalReleases    int                 `json:"total_releases"`
	TotalFieldDefs   int                 `json:"total_field_defs"`
	AugmentCount     int                 `json:"augment_count"`
	Providers        []uiProviderSummary `json:"providers"`
	ByKind           map[string]int      `json:"by_kind"`
	ByStatusCategory map[string]int      `json:"by_status_category"`
	ByMoSCoW         map[string]int      `json:"by_moscow"`
	ByKano           map[string]int      `json:"by_kano"`
	CustomFields     []uiCustomFieldStat `json:"custom_fields"`
	SyncMeta         []uiSyncMeta        `json:"sync_meta"`
}

type uiSnapshot struct {
	Summary uiSummary `json:"summary"`
	Items   []uiItem  `json:"items"`
}

type uiCustomFieldStat struct {
	Provider     string   `json:"provider"`
	Key          string   `json:"key"`
	QueryField   string   `json:"query_field"`
	Name         string   `json:"name,omitempty"`
	Type         string   `json:"type,omitempty"`
	Count        int      `json:"count"`
	Coverage     float64  `json:"coverage"`
	SampleValues []string `json:"sample_values,omitempty"`
}

type uiColumnStat struct {
	Entity       string   `json:"entity"`
	Source       string   `json:"source"`
	Key          string   `json:"key"`
	QueryField   string   `json:"query_field"`
	Label        string   `json:"label"`
	Type         string   `json:"type"`
	Count        int      `json:"count"`
	Coverage     float64  `json:"coverage"`
	SampleValues []string `json:"sample_values,omitempty"`
}

type uiQueryRequest struct {
	Query string `json:"query"`
}

type uiQueryResponse struct {
	Query   string          `json:"query"`
	Columns []uiQueryColumn `json:"columns,omitempty"`
	Items   []uiItem        `json:"items"`
	Issues  []uiQueryIssue  `json:"issues,omitempty"`
}

type uiQueryIssue struct {
	Message string `json:"message"`
}

type uiQueryColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

func uiCmd() *cobra.Command {
	flags := &storeFlags{}
	var address string
	var limit int

	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Serve a local web UI for synced roadmap data",
		Long: `Serve a local dashboard and JSON API over the Dolt-backed store.

This follows the VisionStudio local-control-plane pattern: one Go command
serves static UI plus fresh API snapshots from the local database.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			mux := http.NewServeMux()
			registerUIRoutes(mux, s, limit)

			listener, err := net.Listen("tcp", address)
			if err != nil {
				return fmt.Errorf("listen %s: %w", address, err)
			}

			actual := listener.Addr().String()
			cmd.Printf("OmniRoadmap UI running at http://%s (Ctrl-C to stop)\n", actual)
			return http.Serve(listener, mux) //nolint:gosec // local operator CLI server
		},
	}

	cmd.Flags().StringVar(&address, "address", "127.0.0.1:13317", "Address for the UI server")
	cmd.Flags().IntVar(&limit, "limit", 500, "Maximum items to include in the initial table snapshot")
	flags.register(cmd)
	return cmd
}

func registerUIRoutes(mux *http.ServeMux, s *store.DoltStore, limit int) {
	tmpl := template.Must(template.New("index").Parse(uiIndexHTML))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if !isUIPagePath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.Execute(w, nil)
	})

	mux.HandleFunc("/api/snapshot", func(w http.ResponseWriter, r *http.Request) {
		snap, err := buildUISnapshot(r.Context(), s, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(snap)
	})

	mux.HandleFunc("/api/query", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req uiQueryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON request: "+err.Error(), http.StatusBadRequest)
			return
		}
		resp, err := runUIQuery(r.Context(), s, strings.TrimSpace(req.Query), limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		status := http.StatusOK
		if len(resp.Issues) > 0 {
			status = http.StatusBadRequest
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(resp)
	})

	mux.HandleFunc("/api/columns", func(w http.ResponseWriter, r *http.Request) {
		entity := strings.TrimSpace(r.URL.Query().Get("entity"))
		if entity == "" {
			entity = "items"
		}
		providerName := strings.TrimSpace(r.URL.Query().Get("provider"))
		columns, err := buildUIColumns(r.Context(), s, entity, providerName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(columns)
	})

	handleAnalyticsCatalog := func(w http.ResponseWriter, r *http.Request) {
		catalog, err := analyticscatalog.BuildFromStore(r.Context(), s)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(catalog)
	}
	mux.HandleFunc("/api/analytics/catalog", handleAnalyticsCatalog)
	mux.HandleFunc("/api/v1/analytics/catalog", handleAnalyticsCatalog)

	handleAnalyticsDashboards := func(w http.ResponseWriter, r *http.Request) {
		pack, err := analyticsdashboards.BuildFromStore(r.Context(), s, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(pack)
	}
	mux.HandleFunc("/api/analytics/dashboards", handleAnalyticsDashboards)
	mux.HandleFunc("/api/v1/analytics/dashboards", handleAnalyticsDashboards)
}

func isUIPagePath(path string) bool {
	switch path {
	case "/", "/items", "/initiatives", "/features", "/epics":
		return true
	default:
		return false
	}
}

func buildUISnapshot(ctx context.Context, s *store.DoltStore, limit int) (*uiSnapshot, error) {
	items, err := s.ListItems(ctx, store.ItemFilter{})
	if err != nil {
		return nil, err
	}
	augs, err := s.ListItemAugments(ctx, "")
	if err != nil {
		return nil, err
	}
	meta, err := s.GetSyncMeta(ctx)
	if err != nil {
		return nil, err
	}
	releaseCount, err := s.Client().Release.Query().Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count releases: %w", err)
	}
	fieldDefCount, err := s.Client().CustomFieldDef.Query().Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count custom field defs: %w", err)
	}

	sort.Slice(items, func(i, j int) bool {
		iu := itemUpdated(items[i])
		ju := itemUpdated(items[j])
		if !iu.Equal(ju) {
			return iu.After(ju)
		}
		if items[i].Provider != items[j].Provider {
			return items[i].Provider < items[j].Provider
		}
		return displayRef(items[i]) < displayRef(items[j])
	})

	summary := uiSummary{
		TotalItems:       len(items),
		TotalReleases:    releaseCount,
		TotalFieldDefs:   fieldDefCount,
		AugmentCount:     len(augs),
		ByKind:           map[string]int{},
		ByStatusCategory: map[string]int{},
		ByMoSCoW:         map[string]int{},
		ByKano:           map[string]int{},
	}
	providers := map[string]*uiProviderSummary{}
	fieldStats := newCustomFieldStats(len(items))
	for _, it := range items {
		k := string(it.Kind)
		summary.ByKind[k]++
		if it.Status != nil && it.Status.Category != "" {
			summary.ByStatusCategory[string(it.Status.Category)]++
		} else {
			summary.ByStatusCategory["unset"]++
		}
		if it.MoSCoW != "" {
			summary.ByMoSCoW[it.MoSCoW]++
		} else {
			summary.ByMoSCoW["unset"]++
		}
		if it.Kano != "" {
			summary.ByKano[it.Kano]++
		} else {
			summary.ByKano["unset"]++
		}
		ps := providers[it.Provider]
		if ps == nil {
			ps = &uiProviderSummary{Provider: it.Provider, ByKind: map[string]int{}}
			providers[it.Provider] = ps
		}
		ps.Total++
		ps.ByKind[k]++
		fieldStats.add(&it)
	}
	summary.CustomFields = fieldStats.list()
	for _, ps := range providers {
		summary.Providers = append(summary.Providers, *ps)
	}
	sort.Slice(summary.Providers, func(i, j int) bool {
		return summary.Providers[i].Provider < summary.Providers[j].Provider
	})
	for _, e := range meta {
		summary.SyncMeta = append(summary.SyncMeta, uiSyncMeta{
			Provider:    e.Provider,
			Kind:        e.Kind,
			RecordCount: e.RecordCount,
			LastSync:    e.LastSync.Format(time.RFC3339),
		})
	}

	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	outItems := make([]uiItem, len(items))
	for i := range items {
		outItems[i] = makeUIItem(&items[i])
	}

	return &uiSnapshot{Summary: summary, Items: outItems}, nil
}

func makeUIItem(it *provider.Item) uiItem {
	out := uiItem{
		ID:           it.ID,
		Provider:     it.Provider,
		SourceID:     it.SourceID,
		SourceRef:    it.SourceRef,
		SourceURL:    it.SourceURL,
		WorkspaceRef: it.WorkspaceRef,
		Kind:         string(it.Kind),
		Name:         it.Name,
		ReleaseID:    it.ReleaseID,
		MoSCoW:       it.MoSCoW,
		MoSCoWRank:   moscowRankValue(it.MoSCoW),
		Kano:         it.Kano,
		Progress:     it.Progress,
		DueDate:      it.DueDate,
		UpdatedAt:    it.UpdatedAt,
	}
	if len(it.CustomFields) > 0 {
		out.CustomFields = map[string]any{}
		for _, field := range it.CustomFields {
			if field.Key == "" {
				continue
			}
			out.CustomFields[field.Key] = normalizeCustomValue(field.Value)
		}
	}
	if it.Status != nil {
		out.Status = it.Status.Name
		out.StatusCategory = string(it.Status.Category)
	}
	if it.Owner != nil {
		out.Owner = it.Owner.Name
		if out.Owner == "" {
			out.Owner = it.Owner.Email
		}
	}
	if it.RICE != nil {
		out.RICE = &uiRICE{
			Reach:      it.RICE.Reach,
			Impact:     it.RICE.Impact,
			Confidence: it.RICE.Confidence,
			Effort:     it.RICE.Effort,
			Score:      it.RICE.Score,
		}
	}
	return out
}

func runUIQuery(ctx context.Context, s *store.DoltStore, input string, defaultLimit int) (*uiQueryResponse, error) {
	if input == "" {
		input = fmt.Sprintf("FROM items LIMIT %d", defaultLimit)
	}
	items, err := s.ListItems(ctx, store.ItemFilter{})
	if err != nil {
		return nil, err
	}
	schema := uiQuerySchema(items, defaultLimit)
	q, issues := guardsql.Lint(input, guardsql.LintConfig{
		Schema:       schema,
		AllowedOps:   []guardsql.Operation{guardsql.OperationRead},
		MaxDepth:     8,
		MaxNodes:     80,
		MaxInValues:  100,
		RequireLimit: false,
	})
	if len(issues) > 0 {
		out := make([]uiQueryIssue, len(issues))
		for i, issue := range issues {
			out[i] = uiQueryIssue{Message: issue.Message}
		}
		return &uiQueryResponse{Query: input, Issues: out}, nil
	}
	columns := selectedQueryColumns(q, items, q.From)
	evalQuery := *q
	evalQuery.Select = []guardsql.SelectItem{{Star: true}}
	if len(q.Prioritize) > 0 {
		evalQuery.OrderBy = nil
		evalQuery.Limit = -1
	}
	entityFilter, err := entityKind(q.From)
	if err != nil {
		return nil, err
	}
	rows := make([]guardsql.Row, 0, len(items))
	for i := range items {
		if entityFilter != "" && string(items[i].Kind) != entityFilter {
			continue
		}
		rows = append(rows, uiQueryRow(i, &items[i]))
	}
	filtered, err := guardsql.Eval(&evalQuery, rows)
	if err != nil {
		return nil, err
	}
	if len(q.Prioritize) > 0 {
		var err error
		filtered, err = applyUIPrioritization(filtered, q.Prioritize)
		if err != nil {
			return &uiQueryResponse{Query: input, Issues: []uiQueryIssue{{Message: err.Error()}}}, nil
		}
		if q.Limit >= 0 && len(filtered) > q.Limit {
			filtered = filtered[:q.Limit]
		}
	}
	outItems := make([]uiItem, 0, len(filtered))
	for _, row := range filtered {
		idx, ok := row["_idx"].(int)
		if !ok || idx < 0 || idx >= len(items) {
			continue
		}
		outItems = append(outItems, makeUIItem(&items[idx]))
	}
	return &uiQueryResponse{Query: input, Columns: columns, Items: outItems}, nil
}

func applyUIPrioritization(rows []guardsql.Row, strategies []string) ([]guardsql.Row, error) {
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
				irice := asFloat64(rows[i]["rice_score"])
				jrice := asFloat64(rows[j]["rice_score"])
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

func buildUIColumns(ctx context.Context, s *store.DoltStore, entity, providerName string) ([]uiColumnStat, error) {
	items, err := s.ListItems(ctx, store.ItemFilter{})
	if err != nil {
		return nil, err
	}
	return uiColumnsForItems(items, entity, providerName)
}

func uiColumnsForItems(items []provider.Item, entity, providerName string) ([]uiColumnStat, error) {
	entity = strings.ToLower(strings.TrimSpace(entity))
	if entity == "" {
		entity = "items"
	}
	kind, err := entityKind(entity)
	if err != nil {
		return nil, err
	}
	filtered := make([]provider.Item, 0, len(items))
	for _, it := range items {
		if providerName != "" && it.Provider != providerName {
			continue
		}
		if kind != "" && string(it.Kind) != kind {
			continue
		}
		filtered = append(filtered, it)
	}

	columns := canonicalColumns(entity, len(filtered))
	fieldStats := newCustomFieldStats(len(filtered))
	for i := range filtered {
		fieldStats.add(&filtered[i])
	}
	for _, field := range fieldStats.list() {
		columns = append(columns, uiColumnStat{
			Entity:       entity,
			Source:       "custom",
			Key:          field.Key,
			QueryField:   field.QueryField,
			Label:        firstNonEmpty(field.Name, field.Key),
			Type:         firstNonEmpty(field.Type, "string"),
			Count:        field.Count,
			Coverage:     field.Coverage,
			SampleValues: field.SampleValues,
		})
	}
	sort.Slice(columns, func(i, j int) bool {
		if columns[i].Source != columns[j].Source {
			return columns[i].Source < columns[j].Source
		}
		return columns[i].QueryField < columns[j].QueryField
	})
	return columns, nil
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

func canonicalColumns(entity string, total int) []uiColumnStat {
	defs := []struct {
		key, label, typ string
	}{
		{"id", "ID", "string"},
		{"provider", "Provider", "string"},
		{"source_id", "Source ID", "string"},
		{"source_ref", "Source Ref", "string"},
		{"source_url", "Source URL", "string"},
		{"workspace_ref", "Workspace Ref", "string"},
		{"kind", "Kind", "string"},
		{"name", "Name", "string"},
		{"status", "Status", "string"},
		{"status_category", "Status Category", "string"},
		{"owner", "Owner", "string"},
		{"release_id", "Release ID", "string"},
		{"moscow", "MoSCoW", "string"},
		{"moscow_rank", "MoSCoW Rank", "number"},
		{"kano", "Kano", "string"},
		{"progress", "Progress", "number"},
		{"rice_score", "RICE Score", "number"},
		{"due_date", "Due Date", "date"},
		{"updated_at", "Updated At", "date"},
	}
	out := make([]uiColumnStat, 0, len(defs))
	for _, def := range defs {
		out = append(out, uiColumnStat{
			Entity:     entity,
			Source:     "canonical",
			Key:        def.key,
			QueryField: def.key,
			Label:      def.label,
			Type:       def.typ,
			Count:      total,
			Coverage:   1,
		})
	}
	return out
}

func selectedQueryColumns(q *guardsql.Query, items []provider.Item, entity string) []uiQueryColumn {
	if q == nil || len(q.Select) == 0 {
		return nil
	}
	labels := queryColumnLabels(items, entity)
	var columns []uiQueryColumn
	for _, item := range q.Select {
		if item.Star {
			return nil
		}
		key := item.Field
		label := item.Alias
		if label == "" {
			label = firstNonEmpty(labels[key], key)
		}
		columns = append(columns, uiQueryColumn{Key: key, Label: label})
	}
	return columns
}

func queryColumnLabels(items []provider.Item, entity string) map[string]string {
	columns, err := uiColumnsForItems(items, entity, "")
	if err != nil {
		return nil
	}
	labels := make(map[string]string, len(columns))
	for _, col := range columns {
		labels[col.QueryField] = col.Label
	}
	return labels
}

func uiQuerySchema(items []provider.Item, maxLimit int) guardsql.Schema {
	fields := map[string]guardsql.Field{}
	add := func(name string, typ guardsql.FieldType) {
		fields[name] = guardsql.Field{Name: name, Type: typ, Selectable: true, Filterable: true, Sortable: true}
	}
	add("id", guardsql.FieldString)
	add("provider", guardsql.FieldString)
	add("source_id", guardsql.FieldString)
	add("source_ref", guardsql.FieldString)
	add("source_url", guardsql.FieldString)
	add("workspace_ref", guardsql.FieldString)
	add("kind", guardsql.FieldString)
	add("name", guardsql.FieldString)
	add("status", guardsql.FieldString)
	add("status_category", guardsql.FieldString)
	add("owner", guardsql.FieldString)
	add("release_id", guardsql.FieldString)
	add("moscow", guardsql.FieldString)
	add("moscow_rank", guardsql.FieldNumber)
	add("kano", guardsql.FieldString)
	add("progress", guardsql.FieldNumber)
	add("rice_score", guardsql.FieldNumber)
	add("due_date", guardsql.FieldString)
	add("updated_at", guardsql.FieldString)
	for _, it := range items {
		for _, field := range it.CustomFields {
			queryField := customQueryField(field.Key)
			if queryField == "" {
				continue
			}
			fields[queryField] = guardsql.Field{Name: queryField, Type: inferGuardSQLFieldType(normalizeCustomValue(field.Value)), Selectable: true, Filterable: true, Sortable: true}
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

func uiQueryRow(idx int, it *provider.Item) guardsql.Row {
	row := guardsql.Row{
		"_idx":          idx,
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

func asFloat64(value any) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f
	default:
		return 0
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

func inferGuardSQLFieldType(v any) guardsql.FieldType {
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

type customFieldStats struct {
	total  int
	byKey  map[string]*uiCustomFieldStat
	values map[string]map[string]struct{}
}

func newCustomFieldStats(total int) *customFieldStats {
	return &customFieldStats{
		total:  total,
		byKey:  map[string]*uiCustomFieldStat{},
		values: map[string]map[string]struct{}{},
	}
}

func (s *customFieldStats) add(it *provider.Item) {
	seen := map[string]struct{}{}
	for _, field := range it.CustomFields {
		queryField := customQueryField(field.Key)
		if queryField == "" {
			continue
		}
		stat := s.byKey[queryField]
		if stat == nil {
			stat = &uiCustomFieldStat{
				Provider:   it.Provider,
				Key:        field.Key,
				QueryField: queryField,
				Name:       field.Name,
				Type:       field.Type,
			}
			s.byKey[queryField] = stat
			s.values[queryField] = map[string]struct{}{}
		}
		if stat.Name == "" {
			stat.Name = field.Name
		}
		if stat.Type == "" {
			stat.Type = field.Type
		}
		if _, ok := seen[queryField]; !ok {
			stat.Count++
			seen[queryField] = struct{}{}
		}
		if value := sampleValueString(field.Value); value != "" && len(s.values[queryField]) < 8 {
			s.values[queryField][value] = struct{}{}
		}
	}
}

func (s *customFieldStats) list() []uiCustomFieldStat {
	out := make([]uiCustomFieldStat, 0, len(s.byKey))
	for queryField, stat := range s.byKey {
		next := *stat
		if s.total > 0 {
			next.Coverage = float64(next.Count) / float64(s.total)
		}
		for value := range s.values[queryField] {
			next.SampleValues = append(next.SampleValues, value)
		}
		sort.Strings(next.SampleValues)
		out = append(out, next)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].QueryField < out[j].QueryField
	})
	if len(out) > 30 {
		out = out[:30]
	}
	return out
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

func sampleValueString(v any) string {
	const maxLen = 96
	s := valueString(v)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
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

func itemUpdated(it provider.Item) time.Time {
	switch {
	case it.UpdatedAt != nil:
		return *it.UpdatedAt
	case it.CreatedAt != nil:
		return *it.CreatedAt
	default:
		return time.Time{}
	}
}

func displayRef(it provider.Item) string {
	if it.SourceRef != "" {
		return it.SourceRef
	}
	if it.SourceID != "" {
		return it.SourceID
	}
	return it.ID
}
