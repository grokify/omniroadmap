package render

import (
	"strings"
	"testing"

	"github.com/grokify/prism-roadmap/assessment"
)

func TestPresentationMarpFrontMatter(t *testing.T) {
	md, err := PresentationMarp(minimalReview(t))
	if err != nil {
		t.Fatalf("PresentationMarp: %v", err)
	}
	if !strings.HasPrefix(md, "---\nmarp: true\n") {
		t.Errorf("expected Marp front matter to open the deck, got:\n%s", md)
	}
	if !strings.Contains(md, "Portfolio Roadmap Review") {
		t.Error("expected the presentation title in the front matter header")
	}
}

func TestPresentationMarpOneSlidePerPresentSection(t *testing.T) {
	review := minimalReview(t)
	md, err := PresentationMarp(review)
	if err != nil {
		t.Fatalf("PresentationMarp: %v", err)
	}

	present := review.PresentAgenda()
	for _, s := range present {
		if !strings.Contains(md, "# "+s.Title) {
			t.Errorf("expected a slide headline for present section %q", s.Title)
		}
	}

	// Exactly one level-1 heading per present section — not the front
	// matter's YAML block (which has none) and not any "## " subheading
	// a section body might render.
	h1Count := 0
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(line, "# ") {
			h1Count++
		}
	}
	if h1Count != len(present) {
		t.Errorf("got %d level-1 headings, want exactly %d (one per present section)", h1Count, len(present))
	}
}

func TestPresentationMarpOmitsAbsentSectionSlides(t *testing.T) {
	md, err := PresentationMarp(minimalReview(t))
	if err != nil {
		t.Fatalf("PresentationMarp: %v", err)
	}
	for _, absent := range []string{"# Prioritized Roadmap", "# Capability Stack"} {
		if strings.Contains(md, absent) {
			t.Errorf("expected no slide for absent section, but found %q", absent)
		}
	}
}

func TestPresentationMarpRankingHighlights(t *testing.T) {
	now := minimalReview(t).Dataset.GeneratedAt
	review := assessment.NewPortfolioReview(fullDataset(now))

	md, err := PresentationMarp(review)
	if err != nil {
		t.Fatalf("PresentationMarp: %v", err)
	}
	if !strings.Contains(md, "**#1** Unified Auth Platform (must_have)") {
		t.Errorf("expected the top-ranked opportunity highlighted on the prioritized-roadmap slide, got:\n%s", md)
	}
	// Excluded items must never appear as a highlight.
	if strings.Contains(md, "Deferred Item") {
		t.Error("expected excluded opportunities to never appear in presentation highlights")
	}
}

func TestPresentationMarpOverrideHighlights(t *testing.T) {
	now := minimalReview(t).Dataset.GeneratedAt
	review := assessment.NewPortfolioReview(fullDataset(now))

	md, err := PresentationMarp(review)
	if err != nil {
		t.Fatalf("PresentationMarp: %v", err)
	}
	if !strings.Contains(md, "1 governance override(s) applied") {
		t.Errorf("expected the override count highlighted, got:\n%s", md)
	}
	if !strings.Contains(md, "`OA-2` → #3: deprioritized") {
		t.Error("expected the override detail to render")
	}
}

func TestPresentationMarpDistributionHighlightsTopBucketOnly(t *testing.T) {
	now := minimalReview(t).Dataset.GeneratedAt
	review := assessment.NewPortfolioReview(fullDataset(now))

	md, err := PresentationMarp(review)
	if err != nil {
		t.Fatalf("PresentationMarp: %v", err)
	}
	if !strings.Contains(md, "Largest: must_be (60%)") {
		t.Errorf("expected only the largest kano bucket highlighted, got:\n%s", md)
	}
	// The smaller bucket should not appear on the slide (full detail is
	// reserved for the document's distribution-detail appendix).
	if strings.Contains(md, "Largest: performance") {
		t.Error("expected only the top bucket per dimension, not every bucket")
	}
}
