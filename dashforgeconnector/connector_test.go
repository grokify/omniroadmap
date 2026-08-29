package dashforgeconnector

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/plexusone/dashforge/analytics"
	"github.com/plexusone/dashforge/dashboardir"
)

func TestConnectorRegistered(t *testing.T) {
	factory, ok := analytics.LookupConnector(ConnectorName)
	if !ok {
		t.Fatalf("connector %q not registered via init()", ConnectorName)
	}
	if factory == nil {
		t.Fatal("nil factory registered")
	}
}

func TestNewRequiresDSN(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected error for empty DSN")
	}
}

// TestProviderAgainstLocalStore exercises Catalog and Query against a live
// OmniRoadmap dolt sql-server. Set OMNIROADMAP_TEST_DSN, or a local server on
// the conventional 127.0.0.1:13307 is used; skips when neither is reachable.
func TestProviderAgainstLocalStore(t *testing.T) {
	dsn := os.Getenv("OMNIROADMAP_TEST_DSN")
	if dsn == "" {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:13307", 500*time.Millisecond)
		if err != nil {
			t.Skip("no OMNIROADMAP_TEST_DSN and no dolt sql-server on 127.0.0.1:13307")
		}
		_ = conn.Close()
		dsn = "root:@tcp(127.0.0.1:13307)/omniroadmap"
	}

	provider, err := New(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cerr := provider.Close(); cerr != nil {
			t.Errorf("close: %v", cerr)
		}
	}()

	ctx := context.Background()
	catalog, err := provider.Catalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Sources) == 0 {
		t.Fatal("expected at least one catalog source")
	}
	if len(catalog.Sources[0].Datasets) == 0 {
		t.Fatal("expected datasets in the catalog")
	}

	result, err := provider.Query(ctx, dashboardir.AnalyticsQueryRequest{
		SourceID: ConnectorName,
		Query:    "SELECT name FROM initiatives LIMIT 1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Columns) == 0 {
		t.Fatalf("expected columns in query result, got %+v", result)
	}
}
