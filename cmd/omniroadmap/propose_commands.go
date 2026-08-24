package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	omnisignalcache "github.com/grokify/aha-studio/omnisignalcache"
	studiosync "github.com/grokify/aha-studio/sync"
	"github.com/grokify/omniroadmap/omnisignalbridge"
	"github.com/plexusone/omnisignal"
	"github.com/plexusone/omnisignal/consolidate"
	sqlitestore "github.com/plexusone/omnisignal/store/sqlite"
	"github.com/spf13/cobra"
)

// openAIChatClient implements consolidate.LLMClient against any
// OpenAI-compatible /chat/completions endpoint, configured via
// OMNISIGNAL_LLM_API_KEY / OMNISIGNAL_LLM_BASE_URL. A small, self-contained
// client rather than a new SDK dependency, since RootCauseSynthesizer only
// needs one method.
type openAIChatClient struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func newOpenAIChatClient() (*openAIChatClient, error) {
	apiKey := os.Getenv("OMNISIGNAL_LLM_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OMNISIGNAL_LLM_API_KEY is required for the root-causes track")
	}
	baseURL := os.Getenv("OMNISIGNAL_LLM_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &openAIChatClient{apiKey: apiKey, baseURL: baseURL, http: &http.Client{Timeout: 60 * time.Second}}, nil
}

func (c *openAIChatClient) Complete(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	if model == "" {
		model = "gpt-4o-mini"
	}
	body, err := json.Marshal(map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	})
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("calling LLM: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM returned status %d", resp.StatusCode)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("LLM response had no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func proposeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "propose",
		Short: "Synthesize proposed opportunities from omnisignal RootCauses and curated Idea signals",
	}
	cmd.AddCommand(proposeRootCausesCmd(), proposeIdeasCmd())
	return cmd
}

func proposeRootCausesCmd() *cobra.Command {
	flags := &storeFlags{}
	var (
		omnisignalDB string
		embeddingDim int
		model        string
	)

	cmd := &cobra.Command{
		Use:   "root-causes",
		Short: "Synthesize proposed opportunities from eligible omnisignal RootCauses (fix track, LLM-drafted from evidence)",
		Long: `Synthesizes a proposed opportunity for every eligible RootCause in the
omnisignal SQLite store (store/sqlite): an LLM drafts a solution proposal
from the RootCause's own evidence (title, symptoms, impact) -- no
codebase access -- and the result is persisted as a canvas.OpportunitySpec
+ assessment.OpportunityAssessment in omniroadmap.

Requires OMNISIGNAL_LLM_API_KEY (and optionally OMNISIGNAL_LLM_BASE_URL
for a non-OpenAI-compatible endpoint).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			llmClient, err := newOpenAIChatClient()
			if err != nil {
				return err
			}

			sigStore, err := sqlitestore.Open(omnisignalDB, sqlitestore.WithEmbeddingDimension(embeddingDim))
			if err != nil {
				return fmt.Errorf("opening omnisignal store at %s: %w", omnisignalDB, err)
			}
			defer func() { _ = sigStore.Close() }()

			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			rootSynth := omnisignalbridge.NewRootCauseSynthesizer(llmClient, omnisignalbridge.RootCauseSynthesizerConfig{Model: model})
			bridge := omnisignalbridge.NewBridge(rootSynth, nil, s)

			rootCauses, err := sigStore.ListRootCauses(cmd.Context(), consolidate.RootCauseFilter{})
			if err != nil {
				return fmt.Errorf("listing root causes: %w", err)
			}

			var synthesized, skipped int
			for _, rc := range rootCauses {
				if !omnisignalbridge.IsEligibleRootCause(rc) {
					skipped++
					continue
				}
				if err := bridge.SynthesizeRootCause(cmd.Context(), rc); err != nil {
					return fmt.Errorf("synthesizing root cause %s: %w", rc.ID, err)
				}
				synthesized++
				fmt.Printf("  proposed: %s (from root cause %s)\n", rc.Title, rc.ID)
			}

			fmt.Printf("Synthesized %d proposed opportunities (%d root causes skipped as ineligible)\n", synthesized, skipped)
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().StringVar(&omnisignalDB, "omnisignal-db", "", "Path to the omnisignal SQLite store (store/sqlite)")
	cmd.Flags().IntVar(&embeddingDim, "embedding-dim", 1536, "Embedding dimension the omnisignal store was opened with (unused by this command, but required to open the store)")
	cmd.Flags().StringVar(&model, "model", "gpt-4o-mini", "LLM model for solution drafting")
	_ = cmd.MarkFlagRequired("omnisignal-db")
	return cmd
}

func proposeIdeasCmd() *cobra.Command {
	flags := &storeFlags{}
	var (
		cacheDBPath string
		ahaProduct  string
	)

	cmd := &cobra.Command{
		Use:   "ideas",
		Short: "Synthesize proposed opportunities from curated Idea signals (deterministic, no LLM)",
		Long: `Reads curated enhancement-request signals from aha-studio's local
SQLite cache via the cache-backed omnisignal provider
(aha-studio/omnisignalcache), and synthesizes a proposed opportunity for
each one -- deterministic, no LLM call, since an Idea's own text already
is the proposed solution.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cacheDB, err := studiosync.Open(cacheDBPath)
			if err != nil {
				return fmt.Errorf("opening aha-studio cache at %s: %w", cacheDBPath, err)
			}
			defer func() { _ = cacheDB.Close() }()

			s, _, err := flags.open(cmd.Context())
			if err != nil {
				return err
			}
			defer func() { _ = s.Close() }()

			provider := omnisignalcache.NewProvider(cacheDB, omnisignalcache.WithProduct(ahaProduct))
			signals, err := provider.Fetch(cmd.Context(), omnisignal.FetchOptions{})
			if err != nil {
				return fmt.Errorf("fetching idea signals: %w", err)
			}

			ideaSynth := omnisignalbridge.NewIdeaSynthesizer()
			bridge := omnisignalbridge.NewBridge(nil, ideaSynth, s)

			var synthesized, skipped int
			for _, sig := range signals {
				if !ideaSynth.Qualifies(sig) {
					skipped++
					continue
				}
				if err := bridge.SynthesizeIdea(cmd.Context(), sig); err != nil {
					return fmt.Errorf("synthesizing idea %s: %w", sig.ID, err)
				}
				synthesized++
				fmt.Printf("  proposed: %s (from idea %s)\n", sig.Summary, sig.ID)
			}

			fmt.Printf("Synthesized %d proposed opportunities (%d signals skipped as non-qualifying)\n", synthesized, skipped)
			return nil
		},
	}
	flags.register(cmd)
	cmd.Flags().StringVar(&cacheDBPath, "cache-db", studiosync.DefaultDBPath(), "aha-studio cache DB path")
	cmd.Flags().StringVar(&ahaProduct, "product", "", "Aha product to scope idea signals to")
	_ = cmd.MarkFlagRequired("product")
	return cmd
}
