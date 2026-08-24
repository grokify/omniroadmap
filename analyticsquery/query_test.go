package analyticsquery

import (
	"testing"

	"github.com/grokify/omniroadmap-core/provider"
	"github.com/plexusone/dashforge/dashboardir"
)

func TestExecuteItemsSelectsCustomFields(t *testing.T) {
	items := []provider.Item{
		{
			ID:           "1",
			Provider:     "aha-studio",
			SourceID:     "AHA-1",
			SourceRef:    "PROJ-1",
			WorkspaceRef: "PROJ",
			Kind:         provider.ItemKindInitiative,
			Name:         "Launch reporting",
			CustomFields: []provider.CustomField{{Key: "Launch Tier", Value: "T1"}},
		},
		{
			ID:           "2",
			Provider:     "aha-studio",
			SourceID:     "AHA-2",
			SourceRef:    "PROJ-2",
			WorkspaceRef: "OTHER",
			Kind:         provider.ItemKindFeature,
			Name:         "Ignored feature",
			CustomFields: []provider.CustomField{{Key: "Launch Tier", Value: "T2"}},
		},
	}
	got, err := ExecuteItems(items, dashboardir.AnalyticsQueryRequest{
		Query: `SELECT name, workspace_ref, custom.launch_tier FROM initiatives WHERE workspace_ref = "PROJ" LIMIT 10`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RowCount != 1 {
		t.Fatalf("row count = %d, want 1", got.RowCount)
	}
	if got.Rows[0]["name"] != "Launch reporting" {
		t.Fatalf("name = %v", got.Rows[0]["name"])
	}
	if got.Rows[0]["custom.launch_tier"] != "T1" {
		t.Fatalf("custom.launch_tier = %v", got.Rows[0]["custom.launch_tier"])
	}
}

func TestExecuteItemsPrioritizeMoscowRice(t *testing.T) {
	scoreHigh := 10.0
	scoreLow := 2.0
	items := []provider.Item{
		{ID: "1", Kind: provider.ItemKindInitiative, Name: "Could", MoSCoW: "could_have", RICE: &provider.RICE{Score: &scoreHigh}},
		{ID: "2", Kind: provider.ItemKindInitiative, Name: "Must low", MoSCoW: "must_have", RICE: &provider.RICE{Score: &scoreLow}},
		{ID: "3", Kind: provider.ItemKindInitiative, Name: "Must high", MoSCoW: "must_have", RICE: &provider.RICE{Score: &scoreHigh}},
	}
	got, err := ExecuteItems(items, dashboardir.AnalyticsQueryRequest{
		Query: `SELECT name, moscow, rice_score FROM initiatives PRIORITIZE BY moscow_rice LIMIT 2`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RowCount != 2 {
		t.Fatalf("row count = %d, want 2", got.RowCount)
	}
	if got.Rows[0]["name"] != "Must high" {
		t.Fatalf("first row name = %v, want Must high", got.Rows[0]["name"])
	}
	if got.Rows[1]["name"] != "Must low" {
		t.Fatalf("second row name = %v, want Must low", got.Rows[1]["name"])
	}
}
