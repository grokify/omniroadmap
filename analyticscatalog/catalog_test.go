package analyticscatalog

import (
	"testing"

	"github.com/grokify/omniroadmap-core/provider"
	"github.com/plexusone/dashforge/dashboardir"
)

func TestBuildIncludesStandardAndCustomFields(t *testing.T) {
	cat := Build([]provider.Item{
		{
			ID:           "aha-studio:init-1",
			Provider:     "aha-studio",
			SourceID:     "init-1",
			SourceRef:    "PROJ-S-1",
			WorkspaceRef: "PROJ",
			Kind:         provider.ItemKindInitiative,
			Name:         "Initiative One",
			CustomFields: []provider.CustomField{
				{Key: "product", Name: "Product", Type: "choice", Value: "Platform"},
				{Key: "aha_initiative_rank", Name: "Aha Initiative Rank", Type: "number", Value: float64(10)},
			},
		},
		{
			ID:       "aha-studio:feat-1",
			Provider: "aha-studio",
			SourceID: "feat-1",
			Kind:     provider.ItemKindFeature,
			Name:     "Feature One",
			CustomFields: []provider.CustomField{
				{Key: "feature_only", Name: "Feature Only", Type: "text", Value: "yes"},
			},
		},
	})

	if cat.ID != "omniroadmap" || len(cat.Sources) != 1 {
		t.Fatalf("catalog = %#v", cat)
	}
	initiatives := datasetByID(t, cat.Sources[0].Datasets, "initiatives")
	if fieldByQueryName(initiatives.Fields, "workspace_ref") == nil {
		t.Fatal("missing workspace_ref field")
	}
	product := fieldByQueryName(initiatives.Fields, "custom.product")
	if product == nil {
		t.Fatal("missing custom.product field")
	}
	if product.Name != "Product" || product.Type != "choice" || product.Count != 1 || product.Coverage != 1 {
		t.Fatalf("custom.product = %#v", product)
	}
	if fieldByQueryName(initiatives.Fields, "custom.feature_only") != nil {
		t.Fatal("feature-only field appeared on initiatives dataset")
	}
}

func TestBuildCustomFieldsAlphabetical(t *testing.T) {
	cat := Build([]provider.Item{{
		ID:       "aha-studio:init-1",
		Provider: "aha-studio",
		SourceID: "init-1",
		Kind:     provider.ItemKindInitiative,
		CustomFields: []provider.CustomField{
			{Key: "problem_statement", Name: "Problem Statement", Value: "x"},
			{Key: "acceptance_criteria", Name: "Acceptance Criteria", Value: "y"},
		},
	}})
	initiatives := datasetByID(t, cat.Sources[0].Datasets, "initiatives")
	var customNames []string
	for _, field := range initiatives.Fields {
		if field.Source == "custom" {
			customNames = append(customNames, field.QueryName)
		}
	}
	if len(customNames) != 2 {
		t.Fatalf("custom fields = %#v, want two", customNames)
	}
	if customNames[0] != "custom.acceptance_criteria" {
		t.Fatalf("first custom field = %q, want custom.acceptance_criteria", customNames[0])
	}
}

func datasetByID(t *testing.T, datasets []dashboardir.AnalyticsDataset, id string) dashboardir.AnalyticsDataset {
	t.Helper()
	for _, dataset := range datasets {
		if dataset.ID == id {
			return dataset
		}
	}
	t.Fatalf("missing dataset %q", id)
	return dashboardir.AnalyticsDataset{}
}

func fieldByQueryName(fields []dashboardir.AnalyticsField, queryName string) *dashboardir.AnalyticsField {
	for i := range fields {
		if fields[i].QueryName == queryName {
			return &fields[i]
		}
	}
	return nil
}
