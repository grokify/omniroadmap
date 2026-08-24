package augment

import (
	"reflect"
	"testing"

	"github.com/grokify/omniroadmap-core/provider"
)

func f64(v float64) *float64 { return &v }

func TestApply_MatchBySourceRef(t *testing.T) {
	items := []provider.Item{
		{ID: "aha:1", Provider: "aha", SourceID: "1", SourceRef: "MYPROJ-123", MoSCoW: "should_have"},
		{ID: "aha:2", Provider: "aha", SourceID: "2", SourceRef: "MYPROJ-124"},
	}
	Apply(items, []ItemAugment{
		{Provider: "aha", SourceRef: "MYPROJ-123", MoSCoW: "must_have", Kano: "performance"},
	})

	if items[0].MoSCoW != "must_have" {
		t.Errorf("MoSCoW = %q, want must_have (augment wins over synced value)", items[0].MoSCoW)
	}
	if items[0].Kano != "performance" {
		t.Errorf("Kano = %q, want performance", items[0].Kano)
	}
	if items[1].MoSCoW != "" || items[1].Kano != "" {
		t.Errorf("unaugmented item modified: MoSCoW=%q Kano=%q", items[1].MoSCoW, items[1].Kano)
	}
}

func TestApply_FallbackToSourceID(t *testing.T) {
	// Providers without human-facing refs (e.g. ProductBoard UUIDs) key
	// augments by source ID.
	items := []provider.Item{
		{ID: "productboard:uuid-1", Provider: "productboard", SourceID: "uuid-1"},
	}
	Apply(items, []ItemAugment{
		{Provider: "productboard", SourceRef: "uuid-1", MoSCoW: "could_have"},
	})
	if items[0].MoSCoW != "could_have" {
		t.Errorf("MoSCoW = %q, want could_have (source-ID fallback match)", items[0].MoSCoW)
	}
}

func TestApply_ProviderMustMatch(t *testing.T) {
	items := []provider.Item{
		{ID: "aha:1", Provider: "aha", SourceID: "1", SourceRef: "MYPROJ-123"},
	}
	Apply(items, []ItemAugment{
		{Provider: "aha-studio", SourceRef: "MYPROJ-123", MoSCoW: "must_have"},
	})
	if items[0].MoSCoW != "" {
		t.Errorf("MoSCoW = %q, want unset (augment for a different provider)", items[0].MoSCoW)
	}
}

func TestApply_RICEMergesPerComponent(t *testing.T) {
	items := []provider.Item{
		{
			ID: "aha:1", Provider: "aha", SourceID: "1", SourceRef: "MYPROJ-123",
			// Fieldmap-derived values from the sync.
			RICE: &provider.RICE{Reach: f64(5000), Effort: f64(4)},
		},
	}
	Apply(items, []ItemAugment{
		{
			Provider: "aha", SourceRef: "MYPROJ-123",
			// Hand-authored override for effort only, plus a new component.
			RICE: &provider.RICE{Effort: f64(2), Confidence: f64(0.8)},
		},
	})

	got := items[0].RICE
	if got == nil {
		t.Fatal("RICE = nil, want merged value")
	}
	if got.Reach == nil || *got.Reach != 5000 {
		t.Errorf("Reach = %v, want 5000 preserved from synced value", got.Reach)
	}
	if got.Effort == nil || *got.Effort != 2 {
		t.Errorf("Effort = %v, want 2 (augment wins)", got.Effort)
	}
	if got.Confidence == nil || *got.Confidence != 0.8 {
		t.Errorf("Confidence = %v, want 0.8 (augment adds)", got.Confidence)
	}
}

func TestApply_OKRRefsAndNotesSurfaceInMetadata(t *testing.T) {
	items := []provider.Item{
		{ID: "aha:1", Provider: "aha", SourceID: "1", SourceRef: "MYPROJ-123"},
	}
	Apply(items, []ItemAugment{
		{
			Provider: "aha", SourceRef: "MYPROJ-123",
			OKRRefs:  []string{"OKR-2026-Q3-01"},
			Notes:    "exec ask",
			Metadata: map[string]any{"theme": "growth"},
		},
	})

	md := items[0].Metadata
	if md == nil {
		t.Fatal("Metadata = nil, want augment keys")
	}
	if !reflect.DeepEqual(md[MetadataKeyOKRRefs], []string{"OKR-2026-Q3-01"}) {
		t.Errorf("Metadata[%s] = %v, want [OKR-2026-Q3-01]", MetadataKeyOKRRefs, md[MetadataKeyOKRRefs])
	}
	if md[MetadataKeyNotes] != "exec ask" {
		t.Errorf("Metadata[%s] = %v, want exec ask", MetadataKeyNotes, md[MetadataKeyNotes])
	}
	if md["augment.theme"] != "growth" {
		t.Errorf("Metadata[augment.theme] = %v, want growth", md["augment.theme"])
	}
}

func TestApply_EmptyAugmentFieldsLeaveItemAlone(t *testing.T) {
	items := []provider.Item{
		{
			ID: "aha:1", Provider: "aha", SourceID: "1", SourceRef: "MYPROJ-123",
			MoSCoW: "should_have", Kano: "attractive",
			RICE: &provider.RICE{Reach: f64(100)},
		},
	}
	Apply(items, []ItemAugment{
		{Provider: "aha", SourceRef: "MYPROJ-123", Notes: "only notes"},
	})

	if items[0].MoSCoW != "should_have" || items[0].Kano != "attractive" {
		t.Errorf("MoSCoW/Kano = %q/%q, want synced values preserved", items[0].MoSCoW, items[0].Kano)
	}
	if items[0].RICE == nil || items[0].RICE.Reach == nil || *items[0].RICE.Reach != 100 {
		t.Errorf("RICE = %+v, want synced value preserved", items[0].RICE)
	}
}

func TestIsZero(t *testing.T) {
	empty := ItemAugment{Provider: "aha", SourceRef: "MYPROJ-123"}
	if !empty.IsZero() {
		t.Error("IsZero() = false for augment with no data, want true")
	}
	nonEmpty := ItemAugment{Provider: "aha", SourceRef: "MYPROJ-123", Kano: "must-be"}
	if nonEmpty.IsZero() {
		t.Error("IsZero() = true for augment with kano set, want false")
	}
}
