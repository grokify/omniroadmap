package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ProductBuildersHQ/compass-rice/catalog"
	"github.com/grokify/prism-roadmap/assessment"
)

const validCustomerB2BDoc = `{
	"profileId": "customer/b2b/v1",
	"evidence": {
		"eligibleAccounts": 40,
		"affectedAccounts": 12,
		"eligibleArr": 10000000,
		"affectedArr": 3500000,
		"expectedRetentionOrExpansionImprovementPp": 2,
		"verifiedQuantitativeSources": 2,
		"verifiedQualitativeSources": 1,
		"effortPd": 20
	}
}`

func mustNormalizeCompass(t *testing.T) assessment.CompassAssessment {
	t.Helper()
	n, err := catalog.NormalizeDocument([]byte(validCustomerB2BDoc))
	if err != nil {
		t.Fatalf("NormalizeDocument: %v", err)
	}
	return assessment.CompassAssessment{ProfileID: n.ProfileID, Normalized: n}
}

func TestToAssessRowNoCompass(t *testing.T) {
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", time.Now())
	row := toAssessRow(a)
	if row.Status != "no-compass" {
		t.Errorf("Status = %q, want no-compass", row.Status)
	}
	if row.Score != nil {
		t.Errorf("Score = %v, want nil", row.Score)
	}
}

func TestToAssessRowComputable(t *testing.T) {
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", time.Now())
	c := mustNormalizeCompass(t)
	a.Compass = &c
	row := toAssessRow(a)
	if row.Status != "computable" {
		t.Errorf("Status = %q, want computable", row.Status)
	}
	if row.Score == nil {
		t.Fatal("Score is nil, want set")
	}
	if row.Profile != string(c.ProfileID) {
		t.Errorf("Profile = %q, want %q", row.Profile, c.ProfileID)
	}
}

func TestToAssessRowNeedsReview(t *testing.T) {
	a := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", time.Now())
	c := mustNormalizeCompass(t)
	c.NeedsHumanReview = true
	a.Compass = &c
	row := toAssessRow(a)
	if row.Status != "needs-review" {
		t.Errorf("Status = %q, want needs-review", row.Status)
	}
	if row.Score != nil {
		t.Errorf("Score = %v, want nil while awaiting review", row.Score)
	}
}

func TestParseAssessedAtDefaultsToNow(t *testing.T) {
	before := time.Now()
	got, err := parseAssessedAt("")
	if err != nil {
		t.Fatalf("parseAssessedAt(\"\") error = %v", err)
	}
	if got.Before(before) {
		t.Errorf("parseAssessedAt(\"\") = %v, want >= %v", got, before)
	}
}

func TestParseAssessedAtExplicit(t *testing.T) {
	got, err := parseAssessedAt("2026-08-24T00:00:00Z")
	if err != nil {
		t.Fatalf("parseAssessedAt() error = %v", err)
	}
	want := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parseAssessedAt() = %v, want %v", got, want)
	}
}

func TestParseAssessedAtInvalid(t *testing.T) {
	if _, err := parseAssessedAt("not-a-date"); err == nil {
		t.Error("parseAssessedAt(\"not-a-date\") = nil error, want error")
	}
}

func TestImportFileParsing(t *testing.T) {
	data := []byte(`{
		"specId": "OPP-1",
		"title": "My Opportunity",
		"profileId": "customer/b2b/v1",
		"evidence": {"eligibleArr": 1000},
		"claims": [{"id": "c1", "text": "x", "category": "metadata", "verdict": "verified"}]
	}`)
	var f importFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if f.SpecID != "OPP-1" || f.Title != "My Opportunity" || string(f.ProfileID) != "customer/b2b/v1" {
		t.Errorf("importFile = %+v", f)
	}
	if len(f.Claims) != 1 || f.Claims[0].ID != "c1" {
		t.Errorf("Claims = %+v", f.Claims)
	}
}
