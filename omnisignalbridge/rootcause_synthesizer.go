package omnisignalbridge

import (
	"context"
	"fmt"
	"strings"

	"github.com/plexusone/omnisignal/consolidate"
	"github.com/plexusone/signal-spec/pkg/rootcause"
)

// DefaultRootCauseSynthesizerPrompt drafts a solution proposal from a
// RootCause's own evidence -- no repo/codebase access. Mirrors
// consolidate.DefaultSummarizerPrompt's plain-text section-header format.
const DefaultRootCauseSynthesizerPrompt = `You are a product engineer proposing a fix for a known root cause. Given a root cause's title, description, symptom patterns, and impact, draft a proposed solution. You do not have access to the affected codebase -- work only from the evidence given.

Output format:
- Solution: A brief, actionable solution title (max 100 chars)
- Description: 2-3 sentences describing what the solution does
- Rationale: 1-2 sentences on why this addresses the underlying root cause

Focus on addressing the root cause, not just the symptoms.`

// RootCauseSynthesizerConfig configures an LLM-backed RootCauseSynthesizer.
type RootCauseSynthesizerConfig struct {
	// Model is the LLM model to use.
	Model string

	// SystemPrompt customizes the synthesis behavior. If empty, uses
	// DefaultRootCauseSynthesizerPrompt.
	SystemPrompt string
}

// RootCauseSynthesizer drafts a SolutionProposal for an approved RootCause
// using an LLM, working only from the RootCause's own evidence (title,
// description, symptom patterns, impact) -- no codebase/repo access. See
// package doc comment for why deeper agentic code analysis is out of scope
// here.
type RootCauseSynthesizer struct {
	client consolidate.LLMClient
	config RootCauseSynthesizerConfig
}

// NewRootCauseSynthesizer creates a RootCauseSynthesizer backed by client,
// reusing consolidate.LLMClient rather than redefining an equivalent
// interface.
func NewRootCauseSynthesizer(client consolidate.LLMClient, cfg RootCauseSynthesizerConfig) *RootCauseSynthesizer {
	if cfg.SystemPrompt == "" {
		cfg.SystemPrompt = DefaultRootCauseSynthesizerPrompt
	}
	return &RootCauseSynthesizer{client: client, config: cfg}
}

// Synthesize drafts a SolutionProposal for rc. Callers are responsible for
// only passing RootCauses that qualify (this package doesn't gate on review/
// approval status itself -- see BuildRootCauseFilter for the recommended
// gate).
func (s *RootCauseSynthesizer) Synthesize(ctx context.Context, rc rootcause.RootCause) (SolutionProposal, error) {
	prompt := s.buildPrompt(rc)

	response, err := s.client.Complete(ctx, s.config.Model, s.config.SystemPrompt, prompt)
	if err != nil {
		return SolutionProposal{}, fmt.Errorf("LLM completion: %w", err)
	}

	title, description, rationale := parseSolutionResponse(response)
	if title == "" {
		title = rc.Title
	}

	return SolutionProposal{
		Title:             title,
		Description:       description,
		Rationale:         rationale,
		SourceKind:        SourceKindRootCause,
		SourceID:          rc.ID,
		EvidenceSignalIDs: rc.SignalIDs,
	}, nil
}

func (s *RootCauseSynthesizer) buildPrompt(rc rootcause.RootCause) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Root cause: %s\n", rc.Title)
	if rc.Description != "" {
		fmt.Fprintf(&b, "Description: %s\n", rc.Description)
	}
	if len(rc.SymptomPatterns) > 0 {
		fmt.Fprintf(&b, "Symptom patterns: %s\n", strings.Join(rc.SymptomPatterns, "; "))
	}
	fmt.Fprintf(&b, "Impact: %d signals, %d affected customers\n", rc.Impact.SignalCount, rc.Impact.AffectedCustomers)
	if rc.Impact.EscalationRate > 0 {
		fmt.Fprintf(&b, "Escalation rate: %.0f%%\n", rc.Impact.EscalationRate*100)
	}
	return b.String()
}

// parseSolutionResponse extracts the "Solution:"/"Description:"/
// "Rationale:" sections from a plain-text LLM response, mirroring
// consolidate's extractSections convention (label-prefixed lines, not JSON).
func parseSolutionResponse(response string) (title, description, rationale string) {
	var descLines, rationaleLines []string
	section := ""

	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(strings.ToLower(trimmed), "solution:"):
			title = strings.TrimSpace(trimmed[len("solution:"):])
			section = "solution"
		case strings.HasPrefix(strings.ToLower(trimmed), "description:"):
			descLines = append(descLines, strings.TrimSpace(trimmed[len("description:"):]))
			section = "description"
		case strings.HasPrefix(strings.ToLower(trimmed), "rationale:"):
			rationaleLines = append(rationaleLines, strings.TrimSpace(trimmed[len("rationale:"):]))
			section = "rationale"
		case trimmed == "":
			// skip blank lines
		default:
			switch section {
			case "description":
				descLines = append(descLines, trimmed)
			case "rationale":
				rationaleLines = append(rationaleLines, trimmed)
			}
		}
	}

	return title, strings.Join(descLines, " "), strings.Join(rationaleLines, " ")
}
