package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/grokify/omniroadmap/store"
)

// configCmd shows and edits the store configuration
// (~/.omniroadmap/config.json).
func configCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show resolved store configuration and where each value comes from",
		Long: `Show the resolved store configuration.

Configuration lives at ~/.omniroadmap/config.json (a .config.json variant
is also accepted). Values resolve in order:
CLI flag > environment variable (OMNIROADMAP_DSN/PORT/DATA_DIR) >
config file > built-in default.

Set a distinct port when running several Dolt-backed tools side by side
(e.g. visionstudio's dolt sql-server on 13306).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := store.ConfigPath()
			cfg, err := store.LoadConfig()
			if err != nil {
				return err
			}
			st, err := cfg.Resolve("", 0, "")
			if err != nil {
				return err
			}

			status := "not present (using defaults)"
			if _, statErr := os.Stat(path); statErr == nil {
				status = "loaded"
			}
			fmt.Printf("Config file: %s (%s)\n\n", path, status)
			fmt.Printf("  %-9s %-45s (from %s)\n", "dsn", st.DSN, st.DSNSource)
			fmt.Printf("  %-9s %-45d (from %s)\n", "port", st.Port, st.PortSource)
			fmt.Printf("  %-9s %-45s (from %s)\n", "data-dir", st.DataDir, st.DataDirSource)
			return nil
		},
	}
	cmd.AddCommand(configSetPortCmd())
	return cmd
}

func configSetPortCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set-port <port>",
		Short: "Persist the dolt sql-server port to the config file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var port int
			if _, err := fmt.Sscanf(args[0], "%d", &port); err != nil || port <= 0 || port > 65535 {
				return fmt.Errorf("invalid port %q", args[0])
			}
			cfg, err := store.LoadConfig()
			if err != nil {
				return err
			}
			cfg.Port = port
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Printf("Saved port %d to %s\n", port, store.ConfigPath())
			return nil
		},
	}
}
