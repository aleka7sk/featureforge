package peos

import (
	"encoding/json"
	"fmt"

	"github.com/aleka7sk/PEOS/peos/core"
	"github.com/aleka7sk/PEOS/peos/validation"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// BuildValidationPlan constructs the Validation Plan Artifact and its
// founding Plan Revision, with one Planned Activity per input entry, each
// citing its Requirement at the exact revision level (FF-011 §4.4).
func BuildValidationPlan(in PlanInput) (engineering.ArtifactEnvelope, engineering.RevisionEnvelope, error) {
	artifactID, err := core.NewArtifactID(in.ArtifactID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan artifact id", err)
	}
	artifact, err := core.NewArtifact(artifactID, validation.ArtifactTypeValidationPlan)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan artifact", err)
	}
	plan, err := validation.NewPlan(artifact)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan", err)
	}
	artifactPayload, err := json.Marshal(artifact)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan artifact marshal", err)
	}
	artifactKey, err := engineering.NewArtifactKey(in.ArtifactID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}
	artifactEnv, err := engineering.NewArtifactEnvelope(
		artifactKey, validation.ArtifactTypeValidationPlan.String(), artifactPayload,
		engineering.ComputeDigest(artifactPayload), in.RecordedAt,
	)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}

	activities := make([]validation.PlannedActivity, len(in.Activities))
	for i, a := range in.Activities {
		key, err := core.NewLocalKey(a.Key)
		if err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("activity key", err)
		}
		subjArtID, err := core.NewArtifactID(a.SubjectArtifactID)
		if err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("activity subject artifact id", err)
		}
		subjRevID, err := core.NewArtifactRevisionID(a.SubjectRevisionID)
		if err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("activity subject revision id", err)
		}
		subjRevRef, err := core.NewArtifactRevisionRef(subjArtID, subjRevID)
		if err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("activity subject revision ref", err)
		}
		subject, err := core.EngineeringSubjectRefFromArtifactRevision(subjRevRef)
		if err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("activity subject", err)
		}
		method, err := parseValidationMethod(a.Method)
		if err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
		}
		act, err := validation.NewPlannedActivity(key, subject, method, a.OutcomeInterpretation)
		if err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("planned activity", err)
		}
		reqArtID, err := core.NewArtifactID(a.RequirementArtifactID)
		if err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("activity requirement artifact id", err)
		}
		reqRevID, err := core.NewArtifactRevisionID(a.RequirementRevisionID)
		if err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("activity requirement revision id", err)
		}
		reqRevRef, err := core.NewRequirementArtifactRevisionRef(reqArtID, reqRevID)
		if err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("activity requirement ref", err)
		}
		crit, err := core.CriterionRefFromRequirementRevision(reqRevRef)
		if err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("activity criterion", err)
		}
		act, err = act.WithCriteria([]core.CriterionRef{crit})
		if err != nil {
			return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("activity with criteria", err)
		}
		if len(a.ExpectedEvidence) > 0 {
			act, err = act.WithExpectedEvidence(a.ExpectedEvidence)
			if err != nil {
				return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("activity with expected evidence", err)
			}
		}
		activities[i] = act
	}

	scope, err := core.NewScope(CapabilityScopeKind, in.ScopeArtifactID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan scope", err)
	}
	applicability, err := validation.NewScopedPlanApplicability(scope)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan applicability", err)
	}
	provenance, err := provenanceFor(in.RecordedAt)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}
	planContent, err := validation.NewPlanContent(scope, applicability, provenance, activities)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan content", err)
	}
	contentPayload, err := json.Marshal(planContent)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan content marshal", err)
	}
	contentDigest := engineering.ComputeDigest(contentPayload)

	revisionID, err := core.NewArtifactRevisionID(in.RevisionID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan revision id", err)
	}
	origin, err := core.NewOrigin(core.OriginKindKnown, "")
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan origin", err)
	}
	integrity, err := contentAddressedIntegrity(contentDigest)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan integrity", err)
	}
	coreRev, err := core.NewArtifactRevision(artifactID, revisionID, origin, provenance, integrity)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan core revision", err)
	}
	planRev, err := validation.NewPlanRevision(plan, coreRev, planContent)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, wrapPEOS("plan revision", err)
	}

	revKey, err := engineering.NewRevisionKey(in.ArtifactID, in.RevisionID)
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}
	revEnv, err := buildRevisionEnvelope(revisionEnvelopeInput{
		Key:           revKey,
		Family:        engineering.RevisionFamilyValidationPlan,
		ArtifactType:  validation.ArtifactTypeValidationPlan,
		Core:          coreRev,
		Payload:       planRev,
		ContentDigest: contentDigest,
		SubjectKey:    engineering.ArtifactSubjectKey(in.ScopeArtifactID),
		RecordedAt:    in.RecordedAt,
	})
	if err != nil {
		return engineering.ArtifactEnvelope{}, engineering.RevisionEnvelope{}, err
	}
	return artifactEnv, revEnv, nil
}

// DecodePlanRevision decodes a stored payload back into a
// validation.PlanRevision. Used only by this package's own tests and by
// projection-fidelity checks.
func DecodePlanRevision(payload []byte) (validation.PlanRevision, error) {
	var r validation.PlanRevision
	if err := json.Unmarshal(payload, &r); err != nil {
		return validation.PlanRevision{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return r, nil
}

// DecodePlan decodes a stored Validation Plan artifact payload back into a
// validation.Plan. Used only by this package's own tests.
func DecodePlan(payload []byte) (validation.Plan, error) {
	var p validation.Plan
	if err := json.Unmarshal(payload, &p); err != nil {
		return validation.Plan{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return p, nil
}

// BuildExecution constructs a Validation Execution Record citing the
// planned activity it executed and the evidence it produced, and returns
// its persistence envelope (FF-011 §4.5).
func BuildExecution(in ExecutionInput) (engineering.RecordEnvelope, error) {
	execID, err := core.NewValidationExecutionRecordID(in.ExecutionID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution id", err)
	}
	planArtID, err := core.NewArtifactID(in.PlanArtifactID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution plan artifact id", err)
	}
	planRevID, err := core.NewArtifactRevisionID(in.PlanRevisionID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution plan revision id", err)
	}
	planRevRef, err := core.NewValidationPlanRevisionRef(planArtID, planRevID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution plan revision ref", err)
	}
	activityKey, err := core.NewLocalKey(in.ActivityKey)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution activity key", err)
	}
	activityRef, err := validation.NewPlannedActivityReference(planRevRef, activityKey)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution activity ref", err)
	}
	subjArtID, err := core.NewArtifactID(in.SubjectArtifactID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution subject artifact id", err)
	}
	subjRevID, err := core.NewArtifactRevisionID(in.SubjectRevisionID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution subject revision id", err)
	}
	subjRevRef, err := core.NewArtifactRevisionRef(subjArtID, subjRevID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution subject revision ref", err)
	}
	subject, err := core.EngineeringSubjectRefFromArtifactRevision(subjRevRef)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution subject", err)
	}
	completedAt, err := core.NewTimestamp(in.CompletedAt.UTC())
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution completed at", err)
	}
	provenance, err := provenanceFor(in.RecordedAt)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	method, err := parseValidationMethod(in.Method)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	outcome, err := parseExecutionOutcome(in.Outcome)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	er, err := validation.NewExecutionRecord(execID, activityRef, subject, method, outcome, completedAt, LocalActorRef, provenance)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution record", err)
	}
	evArtID, err := core.NewArtifactID(in.EvidenceArtifactID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution evidence artifact id", err)
	}
	evRevID, err := core.NewArtifactRevisionID(in.EvidenceRevisionID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution evidence revision id", err)
	}
	evidenceRef, err := core.NewEvidenceArtifactRevisionRef(evArtID, evRevID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution evidence ref", err)
	}
	er, err = er.WithProducedEvidence([]core.EvidenceArtifactRevisionRef{evidenceRef})
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution with produced evidence", err)
	}

	payload, err := json.Marshal(er)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("execution marshal", err)
	}
	subjectKey := engineering.ArtifactRevisionSubjectKey(in.SubjectArtifactID, in.SubjectRevisionID)
	evidenceKey := engineering.EvidenceKey(in.EvidenceArtifactID, in.EvidenceRevisionID)
	key, err := engineering.NewRecordKey(engineering.RecordKindExecution, in.ExecutionID)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	return engineering.NewRecordEnvelope(engineering.RecordEnvelopeInput{
		Key:           key,
		SubjectKey:    subjectKey,
		OccurredAt:    in.CompletedAt,
		HasOccurredAt: true,
		Outcome:       outcome.String(),
		EvidenceKeys:  []string{evidenceKey},
		Payload:       payload,
		PayloadDigest: engineering.ComputeDigest(payload),
		RecordedAt:    in.RecordedAt,
	})
}

// DecodeExecution decodes a stored payload back into a
// validation.ExecutionRecord. Used only by this package's own tests and by
// projection-fidelity checks.
func DecodeExecution(payload []byte) (validation.ExecutionRecord, error) {
	var r validation.ExecutionRecord
	if err := json.Unmarshal(payload, &r); err != nil {
		return validation.ExecutionRecord{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return r, nil
}

// BuildClaim constructs a Satisfaction Claim, optionally carrying a
// correction reference, and returns its persistence envelope
// (FF-011 §4.8). PEOS v1.0.0 accepts a self-correcting reference (verified);
// FeatureForge rejects it explicitly before construction (AD-017).
// BuildClaim does not itself reject a self-correcting ClaimInput -- PEOS
// v1.0.0 does not either (verified). That check is the caller's
// responsibility: internal/application's CorrectValidationClaim command
// rejects it before this function is ever invoked (AD-017), and the
// correction-chain query rejects it again on read.
func BuildClaim(in ClaimInput) (engineering.RecordEnvelope, error) {
	claimID, err := core.NewValidationClaimID(in.ClaimID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim id", err)
	}
	subjArtID, err := core.NewArtifactID(in.SubjectArtifactID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim subject artifact id", err)
	}
	subjRevID, err := core.NewArtifactRevisionID(in.SubjectRevisionID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim subject revision id", err)
	}
	subjRevRef, err := core.NewArtifactRevisionRef(subjArtID, subjRevID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim subject revision ref", err)
	}
	subject, err := core.EngineeringSubjectRefFromArtifactRevision(subjRevRef)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim subject", err)
	}
	scope, err := core.NewScope(CapabilityScopeKind, in.ScopeArtifactID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim scope", err)
	}
	reqArtID, err := core.NewArtifactID(in.RequirementArtifactID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim requirement artifact id", err)
	}
	reqRevID, err := core.NewArtifactRevisionID(in.RequirementRevisionID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim requirement revision id", err)
	}
	reqRevRef, err := core.NewRequirementArtifactRevisionRef(reqArtID, reqRevID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim requirement ref", err)
	}
	crit, err := core.CriterionRefFromRequirementRevision(reqRevRef)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim criterion", err)
	}
	evArtID, err := core.NewArtifactID(in.EvidenceArtifactID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim evidence artifact id", err)
	}
	evRevID, err := core.NewArtifactRevisionID(in.EvidenceRevisionID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim evidence revision id", err)
	}
	evidenceRef, err := core.NewEvidenceArtifactRevisionRef(evArtID, evRevID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim evidence ref", err)
	}
	timestamp, err := core.NewTimestamp(in.Timestamp.UTC())
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim timestamp", err)
	}
	provenance, err := provenanceFor(in.RecordedAt)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	outcome, err := parseClaimOutcome(in.Outcome)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	method, err := parseValidationMethod(in.Method)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	claim, err := validation.NewClaim(
		claimID, core.ClaimTypeSatisfaction, subject, scope, outcome, method,
		[]core.CriterionRef{crit}, []core.EvidenceArtifactRevisionRef{evidenceRef},
		timestamp, provenance,
	)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim", err)
	}
	execID, err := core.NewValidationExecutionRecordID(in.ExecutionID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim execution id", err)
	}
	execRef, err := core.NewValidationExecutionRecordRef(execID)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim execution ref", err)
	}
	claim, err = claim.WithExecutionRecords([]core.ValidationExecutionRecordRef{execRef})
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim with execution records", err)
	}
	if in.Reasoning != "" {
		claim, err = claim.WithReasoning(in.Reasoning)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("claim reasoning", err)
		}
	}
	var correctionKind core.CorrectionKind
	if in.HasCorrection {
		targetClaimID, err := core.NewValidationClaimID(in.CorrectionTarget)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("claim correction target id", err)
		}
		targetRef, err := core.NewValidationClaimRef(targetClaimID)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("claim correction target ref", err)
		}
		correctionKind, err = parseCorrectionKind(in.CorrectionKind)
		if err != nil {
			return engineering.RecordEnvelope{}, err
		}
		correction, err := core.NewRecordCorrectionRef(correctionKind, targetRef)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("claim correction ref", err)
		}
		claim, err = claim.WithCorrection(correction)
		if err != nil {
			return engineering.RecordEnvelope{}, wrapPEOS("claim with correction", err)
		}
	}

	payload, err := json.Marshal(claim)
	if err != nil {
		return engineering.RecordEnvelope{}, wrapPEOS("claim marshal", err)
	}
	subjectKey := engineering.ArtifactRevisionSubjectKey(in.SubjectArtifactID, in.SubjectRevisionID)
	requirementRevisionKey, err := engineering.NewRevisionKey(in.RequirementArtifactID, in.RequirementRevisionID)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	criterionKey, err := engineering.RequirementCriterionKey(requirementRevisionKey)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	evidenceKey := engineering.EvidenceKey(in.EvidenceArtifactID, in.EvidenceRevisionID)
	executionKey := engineering.ExecutionKey(in.ExecutionID)
	key, err := engineering.NewRecordKey(engineering.RecordKindClaim, in.ClaimID)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	recordInput := engineering.RecordEnvelopeInput{
		Key:           key,
		SubjectKey:    subjectKey,
		Scope:         projectScope(scope),
		OccurredAt:    in.Timestamp,
		HasOccurredAt: true,
		Outcome:       outcome.String(),
		CriterionKeys: []string{criterionKey},
		EvidenceKeys:  []string{evidenceKey},
		ExecutionKeys: []string{executionKey},
		Payload:       payload,
		PayloadDigest: engineering.ComputeDigest(payload),
		RecordedAt:    in.RecordedAt,
	}
	if in.HasCorrection {
		recordInput.CorrectionKind = correctionKind.String()
		recordInput.CorrectionTargetID = in.CorrectionTarget
	}
	return engineering.NewRecordEnvelope(recordInput)
}

// DecodeClaim decodes a stored payload back into a validation.Claim. Used
// only by this package's own tests and by projection-fidelity checks.
func DecodeClaim(payload []byte) (validation.Claim, error) {
	var c validation.Claim
	if err := json.Unmarshal(payload, &c); err != nil {
		return validation.Claim{}, fmt.Errorf("%w: %w", ErrStoredPayloadInvalid, err)
	}
	return c, nil
}
