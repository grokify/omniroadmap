// Package dashforgeconnector exposes the OmniRoadmap store as a DashForge
// analytics source. DashForge's engine core is connector-free by design
// (ADR-0001 in plexusone/dashforge): consuming application binaries register
// connectors via the public analytics package. Importing this package (blank
// import for its init registration) is what makes the "omniroadmap" connector
// available — see cmd/omniroadmap-server for the composed server binary.
//
// The dependency direction is omniroadmap -> dashforge, never the reverse.
// This package is the interim connector home until the compass app owns the
// composition.
package dashforgeconnector

import (
	"context"
	"fmt"

	"github.com/plexusone/dashforge/analytics"
	"github.com/plexusone/dashforge/dashboardir"

	"github.com/grokify/omniroadmap/analyticscatalog"
	"github.com/grokify/omniroadmap/analyticsquery"
	"github.com/grokify/omniroadmap/store"
)

// ConnectorName is the DashForge connector-registry name.
const ConnectorName = "omniroadmap"

func init() {
	analytics.RegisterConnector(ConnectorName, func(dsn string) (analytics.QueryProvider, error) {
		return New(dsn)
	})
}

// Provider exposes an OmniRoadmap store as a DashForge analytics source.
type Provider struct {
	store *store.DoltStore
}

// New opens an OmniRoadmap store by DSN.
func New(dsn string) (*Provider, error) {
	if dsn == "" {
		return nil, fmt.Errorf("omniroadmap DSN is required")
	}
	s, err := store.New(dsn)
	if err != nil {
		return nil, fmt.Errorf("opening omniroadmap store: %w", err)
	}
	return &Provider{store: s}, nil
}

// Catalog returns the OmniRoadmap analytics catalog.
func (p *Provider) Catalog(ctx context.Context) (dashboardir.AnalyticsCatalog, error) {
	return analyticscatalog.BuildFromStore(ctx, p.store)
}

// Query executes a read-only analytics query against OmniRoadmap.
func (p *Provider) Query(ctx context.Context, req dashboardir.AnalyticsQueryRequest) (dashboardir.AnalyticsQueryResult, error) {
	return analyticsquery.Execute(ctx, p.store, req)
}

// Close closes the underlying OmniRoadmap store.
func (p *Provider) Close() error {
	if p == nil || p.store == nil {
		return nil
	}
	return p.store.Close()
}
