// Package sync pulls data from any omniroadmap provider into a Store,
// applying per-tenant fieldmap enrichment along the way. The engine is
// provider-agnostic and store-agnostic: it works identically against a
// live-API provider (aha, productboard, jpd) or a cache-backed one
// (aha-studio), and against the Dolt store or a test fake.
package sync

import (
	"context"
	"errors"
	"fmt"
	"time"

	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/omniroadmap-core/provider"

	"github.com/grokify/omniroadmap/fieldmap"
)

// defaultPerPage is the page size used when Options.PerPage is unset.
const defaultPerPage = 100

// maxPages bounds pagination as a runaway guard (maxPages × PerPage
// records per kind).
const maxPages = 1000

// Store is the persistence contract the engine writes to. *store.DoltStore
// implements it; tests use a fake.
type Store interface {
	UpsertItems(ctx context.Context, items []provider.Item) error
	UpsertReleases(ctx context.Context, releases []provider.Release) error
	UpsertCustomFieldDefinitions(ctx context.Context, providerName string, defs []provider.CustomFieldDefinition) error
	SetSyncMeta(ctx context.Context, providerName, kind string, lastSync time.Time, count int) error
	// Commit finalizes a sync run (a Dolt commit for the real store; no-op
	// acceptable for others).
	Commit(ctx context.Context, message string) error
}

// Options configures a sync run.
type Options struct {
	// Kinds restricts which item kinds to sync; empty means every kind the
	// provider supports.
	Kinds []provider.ItemKind

	// Fieldmap, when set, maps tenant-specific custom fields onto
	// Item.MoSCoW / Item.RICE before storage.
	Fieldmap *fieldmap.Mapping

	// PerPage is the page size for provider list calls (default 100).
	PerPage int
}

// Report summarizes a completed sync run.
type Report struct {
	Provider            string
	ItemCounts          map[provider.ItemKind]int
	ReleaseCount        int
	CustomFieldDefCount int
}

// TotalItems returns the total item count across kinds.
func (r *Report) TotalItems() int {
	total := 0
	for _, n := range r.ItemCounts {
		total += n
	}
	return total
}

// Run syncs one provider into the store: items per kind (paginated, with
// fieldmap enrichment), releases and custom field definitions when
// supported, sync metadata per kind, and one final Commit.
func Run(ctx context.Context, s Store, p provider.Provider, opts Options) (*Report, error) {
	perPage := opts.PerPage
	if perPage <= 0 {
		perPage = defaultPerPage
	}
	caps := p.Capabilities()

	kinds := opts.Kinds
	if len(kinds) == 0 {
		kinds = caps.Kinds
	}

	report := &Report{
		Provider:   p.Name(),
		ItemCounts: map[provider.ItemKind]int{},
	}

	for _, kind := range kinds {
		items, err := fetchAllItems(ctx, p, kind, perPage)
		if err != nil {
			return nil, fmt.Errorf("sync: listing %s items from %s: %w", kind, p.Name(), err)
		}
		fieldmap.Apply(items, opts.Fieldmap)
		if err := s.UpsertItems(ctx, items); err != nil {
			return nil, fmt.Errorf("sync: storing %s items: %w", kind, err)
		}
		if err := s.SetSyncMeta(ctx, p.Name(), string(kind), time.Now(), len(items)); err != nil {
			return nil, err
		}
		report.ItemCounts[kind] = len(items)
	}

	if caps.SupportsReleases {
		releases, err := fetchAllReleases(ctx, p, perPage)
		if err != nil {
			return nil, fmt.Errorf("sync: listing releases from %s: %w", p.Name(), err)
		}
		if err := s.UpsertReleases(ctx, releases); err != nil {
			return nil, fmt.Errorf("sync: storing releases: %w", err)
		}
		if err := s.SetSyncMeta(ctx, p.Name(), "release", time.Now(), len(releases)); err != nil {
			return nil, err
		}
		report.ReleaseCount = len(releases)
	}

	if caps.SupportsCustomFields {
		resp, err := p.ListCustomFieldDefinitions(ctx, &provider.ListCustomFieldDefinitionsRequest{})
		switch {
		case errors.Is(err, omniroadmap.ErrUnsupportedOperation):
			// Provider has per-record custom fields but can't list
			// definitions (e.g. the aha-studio cache provider) — fine.
		case err != nil:
			return nil, fmt.Errorf("sync: listing custom field definitions from %s: %w", p.Name(), err)
		default:
			if err := s.UpsertCustomFieldDefinitions(ctx, p.Name(), resp.Definitions); err != nil {
				return nil, fmt.Errorf("sync: storing custom field definitions: %w", err)
			}
			report.CustomFieldDefCount = len(resp.Definitions)
		}
	}

	msg := fmt.Sprintf("omniroadmap sync: %s (%d items, %d releases)",
		p.Name(), report.TotalItems(), report.ReleaseCount)
	if err := s.Commit(ctx, msg); err != nil {
		return nil, fmt.Errorf("sync: committing: %w", err)
	}
	return report, nil
}

// fetchAllItems pages through ListItems for one kind. Providers differ in
// pagination behavior (some honor Page/PerPage, some return everything on
// the first call), so paging stops when a page is empty, short, or —
// guarding providers that ignore paging entirely — contributes no new IDs.
func fetchAllItems(ctx context.Context, p provider.Provider, kind provider.ItemKind, perPage int) ([]provider.Item, error) {
	var all []provider.Item
	seen := map[string]bool{}

	for page := 1; page <= maxPages; page++ {
		resp, err := p.ListItems(ctx, &provider.ListItemsRequest{
			Kinds:   []provider.ItemKind{kind},
			Page:    page,
			PerPage: perPage,
		})
		if err != nil {
			return nil, err
		}
		fresh := 0
		for _, item := range resp.Items {
			if seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			all = append(all, item)
			fresh++
		}
		if fresh == 0 || len(resp.Items) < perPage {
			break
		}
	}
	return all, nil
}

// fetchAllReleases pages through ListReleases with the same stopping rules
// as fetchAllItems.
func fetchAllReleases(ctx context.Context, p provider.Provider, perPage int) ([]provider.Release, error) {
	var all []provider.Release
	seen := map[string]bool{}

	for page := 1; page <= maxPages; page++ {
		resp, err := p.ListReleases(ctx, &provider.ListReleasesRequest{
			Page:    page,
			PerPage: perPage,
		})
		if err != nil {
			return nil, err
		}
		fresh := 0
		for _, rel := range resp.Releases {
			if seen[rel.ID] {
				continue
			}
			seen[rel.ID] = true
			all = append(all, rel)
			fresh++
		}
		if fresh == 0 || len(resp.Releases) < perPage {
			break
		}
	}
	return all, nil
}
