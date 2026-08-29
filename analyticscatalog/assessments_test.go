package analyticscatalog

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ProductBuildersHQ/compass-rice/catalog"
	"github.com/ProductBuildersHQ/compass-rice/rice"
	"github.com/grokify/prism-roadmap/assessment"
)

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

var validCustomerB2BDoc = `{"profileId": "customer/b2b/v1", "evidence": ` + validCustomerB2BEvidenceJSON + `}`

func mustNormalizeCompass(t *testing.T) rice.Normalized {
	t.Helper()
	n, err := catalog.NormalizeDocument([]byte(validCustomerB2BDoc))
	if err != nil {
		t.Fatalf("NormalizeDocument: %v", err)
	}
	return n
}

func compassAssessment(t *testing.T, id, specID string) assessment.OpportunityAssessment {
	t.Helper()
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	a := *assessment.NewOpportunityAssessment(id, assessment.OpportunityRef{SpecID: specID}, "Title "+id, now)
	n := mustNormalizeCompass(t)
	a.Compass = &assessment.CompassAssessment{
		ProfileID:    n.ProfileID,
		Normalized:   n,
		EvidenceJSON: json.RawMessage(validCustomerB2BEvidenceJSON),
	}
	return a
}

func TestDatasetForAssessmentsFieldCoverage(t *testing.T) {
	a1 := compassAssessment(t, "OA-1", "OPP-1")
	a2 := *assessment.NewOpportunityAssessment("OA-2", assessment.OpportunityRef{SpecID: "OPP-2"}, "No compass", time.Now()) // no Compass at all

	ranks := map[string]assessment.OpportunityRank{
		"OA-1": {RankedOpportunity: assessment.RankedOpportunity{AssessmentID: "OA-1", CalculatedRank: 1}, FinalRank: 1},
	}

	ds := datasetForAssessments([]assessment.OpportunityAssessment{a1, a2}, ranks)
	if ds.ID != "opportunity_assessments" {
		t.Fatalf("ID = %q, want opportunity_assessments", ds.ID)
	}

	profileField := fieldByQueryName(ds.Fields, "compass_profile_id")
	if profileField == nil {
		t.Fatal("missing compass_profile_id field")
	}
	if profileField.Count != 1 || profileField.Coverage != 0.5 {
		t.Errorf("compass_profile_id = %+v, want Count=1 Coverage=0.5 (only OA-1 has Compass)", profileField)
	}

	scoreField := fieldByQueryName(ds.Fields, "score")
	if scoreField == nil || scoreField.Count != 1 {
		t.Errorf("score field = %+v, want Count=1", scoreField)
	}

	rankField := fieldByQueryName(ds.Fields, "final_rank")
	if rankField == nil || rankField.Count != 1 {
		t.Errorf("final_rank field = %+v, want Count=1 (only OA-1 has a rank)", rankField)
	}

	specField := fieldByQueryName(ds.Fields, "spec_id")
	if specField == nil || specField.Count != 2 || specField.Coverage != 1 {
		t.Errorf("spec_id field = %+v, want Count=2 Coverage=1", specField)
	}
}

func TestDatasetForProfileAssignments(t *testing.T) {
	proposed := assessment.ProposeProfileAssignment("OPP-1", "customer/b2b/v1", "primarily a retention play", "judge")
	confirmed := assessment.ProposeProfileAssignment("OPP-2", "operations/v1", "r", "judge").Confirm("pm@example.com", time.Now())

	ds := datasetForProfileAssignments([]assessment.ProfileAssignment{proposed, confirmed})
	if ds.ID != "profile_assignments" {
		t.Fatalf("ID = %q, want profile_assignments", ds.ID)
	}

	confirmedByField := fieldByQueryName(ds.Fields, "confirmed_by")
	if confirmedByField == nil || confirmedByField.Count != 1 {
		t.Errorf("confirmed_by field = %+v, want Count=1 (only the confirmed one has it)", confirmedByField)
	}
	profileIDField := fieldByQueryName(ds.Fields, "profile_id")
	if profileIDField == nil || profileIDField.Count != 2 {
		t.Errorf("profile_id field = %+v, want Count=2", profileIDField)
	}
}

func TestDatasetsForCompassProfilesOnePerProfile(t *testing.T) {
	a1 := compassAssessment(t, "OA-1", "OPP-1")
	a2 := compassAssessment(t, "OA-2", "OPP-2") // same profile as a1
	noCompass := *assessment.NewOpportunityAssessment("OA-3", assessment.OpportunityRef{SpecID: "OPP-3"}, "No compass", time.Now())

	datasets := datasetsForCompassProfiles([]assessment.OpportunityAssessment{a1, a2, noCompass})
	if len(datasets) != 1 {
		t.Fatalf("len(datasets) = %d, want 1 (both compass assessments share customer/b2b/v1)", len(datasets))
	}
	ds := datasets[0]
	if ds.ID != "compass_customer_b2b_v1" {
		t.Errorf("ID = %q, want compass_customer_b2b_v1", ds.ID)
	}

	// Normalized columns are always-present within a profile-scoped dataset.
	reachField := fieldByQueryName(ds.Fields, "reach")
	if reachField == nil || reachField.Count != 2 || reachField.Coverage != 1 {
		t.Errorf("reach field = %+v, want Count=2 Coverage=1", reachField)
	}

	// Raw evidence fields are flattened dynamically, prefixed "evidence.".
	arrField := fieldByQueryName(ds.Fields, "evidence.eligiblearr")
	if arrField == nil {
		t.Fatal("missing evidence.eligiblearr field -- raw evidence was not flattened")
	}
	if arrField.Count != 2 {
		t.Errorf("evidence.eligiblearr Count = %d, want 2", arrField.Count)
	}
	if arrField.Type != "number" {
		t.Errorf("evidence.eligiblearr Type = %q, want number", arrField.Type)
	}
}

func TestDatasetsForCompassProfilesEmptyWhenNoCompass(t *testing.T) {
	noCompass := *assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Title", time.Now())
	datasets := datasetsForCompassProfiles([]assessment.OpportunityAssessment{noCompass})
	if len(datasets) != 0 {
		t.Errorf("len(datasets) = %d, want 0 when no assessment has a Compass", len(datasets))
	}
}

func TestAssessmentDatasetsIncludedInBuildFromStoreShape(t *testing.T) {
	a1 := compassAssessment(t, "OA-1", "OPP-1")
	assignment := assessment.ProposeProfileAssignment("OPP-1", "customer/b2b/v1", "r", "judge")

	datasets := assessmentDatasets(
		[]assessment.OpportunityAssessment{a1},
		[]assessment.ProfileAssignment{assignment},
		nil,
	)
	wantIDs := map[string]bool{"opportunity_assessments": false, "profile_assignments": false, "compass_customer_b2b_v1": false}
	for _, ds := range datasets {
		if _, ok := wantIDs[ds.ID]; ok {
			wantIDs[ds.ID] = true
		}
	}
	for id, found := range wantIDs {
		if !found {
			t.Errorf("expected dataset %q to be present, datasets = %+v", id, datasets)
		}
	}
}
