package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/grokify/omniroadmap/analyticsdashboards"
)

func analyticsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analytics",
		Short: "Analytics catalog and dashboard commands",
	}
	cmd.AddCommand(exportDashboardsCmd())
	return cmd
}

func exportDashboardsCmd() *cobra.Command {
	flags := &storeFlags{}
	cmd := &cobra.Command{
		Use:   "export-dashboards <dir>",
		Short: "Export the curated DashForge dashboard pack (dashboards + saved questions) as JSON",
		Long: "Writes dashboards.json to <dir>, containing every curated dashboard " +
			"(compass-rice-all, moscow-compass-rice, one compass-profile-<slug> per " +
			"COMPASS-RICE profile present in the current assessment corpus, and " +
			"portfolio-overview) plus the saved questions their widgets reference -- " +
			"the same document served at /api/analytics/dashboards by `omniroadmap ui`. " +
			"Import it into a standalone dashforge-server to reproduce the same " +
			"dashboards there (the Splunk \"app\" distribution model).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			pack, err := analyticsdashboards.BuildFromStore(cmd.Context(), s, time.Now())
			if err != nil {
				return fmt.Errorf("building dashboard pack: %w", err)
			}

			dir := args[0]
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("creating %s: %w", dir, err)
			}
			data, err := json.MarshalIndent(pack, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling dashboard pack: %w", err)
			}
			path := filepath.Join(dir, "dashboards.json")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				return fmt.Errorf("writing %s: %w", path, err)
			}
			fmt.Printf("Exported %d dashboards and %d questions to %s\n", len(pack.Dashboards), len(pack.Questions), path)
			return nil
		},
	}
	flags.register(cmd)
	return cmd
}
