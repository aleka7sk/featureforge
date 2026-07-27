package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

func putStateAssignment(t *testing.T, uow application.UnitOfWork, assignmentID, stateID string, effectiveAt time.Time) {
	t.Helper()
	key, err := engineering.NewRecordKey(engineering.RecordKindStateAssignment, assignmentID)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte(`{"assignment_id":"` + assignmentID + `"}`)
	env, err := engineering.NewRecordEnvelope(engineering.RecordEnvelopeInput{
		Key: key, SubjectKey: engineering.ArtifactSubjectKey("CAP-1"),
		OccurredAt: effectiveAt, HasOccurredAt: true, StateID: stateID,
		Payload: payload, PayloadDigest: engineering.ComputeDigest(payload), RecordedAt: fixedTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	err = uow.Do(context.Background(), func(r application.Repositories) error {
		return r.Records.Put(context.Background(), env)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func resolveLifecycle(t *testing.T, uow application.UnitOfWork) (application.LifecycleStateResult, error) {
	t.Helper()
	var result application.LifecycleStateResult
	err := uow.Do(context.Background(), func(r application.Repositories) error {
		var err error
		result, err = application.ResolveLifecycleState(context.Background(), r, "CAP-1")
		return err
	})
	return result, err
}

func TestNoAssignmentsReturnsNone(t *testing.T) {
	uow := newStoreAndUOW()
	result, err := resolveLifecycle(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if result.Found {
		t.Error("expected Found = false")
	}
}

func TestLatestEffectiveAtWins(t *testing.T) {
	uow := newStoreAndUOW()
	putStateAssignment(t, uow, "SA-1", "featureforge:drafting", fixedTime())
	putStateAssignment(t, uow, "SA-2", "featureforge:under-validation", fixedTime().Add(time.Hour))
	result, err := resolveLifecycle(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || result.Assignment.StateID != "featureforge:under-validation" {
		t.Errorf("result = %+v, want under-validation", result)
	}
}

func TestEqualTimestampSameStateTieBreaks(t *testing.T) {
	uow := newStoreAndUOW()
	putStateAssignment(t, uow, "SA-2", "featureforge:drafting", fixedTime())
	putStateAssignment(t, uow, "SA-1", "featureforge:drafting", fixedTime())
	result, err := resolveLifecycle(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Found || result.Assignment.Key.ID != "SA-1" {
		t.Errorf("result = %+v, want the lower record ID SA-1", result)
	}
	if !result.Rationale.Duplicate {
		t.Error("expected the rationale to note the duplicate")
	}
}

func TestEqualTimestampDifferentStatesFails(t *testing.T) {
	uow := newStoreAndUOW()
	putStateAssignment(t, uow, "SA-1", "featureforge:drafting", fixedTime())
	putStateAssignment(t, uow, "SA-2", "featureforge:specified", fixedTime())
	_, err := resolveLifecycle(t, uow)
	if !errors.Is(err, application.ErrAmbiguousLifecycleState) {
		t.Errorf("err = %v, want ErrAmbiguousLifecycleState", err)
	}
}

func TestAssessedAndNotReadyCoexist(t *testing.T) {
	// AD-018: lifecycle state must not duplicate readiness. A capability
	// can be assessed and still not-ready.
	uow, current := seedReadinessScenario(t)
	putExecution(t, uow, "ER-1", "peos:completed")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:not-satisfied", "ER-1")
	readiness := resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1)})
	if readiness.Status != application.ReadinessNotReady {
		t.Fatalf("precondition failed: readiness = %v, want not-ready", readiness.Status)
	}

	putStateAssignment(t, uow, "SA-1", "featureforge:assessed", fixedTime())
	lifecycle, err := resolveLifecycle(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if !lifecycle.Found || lifecycle.Assignment.StateID != "featureforge:assessed" {
		t.Fatalf("lifecycle = %+v, want assessed", lifecycle)
	}
	// Both hold simultaneously: assessed AND not-ready.
	if readiness.Status != application.ReadinessNotReady || lifecycle.Assignment.StateID != "featureforge:assessed" {
		t.Error("assessed and not-ready must be able to coexist")
	}
}

func TestLifecycleStateNotDerivedFromClaims(t *testing.T) {
	uow, current := seedReadinessScenario(t)
	putStateAssignment(t, uow, "SA-1", "featureforge:under-validation", fixedTime())
	before, err := resolveLifecycle(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	putExecution(t, uow, "ER-1", "peos:completed")
	putReadinessClaim(t, uow, "CLM-1", "CAP-1-REV-2", "REQ-1", "peos:not-satisfied", "ER-1")
	_ = resolveReadiness(t, uow, current, []application.EffectiveRequirement{requirement(t, "REQ-1", 1)})
	after, err := resolveLifecycle(t, uow)
	if err != nil {
		t.Fatal(err)
	}
	if before.Assignment.StateID != after.Assignment.StateID {
		t.Error("recording claims must never change the resolved lifecycle state")
	}
}
