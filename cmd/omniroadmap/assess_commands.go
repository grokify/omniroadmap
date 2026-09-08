// assessCmd is the scoring workflow surface for COMPASS-RICE: list/show
// the current assessment corpus, import an LLM judge's output, or set
// human-entered evidence directly. All are thin wrappers over
// compassbridge/store -- the actual scoring/normalization/gating logic
// lives in the library layer (compassbridge, prism-roadmap/assessment,
// omniroadmap/compile), not here.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ProductBuildersHQ/compass-rice/catalog"
	"github.com/ProductBuildersHQ/compass-rice/judge"
	"github.com/ProductBuildersHQ/compass-rice/rice"
	"github.com/grokify/prism-roadmap/assessment"
	"github.com/plexusone/structured-evaluation/claims"
	"github.com/plexusone/structured-evaluation/rubric"
	"github.com/spf13/cobra"

	"github.com/grokify/omniroadmap/compassbridge"
	"github.com/grokify/omniroadmap/store"
)

func assessCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assess",
		Short: "View and score opportunity assessments (COMPASS-RICE)",
	}
	cmd.AddCommand(assessListCmd(), assessShowCmd(), assessImportCmd(), assessSetCmd())
	return cmd
}

// currentAssessmentForSpec returns the current cycle for one opportunity
// spec, or nil if none has been recorded yet.
func currentAssessmentForSpec(ctx context.Context, s *store.DoltStore, specID string) (*assessment.OpportunityAssessment, error) {
	cycles, err := s.ListOpportunityAssessments(ctx, specID)
	if err != nil {
		return nil, err
	}
	for i := len(cycles) - 1; i >= 0; i-- {
		if cycles[i].Cycle.Current {
			return &cycles[i], nil
		}
	}
	return nil, nil
}

// attachCompassCycle records c as a new assessment cycle for specID:
// the opportunity's first cycle if none exists yet, otherwise the next
// cycle after its current one (never a mutation of a past cycle).
func attachCompassCycle(ctx context.Context, s *store.DoltStore, specID, title string, assessedAt time.Time, c assessment.CompassAssessment) (*assessment.OpportunityAssessment, error) {
	prior, err := currentAssessmentForSpec(ctx, s, specID)
	if err != nil {
		return nil, err
	}
	if prior == nil {
		if title == "" {
			title = specID
		}
		return compassbridge.FirstCycleWithCompass("OA-"+specID, assessment.OpportunityRef{SpecID: specID}, title, assessedAt, c), nil
	}
	id := fmt.Sprintf("OA-%s-c%d", specID, prior.Cycle.Number+1)
	return compassbridge.NextCycleWithCompass(prior, id, assessedAt, c), nil
}

func parseAssessedAt(s string) (time.Time, error) {
	if s == "" {
		return time.Now(), nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing --assessed-at (want RFC3339): %w", err)
	}
	return t, nil
}

// assessRow is one assess-list row: the "what needs scoring next" summary.
type assessRow struct {
	SpecID  string   `json:"specId"`
	MoSCoW  string   `json:"moscow"`
	Profile string   `json:"profile,omitempty"`
	Status  string   `json:"status"`
	Score   *float64 `json:"score,omitempty"`
}

func toAssessRow(a assessment.OpportunityAssessment) assessRow {
	row := assessRow{SpecID: a.Opportunity.SpecID, MoSCoW: a.MoSCoW().String(), Status: "no-compass"}
	if a.Compass == nil {
		return row
	}
	row.Profile = string(a.Compass.ProfileID)
	result := assessment.ResolveCompassRICE(a.Compass)
	switch {
	case result.Computable:
		row.Status = "computable"
		score := result.Score
		row.Score = &score
	case a.Compass.NeedsHumanReview && a.Compass.HumanReview == nil:
		row.Status = "needs-review"
	default:
		row.Status = "uncomputable"
	}
	return row
}

func assessListCmd() *cobra.Command {
	flags := &storeFlags{}
	var profileFilter, statusFilter string
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List current-cycle assessments with COMPASS profile, MoSCoW, and score/status",
		Long: `Lists the current assessment cycle for every opportunity: MoSCoW tier, COMPASS
profile (if assessed), and status -- computable (has a trustworthy score),
needs-review (a judge output is flagged and awaiting a PM), uncomputable
(the profile isn't confirmed, or the assessment doesn't validate), or
no-compass (not yet assessed at all). This is the "what needs scoring
next" query.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			assessments, err := s.ListCurrentOpportunityAssessments(cmd.Context())
			if err != nil {
				return err
			}

			var rows []assessRow
			for _, a := range assessments {
				row := toAssessRow(a)
				if profileFilter != "" && row.Profile != profileFilter {
					continue
				}
				if statusFilter != "" && row.Status != statusFilter {
					continue
				}
				rows = append(rows, row)
			}

			if jsonOutput {
				return printJSON(cmd, rows)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "SPEC ID\tMOSCOW\tPROFILE\tSTATUS\tSCORE")
			for _, row := range rows {
				profile := row.Profile
				if profile == "" {
					profile = "-"
				}
				scoreStr := "-"
				if row.Score != nil {
					scoreStr = fmt.Sprintf("%.4f", *row.Score)
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", row.SpecID, row.MoSCoW, profile, row.Status, scoreStr)
			}
			return w.Flush()
		},
	}
	cmd.Flags().StringVar(&profileFilter, "profile", "", "Filter by COMPASS profile ID")
	cmd.Flags().StringVar(&statusFilter, "status", "", "Filter by status: computable, needs-review, uncomputable, no-compass")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Output as JSON instead of a table")
	flags.register(cmd)
	return cmd
}

func assessShowCmd() *cobra.Command {
	flags := &storeFlags{}
	cmd := &cobra.Command{
		Use:   "show <spec-id>",
		Short: "Show the current assessment cycle for one opportunity: profile, evidence, score, MoSCoW, review state",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			a, err := currentAssessmentForSpec(cmd.Context(), s, args[0])
			if err != nil {
				return err
			}
			if a == nil {
				return fmt.Errorf("no current assessment recorded for spec %q", args[0])
			}
			return printJSON(cmd, a)
		},
	}
	flags.register(cmd)
	return cmd
}

// importFile is the on-disk document `assess import` reads: an LLM judge's
// output plus the opportunity it scores. Unlike compass-rice's judge.Output
// (which has no JSON tags -- it's meant to be constructed in Go code, per
// the compass-rice integration guide), this is a deliberately file-friendly
// shape with lowercase keys.
type importFile struct {
	SpecID     string                  `json:"specId"`
	Title      string                  `json:"title,omitempty"`
	ProfileID  rice.ProfileID          `json:"profileId"`
	Evidence   json.RawMessage         `json:"evidence"`
	Claims     []*claims.Claim         `json:"claims,omitempty"`
	Categories []rubric.CategoryResult `json:"categories,omitempty"`
}

func assessImportCmd() *cobra.Command {
	flags := &storeFlags{}
	var assessedAtStr string

	cmd := &cobra.Command{
		Use:   "import <file.json>",
		Short: "Ingest an LLM judge's COMPASS-RICE output (specId + profileId + evidence + claims)",
		Long: `Reads a judge output document:

  {"specId": "OPP-1", "title": "...", "profileId": "customer/b2b/v1",
   "evidence": {...profile-specific evidence...}, "claims": [...], "categories": [...]}

runs it through the compass-rice catalog normalizer and a claims-backed
confidence integrity check (compassbridge.Ingest), and records it as a new
assessment cycle. Rejects the document -- with repair prompts, if any are
attached to its categories -- if its evidence claims more confidence than
its verified claims actually support.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			var f importFile
			if err := json.Unmarshal(data, &f); err != nil {
				return fmt.Errorf("parsing %s: %w", args[0], err)
			}
			if f.SpecID == "" {
				return fmt.Errorf("specId is required")
			}

			evidence, err := catalog.UnmarshalEvidenceFor(f.ProfileID, f.Evidence)
			if err != nil {
				return fmt.Errorf("unmarshaling evidence for %q: %w", f.ProfileID, err)
			}

			compassAssessment, err := compassbridge.Ingest(judge.Output{
				ProfileID:  f.ProfileID,
				Evidence:   evidence,
				Categories: f.Categories,
				Claims:     f.Claims,
			})
			if err != nil {
				var ingestErr *compassbridge.IngestError
				if errors.As(err, &ingestErr) && len(ingestErr.RepairPrompts) > 0 {
					fmt.Fprintln(cmd.ErrOrStderr(), "Repair prompts:")
					for _, p := range ingestErr.RepairPrompts {
						fmt.Fprintln(cmd.ErrOrStderr(), "  -", p)
					}
				}
				return err
			}

			assessedAt, err := parseAssessedAt(assessedAtStr)
			if err != nil {
				return err
			}

			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			ctx := cmd.Context()
			next, err := attachCompassCycle(ctx, s, f.SpecID, f.Title, assessedAt, compassAssessment)
			if err != nil {
				return err
			}
			if err := s.SaveOpportunityAssessment(ctx, *next); err != nil {
				return err
			}
			if err := s.Commit(ctx, fmt.Sprintf("omniroadmap assess import: %s", next.ID)); err != nil {
				return err
			}
			return printJSON(cmd, next)
		},
	}
	cmd.Flags().StringVar(&assessedAtStr, "assessed-at", "", "Assessment timestamp (RFC3339, default now)")
	flags.register(cmd)
	return cmd
}

func assessSetCmd() *cobra.Command {
	flags := &storeFlags{}
	var specID, profileID, evidenceFile, enteredBy, assessedAtStr string

	cmd := &cobra.Command{
		Use:   "set",
		Short: "Record a COMPASS-RICE assessment from human-entered evidence (no judge output needed)",
		Long: `Reads a profile's raw evidence JSON from -f/--file, normalizes it directly via
the compass-rice catalog, and records it as a new assessment cycle --
already marked human-reviewed (--by is required), since a human entered it
directly rather than an LLM judge proposing it for review.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if specID == "" {
				return fmt.Errorf("--spec-id is required")
			}
			if profileID == "" {
				return fmt.Errorf("--profile is required")
			}
			if enteredBy == "" {
				return fmt.Errorf("--by is required")
			}
			if evidenceFile == "" {
				return fmt.Errorf("-f/--file is required")
			}

			data, err := os.ReadFile(evidenceFile)
			if err != nil {
				return err
			}
			assessedAt, err := parseAssessedAt(assessedAtStr)
			if err != nil {
				return err
			}

			compassAssessment, err := compassbridge.IngestHumanEvidence(rice.ProfileID(profileID), data, enteredBy, assessedAt)
			if err != nil {
				return err
			}

			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			ctx := cmd.Context()
			next, err := attachCompassCycle(ctx, s, specID, "", assessedAt, compassAssessment)
			if err != nil {
				return err
			}
			if err := s.SaveOpportunityAssessment(ctx, *next); err != nil {
				return err
			}
			if err := s.Commit(ctx, fmt.Sprintf("omniroadmap assess set: %s", next.ID)); err != nil {
				return err
			}
			return printJSON(cmd, next)
		},
	}
	cmd.Flags().StringVar(&specID, "spec-id", "", "Opportunity spec ID (required)")
	cmd.Flags().StringVar(&profileID, "profile", "", "COMPASS-RICE profile ID, e.g. customer/b2b/v1 (required)")
	cmd.Flags().StringVarP(&evidenceFile, "file", "f", "", "Path to the profile's raw evidence JSON (required)")
	cmd.Flags().StringVar(&enteredBy, "by", "", "Identity of the human entering this evidence (required)")
	cmd.Flags().StringVar(&assessedAtStr, "assessed-at", "", "Assessment timestamp (RFC3339, default now)")
	flags.register(cmd)
	return cmd
}
