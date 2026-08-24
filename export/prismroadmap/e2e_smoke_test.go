package prismroadmap

import (
	"os"
	"testing"

	ahastudio "github.com/grokify/aha-studio/omniroadmap"
	studiosync "github.com/grokify/aha-studio/sync"
	"github.com/grokify/omniroadmap-core/provider"
)

// TestE2E_CacheToValidatedRoadmapItemSet drives the full chain: aha-studio
// cache -> canonical Items -> rmi.RoadmapItemSet validated by
// prism-roadmap itself. Gated on OMNIROADMAP_E2E_CACHE_DB pointing at a
// cache COPY (never the live ~/.ahastudio/cache.db).
func TestE2E_CacheToValidatedRoadmapItemSet(t *testing.T) {
	path := os.Getenv("OMNIROADMAP_E2E_CACHE_DB")
	if path == "" {
		t.Skip("OMNIROADMAP_E2E_CACHE_DB not set")
	}
	db, err := studiosync.Open(path)
	if err != nil {
		t.Fatalf("open cache: %v", err)
	}
	defer func() { _ = db.Close() }()

	p := ahastudio.NewProvider(db)
	resp, err := p.ListItems(t.Context(), &provider.ListItemsRequest{
		Kinds: []provider.ItemKind{provider.ItemKindInitiative},
	})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(resp.Items) == 0 {
		t.Fatal("no initiatives in cache copy")
	}

	set, err := ToRoadmapItemSet(resp.Items)
	if err != nil {
		t.Fatalf("ToRoadmapItemSet: %v (converted %d)", err, len(set.Items))
	}
	if err := set.Validate(); err != nil {
		t.Fatalf("prism-roadmap set.Validate(): %v", err)
	}
	unprioritized := 0
	for _, item := range set.Items {
		if item.MoSCoW == "" {
			unprioritized++
		}
	}
	t.Logf("converted %d initiatives into a valid RoadmapItemSet (%d unprioritized — MoSCoW optional end-to-end)",
		len(set.Items), unprioritized)
}
