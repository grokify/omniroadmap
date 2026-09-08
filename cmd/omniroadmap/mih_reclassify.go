package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/grokify/prism-roadmap/assessment"
	"github.com/spf13/cobra"
)

const (
	mihDimensionID  = "market-investment-horizon"
	mihLegacySamSom = "sam_som"
)

// mihReclassifyCmd is the one-time migration for the Market Investment Horizon
// SOM/SAM split (RMI-PRISMROADMAP-019 / RMI-COMPASS-009). The dimension moved
// from a combined "sam_som" bucket to the formal four-rung ladder
// KTLO < SOM < SAM < TAM; this command finds current opportunity assessments
// still classified "sam_som", asks an LLM to place each on the SOM vs. SAM rung,
// rebuilds the MIH dimension assignment from the now-4-way dimension, and
// re-saves so the store recomputes the mih_category projection.
//
// It is idempotent (only touches "sam_som" rows; a second run finds none) and a
// dry run by default — nothing is written until --apply. Ambiguous initiatives
// are left as "sam_som" (which is exactly the 3-way rollup key) for human review.
//
// LLM access reuses the same OpenAI-compatible client as `propose`
// (OMNISIGNAL_LLM_API_KEY / OMNISIGNAL_LLM_BASE_URL).
func mihReclassifyCmd() *cobra.Command {
	flags := &storeFlags{}
	var (
		apply bool
		limit int
		model string
	)
	cmd := &cobra.Command{
		Use:   "mih-reclassify",
		Short: "Reclassify legacy MIH 'sam_som' assessments into SOM or SAM via LLM",
		Long: "Split the legacy combined Market Investment Horizon bucket 'sam_som' " +
			"into the formal SOM and SAM rungs. Dry run by default; pass --apply to persist.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			s, _, err := flags.open(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			oas, err := s.ListCurrentOpportunityAssessments(ctx)
			if err != nil {
				return fmt.Errorf("listing current assessments: %w", err)
			}

			// Collect the assessments still in the legacy combined bucket.
			type target struct{ oaIdx, dimIdx int }
			var targets []target
			for i := range oas {
				di := mihAssignmentIndex(oas[i].Dimensions)
				if di >= 0 && oas[i].Dimensions[di].Category != nil &&
					oas[i].Dimensions[di].Category.OptionID == mihLegacySamSom {
					targets = append(targets, target{i, di})
				}
			}
			fmt.Printf("Found %d current assessment(s) in legacy MIH bucket %q.\n", len(targets), mihLegacySamSom)
			if len(targets) == 0 {
				fmt.Println("Nothing to reclassify.")
				return nil
			}

			client, err := newOpenAIChatClient()
			if err != nil {
				return err
			}

			def := assessment.MarketInvestmentHorizonDimension()
			n := len(targets)
			if limit > 0 && limit < n {
				n = limit
				fmt.Printf("Processing first %d (--limit).\n", n)
			}

			var toSOM, toSAM, ambiguous, applied int
			for _, t := range targets[:n] {
				oa := &oas[t.oaIdx]
				decision, rationale, cerr := classifyMIH(ctx, client, model, *oa)
				if cerr != nil {
					fmt.Printf("  ! %s: classify error: %v (left as %s)\n", oa.Title, cerr, mihLegacySamSom)
					continue
				}
				if decision != "som" && decision != "sam" {
					ambiguous++
					fmt.Printf("  ~ %s -> ambiguous, left as %s: %s\n", oa.Title, mihLegacySamSom, rationale)
					continue
				}

				// Carry any prior evidence forward onto the rebuilt answers.
				var priorEvidence []string
				for _, ans := range oa.Dimensions[t.dimIdx].Answers {
					priorEvidence = append(priorEvidence, ans.EvidenceIDs...)
				}
				answers := []assessment.DimensionAnswer{
					{OptionID: "som", QuestionID: "captures-obtainable-demand-now", Answer: decision == "som", Rationale: rationale, EvidenceIDs: priorEvidence},
					{OptionID: "sam", QuestionID: "expands-serviceable-reach", Answer: decision == "sam", Rationale: rationale, EvidenceIDs: priorEvidence},
				}
				oa.Dimensions[t.dimIdx] = assessment.NewDimensionAssignment(def, answers)

				if decision == "som" {
					toSOM++
				} else {
					toSAM++
				}
				fmt.Printf("  %s %s -> %s: %s\n", dryOrApply(apply), oa.Title, strings.ToUpper(decision), rationale)

				if apply {
					if err := s.SaveOpportunityAssessment(ctx, *oa); err != nil {
						return fmt.Errorf("saving assessment %q: %w", oa.Title, err)
					}
					applied++
				}
			}

			fmt.Printf("\nMIH reclassification: %d SOM, %d SAM, %d ambiguous (of %d processed).\n",
				toSOM, toSAM, ambiguous, n)
			if apply {
				fmt.Printf("APPLIED — %d assessment(s) updated; mih_category projection recomputed on save.\n", applied)
			} else {
				fmt.Println("DRY RUN — no writes. Re-run with --apply to persist.")
			}
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().BoolVar(&apply, "apply", false, "Persist changes (default is a dry run)")
	cmd.Flags().IntVar(&limit, "limit", 0, "Max assessments to process (0 = all)")
	cmd.Flags().StringVar(&model, "model", "", "LLM model (default gpt-4o-mini)")
	return cmd
}

func dryOrApply(apply bool) string {
	if apply {
		return "[apply]"
	}
	return "[dry]  "
}

func mihAssignmentIndex(dims []assessment.DimensionAssignment) int {
	for i := range dims {
		if dims[i].DimensionID == mihDimensionID {
			return i
		}
	}
	return -1
}

// classifyMIH asks the LLM to place a legacy sam_som initiative on the SOM or
// SAM rung, using the same incremental-ring definitions as the dimension's
// judge questions. Returns "som", "sam", or "ambiguous".
func classifyMIH(ctx context.Context, client *openAIChatClient, model string, oa assessment.OpportunityAssessment) (decision, rationale string, err error) {
	const system = `You classify a product initiative on the Market Investment Horizon ladder, choosing between two adjacent rungs:

- SOM (Serviceable Obtainable Market): the initiative primarily captures demand we can realistically win NOW, within customers and segments we ALREADY reach and sell to.
- SAM (Serviceable Addressable Market): the initiative primarily expands into serviceable segments our business model CAN address but that we DO NOT yet effectively reach or win.

Classify by the furthest-out horizon the initiative opens: harvesting demand within reach we already have is SOM; extending into serviceable reach we do not yet win is SAM. If genuinely indistinguishable, answer "ambiguous".

Respond with STRICT JSON only, no prose: {"category":"som"|"sam"|"ambiguous","rationale":"one sentence"}`

	user := fmt.Sprintf("Initiative title: %s\nOpportunity: %v\n\nClassify as som, sam, or ambiguous.", oa.Title, oa.Opportunity)

	raw, err := client.Complete(ctx, model, system, user)
	if err != nil {
		return "", "", err
	}
	var out struct {
		Category  string `json:"category"`
		Rationale string `json:"rationale"`
	}
	if uerr := json.Unmarshal([]byte(extractJSONObject(raw)), &out); uerr != nil {
		return "", "", fmt.Errorf("parsing LLM response %q: %w", raw, uerr)
	}
	switch cat := strings.ToLower(strings.TrimSpace(out.Category)); cat {
	case "som", "sam", "ambiguous":
		return cat, strings.TrimSpace(out.Rationale), nil
	default:
		return "ambiguous", fmt.Sprintf("unrecognized category %q", out.Category), nil
	}
}

// extractJSONObject returns the first {...} block in s, tolerating markdown
// fences or surrounding prose the model may emit.
func extractJSONObject(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
