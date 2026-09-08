// Command omniroadmap is a thin CLI over the omniroadmap library layer:
// sync providers into the Dolt-backed canonical store, initialize the
// database, and report sync status.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "omniroadmap",
		Short: "Tool-agnostic roadmap/PM data: sync Aha!, ProductBoard, and JPD into a Dolt-backed canonical store",
	}
	root.AddCommand(syncCmd(), dbCmd(), statusCmd(), augmentCmd(), itemCmd(), configCmd(), uiCmd(), proposeCmd(), analyticsCmd(), assessCmd())
	return root
}
