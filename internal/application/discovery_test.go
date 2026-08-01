package application_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// seedCapability establishes PRJ-1, FC-1, and CAP-1 with its founding
// revision accepted at sequence 1 -- the minimum fixture every discovery
// test in this file builds on (FF-018 §16 step 3).
func seedCapability(t *testing.T, f commandFixture) {
	t.Helper()
	ctx := context.Background()
	if _, err := (application.CreateProjectCommand{ProjectID: "PRJ-1", Name: "Pilot"}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.CreateFeatureCommand{FeatureCardID: "FC-1", ProjectID: "PRJ-1", Title: "Homework"}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-1", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1",
		Content: mustContent(t, "Homework after a lesson"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-1", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-1", State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
}

// establishPlan records a minimal validation plan scoped to CAP-1, for the
// §6.6 plan-selection tests below.
func establishPlan(t *testing.T, f commandFixture, artifactID, revisionID string) {
	t.Helper()
	ctx := context.Background()
	requirementExists := false
	if err := f.uow.Do(ctx, func(r application.Repositories) error {
		var err error
		_, requirementExists, err = r.Revisions.Get(ctx, mustRevKey(t, "REQ-1", "REQ-1-REV-1"))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if !requirementExists {
		if _, err := (application.EstablishRequirementCommand{
			ArtifactID: "REQ-1", RevisionID: "REQ-1-REV-1",
			Statement: "The system SHALL do X.", SubjectArtifactID: "CAP-1",
			SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
			AcceptanceRecordID: memberID("MEM-REQ-1"),
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (application.EstablishValidationPlanCommand{
		ArtifactID: artifactID, RevisionID: revisionID, ScopeArtifactID: "CAP-1",
		AcceptanceRecordID: memberID("MEM-" + artifactID + "-PLAN"),
		Activities: []application.PlanActivityCommandInput{{
			Key: "A-1", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Method: "manual-review", OutcomeInterpretation: "Satisfied when reviewed.",
			RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
			ExpectedEvidence: []string{"reviewer note"},
		}},
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
}

func doDiscovery[T any](t *testing.T, uow application.UnitOfWork, fn func(application.Repositories) (T, error)) T {
	t.Helper()
	var result T
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = fn(r)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func assertStringsEqual(t *testing.T, got, want []string) {
	t.Helper()
	gotSorted := append([]string(nil), got...)
	sort.Strings(gotSorted)
	if len(gotSorted) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if gotSorted[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// TestDiscoverDecisionIDs proves decisions are found by revision subject,
// across every revision of the artifact (not only the current one), and
// that an unrelated capability's decision is excluded (FF-018 §6.3).
func TestDiscoverDecisionIDs(t *testing.T) {
	f := newCommandFixture()
	seedCapability(t, f)
	ctx := context.Background()
	seedEvidenceRevision(t, f, "EV-1", "EV-1-REV-1")
	seedEvidenceRevision(t, f, "EV-2", "EV-2-REV-1")

	for _, id := range []string{"DEC-2", "DEC-1"} { // reversed insertion order
		if _, err := (application.RecordArchitectureDecisionCommand{
			DecisionID: id, SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Question: "Should X?", OutcomeStatement: "X, because Y.",
			EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1",
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
	}
	artifactDecision := application.RecordArchitectureDecisionCommand{
		DecisionID: "DEC-ARTIFACT", SubjectArtifactID: "CAP-1",
		Question: "Which rule governs the capability?", OutcomeStatement: "One rule governs every revision.",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1",
	}
	if _, err := artifactDecision.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("record Artifact-subject decision: %v", err)
	}
	f.clock.Advance(time.Hour)
	if _, err := artifactDecision.Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatalf("replay Artifact-subject decision: %v", err)
	}

	// An unrelated capability's decision must not appear.
	if _, err := (application.CreateFeatureCommand{FeatureCardID: "FC-2", ProjectID: "PRJ-1", Title: "Unrelated"}).Execute(ctx, f.uow, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.EstablishCapabilitySpecificationCommand{
		FeatureCardID: "FC-2", ArtifactID: "CAP-2", RevisionID: "CAP-2-REV-1",
		Content: mustContent(t, "Unrelated capability"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.RecordArchitectureDecisionCommand{
		DecisionID: "DEC-UNRELATED", SubjectArtifactID: "CAP-2", SubjectRevisionID: "CAP-2-REV-1",
		Question: "Should Z?", OutcomeStatement: "Z, because W.",
		EvidenceArtifactID: "EV-2", EvidenceRevisionID: "EV-2-REV-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	got := doDiscovery(t, f.uow, func(r application.Repositories) ([]string, error) {
		return application.DiscoverDecisionIDs(ctx, r, f.rec, "CAP-1")
	})
	assertStringsEqual(t, got, []string{"DEC-1", "DEC-2", "DEC-ARTIFACT"})

	// Every revision is consulted, not only the current one: revising CAP-1
	// must not make DEC-1/DEC-2 (recorded against revision 1) disappear.
	if _, err := (application.ReviseCapabilitySpecificationCommand{
		ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2", Content: mustContent(t, "Homework after a lesson, revised"),
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.AcceptCapabilityRevisionCommand{
		RecordID: "ACC-2", ArtifactID: "CAP-1", RevisionID: "CAP-1-REV-2", State: engineering.AcceptanceStateAccepted,
	}).Execute(ctx, f.uow, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	gotAfterRevision := doDiscovery(t, f.uow, func(r application.Repositories) ([]string, error) {
		return application.DiscoverDecisionIDs(ctx, r, f.rec, "CAP-1")
	})
	assertStringsEqual(t, gotAfterRevision, []string{"DEC-1", "DEC-2", "DEC-ARTIFACT"})
}

// TestDiscoverExecutionAndClaimIDs proves both are found scoped to one
// capability revision, independently deduplicated and sorted (FF-018 §6.3).
func TestDiscoverExecutionAndClaimIDs(t *testing.T) {
	f := newCommandFixture()
	seedCapability(t, f)
	establishPlan(t, f, "VP-1", "VP-1-REV-1")
	ctx := context.Background()

	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: "ER-1", PlanArtifactID: "VP-1", PlanRevisionID: "VP-1-REV-1", ActivityKey: "A-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", EvidenceLocator: "https://evidence.example/EV-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.RecordValidationClaimCommand{
		ClaimID: "CLM-1", ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
		Reasoning: "The specification states it explicitly.",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	var gotExecutions, gotClaims []string
	err := f.uow.Do(ctx, func(r application.Repositories) error {
		var err error
		gotExecutions, gotClaims, err = application.DiscoverExecutionAndClaimIDs(ctx, r, f.rec, "CAP-1", "CAP-1-REV-1")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	assertStringsEqual(t, gotExecutions, []string{"ER-1"})
	assertStringsEqual(t, gotClaims, []string{"CLM-1"})
}

func TestAuthoritativeDiscoveryRejectsInverseProjectionOmission(t *testing.T) {
	t.Run("revision payload hidden under another family projection", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		artifact, revision, err := f.rec.RecordRequirement(engineering.RequirementInput{
			ArtifactID: "REQ-HIDDEN", RevisionID: "REQ-HIDDEN-REV-1",
			Statement:         "The system SHALL reject inverse family projection drift.",
			SubjectArtifactID: "CAP-1", RecordedAt: f.clock.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		revision.RevisionFamily = engineering.RevisionFamilyValidationPlan
		if err := f.uow.Do(context.Background(), func(repos application.Repositories) error {
			if err := repos.Artifacts.Put(context.Background(), artifact); err != nil {
				return err
			}
			return repos.Revisions.Put(context.Background(), revision)
		}); err != nil {
			t.Fatal(err)
		}
		err = f.uow.Do(context.Background(), func(repos application.Repositories) error {
			_, err := application.DiscoverRequirementArtifactIDs(context.Background(), repos, f.rec, "CAP-1")
			return err
		})
		if !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
		}
	})

	t.Run("decision payload hidden under claim kind projection", func(t *testing.T) {
		f := newCommandFixture()
		seedCapability(t, f)
		decision, err := f.rec.RecordDecision(engineering.DecisionInput{
			DecisionID: "DEC-HIDDEN", SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
			Question: "Can kind projection drift be ignored?", OutcomeStatement: "No, discovery validates every record envelope.",
			EvidenceArtifactID: "EV-HIDDEN", EvidenceRevisionID: "EV-HIDDEN-REV-1", RecordedAt: f.clock.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		decision.Key = engineering.RecordKey{Kind: engineering.RecordKindClaim, ID: decision.Key.ID}
		decision.Kind = engineering.RecordKindClaim
		if err := f.uow.Do(context.Background(), func(repos application.Repositories) error {
			return repos.Records.Put(context.Background(), decision)
		}); err != nil {
			t.Fatal(err)
		}
		err = f.uow.Do(context.Background(), func(repos application.Repositories) error {
			_, err := application.DiscoverDecisionIDs(context.Background(), repos, f.rec, "CAP-1")
			return err
		})
		if !errors.Is(err, application.ErrStoredStateIntegrity) {
			t.Fatalf("err = %v, want ErrStoredStateIntegrity", err)
		}
	})
}

// TestDiscoverEvidenceArtifactIDs proves evidence is recovered from
// EvidenceKeys via ParseEvidenceKey, deduplicated across both record kinds
// (FF-018 §6.3, §7).
func TestDiscoverEvidenceArtifactIDs(t *testing.T) {
	f := newCommandFixture()
	seedCapability(t, f)
	establishPlan(t, f, "VP-1", "VP-1-REV-1")
	ctx := context.Background()

	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: "ER-1", PlanArtifactID: "VP-1", PlanRevisionID: "VP-1-REV-1", ActivityKey: "A-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", EvidenceLocator: "https://evidence.example/EV-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	if _, err := (application.RecordValidationClaimCommand{
		ClaimID: "CLM-1", ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
		Reasoning: "The specification states it explicitly.",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	got := doDiscovery(t, f.uow, func(r application.Repositories) ([]string, error) {
		return application.DiscoverEvidenceArtifactIDs(ctx, r, f.rec, []string{"ER-1"}, []string{"CLM-1"})
	})
	// EV-1 is cited by both the execution and the claim; deduplicated to one.
	assertStringsEqual(t, got, []string{"EV-1"})
}

// TestDiscoverRequirementArtifactIDsIsIndependentOfClaims is the FF-011
// REQ-4 counterexample in miniature (AD-025): a requirement with no claim
// must still be discovered, proving DiscoverRequirementArtifactIDs never
// consults claims.
func TestDiscoverRequirementArtifactIDsIsIndependentOfClaims(t *testing.T) {
	f := newCommandFixture()
	seedCapability(t, f)
	ctx := context.Background()

	for _, id := range []string{"REQ-1", "REQ-2"} {
		if _, err := (application.EstablishRequirementCommand{
			ArtifactID: id, RevisionID: id + "-REV-1", Statement: "Statement.", SubjectArtifactID: "CAP-1",
			SourceCapabilityRevisionID: "CAP-1-REV-1", SourceAcceptanceCriterionKey: "AC-1",
			AcceptanceRecordID: memberID("MEM-" + id),
		}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
			t.Fatal(err)
		}
	}
	establishPlan(t, f, "VP-1", "VP-1-REV-1")
	if _, err := (application.RecordValidationRunCommand{
		ExecutionID: "ER-1", PlanArtifactID: "VP-1", PlanRevisionID: "VP-1-REV-1", ActivityKey: "A-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		Method: "manual-review", Outcome: "completed",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", EvidenceLocator: "https://evidence.example/EV-1",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}
	// REQ-2 is never claimed.
	if _, err := (application.RecordValidationClaimCommand{
		ClaimID: "CLM-1", ScopeArtifactID: "CAP-1",
		SubjectArtifactID: "CAP-1", SubjectRevisionID: "CAP-1-REV-1",
		RequirementArtifactID: "REQ-1", RequirementRevisionID: "REQ-1-REV-1",
		Outcome: "satisfied", Method: "manual-review",
		EvidenceArtifactID: "EV-1", EvidenceRevisionID: "EV-1-REV-1", ExecutionID: "ER-1",
		Reasoning: "The specification states it explicitly.",
	}).Execute(ctx, f.uow, f.rec, f.rec, f.clock); err != nil {
		t.Fatal(err)
	}

	got := doDiscovery(t, f.uow, func(r application.Repositories) ([]string, error) {
		return application.DiscoverRequirementArtifactIDs(ctx, r, f.rec, "CAP-1")
	})
	assertStringsEqual(t, got, []string{"REQ-1", "REQ-2"})
}

// TestResolveApplicableValidationPlanIDZero proves zero discovered plans is
// legal: an empty PlanArtifactID with no error (FF-018 §6.6).
func TestResolveApplicableValidationPlanIDZero(t *testing.T) {
	f := newCommandFixture()
	seedCapability(t, f)
	ctx := context.Background()

	got := doDiscovery(t, f.uow, func(r application.Repositories) (string, error) {
		return application.ResolveApplicableValidationPlanID(ctx, r, f.rec, "CAP-1")
	})
	if got != "" {
		t.Errorf("planID = %q, want empty", got)
	}
}

// TestResolveApplicableValidationPlanIDOne proves exactly one discovered
// plan is used (FF-018 §6.6).
func TestResolveApplicableValidationPlanIDOne(t *testing.T) {
	f := newCommandFixture()
	seedCapability(t, f)
	establishPlan(t, f, "VP-1", "VP-1-REV-1")
	ctx := context.Background()

	got := doDiscovery(t, f.uow, func(r application.Repositories) (string, error) {
		return application.ResolveApplicableValidationPlanID(ctx, r, f.rec, "CAP-1")
	})
	if got != "VP-1" {
		t.Errorf("planID = %q, want VP-1", got)
	}
}

// TestResolveApplicableValidationPlanIDMany proves more than one discovered
// plan is ErrValidationPlanAmbiguous, and that no plan is selected -- not
// even the sorted-first one (FF-018 §6.6).
func TestResolveApplicableValidationPlanIDMany(t *testing.T) {
	f := newCommandFixture()
	seedCapability(t, f)
	establishPlan(t, f, "VP-1", "VP-1-REV-1")
	establishPlan(t, f, "VP-2", "VP-2-REV-1")
	ctx := context.Background()

	err := f.uow.Do(ctx, func(r application.Repositories) error {
		got, err := application.ResolveApplicableValidationPlanID(ctx, r, f.rec, "CAP-1")
		if got != "" {
			t.Errorf("planID = %q, want empty (no plan selected on ambiguity)", got)
		}
		return err
	})
	if !errors.Is(err, application.ErrValidationPlanAmbiguous) {
		t.Errorf("err = %v, want ErrValidationPlanAmbiguous", err)
	}
}

// TestResolveApplicableValidationPlanIDOrderingDoesNotResolveAmbiguity
// proves sort order is never mistaken for precedence: two plans established
// in opposite order on two independent stores produce the identical
// ambiguity failure either way (FF-018 §6.6, §18 criterion 7a).
func TestResolveApplicableValidationPlanIDOrderingDoesNotResolveAmbiguity(t *testing.T) {
	ctx := context.Background()

	fA := newCommandFixture()
	seedCapability(t, fA)
	establishPlan(t, fA, "VP-1", "VP-1-REV-1")
	establishPlan(t, fA, "VP-2", "VP-2-REV-1")

	fB := newCommandFixture()
	seedCapability(t, fB)
	establishPlan(t, fB, "VP-2", "VP-2-REV-1")
	establishPlan(t, fB, "VP-1", "VP-1-REV-1")

	var errA, errB error
	_ = fA.uow.Do(ctx, func(r application.Repositories) error {
		_, errA = application.ResolveApplicableValidationPlanID(ctx, r, fA.rec, "CAP-1")
		return nil
	})
	_ = fB.uow.Do(ctx, func(r application.Repositories) error {
		_, errB = application.ResolveApplicableValidationPlanID(ctx, r, fB.rec, "CAP-1")
		return nil
	})

	if !errors.Is(errA, application.ErrValidationPlanAmbiguous) || !errors.Is(errB, application.ErrValidationPlanAmbiguous) {
		t.Fatalf("errA = %v, errB = %v, want both ErrValidationPlanAmbiguous", errA, errB)
	}
	if errA.Error() != errB.Error() {
		t.Errorf("error message differs by insertion order: %q vs %q", errA.Error(), errB.Error())
	}
}
