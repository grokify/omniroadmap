package main

import (
	"context"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/grokify/omniroadmap/review"
)

// newTabWriter returns a tabwriter configured the same way across every
// list-style command, writing to cmd's stdout.
func newTabWriter(cmd *cobra.Command) *tabwriter.Writer {
	return tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
}

// dashIfEmpty renders an empty optional field as "-" in table output.
func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// profileEditStore is the persistence surface profile/moscow edit commands
// need: review.Store to apply the edit, plus Commit for the Dolt version
// commit every write command makes (matching `augment set`'s convention).
type profileEditStore interface {
	review.Store
	Commit(ctx context.Context, message string) error
}
