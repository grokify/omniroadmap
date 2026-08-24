package main

import (
	"testing"

	"github.com/grokify/guardsql"
	"github.com/grokify/omniroadmap-core/provider"
)

func TestCustomQueryField(t *testing.T) {
	got := customQueryField("Launch Tier")
	want := "custom.launch_tier"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNormalizeCustomValueBase64JSON(t *testing.T) {
	got := normalizeCustomValue("Ik5vIg==")
	if got != "No" {
		t.Fatalf("got %#v, want No", got)
	}
}

func TestCustomFieldStatsAlphabetical(t *testing.T) {
	stats := newCustomFieldStats(2)
	stats.add(&provider.Item{Provider: "aha-studio", CustomFields: []provider.CustomField{
		{Key: "problem_statement", Name: "Problem Statement", Value: "x"},
	}})
	stats.add(&provider.Item{Provider: "aha-studio", CustomFields: []provider.CustomField{
		{Key: "problem_statement", Name: "Problem Statement", Value: "y"},
		{Key: "acceptance_criteria", Name: "Acceptance Criteria", Value: "z"},
	}})
	got := stats.list()
	if len(got) < 2 {
		t.Fatalf("got %d stats, want at least 2", len(got))
	}
	if got[0].QueryField != "custom.acceptance_criteria" {
		t.Fatalf("first field = %q, want custom.acceptance_criteria", got[0].QueryField)
	}
}

func TestSelectedQueryColumns(t *testing.T) {
	q, err := guardsql.Parse(`SELECT source_ref, name AS title, custom.launch_tier FROM items LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	got := selectedQueryColumns(q, nil, "items")
	if len(got) != 3 {
		t.Fatalf("got %d columns, want 3", len(got))
	}
	if got[1].Key != "name" || got[1].Label != "title" {
		t.Fatalf("alias column = %#v, want name/title", got[1])
	}
	if got[2].Key != "custom.launch_tier" {
		t.Fatalf("custom column = %#v, want custom.launch_tier", got[2])
	}
}

func TestSelectedQueryColumnsUsesCatalogLabels(t *testing.T) {
	q, err := guardsql.Parse(`SELECT custom.launch_tier FROM initiatives LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	got := selectedQueryColumns(q, []provider.Item{
		{Provider: "aha-studio", Kind: provider.ItemKind("initiative"), CustomFields: []provider.CustomField{
			{Key: "launch_tier", Name: "Launch Tier", Type: "choice", Value: "T2"},
		}},
	}, "initiatives")
	if len(got) != 1 {
		t.Fatalf("got %d columns, want 1", len(got))
	}
	if got[0].Label != "Launch Tier" {
		t.Fatalf("label = %q, want Launch Tier", got[0].Label)
	}
}

func TestUIColumnsForInitiatives(t *testing.T) {
	columns, err := uiColumnsForItems([]provider.Item{
		{Provider: "aha-studio", Kind: provider.ItemKind("initiative"), CustomFields: []provider.CustomField{
			{Key: "launch_tier", Name: "Launch Tier", Type: "choice", Value: "T2"},
		}},
		{Provider: "aha-studio", Kind: provider.ItemKind("feature"), CustomFields: []provider.CustomField{
			{Key: "feature_only", Name: "Feature Only", Type: "text", Value: "yes"},
		}},
	}, "initiatives", "aha-studio")
	if err != nil {
		t.Fatal(err)
	}
	var sawLaunchTier, sawFeatureOnly bool
	for _, col := range columns {
		switch col.QueryField {
		case "custom.launch_tier":
			sawLaunchTier = true
			if col.Label != "Launch Tier" || col.Type != "choice" {
				t.Fatalf("launch tier column = %#v", col)
			}
		case "custom.feature_only":
			sawFeatureOnly = true
		}
	}
	if !sawLaunchTier {
		t.Fatal("missing custom.launch_tier")
	}
	if sawFeatureOnly {
		t.Fatal("feature-only custom field appeared for initiatives")
	}
}

func TestUIQuerySchemaSupportsRoadmapEntities(t *testing.T) {
	schema := uiQuerySchema([]provider.Item{
		{Provider: "aha-studio", Kind: provider.ItemKind("initiative"), CustomFields: []provider.CustomField{
			{Key: "launch_tier", Name: "Launch Tier", Type: "choice", Value: "T2"},
		}},
	}, 100)
	q, issues := guardsql.Lint(`FROM initiatives WHERE custom.launch_tier = "T2" LIMIT 10`, guardsql.LintConfig{
		Schema:     schema,
		AllowedOps: []guardsql.Operation{guardsql.OperationRead},
	})
	if len(issues) > 0 {
		t.Fatalf("lint issues: %#v", issues)
	}
	if q.From != "initiatives" {
		t.Fatalf("from = %q, want initiatives", q.From)
	}
}

func TestUIQuerySchemaSupportsMoscowRankOrder(t *testing.T) {
	schema := uiQuerySchema(nil, 100)
	_, issues := guardsql.Lint(`FROM initiatives WHERE provider = "aha-studio" ORDER BY moscow_rank ASC, rice_score DESC LIMIT 10`, guardsql.LintConfig{
		Schema:     schema,
		AllowedOps: []guardsql.Operation{guardsql.OperationRead},
	})
	if len(issues) > 0 {
		t.Fatalf("lint issues: %#v", issues)
	}
}

func TestUIQuerySchemaSupportsWorkspaceRef(t *testing.T) {
	item := provider.Item{
		ID:           "aha-studio:init-1",
		Provider:     "aha-studio",
		SourceID:     "init-1",
		WorkspaceRef: "PROJ",
		Kind:         provider.ItemKindInitiative,
		Name:         "Initiative One",
	}
	schema := uiQuerySchema([]provider.Item{item}, 100)
	_, issues := guardsql.Lint(`SELECT name, workspace_ref FROM initiatives WHERE provider = "aha-studio" AND workspace_ref = "PROJ" LIMIT 10`, guardsql.LintConfig{
		Schema:     schema,
		AllowedOps: []guardsql.Operation{guardsql.OperationRead},
	})
	if len(issues) > 0 {
		t.Fatalf("lint issues: %#v", issues)
	}
	row := uiQueryRow(0, &item)
	if row["workspace_ref"] != "PROJ" {
		t.Fatalf("workspace_ref row value = %#v, want PROJ", row["workspace_ref"])
	}
}

func TestApplyUIPrioritizationMoscowRice(t *testing.T) {
	rows := []guardsql.Row{
		{"id": "could-low", "moscow": "could_have", "rice_score": 10},
		{"id": "wont-high", "moscow": "wont_have", "rice_score": 999},
		{"id": "must-low", "moscow": "must_have", "rice_score": 10},
		{"id": "must-high", "moscow": "must_have", "rice_score": 20},
		{"id": "should-high", "moscow": "should_have", "rice_score": 100},
	}
	got, err := applyUIPrioritization(rows, []string{"moscow_rice"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"must-high", "must-low", "should-high", "could-low"}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i]["id"] != want[i] {
			t.Fatalf("row %d = %v, want %s; all rows %#v", i, got[i]["id"], want[i], got)
		}
	}
}
