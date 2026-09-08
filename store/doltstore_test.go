package store

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/ProductBuildersHQ/compass-rice/catalog"
	"github.com/ProductBuildersHQ/compass-rice/rice"
	_ "github.com/go-sql-driver/mysql"
	omniroadmap "github.com/grokify/omniroadmap-core"
	"github.com/grokify/omniroadmap-core/provider"
	"github.com/grokify/prism-roadmap/assessment"
	"github.com/plexusone/structured-evaluation/claims"

	"github.com/grokify/omniroadmap/augment"
	"github.com/grokify/omniroadmap/compile"
	"github.com/grokify/omniroadmap/materialize"
	"github.com/grokify/omniroadmap/review"
)

// testPort is a dedicated port for this test's dolt sql-server, away from
// the default to avoid clashing with a developer's running instance.
const testPort = 13399

// startDoltServer launches a dolt sql-server over a temp dir for the test.
// Dolt integration tests are opt-in, not opt-out: they skip by default —
// even when the dolt binary happens to be on PATH — unless OMNIROADMAP_TEST_DOLT
// is set, so a plain `go test ./...` stays a fast, hermetic check of the Go
// library code and never depends on a live Dolt server being reachable. The
// server is killed on cleanup.
func startDoltServer(t *testing.T) {
	t.Helper()
	if os.Getenv("OMNIROADMAP_TEST_DOLT") == "" {
		t.Skip("OMNIROADMAP_TEST_DOLT not set; skipping Dolt integration test (set OMNIROADMAP_TEST_DOLT=1 to run, requires the dolt binary)")
	}
	doltBin, err := exec.LookPath("dolt")
	if err != nil {
		t.Skip("dolt binary not on PATH; skipping Dolt integration test")
	}

	dir := t.TempDir()
	proc := exec.Command(doltBin, "sql-server", "--host", "127.0.0.1", "--port", fmt.Sprintf("%d", testPort))
	proc.Dir = dir
	var output bytes.Buffer
	proc.Stdout = &output
	proc.Stderr = &output
	if err := proc.Start(); err != nil {
		t.Fatalf("starting dolt sql-server: %v", err)
	}
	t.Cleanup(func() {
		_ = proc.Process.Kill()
		_, _ = proc.Process.Wait()
	})

	// A dolt sql-server opens its TCP port before it has finished
	// initializing — dialing the port only proves it's listening, not that
	// it can execute queries yet. Ping over the real MySQL wire protocol
	// instead, so callers never race dolt's own startup (a CREATE DATABASE
	// issued the moment the port opens can fail with "invalid connection").
	// A short per-attempt timeout keeps a single stuck dial from eating the
	// whole retry budget, and dolt's own stdout/stderr are captured so a
	// real startup failure (e.g. the port already in use) is visible in the
	// failure message instead of surfacing only as an opaque EOF.
	dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%d)/?timeout=2s", testPort)
	deadline := time.Now().Add(30 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			lastErr = err
		} else {
			lastErr = db.Ping()
			_ = db.Close()
			if lastErr == nil {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("dolt sql-server did not become ready on 127.0.0.1:%d: %v\ndolt output:\n%s", testPort, lastErr, output.String())
}

func f64(v float64) *float64 { return &v }

func TestDoltStore_Integration(t *testing.T) {
	startDoltServer(t)

	dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%d)/omniroadmap_test", testPort)
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

	now := time.Now().UTC().Truncate(time.Second)
	items := []provider.Item{
		{
			ID:           "aha:1",
			Provider:     "aha",
			SourceID:     "1",
			WorkspaceRef: "PROJ",
			Kind:         provider.ItemKindFeature,
			Name:         "Feature One",
			Status:       &provider.Status{Name: "In Dev", Category: provider.StatusCategoryInProgress},
			MoSCoW:       "must_have",
			RICE:         &provider.RICE{Reach: f64(5000), Effort: f64(4)},
			Tags:         []string{"backend"},
			CustomFields: []provider.CustomField{
				{Key: "priority", Value: []byte(`"P1"`)}, // raw JSON — must not base64
			},
			CreatedAt: &now,
		},
	}

	if err := s.UpsertItems(ctx, items); err != nil {
		t.Fatalf("UpsertItems: %v", err)
	}
	// Upsert again with a change — must update, not duplicate.
	items[0].Name = "Feature One Renamed"
	if err := s.UpsertItems(ctx, items); err != nil {
		t.Fatalf("UpsertItems (second): %v", err)
	}

	stored, err := s.Client().Item.Get(ctx, "aha:1")
	if err != nil {
		t.Fatalf("Item.Get: %v", err)
	}
	if stored.Name != "Feature One Renamed" {
		t.Errorf("Name = %q, want renamed value (upsert should update)", stored.Name)
	}
	if stored.WorkspaceRef != "PROJ" {
		t.Errorf("WorkspaceRef = %q, want PROJ", stored.WorkspaceRef)
	}
	if stored.Moscow != "must_have" {
		t.Errorf("Moscow = %q, want must_have", stored.Moscow)
	}
	if stored.Rice["reach"] != 5000 {
		t.Errorf("Rice[reach] = %v, want 5000", stored.Rice["reach"])
	}
	if len(stored.CustomFields) != 1 || stored.CustomFields[0]["value"] != "P1" {
		t.Errorf("CustomFields = %+v, want [{value: P1}] (raw JSON unwrapped, not base64)", stored.CustomFields)
	}
	listed, err := s.ListItems(ctx, ItemFilter{Provider: "aha"})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(listed) != 1 || listed[0].WorkspaceRef != "PROJ" {
		t.Fatalf("listed items = %#v, want one item with WorkspaceRef PROJ", listed)
	}

	count, err := s.Client().Item.Query().Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 1 {
		t.Errorf("item count = %d, want 1 (no duplicates)", count)
	}

	if err := s.UpsertReleases(ctx, []provider.Release{
		{ID: "aha:r1", Provider: "aha", SourceID: "r1", Name: "R1", Released: true},
	}); err != nil {
		t.Fatalf("UpsertReleases: %v", err)
	}

	if err := s.SetSyncMeta(ctx, "aha", "feature", now, 1); err != nil {
		t.Fatalf("SetSyncMeta: %v", err)
	}
	// Update the same (provider, kind) — must update in place.
	if err := s.SetSyncMeta(ctx, "aha", "feature", now.Add(time.Minute), 2); err != nil {
		t.Fatalf("SetSyncMeta (second): %v", err)
	}
	meta, err := s.GetSyncMeta(ctx)
	if err != nil {
		t.Fatalf("GetSyncMeta: %v", err)
	}
	if len(meta) != 1 {
		t.Fatalf("sync meta entries = %d, want 1", len(meta))
	}
	if meta[0].RecordCount != 2 {
		t.Errorf("RecordCount = %d, want 2 (updated)", meta[0].RecordCount)
	}

	if err := s.Commit(ctx, "test sync"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	// Second commit with no changes must be a clean no-op.
	if err := s.Commit(ctx, "empty"); err != nil {
		t.Fatalf("Commit (no changes): %v", err)
	}
}

// TestDoltStore_AugmentSurvivesResync proves the core augment guarantee:
// re-syncing overwrites provider data on the item wholesale, but
// locally-authored augments live in their own table, survive the re-sync,
// and win on read.
func TestDoltStore_AugmentSurvivesResync(t *testing.T) {
	startDoltServer(t)

	dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%d)/omniroadmap_augment_test", testPort)
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

	// Initial sync: fieldmap derived should_have from an Aha custom field.
	item := provider.Item{
		ID:        "aha-studio:42",
		Provider:  "aha-studio",
		SourceID:  "42",
		SourceRef: "MYPROJ-123",
		Kind:      provider.ItemKindFeature,
		Name:      "Feature X",
		MoSCoW:    "should_have",
		RICE:      &provider.RICE{Reach: f64(5000), Effort: f64(4)},
	}
	if err := s.UpsertItems(ctx, []provider.Item{item}); err != nil {
		t.Fatalf("UpsertItems: %v", err)
	}

	// Local augmentation, keyed by the Aha reference.
	if err := s.SetItemAugment(ctx, augment.ItemAugment{
		Provider:  "aha-studio",
		SourceRef: "MYPROJ-123",
		MoSCoW:    "must_have",
		Kano:      "performance",
		RICE:      &provider.RICE{Effort: f64(2)},
		OKRRefs:   []string{"OKR-2026-Q3-01"},
		Notes:     "exec ask",
	}); err != nil {
		t.Fatalf("SetItemAugment: %v", err)
	}

	// Re-sync from Aha: name changed, tenant flipped the custom field to
	// could_have. Provider data must be overwritten; the augment must not.
	item.Name = "Feature X Renamed"
	item.MoSCoW = "could_have"
	if err := s.UpsertItems(ctx, []provider.Item{item}); err != nil {
		t.Fatalf("UpsertItems (re-sync): %v", err)
	}

	got, err := s.GetItem(ctx, "aha-studio", "MYPROJ-123")
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.Name != "Feature X Renamed" {
		t.Errorf("Name = %q, want re-synced value (Aha data overwritten)", got.Name)
	}
	if got.MoSCoW != "must_have" {
		t.Errorf("MoSCoW = %q, want must_have (augment wins over re-synced could_have)", got.MoSCoW)
	}
	if got.Kano != "performance" {
		t.Errorf("Kano = %q, want performance (augment-only field)", got.Kano)
	}
	if got.RICE == nil || got.RICE.Reach == nil || *got.RICE.Reach != 5000 {
		t.Errorf("RICE.Reach = %+v, want 5000 preserved from sync", got.RICE)
	}
	if got.RICE == nil || got.RICE.Effort == nil || *got.RICE.Effort != 2 {
		t.Errorf("RICE.Effort = %+v, want 2 (augment override)", got.RICE)
	}
	if got.Metadata[augment.MetadataKeyNotes] != "exec ask" {
		t.Errorf("Metadata[%s] = %v, want exec ask", augment.MetadataKeyNotes, got.Metadata[augment.MetadataKeyNotes])
	}

	// Raw view skips the overlay.
	raw, err := s.ListItems(ctx, ItemFilter{Provider: "aha-studio", WithoutAugments: true})
	if err != nil {
		t.Fatalf("ListItems (raw): %v", err)
	}
	if len(raw) != 1 {
		t.Fatalf("raw items = %d, want 1", len(raw))
	}
	if raw[0].MoSCoW != "could_have" {
		t.Errorf("raw MoSCoW = %q, want could_have (synced value, no overlay)", raw[0].MoSCoW)
	}
	if raw[0].Kano != "" {
		t.Errorf("raw Kano = %q, want unset", raw[0].Kano)
	}

	// augment set merges: updating one field preserves the others.
	if err := s.SetItemAugment(ctx, func() augment.ItemAugment {
		existing, err := s.GetItemAugment(ctx, "aha-studio", "MYPROJ-123")
		if err != nil {
			t.Fatalf("GetItemAugment: %v", err)
		}
		existing.Kano = "must-be"
		return *existing
	}()); err != nil {
		t.Fatalf("SetItemAugment (update): %v", err)
	}
	updated, err := s.GetItemAugment(ctx, "aha-studio", "MYPROJ-123")
	if err != nil {
		t.Fatalf("GetItemAugment (after update): %v", err)
	}
	if updated.Kano != "must-be" || updated.MoSCoW != "must_have" || len(updated.OKRRefs) != 1 {
		t.Errorf("updated augment = %+v, want kano changed with other fields intact", updated)
	}

	// Delete, then reads show pure provider data again.
	if err := s.DeleteItemAugment(ctx, "aha-studio", "MYPROJ-123"); err != nil {
		t.Fatalf("DeleteItemAugment: %v", err)
	}
	afterDelete, err := s.GetItem(ctx, "aha-studio", "MYPROJ-123")
	if err != nil {
		t.Fatalf("GetItem (after delete): %v", err)
	}
	if afterDelete.MoSCoW != "could_have" {
		t.Errorf("MoSCoW after augment delete = %q, want could_have", afterDelete.MoSCoW)
	}
	if _, err := s.GetItemAugment(ctx, "aha-studio", "MYPROJ-123"); !omniroadmap.IsNotFound(err) {
		t.Errorf("GetItemAugment after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDoltStore_OpportunityAssessment(t *testing.T) {
	startDoltServer(t)

	dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%d)/omniroadmap_assessment_test", testPort)
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

	t1 := time.Now().UTC().Truncate(time.Second)
	first := assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1", RMIID: "RMI-MYREPO-001"}, "Unified Authorization Platform", t1)
	first.MoSCoWAnswers = []assessment.ThresholdAnswer{
		{LevelID: "must", Satisfied: true, EvidenceIDs: []string{"EV-1"}},
	}
	first.RICE = &assessment.RICEAssessment{
		Reach: assessment.Reach{Fraction: 0.6, EvidenceIDs: []string{"EV-2"}},
		ImpactAnswers: []assessment.ThresholdAnswer{
			{LevelID: "high", Satisfied: true, EvidenceIDs: []string{"EV-3"}},
		},
		ConfidenceAnswers: []assessment.ThresholdAnswer{
			{LevelID: "medium", Satisfied: true, EvidenceIDs: []string{"EV-4"}},
		},
		Effort: assessment.EffortEstimate{
			Expected: 20,
			Gate: assessment.EstimabilityGate{
				ScopeDefined: true, ImplementationIdentified: true, DependenciesIdentified: true,
				TestingIdentified: true, DeploymentIdentified: true,
			},
		},
	}
	first.Dimensions = []assessment.DimensionAssignment{
		{DimensionID: "kano", Category: &assessment.CategorySelection{OptionID: "must_be", Resolved: true}},
	}

	if err := s.SaveOpportunityAssessment(ctx, *first); err != nil {
		t.Fatalf("SaveOpportunityAssessment: %v", err)
	}

	got, err := s.GetOpportunityAssessment(ctx, "OA-1")
	if err != nil {
		t.Fatalf("GetOpportunityAssessment: %v", err)
	}
	if got.Title != "Unified Authorization Platform" || got.Opportunity.RMIID != "RMI-MYREPO-001" {
		t.Errorf("GetOpportunityAssessment canonical = %+v", got)
	}
	if len(got.RICE.ImpactAnswers) != 1 {
		t.Errorf("canonical RICE.ImpactAnswers not round-tripped: %+v", got.RICE)
	}

	// Second cycle supersedes the first — the store must flip OA-1's
	// Current to false in the same operation, not require a second call.
	t2 := t1.Add(90 * 24 * time.Hour)
	second := first.NextCycle("OA-2", t2)
	second.Dimensions = []assessment.DimensionAssignment{
		{DimensionID: "market-investment-horizon", Category: &assessment.CategorySelection{OptionID: "som", Resolved: true}},
	}
	if err := s.SaveOpportunityAssessment(ctx, *second); err != nil {
		t.Fatalf("SaveOpportunityAssessment (cycle 2): %v", err)
	}

	oa1, err := s.GetOpportunityAssessment(ctx, "OA-1")
	if err != nil {
		t.Fatalf("GetOpportunityAssessment (OA-1 after supersede): %v", err)
	}
	if oa1.Cycle.Current {
		t.Error("OA-1.Cycle.Current = true, want false after being superseded by OA-2")
	}

	all, err := s.ListOpportunityAssessments(ctx, "OPP-1")
	if err != nil {
		t.Fatalf("ListOpportunityAssessments: %v", err)
	}
	if len(all) != 2 || all[0].ID != "OA-1" || all[1].ID != "OA-2" {
		t.Errorf("ListOpportunityAssessments = %+v, want [OA-1, OA-2] oldest first", all)
	}

	current, err := s.ListCurrentOpportunityAssessments(ctx)
	if err != nil {
		t.Fatalf("ListCurrentOpportunityAssessments: %v", err)
	}
	if len(current) != 1 || current[0].ID != "OA-2" {
		t.Errorf("ListCurrentOpportunityAssessments = %+v, want exactly [OA-2]", current)
	}

	if err := s.DeleteOpportunityAssessment(ctx, "OA-1"); err != nil {
		t.Fatalf("DeleteOpportunityAssessment: %v", err)
	}
	if _, err := s.GetOpportunityAssessment(ctx, "OA-1"); !omniroadmap.IsNotFound(err) {
		t.Errorf("GetOpportunityAssessment after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDoltStore_PortfolioDimension(t *testing.T) {
	startDoltServer(t)

	dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%d)/omniroadmap_dimension_test", testPort)
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

	// Built-ins register cleanly and idempotently.
	if err := s.EnsureBuiltinDimensions(ctx); err != nil {
		t.Fatalf("EnsureBuiltinDimensions: %v", err)
	}
	if err := s.EnsureBuiltinDimensions(ctx); err != nil {
		t.Fatalf("EnsureBuiltinDimensions (second call): %v", err)
	}

	kano, err := s.GetDimension(ctx, "kano", "1.0")
	if err != nil {
		t.Fatalf("GetDimension(kano): %v", err)
	}
	if kano.Name != "Kano" || len(kano.Options) != 5 {
		t.Errorf("GetDimension(kano) = %+v, want Name=Kano and 5 options", kano)
	}
	if kano.Options[0].ID != "must_be" {
		t.Errorf("GetDimension(kano).Options[0].ID = %q, want must_be (definition order preserved)", kano.Options[0].ID)
	}

	// Registering a custom dimension requires no schema change — just this
	// call.
	custom := assessment.DimensionDefinition{
		ID: "strategic-priority-2026", Name: "2026 Strategic Priority", Version: "1.0", Kind: assessment.DimensionKindTags,
		Options: []assessment.DimensionOption{
			{ID: "ai", Label: "AI"},
			{ID: "growth", Label: "Growth"},
			{ID: "excellence", Label: "Excellence"},
		},
	}
	if err := s.RegisterDimension(ctx, custom, false); err != nil {
		t.Fatalf("RegisterDimension (custom): %v", err)
	}

	got, err := s.GetDimension(ctx, "strategic-priority-2026", "1.0")
	if err != nil {
		t.Fatalf("GetDimension(strategic-priority-2026): %v", err)
	}
	if got.Kind != assessment.DimensionKindTags || len(got.Options) != 3 {
		t.Errorf("GetDimension(strategic-priority-2026) = %+v", got)
	}

	all, err := s.ListDimensions(ctx)
	if err != nil {
		t.Fatalf("ListDimensions: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("ListDimensions() = %d entries, want 3 (kano, mih, custom)", len(all))
	}

	customOnly, err := s.ListCustomDimensions(ctx)
	if err != nil {
		t.Fatalf("ListCustomDimensions: %v", err)
	}
	if len(customOnly) != 1 || customOnly[0].ID != "strategic-priority-2026" {
		t.Errorf("ListCustomDimensions() = %+v, want exactly the custom dimension", customOnly)
	}

	if _, err := s.GetDimension(ctx, "nonexistent", "1.0"); !omniroadmap.IsNotFound(err) {
		t.Errorf("GetDimension(nonexistent): err = %v, want ErrNotFound", err)
	}
}

func TestDoltStore_Evidence(t *testing.T) {
	startDoltServer(t)

	dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%d)/omniroadmap_evidence_test", testPort)
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

	now := time.Now().UTC().Truncate(time.Second)

	fresh := *assessment.NewEvidence("EV-1", "72% of customers run in AWS").
		WithSource("https://example.com/inventory", assessment.EvidenceSystemAnalytics, claims.ExternalCommunity, claims.ReliabilityHigh).
		WithExcerpt("72% of active accounts are hosted on AWS").
		WithCapturedAt(now).
		WithCapturedBy("pm:jwang").
		WithSensitivity(assessment.SensitivityInternal)

	stale := *assessment.NewEvidence("EV-2", "Customer X requires FedRAMP").
		WithSource("https://example.com/contract", assessment.EvidenceSystemContract, claims.ExternalCommunity, claims.ReliabilityHigh).
		WithExcerpt("Section 4.2: Provider shall achieve FedRAMP Moderate").
		WithCapturedAt(now.AddDate(-2, 0, 0)). // 2 years old
		WithSensitivity(assessment.SensitivityRestricted)

	if err := s.SaveEvidence(ctx, fresh); err != nil {
		t.Fatalf("SaveEvidence(fresh): %v", err)
	}
	if err := s.SaveEvidence(ctx, stale); err != nil {
		t.Fatalf("SaveEvidence(stale): %v", err)
	}

	got, err := s.GetEvidence(ctx, "EV-1")
	if err != nil {
		t.Fatalf("GetEvidence: %v", err)
	}
	if got.Excerpt() != "72% of active accounts are hosted on AWS" {
		t.Errorf("GetEvidence canonical excerpt = %q", got.Excerpt())
	}

	all, err := s.ListEvidence(ctx)
	if err != nil {
		t.Fatalf("ListEvidence: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("ListEvidence() = %d entries, want 2", len(all))
	}

	// A contract's DefaultValidityWindow is 0 (never expires); the
	// analytics-sourced evidence is well within its 90-day window — so
	// StaleEvidence should be EMPTY even though EV-2 is chronologically
	// two years old. This exercises the "contracts don't expire the way
	// deployment inventories do" rule.
	staleList, err := s.StaleEvidence(ctx, now)
	if err != nil {
		t.Fatalf("StaleEvidence: %v", err)
	}
	if len(staleList) != 0 {
		t.Errorf("StaleEvidence() = %+v, want empty (contract evidence never expires)", staleList)
	}

	// Re-save EV-1 with an analytics system but no capture time — always
	// stale per the "no recorded capture time" rule — to exercise the
	// citation-degradation path.
	uncaptured := *assessment.NewEvidence("EV-3", "60% of accounts use SSO").
		WithSource("https://example.com/other", assessment.EvidenceSystemAnalytics, claims.ExternalCommunity, claims.ReliabilityHigh)
	if err := s.SaveEvidence(ctx, uncaptured); err != nil {
		t.Fatalf("SaveEvidence(uncaptured): %v", err)
	}

	assessmentRef := assessment.OpportunityRef{SpecID: "OPP-1"}
	a := assessment.NewOpportunityAssessment("OA-1", assessmentRef, "SSO Rollout", now)
	a.RICE = &assessment.RICEAssessment{
		Reach: assessment.Reach{Fraction: 0.6, EvidenceIDs: []string{"EV-3"}},
	}
	if err := s.SaveOpportunityAssessment(ctx, *a); err != nil {
		t.Fatalf("SaveOpportunityAssessment: %v", err)
	}

	citations, err := s.EvidenceCitations(ctx, "EV-3")
	if err != nil {
		t.Fatalf("EvidenceCitations: %v", err)
	}
	if len(citations) != 1 || citations[0].AssessmentID != "OA-1" || citations[0].QuestionID != "rice.reach" {
		t.Errorf("EvidenceCitations(EV-3) = %+v, want one citation from OA-1/rice.reach", citations)
	}

	degraded, err := s.StaleEvidenceCitations(ctx, now)
	if err != nil {
		t.Fatalf("StaleEvidenceCitations: %v", err)
	}
	if refs, ok := degraded["EV-3"]; !ok || len(refs) != 1 || refs[0].AssessmentID != "OA-1" {
		t.Errorf("StaleEvidenceCitations() = %+v, want EV-3 -> [OA-1 citation] (no capture time recorded)", degraded)
	}

	if err := s.DeleteEvidence(ctx, "EV-1"); err != nil {
		t.Fatalf("DeleteEvidence: %v", err)
	}
	if _, err := s.GetEvidence(ctx, "EV-1"); !omniroadmap.IsNotFound(err) {
		t.Errorf("GetEvidence after delete: err = %v, want ErrNotFound", err)
	}
}

// TestDoltStore_Phase5Pipeline exercises compile -> review -> compile ->
// materialize end to end against a real DoltStore, proving *DoltStore
// genuinely satisfies compile.Store/review.Store/materialize.Store (not
// just the compile-time assertions in store_iface.go) and that the three
// packages compose correctly through real persistence.
func TestDoltStore_Phase5Pipeline(t *testing.T) {
	startDoltServer(t)

	dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%d)/omniroadmap_phase5_test", testPort)
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

	now := time.Now().UTC().Truncate(time.Second)
	gate := assessment.EstimabilityGate{
		ScopeDefined: true, ImplementationIdentified: true, DependenciesIdentified: true,
		TestingIdentified: true, DeploymentIdentified: true,
	}

	oa1 := assessment.NewOpportunityAssessment("OA-1", assessment.OpportunityRef{SpecID: "OPP-1"}, "Must-have platform work", now)
	oa1.MoSCoWAnswers = []assessment.ThresholdAnswer{{LevelID: "must", Satisfied: true, EvidenceIDs: []string{"EV-1"}}}
	oa1.RICE = &assessment.RICEAssessment{
		Reach:             assessment.Reach{Fraction: 0.5, EvidenceIDs: []string{"EV-2"}},
		ImpactAnswers:     []assessment.ThresholdAnswer{{LevelID: "high", Satisfied: true, EvidenceIDs: []string{"EV-3"}}},
		ConfidenceAnswers: []assessment.ThresholdAnswer{{LevelID: "high", Satisfied: true, EvidenceIDs: []string{"EV-4"}}},
		Effort:            assessment.EffortEstimate{Expected: 10, Gate: gate},
	}

	oa2 := assessment.NewOpportunityAssessment("OA-2", assessment.OpportunityRef{SpecID: "OPP-2"}, "Should-have improvement", now)
	oa2.MoSCoWAnswers = []assessment.ThresholdAnswer{{LevelID: "should", Satisfied: true, EvidenceIDs: []string{"EV-5"}}}
	oa2.RICE = &assessment.RICEAssessment{
		Reach:             assessment.Reach{Fraction: 0.8, EvidenceIDs: []string{"EV-6"}},
		ImpactAnswers:     []assessment.ThresholdAnswer{{LevelID: "massive", Satisfied: true, EvidenceIDs: []string{"EV-7"}}},
		ConfidenceAnswers: []assessment.ThresholdAnswer{{LevelID: "high", Satisfied: true, EvidenceIDs: []string{"EV-8"}}},
		Effort:            assessment.EffortEstimate{Expected: 5, Gate: gate},
	}

	if err := s.SaveOpportunityAssessment(ctx, *oa1); err != nil {
		t.Fatalf("SaveOpportunityAssessment(OA-1): %v", err)
	}
	if err := s.SaveOpportunityAssessment(ctx, *oa2); err != nil {
		t.Fatalf("SaveOpportunityAssessment(OA-2): %v", err)
	}

	// First compile: no overrides yet, pure calculated ranking. A Should
	// can have a huge RICE score and still not outrank a Must.
	policy := assessment.DefaultRankingPolicy()
	draft, err := compile.Compile(ctx, s, "run-1", now, policy)
	if err != nil {
		t.Fatalf("compile.Compile: %v", err)
	}
	if draft.Ranking[0].AssessmentID != "OA-1" {
		t.Fatalf("Ranking[0] = %+v, want OA-1 (Must beats Should)", draft.Ranking[0])
	}

	// PM review: override OA-2 to rank 1 with a governance rationale.
	edit := review.Edit{
		Kind: review.EditOverride,
		Override: &assessment.RankOverride{
			AssessmentID: "OA-2", FinalRank: 1, Rationale: "exec ask", ApprovedBy: "vp-product",
		},
	}
	if err := review.Apply(ctx, s, edit); err != nil {
		t.Fatalf("review.Apply: %v", err)
	}

	// Recompile: the edit must be reflected without any special-casing —
	// compile just re-reads whatever overrides are currently persisted.
	final, err := compile.Compile(ctx, s, "run-2", now.Add(time.Minute), policy)
	if err != nil {
		t.Fatalf("compile.Compile (after review): %v", err)
	}
	if final.Ranking[0].AssessmentID != "OA-2" || final.Ranking[0].FinalRank != 1 {
		t.Fatalf("Ranking[0] = %+v, want OA-2 at final rank 1 after override", final.Ranking[0])
	}
	if final.Deltas == nil || len(final.Deltas.RankMoves) == 0 {
		t.Errorf("expected Deltas.RankMoves to reflect the reordering, got %+v", final.Deltas)
	}

	// Materialize: write the reviewed ranking back onto the assessment
	// rows and mark the dataset final.
	if err := materialize.Materialize(ctx, s, "run-2", final); err != nil {
		t.Fatalf("materialize.Materialize: %v", err)
	}

	datasetRow, err := s.Client().ReportDataset.Get(ctx, "run-2")
	if err != nil {
		t.Fatalf("ReportDataset.Get: %v", err)
	}
	if datasetRow.Status != materialize.StatusFinal {
		t.Errorf("dataset status = %q, want %q", datasetRow.Status, materialize.StatusFinal)
	}

	// GetOpportunityAssessment returns the canonical record, which does
	// not carry the rank projection columns — check the Ent row directly
	// to verify SetOpportunityRank actually wrote them.
	assessmentRow, err := s.Client().OpportunityAssessment.Get(ctx, "OA-2")
	if err != nil {
		t.Fatalf("OpportunityAssessment.Get(OA-2): %v", err)
	}
	if assessmentRow.OpportunityRankFinal == nil || *assessmentRow.OpportunityRankFinal != 1 {
		t.Errorf("OA-2 OpportunityRankFinal = %v, want 1", assessmentRow.OpportunityRankFinal)
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

func mustNormalizeCompass(t *testing.T) rice.Normalized {
	t.Helper()
	n, err := catalog.NormalizeDocument([]byte(validCustomerB2BDoc))
	if err != nil {
		t.Fatalf("NormalizeDocument: %v", err)
	}
	return n
}

func TestDoltStore_ProfileAssignment(t *testing.T) {
	startDoltServer(t)

	dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%d)/omniroadmap_profileassignment_test", testPort)
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

	proposed := assessment.ProposeProfileAssignment("OPP-1", "customer/b2b/v1", "primarily a retention play", "judge-session-9")
	proposed.Secondary = []rice.Profile{rice.ProfileRisk}
	proposed.EvidenceIDs = []string{"EV-1"}
	if err := s.SaveProfileAssignment(ctx, proposed); err != nil {
		t.Fatalf("SaveProfileAssignment: %v", err)
	}

	got, err := s.GetProfileAssignment(ctx, "OPP-1")
	if err != nil {
		t.Fatalf("GetProfileAssignment: %v", err)
	}
	if got.Status != assessment.ProfileAssignmentProposed {
		t.Errorf("Status = %q, want %q", got.Status, assessment.ProfileAssignmentProposed)
	}
	if len(got.Secondary) != 1 || got.Secondary[0] != rice.ProfileRisk {
		t.Errorf("Secondary = %+v, want [%s]", got.Secondary, rice.ProfileRisk)
	}
	if len(got.EvidenceIDs) != 1 || got.EvidenceIDs[0] != "EV-1" {
		t.Errorf("EvidenceIDs = %+v, want [EV-1]", got.EvidenceIDs)
	}

	confirmed := got.Confirm("pm@example.com", time.Now().UTC().Truncate(time.Second))
	if err := s.SaveProfileAssignment(ctx, confirmed); err != nil {
		t.Fatalf("SaveProfileAssignment (confirmed): %v", err)
	}

	got2, err := s.GetProfileAssignment(ctx, "OPP-1")
	if err != nil {
		t.Fatalf("GetProfileAssignment (after confirm): %v", err)
	}
	if got2.Status != assessment.ProfileAssignmentConfirmed || got2.ConfirmedBy != "pm@example.com" || got2.ConfirmedAt.IsZero() {
		t.Errorf("GetProfileAssignment after confirm = %+v", got2)
	}

	all, err := s.ListProfileAssignments(ctx)
	if err != nil {
		t.Fatalf("ListProfileAssignments: %v", err)
	}
	if len(all) != 1 || all[0].SpecID != "OPP-1" {
		t.Errorf("ListProfileAssignments = %+v, want exactly [OPP-1]", all)
	}

	if err := s.DeleteProfileAssignment(ctx, "OPP-1"); err != nil {
		t.Fatalf("DeleteProfileAssignment: %v", err)
	}
	if _, err := s.GetProfileAssignment(ctx, "OPP-1"); !omniroadmap.IsNotFound(err) {
		t.Errorf("GetProfileAssignment after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDoltStore_OpportunityAssessment_CompassProjection(t *testing.T) {
	startDoltServer(t)

	dsn := fmt.Sprintf("root:@tcp(127.0.0.1:%d)/omniroadmap_compassprojection_test", testPort)
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

	normalized := mustNormalizeCompass(t)
	a := assessment.NewOpportunityAssessment("OA-COMPASS-1", assessment.OpportunityRef{SpecID: "OPP-COMPASS-1"}, "Compass-scored opportunity", time.Now().UTC().Truncate(time.Second))
	a.Compass = &assessment.CompassAssessment{
		ProfileID:  normalized.ProfileID,
		Normalized: normalized,
	}
	// A legacy RICE input is also present, to prove the projection prefers
	// Compass over it -- matching ToRankInput's own precedence.
	a.RICE = &assessment.RICEAssessment{
		Reach:  assessment.Reach{Fraction: 0.9, EvidenceIDs: []string{"EV-1"}},
		Effort: assessment.EffortEstimate{Expected: 1},
	}

	if err := s.SaveOpportunityAssessment(ctx, *a); err != nil {
		t.Fatalf("SaveOpportunityAssessment: %v", err)
	}

	row, err := s.Client().OpportunityAssessment.Get(ctx, "OA-COMPASS-1")
	if err != nil {
		t.Fatalf("OpportunityAssessment.Get: %v", err)
	}
	if row.CompassProfileID != string(normalized.ProfileID) {
		t.Errorf("CompassProfileID = %q, want %q", row.CompassProfileID, normalized.ProfileID)
	}
	if !row.RiceComputable {
		t.Error("RiceComputable = false, want true")
	}
	wantScore, err := normalized.Score()
	if err != nil {
		t.Fatalf("Score: %v", err)
	}
	if row.RiceScore == nil || *row.RiceScore != wantScore {
		t.Errorf("RiceScore = %v, want %v (the Compass score, not the legacy RICE score)", row.RiceScore, wantScore)
	}
}
