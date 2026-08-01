package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// PlanActivityCommandInput is one planned activity within
// EstablishValidationPlanCommand.
type PlanActivityCommandInput struct {
	Key                   string
	SubjectArtifactID     string
	SubjectRevisionID     string
	Method                string
	OutcomeInterpretation string
	RequirementArtifactID string
	RequirementRevisionID string
	ExpectedEvidence      []string
}

// EstablishValidationPlanCommand records the Validation Plan Artifact and
// its founding revision together, for the same reason capability and
// requirement establishment do (FF-010 §3).
type EstablishValidationPlanCommand struct {
	ArtifactID         string
	RevisionID         string
	ScopeArtifactID    string
	Activities         []PlanActivityCommandInput
	AcceptanceRecordID *string
}

// EstablishValidationPlanResult names the keys created.
type EstablishValidationPlanResult struct {
	ArtifactKey engineering.ArtifactKey
	RevisionKey engineering.RevisionKey
}

// Execute validates the command and writes the Plan Artifact and Revision
// in one transaction.
func (c EstablishValidationPlanCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, inspector EngineeringReplayInspector, clock Clock) (EstablishValidationPlanResult, error) {
	if err := requireIdentity("artifact id", c.ArtifactID); err != nil {
		return EstablishValidationPlanResult{}, err
	}
	if err := requireIdentity("revision id", c.RevisionID); err != nil {
		return EstablishValidationPlanResult{}, err
	}
	if err := requireIdentity("scope artifact id", c.ScopeArtifactID); err != nil {
		return EstablishValidationPlanResult{}, err
	}
	if len(c.Activities) == 0 {
		return EstablishValidationPlanResult{}, fmt.Errorf("%w: a validation plan requires at least one activity", ErrInvalidCommand)
	}
	if c.AcceptanceRecordID == nil {
		return EstablishValidationPlanResult{}, &fieldError{field: "acceptance record id", reason: "is required for every validation-plan request"}
	}
	if err := requireAcceptanceMemberIdentity("acceptance record id", *c.AcceptanceRecordID); err != nil {
		return EstablishValidationPlanResult{}, err
	}
	activities := make([]engineering.PlanActivityInput, len(c.Activities))
	activityKeys := make(map[string]struct{}, len(c.Activities))
	for i, a := range c.Activities {
		for field, value := range map[string]string{
			"activity key":                     a.Key,
			"activity subject artifact id":     a.SubjectArtifactID,
			"activity subject revision id":     a.SubjectRevisionID,
			"activity requirement artifact id": a.RequirementArtifactID,
			"activity requirement revision id": a.RequirementRevisionID,
		} {
			if err := requireIdentity(field, value); err != nil {
				return EstablishValidationPlanResult{}, err
			}
		}
		if _, duplicate := activityKeys[a.Key]; duplicate {
			return EstablishValidationPlanResult{}, &fieldError{field: "activity key", reason: "must be unique within the plan"}
		}
		activityKeys[a.Key] = struct{}{}
		if err := requireOneOf("activity method", a.Method, "manual-review", "manual-inspection"); err != nil {
			return EstablishValidationPlanResult{}, err
		}
		if err := requireNonEmpty("activity outcome interpretation", a.OutcomeInterpretation); err != nil {
			return EstablishValidationPlanResult{}, err
		}
		for _, expected := range a.ExpectedEvidence {
			if err := requireNonEmpty("expected evidence", expected); err != nil {
				return EstablishValidationPlanResult{}, err
			}
		}
		activities[i] = engineering.PlanActivityInput{
			Key: a.Key, SubjectArtifactID: a.SubjectArtifactID, SubjectRevisionID: a.SubjectRevisionID,
			Method: a.Method, OutcomeInterpretation: a.OutcomeInterpretation,
			RequirementArtifactID: a.RequirementArtifactID, RequirementRevisionID: a.RequirementRevisionID,
			ExpectedEvidence: a.ExpectedEvidence,
		}
	}
	key, err := engineering.NewRevisionKey(c.ArtifactID, c.RevisionID)
	if err != nil {
		return EstablishValidationPlanResult{}, invalidCommand(err)
	}
	now := normalizeTime(clock.Now())

	var result EstablishValidationPlanResult
	err = uow.Do(ctx, func(r Repositories) error {
		revisionEnv, revisionFound, err := r.Revisions.Get(ctx, key)
		if err != nil {
			return err
		}
		_, contentFound, err := r.StructuredContent.Get(ctx, key)
		if err != nil {
			return err
		}
		order, orderFound, err := r.RevisionOrder.Get(ctx, key)
		if err != nil {
			return err
		}
		journal, err := r.RevisionAcceptance.ListByRevision(ctx, key)
		if err != nil {
			return err
		}
		pOccupied := revisionFound || contentFound || orderFound || len(journal) > 0
		if pOccupied {
			if !revisionFound {
				return integrityError("validation-plan pair is partially occupied", nil)
			}
			artifactEnv, artifactFound, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: c.ArtifactID})
			if err != nil {
				return err
			}
			if !artifactFound {
				return integrityError("occupied validation-plan pair has no owning artifact", nil)
			}
			if err := inspectArtifact(inspector, artifactEnv); err != nil {
				return err
			}
			if err := inspectRevision(inspector, revisionEnv); err != nil {
				return err
			}
			if revisionEnv.RevisionFamily != engineering.RevisionFamilyValidationPlan {
				if err := validateManagedForeignOccupant(ctx, r, inspector, artifactEnv, revisionEnv, orderFound); err != nil {
					return err
				}
				if err := inspectDifferentAcceptanceCandidate(ctx, r, inspector, c.AcceptanceRecordID, ""); err != nil {
					return err
				}
				return immutableConflict("validation-plan pair is occupied by another revision family")
			}
			if !orderFound || order.Key != key || order.Sequence < 1 || order.RecordedAt.IsZero() {
				return integrityError("validation-plan pair has contradictory order metadata", nil)
			}
			if !canonicalTimeEqual(revisionEnv.RecordedAt, order.RecordedAt) {
				return integrityError("validation-plan revision and order metadata disagree on recorded time", nil)
			}
			if _, err := validateManagedHistory(ctx, r, inspector, c.ArtifactID, engineering.RevisionFamilyValidationPlan, true); err != nil {
				return err
			}
			member, err := semanticAcceptanceMember(key, journal)
			if err != nil {
				return err
			}
			if err := inspectDifferentAcceptanceCandidate(ctx, r, inspector, c.AcceptanceRecordID, member.RecordID); err != nil {
				return err
			}
			expectedArtifact, expectedRevision, buildErr := recorder.RecordValidationPlan(engineering.PlanInput{
				ArtifactID: c.ArtifactID, RevisionID: c.RevisionID, ScopeArtifactID: c.ScopeArtifactID,
				Activities: activities, RecordedAt: revisionEnv.RecordedAt,
			})
			if buildErr != nil {
				return invalidCommand(buildErr)
			}
			if !artifactEnv.Equal(expectedArtifact) || !revisionEnv.Equal(expectedRevision) {
				return immutableConflict("validation-plan pair is occupied by different semantics")
			}
			if err := validatePlanReferences(ctx, r, recorder, inspector, c.ScopeArtifactID, activities, true); err != nil {
				return err
			}
			result = EstablishValidationPlanResult{ArtifactKey: artifactEnv.Key, RevisionKey: revisionEnv.Key}
			return nil
		}

		if candidate, found, err := r.RevisionAcceptance.GetByRecordID(ctx, *c.AcceptanceRecordID); err != nil {
			return integrityError("validation-plan acceptance identity lookup", err)
		} else if found {
			if err := validateAcceptanceCandidate(ctx, r, inspector, candidate); err != nil {
				return err
			}
			return immutableConflict("acceptance record identity already belongs to another act")
		}

		artifactEnv, artifactFound, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: c.ArtifactID})
		if err != nil {
			return err
		}
		next := 1
		if artifactFound {
			if err := inspectArtifact(inspector, artifactEnv); err != nil {
				return err
			}
			family, familyErr := inspector.ArtifactFamily(artifactEnv)
			if familyErr != nil {
				return integrityError("inspect validation-plan artifact family", familyErr)
			}
			if family != engineering.RevisionFamilyValidationPlan {
				if err := validateForeignArtifactOccupancy(ctx, r, inspector, artifactEnv); err != nil {
					return err
				}
				return immutableConflict("artifact identity belongs to another family")
			}
			historySize, historyErr := validateManagedHistory(ctx, r, inspector, c.ArtifactID, engineering.RevisionFamilyValidationPlan, true)
			if historyErr != nil {
				return historyErr
			}
			if err := validateRequestedManagedSubject(ctx, r, c.ArtifactID, engineering.ArtifactSubjectKey(c.ScopeArtifactID), "validation-plan artifact"); err != nil {
				return err
			}
			next = historySize + 1
		} else if err := validateAbsentArtifactHistory(ctx, r, inspector, c.ArtifactID); err != nil {
			return err
		}

		if err := validatePlanReferences(ctx, r, recorder, inspector, c.ScopeArtifactID, activities, false); err != nil {
			return err
		}
		if now.IsZero() {
			return fmt.Errorf("application: clock returned a zero time for a new validation-plan act")
		}
		newArtifact, newRevision, err := recorder.RecordValidationPlan(engineering.PlanInput{
			ArtifactID: c.ArtifactID, RevisionID: c.RevisionID, ScopeArtifactID: c.ScopeArtifactID,
			Activities: activities, RecordedAt: now,
		})
		if err != nil {
			return invalidCommand(err)
		}
		if !artifactFound {
			if err := r.Artifacts.Put(ctx, newArtifact); err != nil {
				return err
			}
			artifactEnv = newArtifact
		}
		if err := r.Revisions.Put(ctx, newRevision); err != nil {
			return err
		}
		newOrder, err := engineering.NewRevisionOrderMetadata(newRevision.Key, next, now)
		if err != nil {
			return invalidCommand(err)
		}
		if err := r.RevisionOrder.Put(ctx, newOrder); err != nil {
			return err
		}
		member, err := engineering.NewRevisionAcceptanceRecord(
			*c.AcceptanceRecordID, newRevision.Key, engineering.AcceptanceStateAccepted, now,
			"featureforge:local-user", "validation plan established",
		)
		if err != nil {
			return invalidCommand(err)
		}
		if err := r.RevisionAcceptance.Append(ctx, member); err != nil {
			return err
		}
		result = EstablishValidationPlanResult{ArtifactKey: artifactEnv.Key, RevisionKey: newRevision.Key}
		return nil
	})
	if err != nil {
		return EstablishValidationPlanResult{}, err
	}
	return result, nil
}

func validatePlanReferences(ctx context.Context, r Repositories, recorder EngineeringRecorder, inspector EngineeringReplayInspector, scopeArtifactID string, activities []engineering.PlanActivityInput, storedAct bool) error {
	var ordinaryDependencyError error
	classify := func(err error) error {
		if err == nil {
			return nil
		}
		if !storedAct && errors.Is(err, ErrReferencedValueMissing) {
			if ordinaryDependencyError == nil {
				ordinaryDependencyError = err
			}
			return nil
		}
		return err
	}
	if err := classify(validateCapabilityArtifactReference(ctx, r, recorder, inspector, scopeArtifactID, "validation-plan scope", storedAct)); err != nil {
		return err
	}
	for _, activity := range activities {
		dependencies := []struct {
			label         string
			key           engineering.RevisionKey
			expected      engineering.RevisionFamily
			requireMember bool
		}{
			{label: "activity subject", key: engineering.RevisionKey{ArtifactID: activity.SubjectArtifactID, RevisionID: activity.SubjectRevisionID}, expected: engineering.RevisionFamilyCapability},
			{label: "activity requirement", key: engineering.RevisionKey{ArtifactID: activity.RequirementArtifactID, RevisionID: activity.RequirementRevisionID}, expected: engineering.RevisionFamilyRequirement, requireMember: true},
		}
		for _, dependency := range dependencies {
			label, key := dependency.label, dependency.key
			revision, found, err := r.Revisions.Get(ctx, key)
			if err != nil {
				return err
			}
			if !found {
				if storedAct {
					return integrityError(label+" reference is dangling", nil)
				}
				if err := classify(fmt.Errorf("%w: %s revision %s", ErrReferencedValueMissing, label, key)); err != nil {
					return err
				}
				continue
			}
			if err := inspectRevision(inspector, revision); err != nil {
				return err
			}
			if revision.RevisionFamily != dependency.expected {
				if storedAct {
					return integrityError(label+" names another revision family", nil)
				}
				if err := classify(fmt.Errorf("%w: %s names another revision family", ErrReferencedValueMissing, label)); err != nil {
					return err
				}
				continue
			}
			if _, err := validateManagedHistory(ctx, r, inspector, key.ArtifactID, dependency.expected, dependency.requireMember); err != nil {
				return err
			}
		}
	}
	return ordinaryDependencyError
}

// RecordValidationRunCommand writes the evidence Artifact and revision,
// then the execution record citing that evidence (FF-010 §3.4). One
// transaction: an execution record whose evidence failed to write must not
// exist.
type RecordValidationRunCommand struct {
	ExecutionID        string
	PlanArtifactID     string
	PlanRevisionID     string
	ActivityKey        string
	SubjectArtifactID  string
	SubjectRevisionID  string
	Method             string
	Outcome            string
	CompletedAt        time.Time
	EvidenceArtifactID string
	EvidenceRevisionID string
	EvidenceLocator    string
	HasCompletedAt     bool
}

// RecordValidationRunResult names the records created.
type RecordValidationRunResult struct {
	EvidenceArtifactKey engineering.ArtifactKey
	EvidenceRevisionKey engineering.RevisionKey
	ExecutionKey        engineering.RecordKey
}

// Execute validates the command and writes the evidence and execution
// record in one transaction.
func (c RecordValidationRunCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, inspector EngineeringReplayInspector, clock Clock) (RecordValidationRunResult, error) {
	for field, value := range map[string]string{
		"execution id": c.ExecutionID, "plan artifact id": c.PlanArtifactID, "plan revision id": c.PlanRevisionID,
		"activity key": c.ActivityKey, "subject artifact id": c.SubjectArtifactID, "subject revision id": c.SubjectRevisionID,
		"evidence artifact id": c.EvidenceArtifactID, "evidence revision id": c.EvidenceRevisionID,
	} {
		if err := requireIdentity(field, value); err != nil {
			return RecordValidationRunResult{}, err
		}
	}
	for field, value := range map[string]string{
		"method": c.Method, "outcome": c.Outcome, "evidence locator": c.EvidenceLocator,
	} {
		if err := requireNonEmpty(field, value); err != nil {
			return RecordValidationRunResult{}, err
		}
	}
	if err := requireOneOf("method", c.Method, "manual-review", "manual-inspection"); err != nil {
		return RecordValidationRunResult{}, err
	}
	if err := requireOneOf("outcome", c.Outcome, "completed", "failed", "interrupted", "indeterminate"); err != nil {
		return RecordValidationRunResult{}, err
	}
	hasCompletedAt := c.HasCompletedAt || !zeroTime(c.CompletedAt)
	if c.HasCompletedAt && zeroTime(c.CompletedAt) {
		return RecordValidationRunResult{}, &fieldError{field: "completed at", reason: "must be a valid timestamp when present"}
	}
	now := normalizeTime(clock.Now())
	completedAt := optionalTime(c.CompletedAt, now)
	evidenceKey, err := engineering.NewRevisionKey(c.EvidenceArtifactID, c.EvidenceRevisionID)
	if err != nil {
		return RecordValidationRunResult{}, invalidCommand(err)
	}
	executionKey, err := engineering.NewRecordKey(engineering.RecordKindExecution, c.ExecutionID)
	if err != nil {
		return RecordValidationRunResult{}, invalidCommand(err)
	}

	var result RecordValidationRunResult
	err = uow.Do(ctx, func(r Repositories) error {
		storedArtifact, artifactFound, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: c.EvidenceArtifactID})
		if err != nil {
			return err
		}
		storedRevision, revisionFound, err := r.Revisions.Get(ctx, evidenceKey)
		if err != nil {
			return err
		}
		storedExecution, executionFound, err := r.Records.Get(ctx, executionKey)
		if err != nil {
			return err
		}
		if executionFound {
			ownedArtifact, ownedRevision, ownedKey, validateErr := validateStoredExecutionAct(ctx, r, inspector, storedExecution)
			if validateErr != nil {
				return validateErr
			}
			owners, ownerErr := evidenceOwners(ctx, r, inspector, ownedKey)
			if ownerErr != nil {
				return ownerErr
			}
			if len(owners) != 1 || owners[0].Key != storedExecution.Key {
				return integrityError("execution evidence must belong to exactly one validation-run act", nil)
			}
			if ownedKey != evidenceKey {
				if _, err := classifyEvidenceOccupancy(ctx, r, inspector, evidenceKey, storedArtifact, artifactFound, storedRevision, revisionFound); err != nil {
					return err
				}
				return immutableConflict("execution identity already belongs to another evidence act")
			}
			storedArtifact, storedRevision = ownedArtifact, ownedRevision
			artifactFound, revisionFound = true, true
			if !canonicalTimeEqual(storedArtifact.RecordedAt, storedRevision.RecordedAt) || !canonicalTimeEqual(storedRevision.RecordedAt, storedExecution.RecordedAt) {
				return integrityError("validation-run members disagree on recorded time", nil)
			}
			storedCompletedAt := storedExecution.OccurredAt
			if hasCompletedAt && !canonicalTimeEqual(c.CompletedAt, storedCompletedAt) {
				return immutableConflict("execution identity has a different completed time")
			}
			expectedArtifact, expectedRevision, buildErr := recorder.RecordEvidence(engineering.EvidenceInput{
				ArtifactID: c.EvidenceArtifactID, RevisionID: c.EvidenceRevisionID, Locator: c.EvidenceLocator,
				RecordedAt: storedRevision.RecordedAt,
			})
			if buildErr != nil {
				return invalidCommand(buildErr)
			}
			expectedExecution, buildErr := recorder.RecordExecution(engineering.ExecutionInput{
				ExecutionID: c.ExecutionID, PlanArtifactID: c.PlanArtifactID, PlanRevisionID: c.PlanRevisionID,
				ActivityKey: c.ActivityKey, SubjectArtifactID: c.SubjectArtifactID, SubjectRevisionID: c.SubjectRevisionID,
				Method: c.Method, Outcome: c.Outcome, CompletedAt: storedCompletedAt,
				EvidenceArtifactID: c.EvidenceArtifactID, EvidenceRevisionID: c.EvidenceRevisionID,
				RecordedAt: storedExecution.RecordedAt,
			})
			if buildErr != nil {
				return invalidCommand(buildErr)
			}
			if !storedArtifact.Equal(expectedArtifact) || !storedRevision.Equal(expectedRevision) || !storedExecution.Equal(expectedExecution) {
				return immutableConflict("validation-run identities are occupied by different semantics")
			}
			result = RecordValidationRunResult{EvidenceArtifactKey: storedArtifact.Key, EvidenceRevisionKey: storedRevision.Key, ExecutionKey: storedExecution.Key}
			return nil
		}
		occupancy, err := classifyEvidenceOccupancy(ctx, r, inspector, evidenceKey, storedArtifact, artifactFound, storedRevision, revisionFound)
		if err != nil {
			return err
		}
		if occupancy != evidenceOccupancyAbsent {
			return immutableConflict("evidence pair already belongs to another validation-run act")
		}

		var planRevision engineering.RevisionEnvelope
		var ordinaryDependencyError error
		dependencies := []struct {
			label    string
			ref      engineering.RevisionKey
			expected engineering.RevisionFamily
		}{
			{label: "validation plan", ref: engineering.RevisionKey{ArtifactID: c.PlanArtifactID, RevisionID: c.PlanRevisionID}, expected: engineering.RevisionFamilyValidationPlan},
			{label: "run subject", ref: engineering.RevisionKey{ArtifactID: c.SubjectArtifactID, RevisionID: c.SubjectRevisionID}, expected: engineering.RevisionFamilyCapability},
		}
		for _, dependency := range dependencies {
			label, ref := dependency.label, dependency.ref
			stored, found, err := r.Revisions.Get(ctx, ref)
			if err != nil {
				return err
			}
			if !found {
				if ordinaryDependencyError == nil {
					ordinaryDependencyError = fmt.Errorf("%w: %s revision %s", ErrReferencedValueMissing, label, ref)
				}
				continue
			}
			if err := inspectRevision(inspector, stored); err != nil {
				return err
			}
			if stored.RevisionFamily != dependency.expected {
				if ordinaryDependencyError == nil {
					ordinaryDependencyError = fmt.Errorf("%w: %s names another revision family", ErrReferencedValueMissing, label)
				}
				continue
			}
			requireMember := dependency.expected == engineering.RevisionFamilyValidationPlan
			if _, err := validateManagedHistory(ctx, r, inspector, ref.ArtifactID, dependency.expected, requireMember); err != nil {
				return err
			}
			if dependency.expected == engineering.RevisionFamilyValidationPlan {
				planRevision = stored
			}
		}
		if ordinaryDependencyError != nil {
			return ordinaryDependencyError
		}
		if _, err := validatePlanActivityReference(ctx, r, inspector, planRevision, c.ActivityKey, c.Method, engineering.RevisionKey{ArtifactID: c.SubjectArtifactID, RevisionID: c.SubjectRevisionID}, false); err != nil {
			return err
		}
		if err := requireServerTime(now, "a new validation-run act"); err != nil {
			return err
		}
		evArtEnv, evRevEnv, err := recorder.RecordEvidence(engineering.EvidenceInput{
			ArtifactID: c.EvidenceArtifactID, RevisionID: c.EvidenceRevisionID, Locator: c.EvidenceLocator, RecordedAt: now,
		})
		if err != nil {
			return invalidCommand(err)
		}
		if err := r.Artifacts.Put(ctx, evArtEnv); err != nil {
			return err
		}
		if err := r.Revisions.Put(ctx, evRevEnv); err != nil {
			return err
		}

		execEnv, err := recorder.RecordExecution(engineering.ExecutionInput{
			ExecutionID: c.ExecutionID, PlanArtifactID: c.PlanArtifactID, PlanRevisionID: c.PlanRevisionID,
			ActivityKey: c.ActivityKey, SubjectArtifactID: c.SubjectArtifactID, SubjectRevisionID: c.SubjectRevisionID,
			Method: c.Method, Outcome: c.Outcome, CompletedAt: completedAt,
			EvidenceArtifactID: c.EvidenceArtifactID, EvidenceRevisionID: c.EvidenceRevisionID, RecordedAt: now,
		})
		if err != nil {
			return invalidCommand(err)
		}
		if err := r.Records.Put(ctx, execEnv); err != nil {
			return err
		}

		result = RecordValidationRunResult{EvidenceArtifactKey: evArtEnv.Key, EvidenceRevisionKey: evRevEnv.Key, ExecutionKey: execEnv.Key}
		return nil
	})
	if err != nil {
		return RecordValidationRunResult{}, err
	}
	return result, nil
}

type evidenceOccupancy uint8

const (
	evidenceOccupancyAbsent evidenceOccupancy = iota
	evidenceOccupancyComplete
	evidenceOccupancyForeign
)

func classifyEvidenceOccupancy(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, requestedKey engineering.RevisionKey, artifact engineering.ArtifactEnvelope, artifactFound bool, revision engineering.RevisionEnvelope, revisionFound bool) (evidenceOccupancy, error) {
	if !revisionFound {
		if err := validateUnmanagedRevisionMetadata(ctx, r, requestedKey); err != nil {
			return evidenceOccupancyAbsent, err
		}
		owners, err := evidenceOwners(ctx, r, inspector, requestedKey)
		if err != nil {
			return evidenceOccupancyAbsent, err
		}
		if len(owners) > 0 {
			return evidenceOccupancyAbsent, integrityError("execution ownership exists without its evidence pair", nil)
		}
	}
	if !artifactFound && !revisionFound {
		if err := validateAbsentArtifactHistory(ctx, r, inspector, requestedKey.ArtifactID); err != nil {
			return evidenceOccupancyAbsent, err
		}
		if err := validateAbsentEvidenceReferences(ctx, r, inspector, requestedKey.ArtifactID); err != nil {
			return evidenceOccupancyAbsent, err
		}
		return evidenceOccupancyAbsent, nil
	}
	if revisionFound && !artifactFound {
		return evidenceOccupancyAbsent, integrityError("evidence revision has no owning artifact", nil)
	}
	if err := inspectArtifact(inspector, artifact); err != nil {
		return evidenceOccupancyAbsent, err
	}
	evidenceArtifactErr := inspector.ValidateEvidenceArtifact(artifact)
	if !revisionFound {
		if evidenceArtifactErr == nil {
			revisions, lookupErr := r.Revisions.ListByArtifact(ctx, artifact.Key.ArtifactID)
			if lookupErr != nil {
				return evidenceOccupancyAbsent, lookupErr
			}
			if len(revisions) != 1 {
				return evidenceOccupancyAbsent, integrityError("validation-evidence artifact must have exactly one revision", nil)
			}
			if _, err := validateEvidencePairOccupancy(ctx, r, inspector, artifact, revisions[0]); err != nil {
				return evidenceOccupancyAbsent, err
			}
			return evidenceOccupancyForeign, nil
		}
		if err := validateForeignArtifactOccupancy(ctx, r, inspector, artifact); err != nil {
			return evidenceOccupancyAbsent, err
		}
		owners, err := executionEvidenceOwnersByArtifact(ctx, r, inspector, artifact.Key.ArtifactID)
		if err != nil {
			return evidenceOccupancyAbsent, err
		}
		if len(owners) != 0 {
			return evidenceOccupancyAbsent, integrityError("foreign artifact is named as produced execution evidence", nil)
		}
		return evidenceOccupancyForeign, nil
	}
	if err := inspectRevision(inspector, revision); err != nil {
		return evidenceOccupancyAbsent, err
	}
	if artifact.ArtifactType != revision.ArtifactType {
		return evidenceOccupancyAbsent, integrityError("evidence artifact and revision types disagree", nil)
	}
	if evidenceArtifactErr != nil || revision.RevisionFamily != engineering.RevisionFamilyEvidence {
		if err := validateForeignArtifactOccupancy(ctx, r, inspector, artifact); err != nil {
			return evidenceOccupancyAbsent, err
		}
		owners, err := executionEvidenceOwnersByArtifact(ctx, r, inspector, artifact.Key.ArtifactID)
		if err != nil {
			return evidenceOccupancyAbsent, err
		}
		if len(owners) != 0 {
			return evidenceOccupancyAbsent, integrityError("foreign artifact is named as produced execution evidence", nil)
		}
		return evidenceOccupancyForeign, nil
	}
	hasExecutionOwner, err := validateEvidencePairOccupancy(ctx, r, inspector, artifact, revision)
	if err != nil {
		return evidenceOccupancyAbsent, err
	}
	if !hasExecutionOwner {
		return evidenceOccupancyForeign, nil
	}
	return evidenceOccupancyComplete, nil
}

// validateAbsentEvidenceReferences closes the root-wide reverse-ownership
// gap for a missing Evidence Artifact. Executions own produced evidence, so a
// sibling owner under the same missing ArtifactID makes founding creation
// corrupt. Decisions are intentionally not occupancy: C8 citations may remain
// unresolved and a later C10 act may create the cited evidence pair.
func validateAbsentEvidenceReferences(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifactID string) error {
	owners, err := executionEvidenceOwnersByArtifact(ctx, r, inspector, artifactID)
	if err != nil {
		return err
	}
	if len(owners) != 0 {
		return integrityError("absent evidence artifact has a dangling execution owner", nil)
	}
	return nil
}

func executionEvidenceOwnersByArtifact(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifactID string) ([]engineering.RecordEnvelope, error) {
	records, err := r.Records.ListByKind(ctx, engineering.RecordKindExecution)
	if err != nil {
		return nil, err
	}
	owners := make([]engineering.RecordEnvelope, 0, 1)
	for _, record := range records {
		if err := inspectRecord(inspector, record); err != nil {
			return nil, err
		}
		for _, rawKey := range record.EvidenceKeys {
			evidenceArtifactID, _, err := engineering.ParseEvidenceKey(rawKey)
			if err != nil {
				return nil, integrityError("execution evidence projection is malformed", err)
			}
			if evidenceArtifactID == artifactID {
				owners = append(owners, record)
				break
			}
		}
	}
	return owners, nil
}

func validateEvidencePairOccupancy(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifact engineering.ArtifactEnvelope, revision engineering.RevisionEnvelope) (bool, error) {
	if err := inspectArtifact(inspector, artifact); err != nil {
		return false, err
	}
	if err := inspector.ValidateEvidenceArtifact(artifact); err != nil {
		return false, integrityError("evidence artifact has the wrong family", err)
	}
	if err := inspectRevision(inspector, revision); err != nil {
		return false, err
	}
	if revision.RevisionFamily != engineering.RevisionFamilyEvidence || artifact.ArtifactType != revision.ArtifactType {
		return false, integrityError("evidence artifact and revision families disagree", nil)
	}
	if err := validateNoCapabilityFeatureOwners(ctx, r, artifact.Key.ArtifactID, "evidence artifact is linked from a FeatureCard"); err != nil {
		return false, err
	}
	if err := validateNoStateAssignmentParentOwners(ctx, r, inspector, artifact.Key.ArtifactID, "evidence artifact is named as a state-assignment parent"); err != nil {
		return false, err
	}
	if err := validateUnmanagedRevisionMetadata(ctx, r, revision.Key); err != nil {
		return false, err
	}
	if !canonicalTimeEqual(artifact.RecordedAt, revision.RecordedAt) {
		return false, integrityError("evidence artifact and revision times disagree", nil)
	}
	revisions, err := r.Revisions.ListByArtifact(ctx, artifact.Key.ArtifactID)
	if err != nil {
		return false, err
	}
	if len(revisions) != 1 || revisions[0].Key != revision.Key {
		return false, integrityError("evidence artifact must own exactly the named revision", nil)
	}
	owners, err := evidenceArtifactOwners(ctx, r, inspector, revision.Key.ArtifactID, revision.Key)
	if err != nil {
		return false, err
	}
	if len(owners) == 0 {
		cited, err := evidenceHasDecisionCitation(ctx, r, inspector, revision.Key)
		if err != nil {
			return false, err
		}
		if cited {
			return false, nil
		}
		return false, integrityError("evidence pair has neither an execution owner nor a decision citation", nil)
	}
	if len(owners) != 1 {
		return false, integrityError("evidence pair must belong to exactly one validation-run act", nil)
	}
	if _, _, ownerKey, err := validateStoredExecutionAct(ctx, r, inspector, owners[0]); err != nil {
		return false, err
	} else if ownerKey != revision.Key {
		return false, integrityError("evidence ownership lookup is contradictory", nil)
	}
	return true, nil
}

func evidenceHasDecisionCitation(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, key engineering.RevisionKey) (bool, error) {
	decisions, err := r.Records.ListByKind(ctx, engineering.RecordKindDecision)
	if err != nil {
		return false, err
	}
	want := engineering.EvidenceKey(key.ArtifactID, key.RevisionID)
	found := false
	for _, decision := range decisions {
		if err := inspectRecord(inspector, decision); err != nil {
			return false, err
		}
		cites := false
		for _, evidence := range decision.EvidenceKeys {
			if evidence == want {
				cites = true
				break
			}
		}
		if !cites {
			continue
		}
		if err := validateStoredDecisionReferences(ctx, r, inspector, decision); err != nil {
			return false, err
		}
		found = true
	}
	return found, nil
}

func evidenceOwners(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, key engineering.RevisionKey) ([]engineering.RecordEnvelope, error) {
	executions, err := r.Records.ListByKind(ctx, engineering.RecordKindExecution)
	if err != nil {
		return nil, err
	}
	owners := make([]engineering.RecordEnvelope, 0, 1)
	for _, execution := range executions {
		if err := inspectRecord(inspector, execution); err != nil {
			return nil, err
		}
		if len(execution.EvidenceKeys) != 1 {
			return nil, integrityError("execution must name exactly one evidence revision", nil)
		}
		artifactID, revisionID, err := engineering.ParseEvidenceKey(execution.EvidenceKeys[0])
		if err != nil {
			return nil, integrityError("execution evidence projection is malformed", err)
		}
		if artifactID == key.ArtifactID && revisionID == key.RevisionID {
			owners = append(owners, execution)
		}
	}
	return owners, nil
}

func evidenceArtifactOwners(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifactID string, soleKey engineering.RevisionKey) ([]engineering.RecordEnvelope, error) {
	executions, err := r.Records.ListByKind(ctx, engineering.RecordKindExecution)
	if err != nil {
		return nil, err
	}
	owners := make([]engineering.RecordEnvelope, 0, 1)
	for _, execution := range executions {
		if err := inspectRecord(inspector, execution); err != nil {
			return nil, err
		}
		if len(execution.EvidenceKeys) != 1 {
			return nil, integrityError("execution must name exactly one evidence revision", nil)
		}
		evidenceArtifactID, evidenceRevisionID, err := engineering.ParseEvidenceKey(execution.EvidenceKeys[0])
		if err != nil {
			return nil, integrityError("execution evidence projection is malformed", err)
		}
		if evidenceArtifactID != artifactID {
			continue
		}
		key := engineering.RevisionKey{ArtifactID: evidenceArtifactID, RevisionID: evidenceRevisionID}
		if key != soleKey {
			return nil, integrityError("execution names a missing sibling evidence revision", nil)
		}
		owners = append(owners, execution)
	}
	return owners, nil
}

func validateStoredExecutionAct(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, execution engineering.RecordEnvelope) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, engineering.RevisionKey, error) {
	if err := inspectRecord(inspector, execution); err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
	}
	if len(execution.EvidenceKeys) != 1 {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, integrityError("execution must name exactly one produced evidence revision", nil)
	}
	artifactID, revisionID, err := engineering.ParseEvidenceKey(execution.EvidenceKeys[0])
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, integrityError("execution evidence projection", err)
	}
	key, _ := engineering.NewRevisionKey(artifactID, revisionID)
	artifact, artifactFound, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: artifactID})
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
	}
	revision, revisionFound, err := r.Revisions.Get(ctx, key)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
	}
	if !artifactFound || !revisionFound {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, integrityError("execution has dangling evidence", nil)
	}
	if err := inspector.ValidateEvidenceArtifact(artifact); err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, integrityError("execution evidence artifact has the wrong family", err)
	}
	if err := inspectArtifact(inspector, artifact); err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
	}
	if err := inspectRevision(inspector, revision); err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
	}
	if revision.RevisionFamily != engineering.RevisionFamilyEvidence {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, integrityError("execution evidence names another revision family", nil)
	}
	if err := validateNoCapabilityFeatureOwners(ctx, r, artifactID, "execution evidence artifact is linked from a FeatureCard"); err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
	}
	if err := validateNoStateAssignmentParentOwners(ctx, r, inspector, artifactID, "execution evidence artifact is named as a state-assignment parent"); err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
	}
	if err := validateUnmanagedRevisionMetadata(ctx, r, revision.Key); err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
	}
	if artifact.ArtifactType != revision.ArtifactType {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, integrityError("execution evidence artifact and revision types disagree", nil)
	}
	if !canonicalTimeEqual(artifact.RecordedAt, revision.RecordedAt) || !canonicalTimeEqual(revision.RecordedAt, execution.RecordedAt) {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, integrityError("validation-run members disagree on recorded time", nil)
	}
	revisions, err := r.Revisions.ListByArtifact(ctx, artifactID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
	}
	if len(revisions) != 1 || revisions[0].Key != key {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, integrityError("validation-run evidence artifact must own exactly one revision", nil)
	}
	owners, err := evidenceArtifactOwners(ctx, r, inspector, artifactID, key)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
	}
	if len(owners) != 1 || owners[0].Key != execution.Key {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, integrityError("validation-run evidence must have exactly one execution owner", nil)
	}
	planKey, activityKey, method, err := inspector.ExecutionPlanActivity(execution)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, integrityError("execution plan inspection", err)
	}
	dependencies := []struct {
		label      string
		dependency engineering.RevisionKey
		expected   engineering.RevisionFamily
	}{
		{label: "plan", dependency: planKey, expected: engineering.RevisionFamilyValidationPlan},
		{label: "subject", dependency: mustRevisionSubjectKey(execution.SubjectKey), expected: engineering.RevisionFamilyCapability},
	}
	for _, item := range dependencies {
		label, dependency := item.label, item.dependency
		stored, found, lookupErr := r.Revisions.Get(ctx, dependency)
		if lookupErr != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, lookupErr
		}
		if !found {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, integrityError("execution has dangling "+label+" revision", nil)
		}
		if err := inspectRevision(inspector, stored); err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
		}
		if stored.RevisionFamily != item.expected {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, integrityError("execution "+label+" reference names another revision family", nil)
		}
	}
	plan, _, err := r.Revisions.Get(ctx, planKey)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
	}
	if _, err := validatePlanActivityReference(ctx, r, inspector, plan, activityKey, method, mustRevisionSubjectKey(execution.SubjectKey), true); err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, engineering.RevisionKey{}, err
	}
	return artifact, revision, key, nil
}

type planActivityReference struct {
	scopeArtifactID string
	requirement     engineering.RevisionKey
}

func validatePlanActivityReference(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, plan engineering.RevisionEnvelope, activityKey, method string, subject engineering.RevisionKey, storedAct bool) (planActivityReference, error) {
	if _, err := validateManagedHistory(ctx, r, inspector, plan.Key.ArtifactID, engineering.RevisionFamilyValidationPlan, true); err != nil {
		if storedAct {
			return planActivityReference{}, err
		}
		return planActivityReference{}, fmt.Errorf("%w: validation plan is not a complete accepted act", ErrReferencedValueMissing)
	}
	scope, keys, methods, subjects, requirements, err := inspector.ValidationPlanReferences(plan)
	if err != nil {
		if storedAct {
			return planActivityReference{}, integrityError("validation-plan activity references are unreadable", err)
		}
		return planActivityReference{}, fmt.Errorf("%w: validation-plan activity references are unreadable", ErrReferencedValueMissing)
	}
	found := -1
	for i, key := range keys {
		if key != activityKey {
			continue
		}
		if found >= 0 {
			if storedAct {
				return planActivityReference{}, integrityError("validation plan contains a duplicate activity key", nil)
			}
			return planActivityReference{}, fmt.Errorf("%w: validation plan contains a duplicate activity key", ErrReferencedValueMissing)
		}
		found = i
	}
	if found < 0 || found >= len(subjects) || found >= len(methods) || found >= len(requirements) || subjects[found] != subject || methods[found] != method {
		if storedAct {
			return planActivityReference{}, integrityError("execution cites a missing or differently scoped plan activity", nil)
		}
		return planActivityReference{}, fmt.Errorf("%w: execution cites a missing or differently scoped plan activity", ErrReferencedValueMissing)
	}
	return planActivityReference{scopeArtifactID: scope, requirement: requirements[found]}, nil
}

func mustRevisionSubjectKey(subjectKey string) engineering.RevisionKey {
	kind, artifactID, revisionID, err := engineering.ParseSubjectKey(subjectKey)
	if err != nil || kind != engineering.SubjectKindArtifactRevision {
		return engineering.RevisionKey{}
	}
	return engineering.RevisionKey{ArtifactID: artifactID, RevisionID: revisionID}
}

// RecordValidationClaimCommand records a Validation Claim (FF-010 §3).
type RecordValidationClaimCommand struct {
	ClaimID               string
	ScopeArtifactID       string
	SubjectArtifactID     string
	SubjectRevisionID     string
	RequirementArtifactID string
	RequirementRevisionID string
	Outcome               string
	Method                string
	EvidenceArtifactID    string
	EvidenceRevisionID    string
	ExecutionID           string
	Reasoning             string
	Timestamp             time.Time
	HasTimestamp          bool
}

// RecordValidationClaimResult names the record created.
type RecordValidationClaimResult struct {
	Key engineering.RecordKey
}

// Execute validates the command and writes the Claim in one transaction.
func (c RecordValidationClaimCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, inspector EngineeringReplayInspector, clock Clock) (RecordValidationClaimResult, error) {
	if c.Reasoning != "" {
		if err := requireNonEmpty("reasoning", c.Reasoning); err != nil {
			return RecordValidationClaimResult{}, err
		}
	}
	hasTimestamp := c.HasTimestamp || !zeroTime(c.Timestamp)
	if c.HasTimestamp && zeroTime(c.Timestamp) {
		return RecordValidationClaimResult{}, &fieldError{field: "timestamp", reason: "must be a valid timestamp when present"}
	}
	now := normalizeTime(clock.Now())
	env, err := recordClaim(ctx, uow, recorder, inspector, engineering.ClaimInput{
		ClaimID: c.ClaimID, ScopeArtifactID: c.ScopeArtifactID,
		SubjectArtifactID: c.SubjectArtifactID, SubjectRevisionID: c.SubjectRevisionID,
		RequirementArtifactID: c.RequirementArtifactID, RequirementRevisionID: c.RequirementRevisionID,
		Outcome: c.Outcome, Method: c.Method,
		EvidenceArtifactID: c.EvidenceArtifactID, EvidenceRevisionID: c.EvidenceRevisionID,
		ExecutionID: c.ExecutionID, Reasoning: c.Reasoning, Timestamp: optionalTime(c.Timestamp, now),
		RecordedAt: now,
	}, hasTimestamp)
	if err != nil {
		return RecordValidationClaimResult{}, err
	}
	return RecordValidationClaimResult{Key: env.Key}, nil
}

// CorrectValidationClaimCommand records a new Claim carrying a correction
// reference to an earlier one (FF-010 §3.5). It has four invariants
// RecordValidationClaim does not: the target must exist, must be a claim,
// the resulting chain must have no cycle, and PEOS v1.0.0 accepts a
// self-correction (verified) so FeatureForge rejects it explicitly here
// (AD-017), before any PEOS construction.
type CorrectValidationClaimCommand struct {
	ClaimID               string
	CorrectionTarget      string
	CorrectionKind        string
	ScopeArtifactID       string
	SubjectArtifactID     string
	SubjectRevisionID     string
	RequirementArtifactID string
	RequirementRevisionID string
	Outcome               string
	Method                string
	EvidenceArtifactID    string
	EvidenceRevisionID    string
	ExecutionID           string
	Reasoning             string
	Timestamp             time.Time
	HasTimestamp          bool
}

// CorrectValidationClaimResult names the record created.
type CorrectValidationClaimResult struct {
	Key engineering.RecordKey
}

// Execute validates the correction-specific invariants, then everything
// RecordValidationClaim validates, and writes the correcting Claim in one
// transaction.
func (c CorrectValidationClaimCommand) Execute(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, inspector EngineeringReplayInspector, clock Clock) (CorrectValidationClaimResult, error) {
	if err := requireIdentity("correction target", c.CorrectionTarget); err != nil {
		return CorrectValidationClaimResult{}, err
	}
	if c.CorrectionTarget == c.ClaimID {
		return CorrectValidationClaimResult{}, fmt.Errorf("%w: claim %q cannot correct itself", ErrCorrectionSelfReference, c.ClaimID)
	}
	switch c.CorrectionKind {
	case "correct", "replace", "invalidate":
	default:
		return CorrectValidationClaimResult{}, fmt.Errorf("%w: unsupported correction kind %q", ErrInvalidCommand, c.CorrectionKind)
	}
	if c.Reasoning != "" {
		if err := requireNonEmpty("reasoning", c.Reasoning); err != nil {
			return CorrectValidationClaimResult{}, err
		}
	}

	hasTimestamp := c.HasTimestamp || !zeroTime(c.Timestamp)
	if c.HasTimestamp && zeroTime(c.Timestamp) {
		return CorrectValidationClaimResult{}, &fieldError{field: "timestamp", reason: "must be a valid timestamp when present"}
	}
	now := normalizeTime(clock.Now())
	env, err := recordClaim(ctx, uow, recorder, inspector, engineering.ClaimInput{
		ClaimID: c.ClaimID, ScopeArtifactID: c.ScopeArtifactID,
		SubjectArtifactID: c.SubjectArtifactID, SubjectRevisionID: c.SubjectRevisionID,
		RequirementArtifactID: c.RequirementArtifactID, RequirementRevisionID: c.RequirementRevisionID,
		Outcome: c.Outcome, Method: c.Method,
		EvidenceArtifactID: c.EvidenceArtifactID, EvidenceRevisionID: c.EvidenceRevisionID,
		ExecutionID: c.ExecutionID, Reasoning: c.Reasoning, Timestamp: optionalTime(c.Timestamp, now), RecordedAt: now,
		HasCorrection: true, CorrectionKind: c.CorrectionKind, CorrectionTarget: c.CorrectionTarget,
	}, hasTimestamp)
	if err != nil {
		return CorrectValidationClaimResult{}, err
	}
	return CorrectValidationClaimResult{Key: env.Key}, nil
}

// recordClaim is the shared write path for RecordValidationClaim and
// CorrectValidationClaim.
func recordClaim(ctx context.Context, uow UnitOfWork, recorder EngineeringRecorder, inspector EngineeringReplayInspector, in engineering.ClaimInput, hasTimestamp bool) (engineering.RecordEnvelope, error) {
	if err := validateClaimInput(in); err != nil {
		return engineering.RecordEnvelope{}, err
	}
	key, err := engineering.NewRecordKey(engineering.RecordKindClaim, in.ClaimID)
	if err != nil {
		return engineering.RecordEnvelope{}, invalidCommand(err)
	}
	var env engineering.RecordEnvelope
	err = uow.Do(ctx, func(r Repositories) error {
		if stored, found, err := r.Records.Get(ctx, key); err != nil {
			return err
		} else if found {
			if err := inspectRecord(inspector, stored); err != nil {
				return err
			}
			if err := validateStoredClaimReferences(ctx, r, inspector, stored); err != nil {
				return err
			}
			if stored.HasCorrection() {
				if err := validateClaimChainIntegrity(ctx, r, inspector, stored); err != nil {
					return err
				}
			}
			if hasTimestamp && !canonicalTimeEqual(in.Timestamp, stored.OccurredAt) {
				return immutableConflict("claim identity has a different caller timestamp")
			}
			expectedInput := in
			expectedInput.Timestamp = stored.OccurredAt
			expectedInput.RecordedAt = stored.RecordedAt
			expected, buildErr := recorder.RecordClaim(expectedInput)
			if buildErr != nil {
				return invalidCommand(buildErr)
			}
			if !stored.Equal(expected) {
				return immutableConflict("claim identity is occupied by different semantics")
			}
			env = stored
			return nil
		}
		var ordinaryDependencyError error
		if in.HasCorrection {
			targetKey, err := engineering.NewRecordKey(engineering.RecordKindClaim, in.CorrectionTarget)
			if err != nil {
				return invalidCommand(err)
			}
			if target, found, err := r.Records.Get(ctx, targetKey); err != nil {
				return err
			} else if !found {
				mismatch, err := validateCorrectionTargetFamily(ctx, r, inspector, in.CorrectionTarget)
				if err != nil {
					return err
				}
				if mismatch {
					ordinaryDependencyError = fmt.Errorf("%w: record %s is not a claim", ErrCorrectionFamilyMismatch, in.CorrectionTarget)
				} else {
					ordinaryDependencyError = fmt.Errorf("%w: claim %s", ErrCorrectionTargetMissing, in.CorrectionTarget)
				}
			} else {
				if err := inspectRecord(inspector, target); err != nil {
					return err
				}
				if err := validateStoredClaimReferences(ctx, r, inspector, target); err != nil {
					return err
				}
				if err := validateClaimChainIntegrity(ctx, r, inspector, target); err != nil {
					return err
				}
			}
		}
		if err := validateClaimInputReferences(ctx, r, inspector, in); err != nil {
			if !errors.Is(err, ErrReferencedValueMissing) {
				return err
			}
			if ordinaryDependencyError == nil {
				ordinaryDependencyError = err
			}
		}
		if ordinaryDependencyError != nil {
			return ordinaryDependencyError
		}
		if err := requireServerTime(in.RecordedAt, "a new validation-claim act"); err != nil {
			return err
		}
		built, err := recorder.RecordClaim(in)
		if err != nil {
			return invalidCommand(err)
		}
		if err := r.Records.Put(ctx, built); err != nil {
			return err
		}
		env = built
		if in.HasCorrection {
			_, err = ResolveCurrentClaim(ctx, r, env.SubjectKey, env.Scope, env.CriterionKeys)
			if errors.Is(err, ErrCorrectionAmbiguous) {
				// FF-010 §6 deliberately permits competing correction heads;
				// ambiguity is a query result requiring human resolution, not
				// a malformed write or a reason to roll the correction back.
				return nil
			}
			return err
		}
		return nil
	})
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	return env, nil
}

func validateCorrectionTargetFamily(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, targetID string) (bool, error) {
	foundAny := false
	for _, kind := range []engineering.RecordKind{
		engineering.RecordKindExecution,
		engineering.RecordKindDecision,
		engineering.RecordKindStateAssignment,
	} {
		key, err := engineering.NewRecordKey(kind, targetID)
		if err != nil {
			return false, invalidCommand(err)
		}
		stored, found, err := r.Records.Get(ctx, key)
		if err != nil {
			return false, err
		}
		if !found {
			continue
		}
		foundAny = true
		if err := inspectRecord(inspector, stored); err != nil {
			return false, err
		}
		switch kind {
		case engineering.RecordKindExecution:
			if _, _, _, err := validateStoredExecutionAct(ctx, r, inspector, stored); err != nil {
				return false, err
			}
		case engineering.RecordKindDecision:
			if err := validateStoredDecisionReferences(ctx, r, inspector, stored); err != nil {
				return false, err
			}
		case engineering.RecordKindStateAssignment:
			if err := validateStateAssignmentAct(ctx, r, inspector, stored); err != nil {
				return false, err
			}
		}
	}
	return foundAny, nil
}

func validateClaimInput(in engineering.ClaimInput) error {
	for field, value := range map[string]string{
		"claim id": in.ClaimID, "scope artifact id": in.ScopeArtifactID,
		"subject artifact id": in.SubjectArtifactID, "subject revision id": in.SubjectRevisionID,
		"requirement artifact id": in.RequirementArtifactID, "requirement revision id": in.RequirementRevisionID,
		"evidence artifact id": in.EvidenceArtifactID, "evidence revision id": in.EvidenceRevisionID,
		"execution id": in.ExecutionID,
	} {
		if err := requireIdentity(field, value); err != nil {
			return err
		}
	}
	for field, value := range map[string]string{"outcome": in.Outcome, "method": in.Method} {
		if err := requireNonEmpty(field, value); err != nil {
			return err
		}
	}
	if err := requireOneOf("outcome", in.Outcome, "satisfied", "not-satisfied", "inconclusive"); err != nil {
		return err
	}
	if err := requireOneOf("method", in.Method, "manual-review", "manual-inspection"); err != nil {
		return err
	}
	if in.Reasoning != "" {
		if err := requireNonEmpty("reasoning", in.Reasoning); err != nil {
			return err
		}
	}
	return nil
}
