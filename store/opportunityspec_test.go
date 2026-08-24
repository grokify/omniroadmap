package store

import (
	"fmt"
	"testing"

	"github.com/grokify/prism-roadmap/canvas"
)

func TestDoltStore_OpportunitySpec(t *testing.T) {
	startDoltServer(t)

	dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%d)/omniroadmap_opportunityspec_test", testPort)
	if err := InitDatabase(dsn); err != nil {
		t.Fatalf("InitDatabase: %v", err)
	}
	s, err := New(dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	ctx := t.Context()
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	spec := canvas.NewOpportunitySpec("OPP-1", "Fix SSO login failures")
	spec.UsersAndProblem = canvas.OSUsersAndProblem{
		ProblemStatement: "SSO login fails intermittently for enterprise users after deploy",
		Evidence:         "12 support tickets over 30 days, 3 distinct customer accounts",
	}
	spec.SolutionIdeas = canvas.OSSolutionIdeas{
		Ideas: []canvas.OSSolutionIdea{
			{ID: "idea-1", Name: "Fix token refresh race", Description: "Serialize token refresh to avoid the race condition"},
		},
		RecommendedIdea: "idea-1",
	}

	if err := s.SaveOpportunitySpec(ctx, *spec); err != nil {
		t.Fatalf("SaveOpportunitySpec: %v", err)
	}

	got, err := s.GetOpportunitySpec(ctx, "OPP-1")
	if err != nil {
		t.Fatalf("GetOpportunitySpec: %v", err)
	}
	if got.Metadata.Title != "Fix SSO login failures" {
		t.Errorf("Metadata.Title = %q, want %q", got.Metadata.Title, "Fix SSO login failures")
	}
	if got.UsersAndProblem.ProblemStatement != spec.UsersAndProblem.ProblemStatement {
		t.Errorf("UsersAndProblem.ProblemStatement = %q, want %q", got.UsersAndProblem.ProblemStatement, spec.UsersAndProblem.ProblemStatement)
	}
	if len(got.SolutionIdeas.Ideas) != 1 || got.SolutionIdeas.Ideas[0].Name != "Fix token refresh race" {
		t.Errorf("SolutionIdeas.Ideas = %+v, want one idea named %q", got.SolutionIdeas.Ideas, "Fix token refresh race")
	}

	// Re-save with a changed field fully replaces the prior record.
	spec.UsersAndProblem.ProblemStatement = "Updated statement"
	if err := s.SaveOpportunitySpec(ctx, *spec); err != nil {
		t.Fatalf("SaveOpportunitySpec (update): %v", err)
	}
	got2, err := s.GetOpportunitySpec(ctx, "OPP-1")
	if err != nil {
		t.Fatalf("GetOpportunitySpec (after update): %v", err)
	}
	if got2.UsersAndProblem.ProblemStatement != "Updated statement" {
		t.Errorf("ProblemStatement after update = %q, want %q", got2.UsersAndProblem.ProblemStatement, "Updated statement")
	}
}

func TestDoltStore_GetOpportunitySpec_NotFound(t *testing.T) {
	startDoltServer(t)

	dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%d)/omniroadmap_opportunityspec_notfound_test", testPort)
	if err := InitDatabase(dsn); err != nil {
		t.Fatalf("InitDatabase: %v", err)
	}
	s, err := New(dsn)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	ctx := t.Context()
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if _, err := s.GetOpportunitySpec(ctx, "missing"); err == nil {
		t.Fatal("expected error for missing spec")
	}
}
