package compassbridge

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ProductBuildersHQ/compass-rice/judge"
	customerb2b "github.com/ProductBuildersHQ/compass-rice/profiles/customer/b2b"
	"github.com/ProductBuildersHQ/compass-rice/rice"
	"github.com/grokify/prism-roadmap/assessment"
	"github.com/plexusone/structured-evaluation/claims"
)

const customerB2BProfileID rice.ProfileID = "customer/b2b/v1"

func validCustomerB2BEvidence() customerb2b.Evidence {
	return customerb2b.Evidence{
		EligibleAccounts: 40,
		AffectedAccounts: 12,
		EligibleARR:      10_000_000,
		AffectedARR:      3_500_000,
		ExpectedRetentionOrExpansionImprovementPP: 2,
		VerifiedQuantitativeSources:               2,
		VerifiedQualitativeSources:                1,
		EffortPD:                                  20,
	}
}

func verifiedQuantitativeClaim(id string) *claims.Claim {
	c := claims.NewClaim(id, "affected ARR is $3.5M", claims.ClaimMetadata, claims.Location{})
	c.Validation = &claims.Validation{Type: claims.SourceInternal, Internal: &claims.InternalValidation{Method: claims.MethodLogAnalysis}}
	c.Verdict = claims.VerdictVerified
	return c
}

func verifiedQualitativeClaim(id string) *claims.Claim {
	c := claims.NewClaim(id, "PM interview confirms retention risk", claims.ClaimMetadata, claims.Location{})
	c.Verdict = claims.VerdictVerified
	return c
}

func validOutput() judge.Output {
	return judge.Output{
		ProfileID: customerB2BProfileID,
		Evidence:  validCustomerB2BEvidence(),
		Claims: []*claims.Claim{
			verifiedQuantitativeClaim("claim-1"),
			verifiedQuantitativeClaim("claim-2"),
			verifiedQualitativeClaim("claim-3"),
		},
	}
}

func TestIngestValid(t *testing.T) {
	c, err := Ingest(validOutput())
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	if c.ProfileID != customerB2BProfileID {
		t.Errorf("ProfileID = %q, want %q", c.ProfileID, customerB2BProfileID)
	}
	if c.Normalized.Confidence != rice.ConfidenceHigh {
		t.Errorf("Normalized.Confidence = %v, want %v", c.Normalized.Confidence, rice.ConfidenceHigh)
	}
	var roundTripped customerb2b.Evidence
	if err := json.Unmarshal(c.EvidenceJSON, &roundTripped); err != nil {
		t.Fatalf("unmarshal EvidenceJSON error = %v", err)
	}
	if roundTripped != validCustomerB2BEvidence() {
		t.Errorf("EvidenceJSON round-trip = %+v, want %+v", roundTripped, validCustomerB2BEvidence())
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestIngestConfidenceMismatchRejected(t *testing.T) {
	output := validOutput()
	output.Claims = nil // evidence claims High confidence, but no verified claims back it
	_, err := Ingest(output)
	if err == nil {
		t.Fatal("Ingest() with no backing claims = nil error, want error")
	}
	if !strings.Contains(err.Error(), "exceeds claims-derived confidence") {
		t.Errorf("error = %v, want it to mention confidence mismatch", err)
	}
	var ingestErr *IngestError
	if !errors.As(err, &ingestErr) {
		t.Fatal("error is not an *IngestError")
	}
}

func TestIngestUnknownProfile(t *testing.T) {
	output := validOutput()
	output.ProfileID = "bogus/v1"
	if _, err := Ingest(output); err == nil {
		t.Error("Ingest() with unknown profileId = nil error, want error")
	}
}

func TestIngestWrongEvidenceType(t *testing.T) {
	output := validOutput()
	output.Evidence = "not evidence"
	if _, err := Ingest(output); err == nil {
		t.Error("Ingest() with wrong evidence type = nil error, want error")
	}
}

func TestIngestHumanEvidenceValid(t *testing.T) {
	evidenceJSON, err := json.Marshal(validCustomerB2BEvidence())
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}
	now := time.Now()
	c, err := IngestHumanEvidence(customerB2BProfileID, evidenceJSON, "pm@example.com", now)
	if err != nil {
		t.Fatalf("IngestHumanEvidence() error = %v", err)
	}
	if c.ProfileID != customerB2BProfileID {
		t.Errorf("ProfileID = %q, want %q", c.ProfileID, customerB2BProfileID)
	}
	if c.NeedsHumanReview {
		t.Error("NeedsHumanReview = true, want false for a human-entered assessment")
	}
	if c.HumanReview == nil {
		t.Fatal("HumanReview is nil, want set")
	}
	if c.HumanReview.ReviewedBy != "pm@example.com" || !c.HumanReview.ReviewedAt.Equal(now) {
		t.Errorf("HumanReview = %+v", c.HumanReview)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestIngestHumanEvidenceUnknownProfile(t *testing.T) {
	evidenceJSON, _ := json.Marshal(validCustomerB2BEvidence())
	if _, err := IngestHumanEvidence("bogus/v1", evidenceJSON, "pm@example.com", time.Now()); err == nil {
		t.Error("IngestHumanEvidence() with unknown profileId = nil error, want error")
	}
}

func TestIngestHumanEvidenceInvalidEvidence(t *testing.T) {
	if _, err := IngestHumanEvidence(customerB2BProfileID, []byte(`{"eligibleArr": -1}`), "pm@example.com", time.Now()); err == nil {
		t.Error("IngestHumanEvidence() with invalid evidence = nil error, want error")
	}
}

func TestFirstCycleWithCompass(t *testing.T) {
	c, err := Ingest(validOutput())
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	a := FirstCycleWithCompass("OA-001", assessment.OpportunityRef{SpecID: "OS-001"}, "Test opportunity", time.Now(), c)
	if a.Compass == nil {
		t.Fatal("Compass is nil, want set")
	}
	if a.Cycle.Number != 1 {
		t.Errorf("Cycle.Number = %d, want 1", a.Cycle.Number)
	}
	if err := a.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}

func TestNextCycleWithCompass(t *testing.T) {
	c, err := Ingest(validOutput())
	if err != nil {
		t.Fatalf("Ingest() error = %v", err)
	}
	first := FirstCycleWithCompass("OA-001", assessment.OpportunityRef{SpecID: "OS-001"}, "Test opportunity", time.Now(), c)
	next := NextCycleWithCompass(first, "OA-002", time.Now(), c)
	if next.Cycle.Number != 2 {
		t.Errorf("Cycle.Number = %d, want 2", next.Cycle.Number)
	}
	if next.Cycle.SupersedesID != "OA-001" {
		t.Errorf("Cycle.SupersedesID = %q, want OA-001", next.Cycle.SupersedesID)
	}
	if next.Compass == nil {
		t.Fatal("Compass is nil, want set")
	}
}

func TestProposeConfirmProfile(t *testing.T) {
	p, err := ProposeProfile("OS-001", customerB2BProfileID, "primarily a retention play", "judge-session-9")
	if err != nil {
		t.Fatalf("ProposeProfile() error = %v", err)
	}
	if p.Status != assessment.ProfileAssignmentProposed {
		t.Errorf("Status = %q, want %q", p.Status, assessment.ProfileAssignmentProposed)
	}

	confirmed, err := ConfirmProfile(p, "pm@example.com", time.Now())
	if err != nil {
		t.Fatalf("ConfirmProfile() error = %v", err)
	}
	if confirmed.Status != assessment.ProfileAssignmentConfirmed {
		t.Errorf("Status = %q, want %q", confirmed.Status, assessment.ProfileAssignmentConfirmed)
	}
}

func TestConfirmProfileRequiresProposedStatus(t *testing.T) {
	p, err := ProposeProfile("OS-001", customerB2BProfileID, "r", "judge")
	if err != nil {
		t.Fatalf("ProposeProfile() error = %v", err)
	}
	confirmed, err := ConfirmProfile(p, "pm@example.com", time.Now())
	if err != nil {
		t.Fatalf("ConfirmProfile() error = %v", err)
	}
	if _, err := ConfirmProfile(confirmed, "pm2@example.com", time.Now()); err == nil {
		t.Error("ConfirmProfile() on an already-confirmed assignment = nil error, want error")
	}
}

func TestRejectProfile(t *testing.T) {
	p, err := ProposeProfile("OS-001", customerB2BProfileID, "r", "judge")
	if err != nil {
		t.Fatalf("ProposeProfile() error = %v", err)
	}
	rejected, err := RejectProfile(p, "pm@example.com", time.Now(), "wrong profile, should be platform")
	if err != nil {
		t.Fatalf("RejectProfile() error = %v", err)
	}
	if rejected.Status != assessment.ProfileAssignmentRejected {
		t.Errorf("Status = %q, want %q", rejected.Status, assessment.ProfileAssignmentRejected)
	}
	if _, err := RejectProfile(rejected, "pm@example.com", time.Now(), "again"); err == nil {
		t.Error("RejectProfile() on an already-rejected assignment = nil error, want error")
	}
}

func TestAssignProfileDirectly(t *testing.T) {
	confirmed, err := AssignProfileDirectly("OS-001", customerB2BProfileID, "PM chose directly", "pm@example.com", time.Now())
	if err != nil {
		t.Fatalf("AssignProfileDirectly() error = %v", err)
	}
	if confirmed.Status != assessment.ProfileAssignmentConfirmed {
		t.Errorf("Status = %q, want %q", confirmed.Status, assessment.ProfileAssignmentConfirmed)
	}
	if confirmed.ProposedBy != confirmed.ConfirmedBy {
		t.Errorf("ProposedBy = %q, ConfirmedBy = %q, want equal for a direct assignment", confirmed.ProposedBy, confirmed.ConfirmedBy)
	}
}
