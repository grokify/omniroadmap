package analyticsdashboards

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
		EvidenceJSON: json.RawMessage(validCustomerB2BEvidenceJSON),
	}
	return a
}

func TestBuildEmptyAssessmentsStillProducesPortfolioDashboards(t *testing.T) {
	pack := Build(nil, time.Now())

	wantIDs := map[string]bool{"compass-rice-all": false, "moscow-compass-rice": false, "portfolio-overview": false}
	for _, d := range pack.Dashboards {
		if _, ok := wantIDs[d.ID]; ok {
			wantIDs[d.ID] = true
		}
	}
	for id, found := range wantIDs {
		if !found {
			t.Errorf("expected dashboard %q even with no assessments", id)
		}
	}
	if len(pack.Dashboards) != 3 {
		t.Errorf("len(Dashboards) = %d, want 3 (no compass-profile dashboards without any Compass assessment)", len(pack.Dashboards))
	}
}

func TestBuildOneCompassProfileDashboardPerProfile(t *testing.T) {
	a1 := compassAssessment(t, "OA-1", "OPP-1")
	a2 := compassAssessment(t, "OA-2", "OPP-2") // same profile as a1

	pack := Build([]assessment.OpportunityAssessment{a1, a2}, time.Now())

	var profileDashboards int
	for _, d := range pack.Dashboards {
		if d.ID == "compass-profile-customer_b2b_v1" {
			profileDashboards++
		}
	}
	if profileDashboards != 1 {
		t.Errorf("compass-profile-customer_b2b_v1 dashboards = %d, want exactly 1 (both assessments share one profile)", profileDashboards)
	}
	if len(pack.Dashboards) != 4 {
		t.Errorf("len(Dashboards) = %d, want 4 (3 portfolio-wide + 1 profile-specific)", len(pack.Dashboards))
	}
}

func TestBuildWidgetQuestionIDsResolve(t *testing.T) {
	a1 := compassAssessment(t, "OA-1", "OPP-1")
	pack := Build([]assessment.OpportunityAssessment{a1}, time.Now())

	questionByID := map[string]bool{}
	for _, q := range pack.Questions {
		questionByID[q.ID] = true
	}
	for _, d := range pack.Dashboards {
		for _, w := range d.Widgets {
			if w.QuestionID == "" {
				t.Errorf("dashboard %q widget %q has no QuestionID", d.ID, w.ID)
				continue
			}
			if !questionByID[w.QuestionID] {
				t.Errorf("dashboard %q widget %q references unknown question %q", d.ID, w.ID, w.QuestionID)
			}
		}
	}
}

func TestBuildDashboardAndQuestionIDsUnique(t *testing.T) {
	a1 := compassAssessment(t, "OA-1", "OPP-1")
	pack := Build([]assessment.OpportunityAssessment{a1}, time.Now())

	seenDashboards := map[string]bool{}
	for _, d := range pack.Dashboards {
		if seenDashboards[d.ID] {
			t.Errorf("duplicate dashboard ID %q", d.ID)
		}
		seenDashboards[d.ID] = true
	}

	seenQuestions := map[string]bool{}
	for _, q := range pack.Questions {
		if seenQuestions[q.ID] {
			t.Errorf("duplicate question ID %q", q.ID)
		}
		seenQuestions[q.ID] = true
	}
}

func TestBuildQuestionsReferenceKnownDatasets(t *testing.T) {
	a1 := compassAssessment(t, "OA-1", "OPP-1")
	pack := Build([]assessment.OpportunityAssessment{a1}, time.Now())

	known := map[string]bool{
		itemsDataset:                  true,
		opportunityAssessmentsDataset: true,
		profileAssignmentsDataset:     true,
		"compass_customer_b2b_v1":     true,
	}
	for _, q := range pack.Questions {
		if !known[q.DatasetID] {
			t.Errorf("question %q references unknown dataset %q", q.ID, q.DatasetID)
		}
		if q.SourceID != SourceID {
			t.Errorf("question %q SourceID = %q, want %q", q.ID, q.SourceID, SourceID)
		}
	}
}

func TestCompassProfileDashboardIncludesRawEvidenceColumns(t *testing.T) {
	a1 := compassAssessment(t, "OA-1", "OPP-1")
	pack := Build([]assessment.OpportunityAssessment{a1}, time.Now())

	var found *dashboardWidgetColumns
	for i, d := range pack.Dashboards {
		if d.ID != "compass-profile-customer_b2b_v1" {
			continue
		}
		w := pack.Dashboards[i].Widgets[0]
		var cfg struct {
			Columns []struct {
				Field string `json:"field"`
			} `json:"columns"`
		}
		if err := json.Unmarshal(w.Config, &cfg); err != nil {
			t.Fatalf("unmarshal widget config: %v", err)
		}
		var fields []string
		for _, c := range cfg.Columns {
			fields = append(fields, c.Field)
		}
		found = &dashboardWidgetColumns{fields: fields}
		break
	}
	if found == nil {
		t.Fatal("compass-profile-customer_b2b_v1 dashboard not found")
	}
	hasEvidenceCol := false
	for _, f := range found.fields {
		if f == "evidence.eligiblearr" {
			hasEvidenceCol = true
		}
	}
	if !hasEvidenceCol {
		t.Errorf("expected evidence.eligiblearr column in compass-profile widget, got fields %+v", found.fields)
	}
}

type dashboardWidgetColumns struct {
	fields []string
}

func TestBuildIsReproducible(t *testing.T) {
	a1 := compassAssessment(t, "OA-1", "OPP-1")
	now := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	pack1 := Build([]assessment.OpportunityAssessment{a1}, now)
	pack2 := Build([]assessment.OpportunityAssessment{a1}, now)

	data1, err := json.Marshal(pack1)
	if err != nil {
		t.Fatalf("marshal pack1: %v", err)
	}
	data2, err := json.Marshal(pack2)
	if err != nil {
		t.Fatalf("marshal pack2: %v", err)
	}
	if string(data1) != string(data2) {
		t.Error("Build() with identical inputs produced different output -- expected byte-identical, reproducible packs")
	}
}
