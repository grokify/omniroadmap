// profileCmd manages the two-phase (LLM-proposed, PM-confirmed) COMPASS-RICE
// profile assignment lifecycle -- thin wrappers over compassbridge/review;
// see review.EditProfile for the persistence contract.
package main

import (
	"fmt"
	"time"

	"github.com/ProductBuildersHQ/compass-rice/rice"
	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/prism-roadmap/assessment"
	"github.com/spf13/cobra"

	"github.com/grokify/omniroadmap/compassbridge"
	"github.com/grokify/omniroadmap/review"
)

func profileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Manage the two-phase COMPASS-RICE profile assignment lifecycle (propose, confirm, reject)",
	}
	cmd.AddCommand(profileListCmd(), profileProposeCmd(), profileConfirmCmd(), profileRejectCmd())
	return cmd
}

func profileListCmd() *cobra.Command {
	flags := &storeFlags{}
	var statusFilter string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List profile assignments, optionally filtered by status",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			all, err := s.ListProfileAssignments(cmd.Context())
			if err != nil {
				return err
			}
			var filtered []assessment.ProfileAssignment
			for _, p := range all {
				if statusFilter != "" && string(p.Status) != statusFilter {
					continue
				}
				filtered = append(filtered, p)
			}

			if jsonOutput {
				return printJSON(cmd, filtered)
			}
			w := newTabWriter(cmd)
			fmt.Fprintln(w, "SPEC ID\tPROFILE\tSTATUS\tPROPOSED BY\tCONFIRMED BY")
			for _, p := range filtered {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", p.SpecID, p.ProfileID, p.Status, p.ProposedBy, dashIfEmpty(p.ConfirmedBy))
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&statusFilter, "status", "", "Filter by status: proposed, confirmed, rejected")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON instead of a table")
	flags.register(cmd)
	return cmd
}

func profileProposeCmd() *cobra.Command {
	flags := &storeFlags{}
	var specID, profileID, rationale, proposedBy string

	cmd := &cobra.Command{
		Use:   "propose",
		Short: "Propose a primary COMPASS-RICE investment thesis for an opportunity (the judge phase)",
		Long: `Records a proposed profile assignment -- the judge phase of the two-phase
workflow. A PM must separately run "profile confirm" (or "profile reject")
before this ProfileID is trusted for scoring; compile excludes any
opportunity whose Compass.ProfileID doesn't have a matching confirmed
assignment.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if specID == "" {
				return fmt.Errorf("--spec-id is required")
			}
			if profileID == "" {
				return fmt.Errorf("--profile is required")
			}
			if rationale == "" {
				return fmt.Errorf("--rationale is required")
			}
			if proposedBy == "" {
				return fmt.Errorf("--by is required")
			}

			p, err := compassbridge.ProposeProfile(specID, rice.ProfileID(profileID), rationale, proposedBy)
			if err != nil {
				return err
			}
			return applyProfileEdit(cmd, flags, p, "propose")
		},
	}
	cmd.Flags().StringVar(&specID, "spec-id", "", "Opportunity spec ID (required)")
	cmd.Flags().StringVar(&profileID, "profile", "", "COMPASS-RICE profile ID, e.g. customer/b2b/v1 (required)")
	cmd.Flags().StringVar(&rationale, "rationale", "", "Why this profile is the primary investment thesis (required)")
	cmd.Flags().StringVar(&proposedBy, "by", "", "Identity of the judge proposing this (required, e.g. a model/session name)")
	flags.register(cmd)
	return cmd
}

func profileConfirmCmd() *cobra.Command {
	flags := &storeFlags{}
	var specID, confirmedBy string

	cmd := &cobra.Command{
		Use:   "confirm",
		Short: "Confirm a proposed profile assignment (the PM phase)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if specID == "" {
				return fmt.Errorf("--spec-id is required")
			}
			if confirmedBy == "" {
				return fmt.Errorf("--by is required")
			}

			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			current, err := s.GetProfileAssignment(cmd.Context(), specID)
			if err != nil {
				if omniroadmap.IsNotFound(err) {
					return fmt.Errorf("no profile assignment proposed yet for spec %q -- run `omniroadmap profile propose` first", specID)
				}
				return err
			}

			confirmed, err := compassbridge.ConfirmProfile(*current, confirmedBy, time.Now())
			if err != nil {
				return err
			}
			return saveProfileEdit(cmd, s, confirmed, "confirm")
		},
	}
	cmd.Flags().StringVar(&specID, "spec-id", "", "Opportunity spec ID (required)")
	cmd.Flags().StringVar(&confirmedBy, "by", "", "Identity of the PM confirming this (required)")
	flags.register(cmd)
	return cmd
}

func profileRejectCmd() *cobra.Command {
	flags := &storeFlags{}
	var specID, rejectedBy, rationale string

	cmd := &cobra.Command{
		Use:   "reject",
		Short: "Reject a proposed profile assignment, recording why (the PM phase)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if specID == "" {
				return fmt.Errorf("--spec-id is required")
			}
			if rejectedBy == "" {
				return fmt.Errorf("--by is required")
			}
			if rationale == "" {
				return fmt.Errorf("--rationale is required (e.g. the judge proposed the wrong profile)")
			}

			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			current, err := s.GetProfileAssignment(cmd.Context(), specID)
			if err != nil {
				if omniroadmap.IsNotFound(err) {
					return fmt.Errorf("no profile assignment proposed yet for spec %q", specID)
				}
				return err
			}

			rejected, err := compassbridge.RejectProfile(*current, rejectedBy, time.Now(), rationale)
			if err != nil {
				return err
			}
			return saveProfileEdit(cmd, s, rejected, "reject")
		},
	}
	cmd.Flags().StringVar(&specID, "spec-id", "", "Opportunity spec ID (required)")
	cmd.Flags().StringVar(&rejectedBy, "by", "", "Identity of the PM rejecting this (required)")
	cmd.Flags().StringVar(&rationale, "rationale", "", "Why this proposal is being rejected (required)")
	flags.register(cmd)
	return cmd
}

// applyProfileEdit opens the store, persists p through the review gate,
// and prints the result -- shared by profileProposeCmd (which doesn't
// already hold an open store).
func applyProfileEdit(cmd *cobra.Command, flags *storeFlags, p assessment.ProfileAssignment, verb string) error {
	s, _, err := flags.open(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()
	return saveProfileEdit(cmd, s, p, verb)
}

func saveProfileEdit(cmd *cobra.Command, s profileEditStore, p assessment.ProfileAssignment, verb string) error {
	ctx := cmd.Context()
	if err := review.Apply(ctx, s, review.Edit{Kind: review.EditProfile, Profile: &p}); err != nil {
		return err
	}
	if err := s.Commit(ctx, fmt.Sprintf("omniroadmap profile %s: %s -> %s", verb, p.SpecID, p.ProfileID)); err != nil {
		return err
	}
	return printJSON(cmd, p)
}
