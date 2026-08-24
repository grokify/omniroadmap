package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	aha "github.com/grokify/aha-go"
	ahastudio "github.com/grokify/aha-studio/omniroadmap"
	studiosync "github.com/grokify/aha-studio/sync"
	jira "github.com/grokify/go-atlassian/jira"
	"github.com/grokify/omniroadmap-core/provider"
	productboard "github.com/grokify/productboard-go"
	"github.com/spf13/cobra"

	omniroadmap "github.com/grokify/omniroadmap"
	"github.com/grokify/omniroadmap/fieldmap"
	"github.com/grokify/omniroadmap/store"
	"github.com/grokify/omniroadmap/sync"
)

type storeFlags struct {
	dsn         string
	port        int
	dataDir     string
	startServer bool
}

func (f *storeFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.dsn, "dsn", "", "MySQL-wire DSN for the dolt sql-server (default from ~/.omniroadmap/config.json, else root:@tcp(127.0.0.1:<port>)/omniroadmap)")
	cmd.Flags().IntVar(&f.port, "port", 0, "dolt sql-server port (default from config file, else 13307); ignored when --dsn is set")
	cmd.Flags().StringVar(&f.dataDir, "data-dir", "", "Dolt data directory (default from config file, else ~/.omniroadmap)")
	cmd.Flags().BoolVar(&f.startServer, "start-server", true, "Start a local dolt sql-server if none is reachable")
}

// settings resolves flags > environment > config file > defaults.
func (f *storeFlags) settings() (store.Settings, error) {
	cfg, err := store.LoadConfig()
	if err != nil {
		return store.Settings{}, err
	}
	return cfg.Resolve(f.dsn, f.port, f.dataDir)
}

func (f *storeFlags) open(ctx context.Context) (*store.DoltStore, store.Settings, error) {
	st, err := f.settings()
	if err != nil {
		return nil, store.Settings{}, err
	}
	if f.startServer {
		if err := store.EnsureServer(st.DataDir, st.Port); err != nil {
			return nil, st, err
		}
	}
	if err := store.InitDatabase(st.DSN); err != nil {
		return nil, st, err
	}
	s, err := store.New(st.DSN)
	if err != nil {
		return nil, st, err
	}
	if err := s.Migrate(ctx); err != nil {
		_ = s.Close()
		return nil, st, fmt.Errorf("migrating schema: %w", err)
	}
	return s, st, nil
}

func dbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database management",
	}

	flags := &storeFlags{}
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create the omniroadmap database and schema (starts a local dolt sql-server if needed)",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, st, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()
			fmt.Println("Database initialized:", st.DSN)
			return nil
		},
	}
	flags.register(initCmd)
	cmd.AddCommand(initCmd)
	return cmd
}

func statusCmd() *cobra.Command {
	flags := &storeFlags{}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show last-sync time and record counts per provider and kind",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			entries, err := s.GetSyncMeta(cmd.Context())
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				fmt.Println("No syncs recorded yet. Run: omniroadmap sync --provider <name>")
				return nil
			}
			for _, e := range entries {
				fmt.Printf("%-14s %-12s %6d records  last sync %s\n",
					e.Provider, e.Kind, e.RecordCount, e.LastSync.Format("2006-01-02 15:04:05"))
			}
			return nil
		},
	}
	flags.register(cmd)
	return cmd
}

func syncCmd() *cobra.Command {
	flags := &storeFlags{}
	var (
		providerName string
		fieldmapPath string
		kindsCSV     string
		cacheDBPath  string
		ahaProduct   string
	)

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync a provider into the canonical store",
		Long: `Sync a provider into the canonical store.

Provider credentials come from environment variables:

  aha           AHA_SUBDOMAIN, AHA_API_KEY (optional AHA_PRODUCT for releases/statuses)
  aha-studio    none — reads aha-studio's local SQLite cache (--cache-db,
                default ~/.ahastudio/cache.db); use --product to scope
  productboard  PRODUCTBOARD_API_TOKEN
  jpd           JIRA_URL, JIRA_USER, JIRA_TOKEN (JPD_PROJECT_KEY for the ideas project)

A per-tenant fieldmap config (--fieldmap tenant.json) maps custom fields
onto MoSCoW/RICE — see the fieldmap package docs for the format.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			p, cleanup, err := buildProvider(providerName, cacheDBPath, ahaProduct)
			if err != nil {
				return err
			}
			defer cleanup()

			opts := sync.Options{}
			if fieldmapPath != "" {
				m, err := fieldmap.Load(fieldmapPath)
				if err != nil {
					return err
				}
				opts.Fieldmap = m
			}
			if kindsCSV != "" {
				for _, k := range strings.Split(kindsCSV, ",") {
					opts.Kinds = append(opts.Kinds, provider.ItemKind(strings.TrimSpace(k)))
				}
			}

			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			report, err := sync.Run(cmd.Context(), s, p, opts)
			if err != nil {
				return err
			}

			fmt.Printf("Synced %s:\n", report.Provider)
			for kind, n := range report.ItemCounts {
				fmt.Printf("  %-12s %d\n", kind, n)
			}
			if report.ReleaseCount > 0 {
				fmt.Printf("  %-12s %d\n", "releases", report.ReleaseCount)
			}
			if report.CustomFieldDefCount > 0 {
				fmt.Printf("  %-12s %d\n", "field defs", report.CustomFieldDefCount)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&providerName, "provider", "", "Provider to sync: "+strings.Join(omniroadmap.RegisteredProviders(), ", "))
	cmd.Flags().StringVar(&fieldmapPath, "fieldmap", "", "Per-tenant fieldmap JSON (custom fields -> MoSCoW/RICE)")
	cmd.Flags().StringVar(&kindsCSV, "kinds", "", "Comma-separated item kinds to sync (default: all supported)")
	cmd.Flags().StringVar(&cacheDBPath, "cache-db", "", "aha-studio cache DB path (aha-studio provider; default ~/.ahastudio/cache.db)")
	cmd.Flags().StringVar(&ahaProduct, "product", "", "Aha product/workspace scope (aha-studio provider)")
	flags.register(cmd)
	_ = cmd.MarkFlagRequired("provider")
	return cmd
}

// buildProvider constructs a provider by name from environment-variable
// credentials (or the local cache for aha-studio). The returned cleanup
// closes provider-owned resources and is always safe to call.
func buildProvider(name, cacheDBPath, product string) (provider.Provider, func(), error) {
	noop := func() {}
	switch name {
	case "aha":
		client, err := aha.NewClient() // reads AHA_SUBDOMAIN / AHA_API_KEY
		if err != nil {
			return nil, noop, fmt.Errorf("aha client: %w", err)
		}
		p, err := omniroadmap.NewProvider(name, client)
		return p, noop, err

	case "aha-studio":
		if cacheDBPath == "" {
			cacheDBPath = studiosync.DefaultDBPath()
		}
		db, err := studiosync.Open(cacheDBPath)
		if err != nil {
			return nil, noop, fmt.Errorf("opening aha-studio cache %s: %w", cacheDBPath, err)
		}
		var opts []ahastudio.Option
		if product != "" {
			opts = append(opts, ahastudio.WithProduct(product))
		}
		return ahastudio.NewProvider(db, opts...), func() { _ = db.Close() }, nil

	case "productboard":
		client, err := productboard.NewClient() // reads PRODUCTBOARD_API_TOKEN
		if err != nil {
			return nil, noop, fmt.Errorf("productboard client: %w", err)
		}
		p, err := omniroadmap.NewProvider(name, client)
		return p, noop, err

	case "jpd":
		serverURL := os.Getenv("JIRA_URL")
		user := os.Getenv("JIRA_USER")
		token := os.Getenv("JIRA_TOKEN")
		if serverURL == "" || user == "" || token == "" {
			return nil, noop, fmt.Errorf("jpd provider requires JIRA_URL, JIRA_USER, and JIRA_TOKEN")
		}
		client, err := jira.NewClientFromBasicAuth(serverURL, user, token, false)
		if err != nil {
			return nil, noop, fmt.Errorf("jira client: %w", err)
		}
		p, err := omniroadmap.NewProvider(name, client)
		return p, noop, err

	default:
		return nil, noop, fmt.Errorf("unknown provider %q (registered: %s)",
			name, strings.Join(omniroadmap.RegisteredProviders(), ", "))
	}
}
