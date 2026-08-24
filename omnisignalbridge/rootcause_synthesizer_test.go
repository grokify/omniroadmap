package omnisignalbridge

import (
	"context"
	"testing"
	"time"

	"github.com/plexusone/signal-spec/pkg/rootcause"
)

type fakeLLMClient struct {
	response                                 string
	err                                      error
	gotModel, gotSystemPrompt, gotUserPrompt string
}

func (f *fakeLLMClient) Complete(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	f.gotModel = model
	f.gotSystemPrompt = systemPrompt
	f.gotUserPrompt = userPrompt
	if f.err != nil {
		return "", f.err
	}
	return f.response, nil
}

func testRootCause() rootcause.RootCause {
	return rootcause.RootCause{
		ID:          "rc-1",
		Title:       "SSO login failures",
		Description: "OAuth token refresh regresses after auth-service deploys.",
		Status:      rootcause.StatusActive,
		SymptomPatterns: []string{
			"login timeout",
			"invalid token",
		},
		SignalIDs: []string{"sig-1", "sig-2", "sig-3"},
		Impact: rootcause.ImpactMetrics{
			SignalCount:       3,
			AffectedCustomers: 2,
		},
		FirstSeen: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		LastSeen:  time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
	}
}

const fakeLLMResponse = `Solution: Serialize token refresh
Description: Add a mutex around the token refresh path to avoid the race
condition triggering re-auth.
Rationale: Directly addresses the root cause of concurrent refresh
requests invalidating each other's tokens.`

func TestRootCauseSynthesizer_Synthesize(t *testing.T) {
	client := &fakeLLMClient{response: fakeLLMResponse}
	synth := NewRootCauseSynthesizer(client, RootCauseSynthesizerConfig{Model: "gpt-4o-mini"})

	proposal, err := synth.Synthesize(context.Background(), testRootCause())
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}

	if proposal.Title != "Serialize token refresh" {
		t.Errorf("Title = %q, want %q", proposal.Title, "Serialize token refresh")
	}
	if proposal.Description == "" {
		t.Error("Description is empty")
	}
	if proposal.Rationale == "" {
		t.Error("Rationale is empty")
	}
	if proposal.SourceKind != SourceKindRootCause {
		t.Errorf("SourceKind = %q, want %q", proposal.SourceKind, SourceKindRootCause)
	}
	if proposal.SourceID != "rc-1" {
		t.Errorf("SourceID = %q, want rc-1", proposal.SourceID)
	}
	if len(proposal.EvidenceSignalIDs) != 3 {
		t.Errorf("EvidenceSignalIDs = %v, want 3 signal IDs", proposal.EvidenceSignalIDs)
	}

	if client.gotModel != "gpt-4o-mini" {
		t.Errorf("model passed to LLM = %q, want gpt-4o-mini", client.gotModel)
	}
	if client.gotSystemPrompt != DefaultRootCauseSynthesizerPrompt {
		t.Error("system prompt should default to DefaultRootCauseSynthesizerPrompt")
	}
	if client.gotUserPrompt == "" {
		t.Error("user prompt is empty")
	}
}

func TestRootCauseSynthesizer_FallsBackToRootCauseTitle(t *testing.T) {
	client := &fakeLLMClient{response: "Description: just a description, no Solution: line"}
	synth := NewRootCauseSynthesizer(client, RootCauseSynthesizerConfig{})

	rc := testRootCause()
	proposal, err := synth.Synthesize(context.Background(), rc)
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if proposal.Title != rc.Title {
		t.Errorf("Title = %q, want fallback to RootCause.Title %q", proposal.Title, rc.Title)
	}
}

func TestRootCauseSynthesizer_LLMError(t *testing.T) {
	client := &fakeLLMClient{err: context.DeadlineExceeded}
	synth := NewRootCauseSynthesizer(client, RootCauseSynthesizerConfig{})

	if _, err := synth.Synthesize(context.Background(), testRootCause()); err == nil {
		t.Fatal("expected error when LLM call fails")
	}
}

func TestIsEligibleRootCause(t *testing.T) {
	tests := []struct {
		status rootcause.Status
		want   bool
	}{
		{rootcause.StatusNew, true},
		{rootcause.StatusActive, true},
		{rootcause.StatusResolved, true},
		{rootcause.StatusArchived, false},
	}
	for _, tt := range tests {
		rc := testRootCause()
		rc.Status = tt.status
		if got := IsEligibleRootCause(rc); got != tt.want {
			t.Errorf("IsEligibleRootCause(status=%s) = %v, want %v", tt.status, got, tt.want)
		}
	}
}
