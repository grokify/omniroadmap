package analyticsquery

import (
	"testing"
	"time"

	"github.com/ProductBuildersHQ/compass-rice/catalog"
	"github.com/grokify/omniroadmap-core/provider"
	"github.com/grokify/prism-roadmap/assessment"
	"github.com/plexusone/dashforge/dashboardir"
)

func TestExecuteItemsSelectsCustomFields(t *testing.T) {
	items := []provider.Item{
		{
			ID:           "1",
			Provider:     "aha-studio",
			SourceID:     "AHA-1",
			SourceRef:    "PROJ-1",
			WorkspaceRef: "PROJ",
			Kind:         provider.ItemKindInitiative,
			Name:         "Launch reporting",
			CustomFields: []provider.CustomField{{Key: "Launch Tier", Value: "T1"}},
		},
		{
			ID:           "2",
			Provider:     "aha-studio",
			SourceID:     "AHA-2",
			SourceRef:    "PROJ-2",
			WorkspaceRef: "OTHER",
			Kind:         provider.ItemKindFeature,
			Name:         "Ignored feature",
			CustomFields: []provider.CustomField{{Key: "Launch Tier", Value: "T2"}},
		},
	}
	got, err := ExecuteItems(items, dashboardir.AnalyticsQueryRequest{
		Query: `SELECT name, workspace_ref, custom.launch_tier FROM initiatives WHERE workspace_ref = "PROJ" LIMIT 10`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RowCount != 1 {
		t.Fatalf("row count = %d, want 1", got.RowCount)
	}
	if got.Rows[0]["name"] != "Launch reporting" {
		t.Fatalf("name = %v", got.Rows[0]["name"])
	}
	if got.Rows[0]["custom.launch_tier"] != "T1" {
		t.Fatalf("custom.launch_tier = %v", got.Rows[0]["custom.launch_tier"])
	}
}

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

const validCustomerB2BEvidenceJSON = `{
	"eligibleAccounts": 40,
	"affectedAccounts": 12,
	"eligibleArr": 10000000,
	"affectedArr": 3500000,
	"expectedRetentionOrExpansionImprovementPp": 2,
	"verifiedQuantitativeSources": 2,
	"verifiedQualitativeSources": 1,
	"effortPd": 20
}`

func compassAssessment(t *testing.T, id, specID string) assessment.OpportunityAssessment {
	t.Helper()
	n, err := catalog.NormalizeDocument([]byte(validCustomerB2BDoc))
	if err != nil {
		t.Fatalf("NormalizeDocument: %v", err)
	}
	a := *assessment.NewOpportunityAssessment(id, assessment.OpportunityRef{SpecID: specID}, "Title "+id, time.Now())
	a.Compass = &assessment.CompassAssessment{
		ProfileID:    n.ProfileID,
		Normalized:   n,
		EvidenceJSON: []byte(validCustomerB2BEvidenceJSON),
	}
	return a
}

func TestExecuteOpportunityAssessmentsFiltersByComputable(t *testing.T) {
	a1 := compassAssessment(t, "OA-1", "OPP-1")
	a2 := *assessment.NewOpportunityAssessment("OA-2", assessment.OpportunityRef{SpecID: "OPP-2"}, "No compass", time.Now())

	ranks := map[string]assessment.OpportunityRank{
		"OA-1": {RankedOpportunity: assessment.RankedOpportunity{AssessmentID: "OA-1", CalculatedRank: 1}, FinalRank: 1},
	}

	got, err := execute(nil, []assessment.OpportunityAssessment{a1, a2}, nil, ranks, dashboardir.AnalyticsQueryRequest{
		Query: `SELECT spec_id, score, final_rank FROM opportunity_assessments WHERE computable = true LIMIT 10`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RowCount != 1 {
		t.Fatalf("row count = %d, want 1", got.RowCount)
	}
	if got.Rows[0]["spec_id"] != "OPP-1" {
		t.Fatalf("spec_id = %v, want OPP-1", got.Rows[0]["spec_id"])
	}
	if got.Rows[0]["final_rank"] != float64(1) {
		t.Fatalf("final_rank = %v, want 1", got.Rows[0]["final_rank"])
	}
}

func TestExecuteProfileAssignments(t *testing.T) {
	proposed := assessment.ProposeProfileAssignment("OPP-1", "customer/b2b/v1", "r", "judge")
	confirmed := assessment.ProposeProfileAssignment("OPP-2", "operations/v1", "r2", "judge").Confirm("pm@example.com", time.Now())

	got, err := execute(nil, nil, []assessment.ProfileAssignment{proposed, confirmed}, nil, dashboardir.AnalyticsQueryRequest{
		Query: `SELECT spec_id, status FROM profile_assignments WHERE status = "confirmed" LIMIT 10`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RowCount != 1 || got.Rows[0]["spec_id"] != "OPP-2" {
		t.Fatalf("rows = %+v, want exactly OPP-2 confirmed", got.Rows)
	}
}

func TestExecuteCompassProfileDatasetIncludesRawEvidence(t *testing.T) {
	a1 := compassAssessment(t, "OA-1", "OPP-1")

	got, err := execute(nil, []assessment.OpportunityAssessment{a1}, nil, nil, dashboardir.AnalyticsQueryRequest{
		Query: `SELECT spec_id, score, evidence.eligiblearr FROM compass_customer_b2b_v1 LIMIT 10`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RowCount != 1 {
		t.Fatalf("row count = %d, want 1", got.RowCount)
	}
	if got.Rows[0]["evidence.eligiblearr"] != float64(10000000) {
		t.Fatalf("evidence.eligiblearr = %v, want 1e7", got.Rows[0]["evidence.eligiblearr"])
	}
}

func TestExecuteUnknownCompassProfileErrors(t *testing.T) {
	_, err := execute(nil, nil, nil, nil, dashboardir.AnalyticsQueryRequest{
		Query: `SELECT spec_id FROM compass_bogus_v1 LIMIT 10`,
	})
	if err == nil {
		t.Fatal("expected an error querying a nonexistent compass profile entity")
	}
}

func TestExecuteItemsPrioritizeMoscowRice(t *testing.T) {
	scoreHigh := 10.0
	scoreLow := 2.0
	items := []provider.Item{
		{ID: "1", Kind: provider.ItemKindInitiative, Name: "Could", MoSCoW: "could_have", RICE: &provider.RICE{Score: &scoreHigh}},
		{ID: "2", Kind: provider.ItemKindInitiative, Name: "Must low", MoSCoW: "must_have", RICE: &provider.RICE{Score: &scoreLow}},
		{ID: "3", Kind: provider.ItemKindInitiative, Name: "Must high", MoSCoW: "must_have", RICE: &provider.RICE{Score: &scoreHigh}},
	}
	got, err := ExecuteItems(items, dashboardir.AnalyticsQueryRequest{
		Query: `SELECT name, moscow, rice_score FROM initiatives PRIORITIZE BY moscow_rice LIMIT 2`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.RowCount != 2 {
		t.Fatalf("row count = %d, want 2", got.RowCount)
	}
	if got.Rows[0]["name"] != "Must high" {
		t.Fatalf("first row name = %v, want Must high", got.Rows[0]["name"])
	}
	if got.Rows[1]["name"] != "Must low" {
		t.Fatalf("second row name = %v, want Must low", got.Rows[1]["name"])
	}
}
