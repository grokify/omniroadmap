package fieldmap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grokify/omniroadmap-core/provider"
)

func itemWithFields(fields ...provider.CustomField) provider.Item {
	return provider.Item{
		ID:           "aha:1",
		Kind:         provider.ItemKindFeature,
		CustomFields: fields,
	}
}

func TestApply_MoSCoW(t *testing.T) {
	tests := []struct {
		name  string
		value any
		rule  *FieldRule
		want  string
	}{
		{"canonical value", "must_have", &FieldRule{Key: "priority"}, MoSCoWMustHave},
		{"display label", "Must Have", &FieldRule{Key: "priority"}, MoSCoWMustHave},
		{"short alias", "should", &FieldRule{Key: "priority"}, MoSCoWShouldHave},
		{"hyphenated", "could-have", &FieldRule{Key: "priority"}, MoSCoWCouldHave},
		{"wont with apostrophe", "Won't Have", &FieldRule{Key: "priority"}, MoSCoWWontHave},
		{
			"tenant vocabulary via Values",
			"P1",
			&FieldRule{Key: "priority", Values: map[string]string{"P1": "must_have", "P2": "should_have"}},
			MoSCoWMustHave,
		},
		{
			"values lookup is case-insensitive",
			"p2",
			&FieldRule{Key: "priority", Values: map[string]string{"P2": "should_have"}},
			MoSCoWShouldHave,
		},
		{"unmappable value leaves unset", "banana", &FieldRule{Key: "priority"}, ""},
		{"raw JSON string value", []byte(`"Must Have"`), &FieldRule{Key: "priority"}, MoSCoWMustHave},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []provider.Item{itemWithFields(provider.CustomField{Key: "priority", Value: tt.value})}
			Apply(items, &Mapping{MoSCoW: tt.rule})
			if items[0].MoSCoW != tt.want {
				t.Errorf("MoSCoW = %q, want %q", items[0].MoSCoW, tt.want)
			}
		})
	}
}

func TestApply_Kano(t *testing.T) {
	tests := []struct {
		name  string
		value any
		rule  *FieldRule
		want  string
	}{
		{"canonical value", "must-be", &FieldRule{Key: "kano"}, KanoMustBe},
		{"underscored", "must_be", &FieldRule{Key: "kano"}, KanoMustBe},
		{"alias basic", "Basic", &FieldRule{Key: "kano"}, KanoMustBe},
		{"alias delighter", "Delighter", &FieldRule{Key: "kano"}, KanoAttractive},
		{"alias one-dimensional", "One Dimensional", &FieldRule{Key: "kano"}, KanoPerformance},
		{"plain category", "indifferent", &FieldRule{Key: "kano"}, KanoIndifferent},
		{
			"tenant vocabulary via Values",
			"Table Stakes",
			&FieldRule{Key: "kano", Values: map[string]string{"Table Stakes": "must-be"}},
			KanoMustBe,
		},
		{"unmappable value leaves unset", "banana", &FieldRule{Key: "kano"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []provider.Item{itemWithFields(provider.CustomField{Key: "kano", Value: tt.value})}
			Apply(items, &Mapping{Kano: tt.rule})
			if items[0].Kano != tt.want {
				t.Errorf("Kano = %q, want %q", items[0].Kano, tt.want)
			}
		})
	}
}

func TestApply_RICE(t *testing.T) {
	items := []provider.Item{itemWithFields(
		provider.CustomField{Key: "rice_reach", Value: float64(5000)},
		provider.CustomField{Key: "rice_impact", Value: "2.0"},         // numeric string
		provider.CustomField{Key: "rice_confidence", Value: "High"},    // label vocabulary
		provider.CustomField{Key: "rice_effort", Value: []byte(`3.5`)}, // raw JSON number
	)}

	Apply(items, &Mapping{
		Reach:      &FieldRule{Key: "rice_reach"},
		Impact:     &FieldRule{Key: "rice_impact"},
		Confidence: &FieldRule{Key: "rice_confidence", Values: map[string]string{"High": "1.0", "Medium": "0.8", "Low": "0.5"}},
		Effort:     &FieldRule{Key: "rice_effort"},
		Score:      &FieldRule{Key: "rice_score"}, // absent on the item — should stay nil
	})

	rice := items[0].RICE
	if rice == nil {
		t.Fatal("RICE = nil, want populated")
	}
	if rice.Reach == nil || *rice.Reach != 5000 {
		t.Errorf("Reach = %v, want 5000", rice.Reach)
	}
	if rice.Impact == nil || *rice.Impact != 2.0 {
		t.Errorf("Impact = %v, want 2.0", rice.Impact)
	}
	if rice.Confidence == nil || *rice.Confidence != 1.0 {
		t.Errorf("Confidence = %v, want 1.0", rice.Confidence)
	}
	if rice.Effort == nil || *rice.Effort != 3.5 {
		t.Errorf("Effort = %v, want 3.5", rice.Effort)
	}
	if rice.Score != nil {
		t.Errorf("Score = %v, want nil (field absent)", rice.Score)
	}
}

func TestApply_NoMappedFields_LeavesItemUntouched(t *testing.T) {
	items := []provider.Item{itemWithFields(provider.CustomField{Key: "other", Value: "x"})}
	Apply(items, &Mapping{
		MoSCoW: &FieldRule{Key: "priority"},
		Reach:  &FieldRule{Key: "rice_reach"},
	})
	if items[0].MoSCoW != "" {
		t.Errorf("MoSCoW = %q, want empty", items[0].MoSCoW)
	}
	if items[0].RICE != nil {
		t.Errorf("RICE = %+v, want nil", items[0].RICE)
	}
}

func TestApply_NilMapping_NoOp(t *testing.T) {
	items := []provider.Item{itemWithFields(provider.CustomField{Key: "priority", Value: "must_have"})}
	Apply(items, nil)
	if items[0].MoSCoW != "" {
		t.Errorf("MoSCoW = %q, want empty after nil-mapping Apply", items[0].MoSCoW)
	}
}

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tenant.json")
	content := `{
		"description": "Acme Aha workspace",
		"moscow": {"key": "priority_moscow", "values": {"P1": "must_have"}},
		"reach": {"key": "rice_reach"},
		"effort": {"key": "rice_effort"}
	}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.MoSCoW == nil || m.MoSCoW.Key != "priority_moscow" {
		t.Errorf("MoSCoW rule = %+v, want Key=priority_moscow", m.MoSCoW)
	}
	if m.MoSCoW.Values["P1"] != "must_have" {
		t.Errorf("Values[P1] = %q, want must_have", m.MoSCoW.Values["P1"])
	}
	if m.Impact != nil {
		t.Errorf("Impact = %+v, want nil (not in config)", m.Impact)
	}
}

func TestLoad_Missing(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err == nil {
		t.Fatal("Load on missing file: error = nil, want error")
	}
}
