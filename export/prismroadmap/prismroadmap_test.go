package prismroadmap

import (
	"testing"
	"time"

	"github.com/grokify/prism-roadmap/prioritization"
	"github.com/grokify/prism-roadmap/rmi"

	"github.com/grokify/omniroadmap-core/provider"
)

func f64(v float64) *float64 { return &v }

func TestToRoadmapItem_Full(t *testing.T) {
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	due := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	progress := 40.0

	item := &provider.Item{
		ID:          "aha:123",
		Provider:    "aha",
		SourceRef:   "SE-2",
		SourceURL:   "https://test.aha.io/features/SE-2",
		Kind:        provider.ItemKindFeature,
		Name:        "SSO Integration",
		Description: "Add single sign-on",
		Status:      &provider.Status{Name: "In Development", Category: provider.StatusCategoryInProgress},
		Progress:    &progress,
		StartDate:   &start,
		DueDate:     &due,
		CreatedAt:   &created,
		Owner:       &provider.Person{Name: "Alice", Email: "alice@example.com"},
		Tags:        []string{"security"},
		MoSCoW:      "must_have",
		RICE: &provider.RICE{
			Reach:      f64(5000),
			Impact:     f64(2.0),
			Confidence: f64(0.8),
			Effort:     f64(4),
		},
	}

	out, err := ToRoadmapItem(item)
	if err != nil {
		t.Fatalf("ToRoadmapItem: %v", err)
	}

	if out.ID != "aha:123" {
		t.Errorf("ID = %q, want aha:123", out.ID)
	}
	if out.MoSCoW != prioritization.MoSCoWMustHave {
		t.Errorf("MoSCoW = %q, want must_have", out.MoSCoW)
	}
	if out.Status != rmi.RMIStatusInProgress {
		t.Errorf("Status = %q, want in_progress", out.Status)
	}
	if out.StartDate != "2026-09-01" || out.DueDate != "2026-09-30" {
		t.Errorf("dates = %q/%q, want 2026-09-01/2026-09-30", out.StartDate, out.DueDate)
	}
	if out.Owner != "Alice" {
		t.Errorf("Owner = %q, want Alice", out.Owner)
	}
	if out.Progress == nil || *out.Progress != 40 {
		t.Errorf("Progress = %v, want 40", out.Progress)
	}
	if out.Notes != "Source: SE-2 (https://test.aha.io/features/SE-2)" {
		t.Errorf("Notes = %q", out.Notes)
	}

	rice := out.RICE
	if rice == nil {
		t.Fatal("RICE = nil, want populated")
	}
	if rice.Reach != 5000 {
		t.Errorf("Reach = %d, want 5000", rice.Reach)
	}
	if rice.Impact != prioritization.ImpactHigh {
		t.Errorf("Impact = %q, want high (2.0)", rice.Impact)
	}
	if rice.Confidence != prioritization.ConfidenceMedium {
		t.Errorf("Confidence = %q, want medium (0.8)", rice.Confidence)
	}
	// Score computed: (5000 × 2.0 × 0.8) / 4 = 2000
	if rice.Score != 2000 {
		t.Errorf("Score = %v, want 2000", rice.Score)
	}
}

func TestToRoadmapItem_Unprioritized(t *testing.T) {
	item := &provider.Item{
		ID:   "productboard:x",
		Kind: provider.ItemKindFeature,
		Name: "Imported feature",
	}

	out, err := ToRoadmapItem(item)
	if err != nil {
		t.Fatalf("ToRoadmapItem: %v", err)
	}
	if out.MoSCoW != prioritization.MoSCoWUnspecified {
		t.Errorf("MoSCoW = %q, want unset", out.MoSCoW)
	}
	if out.RICE != nil {
		t.Errorf("RICE = %+v, want nil", out.RICE)
	}
	if out.Status != rmi.RMIStatusPlanned {
		t.Errorf("Status = %q, want planned default", out.Status)
	}
	// The converted item must satisfy prism-roadmap's own validation —
	// this is the "MoSCoW optional, then used" contract end-to-end.
	if err := out.Validate(); err != nil {
		t.Errorf("Validate() on unprioritized item: %v", err)
	}
}

func TestToRoadmapItem_ConfidenceAsPercentage(t *testing.T) {
	item := &provider.Item{
		ID:   "aha:1",
		Name: "X",
		RICE: &provider.RICE{Confidence: f64(80)}, // percentage-style
	}
	out, err := ToRoadmapItem(item)
	if err != nil {
		t.Fatalf("ToRoadmapItem: %v", err)
	}
	if out.RICE.Confidence != prioritization.ConfidenceMedium {
		t.Errorf("Confidence = %q, want medium (80%%)", out.RICE.Confidence)
	}
}

func TestToRoadmapItem_MissingIdentity(t *testing.T) {
	if _, err := ToRoadmapItem(&provider.Item{ID: "x"}); err == nil {
		t.Fatal("expected error for missing name")
	}
	if _, err := ToRoadmapItem(&provider.Item{Name: "x"}); err == nil {
		t.Fatal("expected error for missing id")
	}
}

func TestToRoadmapItemSet(t *testing.T) {
	items := []provider.Item{
		{ID: "aha:1", Name: "One", MoSCoW: "should_have"},
		{ID: "aha:2", Name: "Two"},
		{ID: "aha:3"}, // missing name — skipped
	}

	set, err := ToRoadmapItemSet(items)
	if err == nil {
		t.Fatal("expected a skip-report error for the invalid item")
	}
	if len(set.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(set.Items))
	}
	if err := set.Validate(); err != nil {
		t.Errorf("set.Validate(): %v", err)
	}
}

func TestConvertStatus_Categories(t *testing.T) {
	tests := []struct {
		category provider.StatusCategory
		want     rmi.RMIStatus
	}{
		{provider.StatusCategoryTodo, rmi.RMIStatusPlanned},
		{provider.StatusCategoryInProgress, rmi.RMIStatusInProgress},
		{provider.StatusCategoryDone, rmi.RMIStatusCompleted},
		{provider.StatusCategoryCanceled, rmi.RMIStatusCancelled},
		{"", rmi.RMIStatusPlanned},
	}
	for _, tt := range tests {
		got := convertStatus(&provider.Status{Category: tt.category})
		if got != tt.want {
			t.Errorf("convertStatus(%q) = %q, want %q", tt.category, got, tt.want)
		}
	}
	if got := convertStatus(nil); got != rmi.RMIStatusPlanned {
		t.Errorf("convertStatus(nil) = %q, want planned", got)
	}
}
