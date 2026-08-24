// Package store provides the Dolt-backed canonical store for omniroadmap,
// following visionstudio's Dolt wiring: a MySQL-wire connection to a
// `dolt sql-server` (launched as a subprocess if not already running),
// Ent over the MySQL dialect, and Dolt commits wrapped around sync runs.
// Server lifecycle, DSN handling, and commit plumbing delegate to
// github.com/grokify/godolt, the shared Dolt integration module.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/grokify/godolt"

	"github.com/grokify/omniroadmap/ent"
)

const (
	// DefaultPort is the default dolt sql-server port for omniroadmap
	// (distinct from visionstudio's 13306).
	DefaultPort = 13307

	// DefaultDatabase is the Dolt database name.
	DefaultDatabase = "omniroadmap"
)

// DefaultDSN returns the default MySQL-wire DSN for the local dolt
// sql-server.
func DefaultDSN() string {
	return godolt.LocalDSN(DefaultPort, DefaultDatabase)
}

// DefaultDataDir returns the default Dolt data directory
// (~/.omniroadmap).
func DefaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".omniroadmap"
	}
	return filepath.Join(home, ".omniroadmap")
}

// DoltStore is an Ent client over a Dolt database, reached via the MySQL
// wire protocol.
type DoltStore struct {
	client *ent.Client
	db     *sql.DB
	dolt   *godolt.Client
}

// New opens a DoltStore from a MySQL-compatible DSN. The target database
// must already exist — see InitDatabase.
func New(dsn string) (*DoltStore, error) {
	dsn = godolt.EnsureParseTime(dsn)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open database: %w", err)
	}
	drv := entsql.OpenDB(dialect.MySQL, db)
	client := ent.NewClient(ent.Driver(drv))
	return &DoltStore{client: client, db: db, dolt: godolt.New(db)}, nil
}

// Close closes the underlying connections.
func (s *DoltStore) Close() error {
	if err := s.client.Close(); err != nil {
		return err
	}
	return s.db.Close()
}

// Client exposes the Ent client for queries beyond the Store interface.
func (s *DoltStore) Client() *ent.Client {
	return s.client
}

// Migrate creates/updates the schema (plain append-style Schema.Create,
// matching visionstudio).
func (s *DoltStore) Migrate(ctx context.Context) error {
	return s.client.Schema.Create(ctx)
}

// Commit stages and commits all Dolt changes with the given message. A
// clean working set is a no-op.
func (s *DoltStore) Commit(ctx context.Context, message string) error {
	if _, err := s.dolt.CommitAll(ctx, message); err != nil {
		return fmt.Errorf("store: dolt commit: %w", err)
	}
	return nil
}

// InitDatabase connects to the server addressed by dsn without selecting
// its database, creates that database if needed, and returns nil. Use
// before New on a fresh server.
func InitDatabase(dsn string) error {
	base, dbName, err := godolt.SplitDSN(godolt.EnsureParseTime(dsn))
	if err != nil {
		return fmt.Errorf("store: %w", err)
	}
	db, err := sql.Open("mysql", base)
	if err != nil {
		return fmt.Errorf("store: open server: %w", err)
	}
	defer func() { _ = db.Close() }()
	if err := godolt.CreateDatabase(context.Background(), db, dbName); err != nil {
		return fmt.Errorf("store: %w", err)
	}
	return nil
}

// EnsureServer checks that a dolt sql-server is reachable on the given
// port, launching one as a subprocess over dataDir if not. Requires the
// `dolt` binary on PATH when a launch is needed. The launched process is
// detached — it keeps serving after the caller exits, matching
// visionstudio's ensureDoltRunning behavior.
func EnsureServer(dataDir string, port int) error {
	if err := godolt.EnsureServer(dataDir, port); err != nil {
		return fmt.Errorf("store: %w", err)
	}
	return nil
}
