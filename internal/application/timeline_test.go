package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

func seedTimelineFixture(t *testing.T, uow application.UnitOfWork) application.TimelineInput {
	t.Helper()
	pid, err := domain.NewProjectID("PRJ-1")
	if err != nil {
		t.Fatal(err)
	}
	project, err := domain.NewProject(pid, "Belcanto Pilot", fixedTime())
	if err != nil {
		t.Fatal(err)
	}
	fid, err := domain.NewFeatureCardID("FC-1")
	if err != nil {
		t.Fatal(err)
	}
	card, err := domain.NewFeatureCard(fid, pid, "Homework after a lesson", "", fixedTime().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Projects.Put(context.Background(), project); err != nil {
			return err
		}
		return r.FeatureCards.Put(context.Background(), card)
	})
	if err != nil {
		t.Fatal(err)
	}

	err = uow.Do(context.Background(), func(r application.Repositories) error {
		if err := r.Artifacts.Put(context.Background(), mustArtEnv(t, "CAP-1")); err != nil {
			return err
		}
		if err := r.Revisions.Put(context.Background(), mustRevEnv(t, "CAP-1", "CAP-1-REV-1")); err != nil {
			return err
		}
		order := mustOrder(t, "CAP-1", "CAP-1-REV-1", 1, fixedTime())
		if err := r.RevisionOrder.Put(context.Background(), order); err != nil {
			return err
		}
		acc := mustAcceptance(t, "ACC-1", "CAP-1", "CAP-1-REV-1", engineering.AcceptanceStateAccepted, fixedTime().Add(2*time.Minute))
		return r.RevisionAcceptance.Append(context.Background(), acc)
	})
	if err != nil {
		t.Fatal(err)
	}

	return application.TimelineInput{
		Project: project, FeatureCard: card, CapabilityArtifactID: "CAP-1",
	}
}

func TestCanonicalTimelineOrdering(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	in := seedTimelineFixture(t, uow)

	var result application.TimelineResult
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = application.GetFeatureTimeline(context.Background(), r, in)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dated) < 4 {
		t.Fatalf("expected at least 4 dated events (project, feature, capability, revision, acceptance), got %d: %+v", len(result.Dated), result.Dated)
	}
	for i := 1; i < len(result.Dated); i++ {
		a, b := result.Dated[i-1], result.Dated[i]
		if a.OccurredAt.After(b.OccurredAt) {
			t.Errorf("event %d occurs after event %d: %v > %v", i-1, i, a.OccurredAt, b.OccurredAt)
		}
	}
	if result.Dated[0].Kind != application.EventProjectCreated {
		t.Errorf("first event = %v, want project.created", result.Dated[0].Kind)
	}
}

func TestTimelineInsertionOrderIndependence(t *testing.T) {
	uow1 := newStoreWithRecordSubject(t)
	in1 := seedTimelineFixture(t, uow1)
	var result1 application.TimelineResult
	err := uow1.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result1, err = application.GetFeatureTimeline(context.Background(), r, in1)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	uow2 := newStoreWithRecordSubject(t)
	in2 := seedTimelineFixture(t, uow2)
	var result2 application.TimelineResult
	err = uow2.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result2, err = application.GetFeatureTimeline(context.Background(), r, in2)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result1.Dated) != len(result2.Dated) {
		t.Fatalf("lengths differ: %d vs %d", len(result1.Dated), len(result2.Dated))
	}
	for i := range result1.Dated {
		if result1.Dated[i].EventID != result2.Dated[i].EventID {
			t.Errorf("event %d differs: %s vs %s", i, result1.Dated[i].EventID, result2.Dated[i].EventID)
		}
	}
}

func TestEventIDIsDerived(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	in := seedTimelineFixture(t, uow)
	var result application.TimelineResult
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = application.GetFeatureTimeline(context.Background(), r, in)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range result.Dated {
		want := string(e.Kind) + ":" + e.SourceIdentity
		if e.EventID != want {
			t.Errorf("EventID = %q, want %q", e.EventID, want)
		}
	}
}

func TestUndatedGroupIsSeparate(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	in := seedTimelineFixture(t, uow)
	// Add a decision record with no OccurredAt.
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		key, err := engineering.NewRecordKey(engineering.RecordKindDecision, "DEC-1")
		if err != nil {
			return err
		}
		payload := []byte(`{"decision_id":"DEC-1"}`)
		env, err := engineering.NewRecordEnvelope(engineering.RecordEnvelopeInput{
			Key: key, SubjectKey: "artifact-revision:CAP-1/CAP-1-REV-1", HasOccurredAt: false,
			Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedTime(),
		})
		if err != nil {
			return err
		}
		return r.Records.Put(context.Background(), env)
	})
	if err != nil {
		t.Fatal(err)
	}
	in.DecisionIDs = []string{"DEC-1"}

	var result application.TimelineResult
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = application.GetFeatureTimeline(context.Background(), r, in)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Undated) != 1 || result.Undated[0].SourceIdentity != "decision:DEC-1" {
		t.Errorf("Undated = %+v, want exactly the undated decision", result.Undated)
	}
	for _, e := range result.Dated {
		if e.SourceIdentity == "decision:DEC-1" {
			t.Error("the undated event must not appear in the dated slice")
		}
	}
}

func TestCorrectedClaimRendersLink(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	in := seedTimelineFixture(t, uow)
	original := mustClaimEnv(t, "CLM-2", "peos:satisfied", nil, "")
	putClaim(t, uow, original)
	corrected := mustClaimEnv(t, "CLM-4", "peos:not-satisfied", &original, "peos:correct")
	putClaim(t, uow, corrected)
	in.ClaimIDs = []string{"CLM-2", "CLM-4"}

	var result application.TimelineResult
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = application.GetFeatureTimeline(context.Background(), r, in)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundOriginal, foundCorrecting bool
	for _, e := range result.Dated {
		if e.SourceIdentity == "claim:CLM-2" {
			foundOriginal = true
			if e.Kind != application.EventClaimRecorded {
				t.Errorf("original claim kind = %v, want claim.recorded", e.Kind)
			}
		}
		if e.SourceIdentity == "claim:CLM-4" {
			foundCorrecting = true
			if e.Kind != application.EventClaimCorrected || e.Corrected != "CLM-2" {
				t.Errorf("correcting claim event = %+v, want claim.corrected linking to CLM-2", e)
			}
		}
	}
	if !foundOriginal || !foundCorrecting {
		t.Error("both the original and correcting claim must appear on the timeline")
	}
}

func TestDanglingReferenceFails(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	in := seedTimelineFixture(t, uow)
	ghost := engineering.RecordEnvelope{Key: engineering.RecordKey{Kind: engineering.RecordKindClaim, ID: "CLM-GHOST"}}
	dangling := mustClaimEnv(t, "CLM-1", "peos:not-satisfied", &ghost, "peos:correct")
	putClaim(t, uow, dangling)
	in.ClaimIDs = []string{"CLM-1"}

	err := uow.Do(context.Background(), func(r application.Repositories) error {
		_, err := application.GetFeatureTimeline(context.Background(), r, in)
		return err
	})
	if !errors.Is(err, application.ErrTimelineSourceInvalid) {
		t.Errorf("err = %v, want ErrTimelineSourceInvalid", err)
	}
}

func TestInterruptedOutcomeRenderedVerbatim(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	in := seedTimelineFixture(t, uow)
	putExecution(t, uow, "ER-1", "peos:interrupted")
	in.ExecutionIDs = []string{"ER-1"}

	var result application.TimelineResult
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = application.GetFeatureTimeline(context.Background(), r, in)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range result.Dated {
		if e.SourceIdentity == "execution:ER-1" {
			found = true
			if e.Summary != "peos:interrupted" {
				t.Errorf("Summary = %q, want peos:interrupted rendered verbatim", e.Summary)
			}
		}
	}
	if !found {
		t.Error("expected the execution event to be present")
	}
}

func TestTimelineIsDeterministic(t *testing.T) {
	uow := newStoreWithRecordSubject(t)
	in := seedTimelineFixture(t, uow)
	var first application.TimelineResult
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		first, err = application.GetFeatureTimeline(context.Background(), r, in)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for i := range 10 {
		var got application.TimelineResult
		err := uow.Do(context.Background(), func(r application.Repositories) error {
			var err error
			got, err = application.GetFeatureTimeline(context.Background(), r, in)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Dated) != len(first.Dated) {
			t.Fatalf("iteration %d: length diverged", i)
		}
		for j := range got.Dated {
			if got.Dated[j].EventID != first.Dated[j].EventID {
				t.Fatalf("iteration %d: event %d diverged", i, j)
			}
		}
	}
}
