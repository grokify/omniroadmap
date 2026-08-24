package sync

import (
	"context"
	"fmt"
	"testing"
	"time"

	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/omniroadmap-core/provider"

	"github.com/grokify/omniroadmap/fieldmap"
)

// fakeStore records everything written to it.
type fakeStore struct {
	items     []provider.Item
	releases  []provider.Release
	defs      []provider.CustomFieldDefinition
	syncMeta  map[string]int // "provider/kind" -> count
	commits   []string
	failUpser bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{syncMeta: map[string]int{}}
}

func (f *fakeStore) UpsertItems(ctx context.Context, items []provider.Item) error {
	if f.failUpser {
		return fmt.Errorf("boom")
	}
	f.items = append(f.items, items...)
	return nil
}

func (f *fakeStore) UpsertReleases(ctx context.Context, releases []provider.Release) error {
	f.releases = append(f.releases, releases...)
	return nil
}

func (f *fakeStore) UpsertCustomFieldDefinitions(ctx context.Context, providerName string, defs []provider.CustomFieldDefinition) error {
	f.defs = append(f.defs, defs...)
	return nil
}

func (f *fakeStore) SetSyncMeta(ctx context.Context, providerName, kind string, lastSync time.Time, count int) error {
	f.syncMeta[providerName+"/"+kind] = count
	return nil
}

func (f *fakeStore) Commit(ctx context.Context, message string) error {
	f.commits = append(f.commits, message)
	return nil
}

// fakeProvider serves canned items with configurable pagination behavior.
type fakeProvider struct {
	items           map[provider.ItemKind][]provider.Item
	releases        []provider.Release
	defs            []provider.CustomFieldDefinition
	honorsPaging    bool
	defsUnsupported bool
	caps            provider.Capabilities
}

func (p *fakeProvider) Name() string                        { return "fake" }
func (p *fakeProvider) Close() error                        { return nil }
func (p *fakeProvider) Capabilities() provider.Capabilities { return p.caps }

func (p *fakeProvider) ListItems(ctx context.Context, req *provider.ListItemsRequest) (*provider.ListItemsResponse, error) {
	var pool []provider.Item
	for _, kind := range req.Kinds {
		pool = append(pool, p.items[kind]...)
	}
	if !p.honorsPaging {
		// Ignores paging entirely: returns the full list on every page.
		return &provider.ListItemsResponse{Items: pool}, nil
	}
	start := (req.Page - 1) * req.PerPage
	if start >= len(pool) {
		return &provider.ListItemsResponse{}, nil
	}
	end := min(start+req.PerPage, len(pool))
	return &provider.ListItemsResponse{Items: pool[start:end]}, nil
}

func (p *fakeProvider) GetItem(ctx context.Context, req *provider.GetItemRequest) (*provider.Item, error) {
	return nil, omniroadmap.ErrNotFound
}

func (p *fakeProvider) ListReleases(ctx context.Context, req *provider.ListReleasesRequest) (*provider.ListReleasesResponse, error) {
	return &provider.ListReleasesResponse{Releases: p.releases}, nil
}

func (p *fakeProvider) ListStatuses(ctx context.Context, req *provider.ListStatusesRequest) (*provider.ListStatusesResponse, error) {
	return &provider.ListStatusesResponse{}, nil
}

func (p *fakeProvider) ListCustomFieldDefinitions(ctx context.Context, req *provider.ListCustomFieldDefinitionsRequest) (*provider.ListCustomFieldDefinitionsResponse, error) {
	if p.defsUnsupported {
		return nil, omniroadmap.ErrUnsupportedOperation
	}
	return &provider.ListCustomFieldDefinitionsResponse{Definitions: p.defs}, nil
}

func makeItems(kind provider.ItemKind, n int) []provider.Item {
	items := make([]provider.Item, n)
	for i := range items {
		items[i] = provider.Item{
			ID:       fmt.Sprintf("fake:%s-%d", kind, i),
			Provider: "fake",
			Kind:     kind,
			Name:     fmt.Sprintf("Item %d", i),
		}
	}
	return items
}

func TestRun_PaginatedProvider(t *testing.T) {
	p := &fakeProvider{
		items: map[provider.ItemKind][]provider.Item{
			provider.ItemKindFeature: makeItems(provider.ItemKindFeature, 250),
		},
		honorsPaging: true,
		caps: provider.Capabilities{
			Kinds: []provider.ItemKind{provider.ItemKindFeature},
		},
	}
	s := newFakeStore()

	report, err := Run(t.Context(), s, p, Options{PerPage: 100})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := report.ItemCounts[provider.ItemKindFeature]; got != 250 {
		t.Errorf("feature count = %d, want 250", got)
	}
	if len(s.items) != 250 {
		t.Errorf("stored items = %d, want 250", len(s.items))
	}
	if s.syncMeta["fake/feature"] != 250 {
		t.Errorf("sync meta = %d, want 250", s.syncMeta["fake/feature"])
	}
	if len(s.commits) != 1 {
		t.Errorf("commits = %d, want 1", len(s.commits))
	}
}

func TestRun_ProviderIgnoresPaging_NoInfiniteLoop(t *testing.T) {
	p := &fakeProvider{
		items: map[provider.ItemKind][]provider.Item{
			// 250 items, all returned on every page regardless of Page —
			// the dedup guard must terminate pagination.
			provider.ItemKindFeature: makeItems(provider.ItemKindFeature, 250),
		},
		honorsPaging: false,
		caps: provider.Capabilities{
			Kinds: []provider.ItemKind{provider.ItemKindFeature},
		},
	}
	s := newFakeStore()

	report, err := Run(t.Context(), s, p, Options{PerPage: 100})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := report.ItemCounts[provider.ItemKindFeature]; got != 250 {
		t.Errorf("feature count = %d, want 250 (deduped)", got)
	}
	if len(s.items) != 250 {
		t.Errorf("stored items = %d, want 250 (no duplicates)", len(s.items))
	}
}

func TestRun_ReleasesAndDefs(t *testing.T) {
	p := &fakeProvider{
		items: map[provider.ItemKind][]provider.Item{
			provider.ItemKindFeature: makeItems(provider.ItemKindFeature, 3),
		},
		releases: []provider.Release{{ID: "fake:r1", Provider: "fake", Name: "R1"}},
		defs:     []provider.CustomFieldDefinition{{Key: "priority", Name: "Priority"}},
		caps: provider.Capabilities{
			Kinds:                []provider.ItemKind{provider.ItemKindFeature},
			SupportsReleases:     true,
			SupportsCustomFields: true,
		},
	}
	s := newFakeStore()

	report, err := Run(t.Context(), s, p, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.ReleaseCount != 1 || len(s.releases) != 1 {
		t.Errorf("releases = %d/%d, want 1/1", report.ReleaseCount, len(s.releases))
	}
	if report.CustomFieldDefCount != 1 || len(s.defs) != 1 {
		t.Errorf("defs = %d/%d, want 1/1", report.CustomFieldDefCount, len(s.defs))
	}
}

func TestRun_DefsUnsupported_Tolerated(t *testing.T) {
	p := &fakeProvider{
		items: map[provider.ItemKind][]provider.Item{
			provider.ItemKindFeature: makeItems(provider.ItemKindFeature, 1),
		},
		defsUnsupported: true,
		caps: provider.Capabilities{
			Kinds:                []provider.ItemKind{provider.ItemKindFeature},
			SupportsCustomFields: true, // per-record fields yes, defs no (aha-studio case)
		},
	}
	s := newFakeStore()

	report, err := Run(t.Context(), s, p, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.CustomFieldDefCount != 0 {
		t.Errorf("defs count = %d, want 0", report.CustomFieldDefCount)
	}
}

func TestRun_FieldmapApplied(t *testing.T) {
	items := makeItems(provider.ItemKindFeature, 1)
	items[0].CustomFields = []provider.CustomField{{Key: "priority_moscow", Value: "Must Have"}}
	p := &fakeProvider{
		items:        map[provider.ItemKind][]provider.Item{provider.ItemKindFeature: items},
		honorsPaging: true,
		caps:         provider.Capabilities{Kinds: []provider.ItemKind{provider.ItemKindFeature}},
	}
	s := newFakeStore()

	_, err := Run(t.Context(), s, p, Options{
		Fieldmap: &fieldmap.Mapping{MoSCoW: &fieldmap.FieldRule{Key: "priority_moscow"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(s.items) != 1 || s.items[0].MoSCoW != "must_have" {
		t.Errorf("stored MoSCoW = %q, want must_have", s.items[0].MoSCoW)
	}
}

func TestRun_KindsFilter(t *testing.T) {
	p := &fakeProvider{
		items: map[provider.ItemKind][]provider.Item{
			provider.ItemKindFeature: makeItems(provider.ItemKindFeature, 2),
			provider.ItemKindEpic:    makeItems(provider.ItemKindEpic, 3),
		},
		honorsPaging: true,
		caps: provider.Capabilities{
			Kinds: []provider.ItemKind{provider.ItemKindFeature, provider.ItemKindEpic},
		},
	}
	s := newFakeStore()

	report, err := Run(t.Context(), s, p, Options{Kinds: []provider.ItemKind{provider.ItemKindEpic}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.ItemCounts) != 1 || report.ItemCounts[provider.ItemKindEpic] != 3 {
		t.Errorf("ItemCounts = %v, want epic:3 only", report.ItemCounts)
	}
}

func TestRun_UpsertFailurePropagates(t *testing.T) {
	p := &fakeProvider{
		items: map[provider.ItemKind][]provider.Item{
			provider.ItemKindFeature: makeItems(provider.ItemKindFeature, 1),
		},
		honorsPaging: true,
		caps:         provider.Capabilities{Kinds: []provider.ItemKind{provider.ItemKindFeature}},
	}
	s := newFakeStore()
	s.failUpser = true

	if _, err := Run(t.Context(), s, p, Options{}); err == nil {
		t.Fatal("expected upsert failure to propagate")
	}
	if len(s.commits) != 0 {
		t.Errorf("commits = %d, want 0 on failure", len(s.commits))
	}
}
