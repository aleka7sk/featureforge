package peos

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/aleka7sk/PEOS/peos/core"
	"github.com/aleka7sk/PEOS/peos/decision"
	"github.com/aleka7sk/PEOS/peos/lifecycle"
	"github.com/aleka7sk/PEOS/peos/requirement"
	"github.com/aleka7sk/PEOS/peos/validation"
	"github.com/aleka7sk/featureforge/internal/canonicaltime"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// ValidateArtifact implements application.EngineeringReplayInspector.  It
// treats Payload as authoritative, then proves that every stored projection
// agrees with the decoded PEOS value.
func (Recorder) ValidateArtifact(env engineering.ArtifactEnvelope) error {
	if env.Kind != engineering.RecordKindArtifact || env.Key.IsZero() {
		return fmt.Errorf("artifact envelope key/kind mismatch")
	}
	if err := validatePayloadRoundTrip(env.Payload, env.PayloadDigest, func() any { return &core.Artifact{} }); err != nil {
		return err
	}
	artifact, err := DecodeArtifact(env.Payload)
	if err != nil {
		return err
	}
	if artifact.ID().String() != env.Key.ArtifactID || artifact.Type().String() != env.ArtifactType {
		return fmt.Errorf("artifact payload/projection mismatch")
	}
	if _, hasScope := artifact.Scope(); hasScope || !artifact.Extension().IsZero() {
		return fmt.Errorf("artifact carries unsupported scope or extension metadata")
	}
	roles := artifact.Roles()
	if env.ArtifactType == ArtifactTypeValidationEvidence.String() {
		if len(roles) != 1 || roles[0].String() != core.ArtifactRoleEvidence.String() {
			return fmt.Errorf("validation evidence artifact must have exactly the evidence role")
		}
	} else if len(roles) != 0 {
		return fmt.Errorf("FeatureForge artifact carries unsupported roles")
	}
	if env.RecordedAt.IsZero() {
		return fmt.Errorf("artifact recorded-at is missing")
	}
	return nil
}

// ArtifactFamily classifies a validated FeatureForge Artifact using the same
// closed PEOS vocabulary used by revision inspection.
func (Recorder) ArtifactFamily(env engineering.ArtifactEnvelope) (engineering.RevisionFamily, error) {
	if err := (Recorder{}).ValidateArtifact(env); err != nil {
		return "", err
	}
	switch env.ArtifactType {
	case ArtifactTypeProductCapability.String():
		return engineering.RevisionFamilyCapability, nil
	case requirement.ArtifactTypeRequirement.String():
		return engineering.RevisionFamilyRequirement, nil
	case validation.ArtifactTypeValidationPlan.String():
		return engineering.RevisionFamilyValidationPlan, nil
	case ArtifactTypeValidationEvidence.String():
		return engineering.RevisionFamilyEvidence, nil
	case lifecycle.ArtifactTypeTransitionRecord.String():
		return engineering.RevisionFamilyTransitionRecord, nil
	default:
		return "", fmt.Errorf("unsupported FeatureForge artifact type %q", env.ArtifactType)
	}
}

// ValidateCapabilityArtifact validates the envelope and proves its decoded
// Artifact type is the FeatureForge capability type.
func (Recorder) ValidateCapabilityArtifact(env engineering.ArtifactEnvelope) error {
	if err := (Recorder{}).ValidateArtifact(env); err != nil {
		return err
	}
	if env.ArtifactType != ArtifactTypeProductCapability.String() {
		return fmt.Errorf("artifact is not a product capability")
	}
	return nil
}

// ValidateEvidenceArtifact validates the envelope and proves its decoded
// Artifact type is the FeatureForge validation-evidence type.
func (Recorder) ValidateEvidenceArtifact(env engineering.ArtifactEnvelope) error {
	if err := (Recorder{}).ValidateArtifact(env); err != nil {
		return err
	}
	if env.ArtifactType != ArtifactTypeValidationEvidence.String() {
		return fmt.Errorf("artifact is not validation evidence")
	}
	artifact, err := DecodeArtifact(env.Payload)
	if err != nil {
		return err
	}
	roles := artifact.Roles()
	if len(roles) != 1 || roles[0].String() != core.ArtifactRoleEvidence.String() {
		return fmt.Errorf("validation evidence artifact must have exactly the evidence role")
	}
	return nil
}

func validateEvidenceRevision(revision core.ArtifactRevision) error {
	representations := revision.Representations()
	if len(representations) != 1 {
		return fmt.Errorf("evidence revision must have exactly one representation")
	}
	representation := representations[0]
	locator, ok := representation.Content().AsExternalReference()
	if !ok || locator == "" {
		return fmt.Errorf("evidence representation must be an external reference")
	}
	if representation.MediaType().String() != MediaTypeValidationReport.String() {
		return fmt.Errorf("evidence representation has the wrong media type")
	}
	classification := representation.Classification()
	if len(classification) != 1 || classification[0].String() != core.RepresentationRoleAuthoritative.String() {
		return fmt.Errorf("evidence representation must be uniquely authoritative")
	}
	if _, present := representation.Language(); present {
		return fmt.Errorf("evidence representation carries an unsupported language")
	}
	if _, present := representation.Transformation(); present || !representation.Extension().IsZero() {
		return fmt.Errorf("evidence representation carries unsupported transformation or extension metadata")
	}
	integrity := revision.Integrity()
	if integrity.Mechanism().String() != core.IntegrityMechanismImmutableVersionIdentifier.String() || integrity.Value() != locator {
		return fmt.Errorf("evidence integrity must identify its external representation")
	}
	scopes := integrity.ProtectedScopes()
	if len(scopes) != 1 || scopes[0].String() != core.IntegrityProtectedScopeRepresentation.String() {
		return fmt.Errorf("evidence integrity must protect exactly the representation")
	}
	return nil
}

// ValidateCapabilityContent proves that the product-owned structured content
// persisted beside a capability revision is the exact content addressed by
// the authoritative PEOS revision.
func (Recorder) ValidateCapabilityContent(env engineering.RevisionEnvelope, content engineering.CapabilitySpecificationContent) error {
	if err := (Recorder{}).ValidateRevision(env); err != nil {
		return err
	}
	if env.RevisionFamily != engineering.RevisionFamilyCapability {
		return fmt.Errorf("structured capability content belongs to another revision family")
	}
	return (Recorder{}).VerifyContentDigest(env, content)
}

// ValidationPlanReferences returns the governed reference projections hidden
// inside a validated plan payload without exposing PEOS types to application.
func (Recorder) ValidationPlanReferences(env engineering.RevisionEnvelope) (string, []string, []string, []engineering.RevisionKey, []engineering.RevisionKey, error) {
	if err := (Recorder{}).ValidateRevision(env); err != nil {
		return "", nil, nil, nil, nil, err
	}
	if env.RevisionFamily != engineering.RevisionFamilyValidationPlan {
		return "", nil, nil, nil, nil, fmt.Errorf("revision is not a validation plan")
	}
	revision, err := DecodePlanRevision(env.Payload)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	content := revision.Content()
	scopeArtifactID := content.Scope().Expression()
	activities := content.Activities()
	keys := make([]string, 0, len(activities))
	methods := make([]string, 0, len(activities))
	subjects := make([]engineering.RevisionKey, 0, len(activities))
	requirements := make([]engineering.RevisionKey, 0, len(activities))
	for _, activity := range activities {
		keys = append(keys, activity.Key().String())
		methods = append(methods, strings.TrimPrefix(activity.Method().String(), Namespace+":"))
		subjectRef, ok := activity.Subject().AsArtifactRevision()
		if !ok {
			return "", nil, nil, nil, nil, fmt.Errorf("plan activity subject is not an artifact revision")
		}
		subject, err := engineering.NewRevisionKey(subjectRef.ArtifactID().String(), subjectRef.RevisionID().String())
		if err != nil {
			return "", nil, nil, nil, nil, err
		}
		subjects = append(subjects, subject)
		criteria := activity.Criteria()
		if len(criteria) != 1 {
			return "", nil, nil, nil, nil, fmt.Errorf("plan activity must name exactly one requirement criterion")
		}
		requirementRef, ok := criteria[0].AsRequirementRevision()
		if !ok {
			return "", nil, nil, nil, nil, fmt.Errorf("plan activity criterion is not a requirement revision")
		}
		requirementKey, err := engineering.NewRevisionKey(requirementRef.ArtifactID().String(), requirementRef.RevisionID().String())
		if err != nil {
			return "", nil, nil, nil, nil, err
		}
		requirements = append(requirements, requirementKey)
	}
	return scopeArtifactID, keys, methods, subjects, requirements, nil
}

// ValidateRevision implements application.EngineeringReplayInspector.
func (Recorder) ValidateRevision(env engineering.RevisionEnvelope) error {
	if env.Kind != engineering.RecordKindRevision || env.Key.IsZero() {
		return fmt.Errorf("revision envelope key/kind mismatch")
	}
	if !engineering.ComputeDigest(env.Payload).Equal(env.PayloadDigest) {
		return fmt.Errorf("revision payload digest mismatch")
	}

	var (
		coreRevision core.ArtifactRevision
		content      any
		expectedType string
		subjectKey   string
	)
	switch env.RevisionFamily {
	case engineering.RevisionFamilyCapability:
		revision, err := DecodeArtifactRevision(env.Payload)
		if err != nil {
			return err
		}
		coreRevision = revision
		expectedType = ArtifactTypeProductCapability.String()
	case engineering.RevisionFamilyEvidence:
		revision, err := DecodeArtifactRevision(env.Payload)
		if err != nil {
			return err
		}
		coreRevision = revision
		expectedType = ArtifactTypeValidationEvidence.String()
		if err := validateEvidenceRevision(revision); err != nil {
			return err
		}
	case engineering.RevisionFamilyRequirement:
		revision, err := DecodeRequirementRevision(env.Payload)
		if err != nil {
			return err
		}
		coreRevision = revision.Core()
		content = revision.Content()
		expectedType = requirement.ArtifactTypeRequirement.String()
		subjects := revision.Content().Subjects()
		statements := revision.Content().Statements()
		if len(statements) != 1 || len(subjects) != 1 {
			return fmt.Errorf("requirement revision must have exactly one statement and subject")
		}
		if revision.Content().SubjectCombination().Value().String() != requirement.SubjectCombinationIndependent.Value().String() || !revision.Content().Applicability().IsUnrestricted() {
			return fmt.Errorf("requirement revision does not use the configured subject combination and applicability")
		}
		if len(revision.Content().Origins()) != 0 || len(revision.Content().Authorities()) != 0 || len(revision.Content().Classifications()) != 0 || !revision.Content().Rationale().IsZero() {
			return fmt.Errorf("requirement revision carries unsupported optional content")
		}
		subjectKey, err = engineeringSubjectKey(subjects[0])
		if err != nil {
			return err
		}
	case engineering.RevisionFamilyValidationPlan:
		revision, err := DecodePlanRevision(env.Payload)
		if err != nil {
			return err
		}
		coreRevision = revision.Core()
		content = revision.Content()
		if err := validateValidationPlanContent(revision.Content(), env.RecordedAt); err != nil {
			return err
		}
		expectedType = validation.ArtifactTypeValidationPlan.String()
		scope := revision.Content().Scope()
		if scope.Kind().String() != CapabilityScopeKind.String() || scope.Expression() == "" {
			return fmt.Errorf("validation-plan scope is invalid")
		}
		subjectKey = engineering.ArtifactSubjectKey(scope.Expression())
	case engineering.RevisionFamilyTransitionRecord:
		revision, err := DecodeTransitionRecordRevision(env.Payload)
		if err == nil {
			if revision.Content().DefinitionVersion() != definitionVersionRef {
				return fmt.Errorf("transition revision names another lifecycle definition version")
			}
			if !revision.Content().Outcome().Equal(lifecycle.TransitionOutcomeSucceeded) {
				return fmt.Errorf("transition revision does not record the configured successful outcome")
			}
			authority, hasAuthority := revision.Content().Authority()
			if !hasAuthority || authority != LocalAuthorityRef {
				return fmt.Errorf("transition revision does not name the configured local authority")
			}
			target, hasTarget := revision.Content().ToState()
			transitionTargets := map[string]string{
				TransitionSpecify.String():         StateSpecified.String(),
				TransitionBeginValidation.String(): StateUnderValidation.String(),
				TransitionAssess.String():          StateAssessed.String(),
			}
			expectedTarget, supported := transitionTargets[revision.Content().Transition().String()]
			if !supported || !hasTarget || target.String() != expectedTarget {
				return fmt.Errorf("transition revision has an unsupported transition or target state")
			}
			if !revision.Content().Extension().IsZero() {
				return fmt.Errorf("transition revision carries an unsupported extension")
			}
			coreRevision = revision.Core()
			content = revision.Content()
			expectedType = lifecycle.ArtifactTypeTransitionRecord.String()
			subjectKey, err = lifecycleSubjectKey(revision.Content().Subject())
			if err != nil {
				return err
			}
		} else {
			entry, entryErr := DecodeArtifactRevision(env.Payload)
			if entryErr != nil {
				return fmt.Errorf("transition revision decodes as neither content-bearing nor entry revision: %v; %v", err, entryErr)
			}
			coreRevision = entry
			expectedType = lifecycle.ArtifactTypeTransitionRecord.String()
			if _, artifactID, _, parseErr := engineering.ParseSubjectKey(env.SubjectKey); parseErr != nil || artifactID == "" {
				return fmt.Errorf("entry transition subject projection is invalid")
			}
			subjectKey = env.SubjectKey // content-free by the governed entry representation
		}
	default:
		return fmt.Errorf("unsupported revision family %q", env.RevisionFamily)
	}

	if err := validateCanonicalPayload(env.Payload, coreRevision, content); err != nil {
		return err
	}
	if coreRevision.ArtifactID().String() != env.Key.ArtifactID || coreRevision.RevisionID().String() != env.Key.RevisionID {
		return fmt.Errorf("revision payload/key mismatch")
	}
	if env.ArtifactType != expectedType || env.SubjectKey != subjectKey {
		return fmt.Errorf("revision type/subject projection mismatch")
	}
	if env.IntegrityValue != coreRevision.Integrity().Value() {
		return fmt.Errorf("revision integrity projection mismatch")
	}
	if err := validateProvenanceProjection(coreRevision.Provenance(), env.ProvenanceActor, env.HasProvenanceActor, env.ProvenanceRecordedAt, env.HasProvenanceTime, env.RecordedAt); err != nil {
		return err
	}

	if content != nil {
		contentPayload, err := json.Marshal(content)
		if err != nil {
			return err
		}
		if !engineering.ComputeDigest(contentPayload).Equal(env.ContentDigest) {
			return fmt.Errorf("revision content digest projection mismatch")
		}
		if env.IntegrityValue != "sha256:"+env.ContentDigest.Hex() {
			return fmt.Errorf("revision content integrity mismatch")
		}
	} else if env.RevisionFamily == engineering.RevisionFamilyCapability {
		if env.ContentDigest.IsZero() || env.IntegrityValue != "sha256:"+env.ContentDigest.Hex() {
			return fmt.Errorf("capability content digest/integrity mismatch")
		}
	} else if !env.ContentDigest.IsZero() {
		return fmt.Errorf("content-free revision projects a content digest")
	}
	if err := validateFeatureForgeRevisionRepresentation(env, coreRevision, content != nil); err != nil {
		return err
	}
	return nil
}

func validateFeatureForgeRevisionRepresentation(env engineering.RevisionEnvelope, revision core.ArtifactRevision, hasContent bool) error {
	if !revision.Extension().IsZero() || !revision.Integrity().Extension().IsZero() {
		return fmt.Errorf("revision or integrity identity carries an unsupported extension")
	}
	origin := revision.Origin()
	if origin.Kind().String() != core.OriginKindKnown.String() {
		return fmt.Errorf("revision origin is not the configured known origin")
	}
	if _, hasNote := origin.Note(); hasNote || !origin.Extension().IsZero() {
		return fmt.Errorf("revision origin carries unsupported metadata")
	}

	switch env.RevisionFamily {
	case engineering.RevisionFamilyEvidence:
		return nil // validateEvidenceRevision already proves its special representation contract.
	case engineering.RevisionFamilyCapability:
		if err := validateContentAddressedRevisionIntegrity(revision, env.ContentDigest); err != nil {
			return err
		}
		representations := revision.Representations()
		if len(representations) != 1 {
			return fmt.Errorf("capability revision must have exactly one representation")
		}
		representation := representations[0]
		if _, ok := representation.Language(); ok {
			return fmt.Errorf("capability representation carries an unsupported language")
		}
		if _, ok := representation.Transformation(); ok || !representation.Extension().IsZero() {
			return fmt.Errorf("capability representation carries unsupported transformation or extension metadata")
		}
		algorithm, digest, ok := representation.Content().AsContentAddress()
		if !ok || algorithm.String() != ContentAddressAlgorithm.String() || digest != env.ContentDigest.Hex() {
			return fmt.Errorf("capability representation does not address its canonical content")
		}
		classification := representation.Classification()
		if representation.MediaType().String() != MediaTypeSpecificationContent.String() || len(classification) != 1 || classification[0].String() != core.RepresentationRoleAuthoritative.String() {
			return fmt.Errorf("capability representation has invalid media type or authority")
		}
		return nil
	case engineering.RevisionFamilyRequirement, engineering.RevisionFamilyValidationPlan:
		if err := validateContentAddressedRevisionIntegrity(revision, env.ContentDigest); err != nil {
			return err
		}
		if len(revision.Representations()) != 0 {
			return fmt.Errorf("embedded-content revision has unexpected representations")
		}
		return nil
	case engineering.RevisionFamilyTransitionRecord:
		if hasContent {
			if err := validateContentAddressedRevisionIntegrity(revision, env.ContentDigest); err != nil {
				return err
			}
		} else {
			integrity := revision.Integrity()
			scopes := integrity.ProtectedScopes()
			wantValue := env.Key.ArtifactID + "/" + env.Key.RevisionID
			if integrity.Mechanism().String() != core.IntegrityMechanismImmutableVersionIdentifier.String() || integrity.Value() != wantValue || len(scopes) != 1 || scopes[0].String() != core.IntegrityProtectedScopeMetadata.String() {
				return fmt.Errorf("entry transition revision has invalid identity integrity")
			}
		}
		if len(revision.Representations()) != 0 {
			return fmt.Errorf("transition revision has unexpected representations")
		}
		return nil
	default:
		return fmt.Errorf("unsupported FeatureForge revision family %q", env.RevisionFamily)
	}
}

func validateContentAddressedRevisionIntegrity(revision core.ArtifactRevision, digest engineering.Digest) error {
	integrity := revision.Integrity()
	scopes := integrity.ProtectedScopes()
	if integrity.Mechanism().String() != core.IntegrityMechanismContentAddressedReference.String() ||
		integrity.Value() != "sha256:"+digest.Hex() || len(scopes) != 1 || scopes[0].String() != core.IntegrityProtectedScopeContent.String() {
		return fmt.Errorf("revision content integrity mechanism or scope is invalid")
	}
	return nil
}

func validateValidationPlanContent(content validation.PlanContent, recordedAt time.Time) error {
	scope := content.Scope()
	if scope.Kind().String() != CapabilityScopeKind.String() || scope.Expression() == "" {
		return fmt.Errorf("validation-plan scope is not the configured capability scope")
	}
	applicabilityScope, scoped := content.Applicability().Scope()
	if !scoped || !applicabilityScope.Equal(scope) {
		return fmt.Errorf("validation-plan applicability does not match its scope")
	}
	if err := validateFeatureForgeProvenance(content.Provenance()); err != nil {
		return fmt.Errorf("validation-plan content provenance: %w", err)
	}
	provenanceTime, _ := content.Provenance().RecordedAt()
	if !canonicaltime.Equal(provenanceTime.Time(), recordedAt) {
		return fmt.Errorf("validation-plan content provenance time mismatch")
	}
	if _, present := content.AcceptanceRules(); present || !content.Extension().IsZero() {
		return fmt.Errorf("validation-plan content carries unsupported rules or extension")
	}
	activities := content.Activities()
	if len(activities) == 0 {
		return fmt.Errorf("validation plan has no activities")
	}
	keys := make(map[string]struct{}, len(activities))
	for _, activity := range activities {
		if _, duplicate := keys[activity.Key().String()]; duplicate {
			return fmt.Errorf("validation plan contains duplicate activity keys")
		}
		keys[activity.Key().String()] = struct{}{}
		if !isSupportedValidationMethod(activity.Method().String()) || strings.TrimSpace(activity.OutcomeInterpretation()) == "" {
			return fmt.Errorf("validation-plan activity uses unsupported method or empty outcome interpretation")
		}
		if _, ok := activity.Subject().AsArtifactRevision(); !ok {
			return fmt.Errorf("validation-plan activity subject is not an artifact revision")
		}
		criteria := activity.Criteria()
		if len(criteria) != 1 {
			return fmt.Errorf("validation-plan activity must have exactly one criterion")
		}
		if _, ok := criteria[0].AsRequirementRevision(); !ok {
			return fmt.Errorf("validation-plan activity criterion is not a requirement revision")
		}
		for _, expected := range activity.ExpectedEvidence() {
			if strings.TrimSpace(expected) == "" {
				return fmt.Errorf("validation-plan activity has empty expected evidence")
			}
		}
		_, hasRole := activity.ResponsibleRole()
		_, hasAuthority := activity.RequiredAuthority()
		_, hasMethodDefinition := activity.MethodDefinition()
		if len(activity.Prerequisites()) != 0 || len(activity.Dependencies()) != 0 || hasRole || hasAuthority || hasMethodDefinition || !activity.Extension().IsZero() {
			return fmt.Errorf("validation-plan activity carries unsupported optional metadata")
		}
	}
	return nil
}

// ValidateRecord implements application.EngineeringReplayInspector.
func (Recorder) ValidateRecord(env engineering.RecordEnvelope) error {
	if env.Key.IsZero() || env.Kind != env.Key.Kind {
		return fmt.Errorf("record envelope key/kind mismatch")
	}
	if !engineering.ComputeDigest(env.Payload).Equal(env.PayloadDigest) {
		return fmt.Errorf("record payload digest mismatch")
	}
	if err := validateRecordProjectionShape(env); err != nil {
		return err
	}

	switch env.Kind {
	case engineering.RecordKindDecision:
		value, err := DecodeDecision(env.Payload)
		if err != nil {
			return err
		}
		if err := validateRoundTrip(env.Payload, value); err != nil {
			return err
		}
		outcome := value.Outcome()
		if !outcome.CommitmentEffect().Equal(decision.CommitmentEffectEstablishes) {
			return fmt.Errorf("decision has an unsupported commitment effect")
		}
		if _, present := outcome.Kind(); present || len(outcome.Commitments()) != 0 || !outcome.Extension().IsZero() {
			return fmt.Errorf("decision outcome carries unsupported optional metadata")
		}
		authority := value.Authority()
		requirements, bases := authority.Requirements(), authority.Bases()
		if len(requirements) != 1 || requirements[0] != LocalAuthorityRef || len(bases) != 1 || bases[0] != LocalAuthorityRef {
			return fmt.Errorf("decision authority is not the configured local authority")
		}
		subjects := value.Subjects()
		if len(subjects) != 1 || value.ID().String() != env.Key.ID {
			return fmt.Errorf("decision identity/subject mismatch")
		}
		subjectRevision, subjectIsRevision := subjects[0].AsArtifactRevision()
		if !subjectIsRevision || value.Applicability().Kind().String() != CapabilityScopeKind.String() || value.Applicability().Expression() != subjectRevision.ArtifactID().String() {
			return fmt.Errorf("decision subject and applicability do not use the configured capability scope")
		}
		if _, hasQuestion := value.Question(); !hasQuestion || len(value.Roles()) != 0 || len(value.Consequences()) != 0 || !value.Extension().IsZero() {
			return fmt.Errorf("decision carries unsupported shape or optional metadata")
		}
		for _, alternative := range value.Alternatives() {
			if _, hasNote := alternative.Note(); hasNote || !alternative.Extension().IsZero() {
				return fmt.Errorf("decision alternative carries unsupported optional metadata")
			}
		}
		subjectKey, err := engineeringSubjectKey(subjects[0])
		if err != nil || subjectKey != env.SubjectKey || projectScope(value.Applicability()) != env.Scope {
			return fmt.Errorf("decision projection mismatch")
		}
		provenance, ok := value.Provenance()
		if !ok || validateRecordTime(provenance, env) != nil {
			return fmt.Errorf("decision provenance projection mismatch")
		}
		occurredAt, hasOccurredAt := provenance.RecordedAt()
		if !hasOccurredAt || !env.HasOccurredAt || !canonicaltime.Equal(occurredAt.Time(), env.OccurredAt) {
			return fmt.Errorf("decision occurrence projection mismatch")
		}
		evidence := []string(nil)
		if basis, ok := value.Basis(); ok {
			if !basis.Extension().IsZero() {
				return fmt.Errorf("decision basis carries an unsupported extension")
			}
			for _, assumption := range basis.Assumptions() {
				_, hasScope := assumption.Scope()
				_, hasSource := assumption.Source()
				_, hasUncertainty := assumption.Uncertainty()
				_, hasCondition := assumption.ExpectedValidationCondition()
				_, hasConsequence := assumption.ConsequenceIfFalse()
				if hasScope || hasSource || hasUncertainty || hasCondition || hasConsequence || !assumption.Extension().IsZero() {
					return fmt.Errorf("decision assumption carries unsupported optional metadata")
				}
			}
			for _, constraint := range basis.Constraints() {
				if _, hasSource := constraint.Source(); hasSource || !constraint.Extension().IsZero() {
					return fmt.Errorf("decision constraint carries unsupported optional metadata")
				}
			}
			for _, uncertainty := range basis.Uncertainties() {
				if !uncertainty.Extension().IsZero() {
					return fmt.Errorf("decision uncertainty carries an unsupported extension")
				}
			}
			for _, ref := range basis.Evidence() {
				evidence = append(evidence, engineering.EvidenceKey(ref.ArtifactID().String(), ref.RevisionID().String()))
			}
		}
		if len(evidence) != 1 {
			return fmt.Errorf("decision must cite exactly one evidence revision")
		}
		if !reflect.DeepEqual(evidence, env.EvidenceKeys) {
			return fmt.Errorf("decision evidence projection mismatch")
		}
	case engineering.RecordKindExecution:
		value, err := DecodeExecution(env.Payload)
		if err != nil {
			return err
		}
		if err := validateRoundTrip(env.Payload, value); err != nil {
			return err
		}
		if !isSupportedValidationMethod(value.Method().String()) || !isSupportedExecutionOutcome(value.Outcome().String()) {
			return fmt.Errorf("execution uses unsupported method or outcome vocabulary")
		}
		if _, _, planned := value.Activity().AsPlanned(); !planned {
			return fmt.Errorf("execution does not cite a planned activity")
		}
		if _, ok := value.Subject().AsArtifactRevision(); !ok {
			return fmt.Errorf("execution subject is not an artifact revision")
		}
		_, hasStarted := value.StartedAt()
		_, hasEnvironment := value.Environment()
		_, hasLimitations := value.Limitations()
		_, hasUncertainty := value.Uncertainty()
		_, hasCorrection := value.Correction()
		if hasStarted || len(value.Criteria()) != 0 || len(value.Events()) != 0 || len(value.ProducedEvidence()) != 1 || len(value.ReliedUponEvidence()) != 0 || hasEnvironment || hasLimitations || hasUncertainty || hasCorrection || !value.Extension().IsZero() {
			return fmt.Errorf("execution carries unsupported optional metadata or evidence shape")
		}
		subjectKey, err := engineeringSubjectKey(value.Subject())
		if err != nil || value.ID().String() != env.Key.ID || subjectKey != env.SubjectKey {
			return fmt.Errorf("execution identity/subject projection mismatch")
		}
		if !env.HasOccurredAt || !canonicaltime.Equal(value.CompletedAt().Time(), env.OccurredAt) || value.Outcome().String() != env.Outcome {
			return fmt.Errorf("execution time/outcome projection mismatch")
		}
		if value.Actor() != LocalActorRef {
			return fmt.Errorf("execution actor is not the configured local actor")
		}
		if _, hasAuthority := value.Authority(); hasAuthority {
			return fmt.Errorf("execution carries unsupported authority metadata")
		}
		if err := validateRecordTime(value.Provenance(), env); err != nil {
			return err
		}
		evidence := make([]string, 0, len(value.ProducedEvidence()))
		for _, ref := range value.ProducedEvidence() {
			evidence = append(evidence, engineering.EvidenceKey(ref.ArtifactID().String(), ref.RevisionID().String()))
		}
		if !reflect.DeepEqual(evidence, env.EvidenceKeys) {
			return fmt.Errorf("execution evidence projection mismatch")
		}
	case engineering.RecordKindClaim:
		value, err := DecodeClaim(env.Payload)
		if err != nil {
			return err
		}
		if err := validateRoundTrip(env.Payload, value); err != nil {
			return err
		}
		if value.ClaimType().String() != core.ClaimTypeSatisfaction.String() || !isSupportedValidationMethod(value.Method().String()) || !isSupportedClaimOutcome(value.Outcome().String()) {
			return fmt.Errorf("claim uses unsupported type, method, or outcome vocabulary")
		}
		if _, ok := value.Subject().AsArtifactRevision(); !ok || value.Scope().Kind().String() != CapabilityScopeKind.String() || value.Scope().Expression() == "" {
			return fmt.Errorf("claim subject or scope does not use the configured revision/capability shape")
		}
		if len(value.Criteria()) != 1 || len(value.Evidence()) != 1 || len(value.ExecutionRecords()) != 1 || !value.Extension().IsZero() {
			return fmt.Errorf("claim has unsupported reference cardinality or extension metadata")
		}
		if _, hasAuthority := value.Authority(); hasAuthority {
			return fmt.Errorf("claim carries unsupported authority metadata")
		}
		subjectKey, err := engineeringSubjectKey(value.Subject())
		if err != nil || value.ID().String() != env.Key.ID || subjectKey != env.SubjectKey || projectScope(value.Scope()) != env.Scope {
			return fmt.Errorf("claim identity/subject/scope projection mismatch")
		}
		if !env.HasOccurredAt || !canonicaltime.Equal(value.Timestamp().Time(), env.OccurredAt) || value.Outcome().String() != env.Outcome {
			return fmt.Errorf("claim time/outcome projection mismatch")
		}
		if err := validateRecordTime(value.Provenance(), env); err != nil {
			return err
		}
		criteria := make([]string, 0, len(value.Criteria()))
		for _, criterion := range value.Criteria() {
			ref, ok := criterion.AsRequirementRevision()
			if !ok {
				return fmt.Errorf("claim criterion is not a requirement revision")
			}
			key, _ := engineering.NewRevisionKey(ref.ArtifactID().String(), ref.RevisionID().String())
			projected, _ := engineering.RequirementCriterionKey(key)
			criteria = append(criteria, projected)
		}
		evidence := make([]string, 0, len(value.Evidence()))
		for _, ref := range value.Evidence() {
			evidence = append(evidence, engineering.EvidenceKey(ref.ArtifactID().String(), ref.RevisionID().String()))
		}
		executions := make([]string, 0, len(value.ExecutionRecords()))
		for _, ref := range value.ExecutionRecords() {
			executions = append(executions, engineering.ExecutionKey(ref.RecordID().String()))
		}
		if !reflect.DeepEqual(criteria, env.CriterionKeys) || !reflect.DeepEqual(evidence, env.EvidenceKeys) || !reflect.DeepEqual(executions, env.ExecutionKeys) {
			return fmt.Errorf("claim reference projection mismatch")
		}
		correctionKind, correctionTarget := "", ""
		if correction, ok := value.Correction(); ok {
			correctionKind = correction.Kind().String()
			correctionTarget = correction.Target().ClaimID().String()
			if correctionTarget == value.ID().String() || !isSupportedCorrectionKind(correctionKind) {
				return fmt.Errorf("claim has an unsupported correction")
			}
		}
		if correctionKind != env.CorrectionKind || correctionTarget != env.CorrectionTargetID {
			return fmt.Errorf("claim correction projection mismatch")
		}
	case engineering.RecordKindStateAssignment:
		value, err := DecodeStateAssignment(env.Payload)
		if err != nil {
			return err
		}
		if err := validateRoundTrip(env.Payload, value); err != nil {
			return err
		}
		if value.DefinitionVersion() != definitionVersionRef {
			return fmt.Errorf("state assignment names another lifecycle definition version")
		}
		allowedStates := map[string]struct{}{
			StateDrafting.String(): {}, StateSpecified.String(): {},
			StateUnderValidation.String(): {}, StateAssessed.String(): {},
		}
		if _, supported := allowedStates[value.State().String()]; !supported {
			return fmt.Errorf("state assignment names an unsupported lifecycle state")
		}
		if _, hasAuthority := value.Authority(); hasAuthority {
			return fmt.Errorf("state assignment carries unsupported authority metadata")
		}
		if !value.Extension().IsZero() {
			return fmt.Errorf("state assignment carries an unsupported extension")
		}
		subjectKey, err := lifecycleSubjectKey(value.Subject())
		if err != nil || value.ID().String() != env.Key.ID || subjectKey != env.SubjectKey || value.State().String() != env.StateID {
			return fmt.Errorf("state-assignment projection mismatch")
		}
		if !env.HasOccurredAt || !canonicaltime.Equal(value.EffectiveAt().Time(), env.OccurredAt) {
			return fmt.Errorf("state-assignment time projection mismatch")
		}
		if err := validateRecordTime(value.Provenance(), env); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported record kind %q", env.Kind)
	}
	return nil
}

func validateRecordProjectionShape(env engineering.RecordEnvelope) error {
	hasCriteria := len(env.CriterionKeys) != 0
	hasEvidence := len(env.EvidenceKeys) != 0
	hasExecutions := len(env.ExecutionKeys) != 0
	hasCorrection := env.CorrectionKind != "" || env.CorrectionTargetID != ""
	switch env.Kind {
	case engineering.RecordKindDecision:
		if env.Outcome != "" || hasCriteria || hasExecutions || hasCorrection || env.StateID != "" {
			return fmt.Errorf("decision envelope carries unsupported projections")
		}
	case engineering.RecordKindExecution:
		if env.Scope != "" || hasCriteria || hasExecutions || hasCorrection || env.StateID != "" {
			return fmt.Errorf("execution envelope carries unsupported projections")
		}
	case engineering.RecordKindClaim:
		if env.StateID != "" {
			return fmt.Errorf("claim envelope carries unsupported projections")
		}
	case engineering.RecordKindStateAssignment:
		if env.Scope != "" || env.Outcome != "" || hasCriteria || hasEvidence || hasExecutions || hasCorrection {
			return fmt.Errorf("state-assignment envelope carries unsupported projections")
		}
	}
	return nil
}

func isSupportedValidationMethod(value string) bool {
	return value == ValidationMethodManualReview.String() || value == ValidationMethodManualInspection.String()
}

func isSupportedExecutionOutcome(value string) bool {
	return value == core.ExecutionOutcomeCompleted.String() || value == core.ExecutionOutcomeFailed.String() ||
		value == core.ExecutionOutcomeInterrupted.String() || value == core.ExecutionOutcomeIndeterminate.String()
}

func isSupportedClaimOutcome(value string) bool {
	return value == core.ClaimOutcomeSatisfied.String() || value == core.ClaimOutcomeNotSatisfied.String() || value == core.ClaimOutcomeInconclusive.String()
}

func isSupportedCorrectionKind(value string) bool {
	return value == core.CorrectionKindCorrect.String() || value == core.CorrectionKindReplace.String() || value == core.CorrectionKindInvalidate.String()
}

// TransitionTimes returns the two caller-semantic times hidden inside a
// content-bearing Transition Record Revision.
func (Recorder) TransitionTimes(env engineering.RevisionEnvelope) (time.Time, time.Time, error) {
	if err := (Recorder{}).ValidateRevision(env); err != nil {
		return time.Time{}, time.Time{}, err
	}
	revision, err := DecodeTransitionRecordRevision(env.Payload)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	completed, ok := revision.Content().CompletedAt()
	if !ok {
		return time.Time{}, time.Time{}, fmt.Errorf("transition completed-at is missing")
	}
	return revision.Content().AttemptedAt().Time(), completed.Time(), nil
}

// TransitionPredecessor returns the State Assignment cited as the source of
// a content-bearing transition. Entry revisions are content-free and have no
// predecessor.
func (Recorder) TransitionPredecessor(env engineering.RevisionEnvelope) (string, bool, error) {
	if err := (Recorder{}).ValidateRevision(env); err != nil {
		return "", false, err
	}
	revision, err := DecodeTransitionRecordRevision(env.Payload)
	if err != nil {
		if _, entryErr := DecodeArtifactRevision(env.Payload); entryErr == nil {
			return "", false, nil
		}
		return "", false, err
	}
	ref := revision.Content().FromAssignment()
	if ref.IsZero() {
		return "", false, fmt.Errorf("transition predecessor is missing")
	}
	return ref.StateAssignmentID().String(), true, nil
}

// TransitionResultingAssignment returns the semantic member named by a
// content-bearing transition revision.  Entry revisions are deliberately
// content-free and return present=false.
func (Recorder) TransitionResultingAssignment(env engineering.RevisionEnvelope) (string, bool, error) {
	if err := (Recorder{}).ValidateRevision(env); err != nil {
		return "", false, err
	}
	revision, err := DecodeTransitionRecordRevision(env.Payload)
	if err != nil {
		if _, entryErr := DecodeArtifactRevision(env.Payload); entryErr == nil {
			return "", false, nil
		}
		return "", false, err
	}
	ref, ok := revision.Content().ResultingAssignment()
	if !ok {
		return "", false, fmt.Errorf("transition has no resulting assignment")
	}
	return ref.StateAssignmentID().String(), true, nil
}

// TransitionTargetState returns the configured state established by a
// content-bearing transition. Entry revisions are content-free and have no
// target state of their own.
func (Recorder) TransitionTargetState(env engineering.RevisionEnvelope) (string, bool, error) {
	if err := (Recorder{}).ValidateRevision(env); err != nil {
		return "", false, err
	}
	revision, err := DecodeTransitionRecordRevision(env.Payload)
	if err != nil {
		if _, entryErr := DecodeArtifactRevision(env.Payload); entryErr == nil {
			return "", false, nil
		}
		return "", false, err
	}
	target, ok := revision.Content().ToState()
	if !ok {
		return "", false, fmt.Errorf("transition target state is missing")
	}
	return target.String(), true, nil
}

// StateAssignmentEstablishedBy exposes the exact transition revision cited
// by a validated assignment without leaking a PEOS reference type.
func (Recorder) StateAssignmentEstablishedBy(env engineering.RecordEnvelope) (engineering.RevisionKey, error) {
	if err := (Recorder{}).ValidateRecord(env); err != nil {
		return engineering.RevisionKey{}, err
	}
	assignment, err := DecodeStateAssignment(env.Payload)
	if err != nil {
		return engineering.RevisionKey{}, err
	}
	ref := assignment.EstablishedBy()
	return engineering.NewRevisionKey(ref.ArtifactID().String(), ref.RevisionID().String())
}

// ExecutionPlanActivity returns the exact Validation Plan Revision and local
// planned-activity key cited by a validated execution record.
func (Recorder) ExecutionPlanActivity(env engineering.RecordEnvelope) (engineering.RevisionKey, string, string, error) {
	if err := (Recorder{}).ValidateRecord(env); err != nil {
		return engineering.RevisionKey{}, "", "", err
	}
	execution, err := DecodeExecution(env.Payload)
	if err != nil {
		return engineering.RevisionKey{}, "", "", err
	}
	plan, activity, ok := execution.Activity().AsPlanned()
	if !ok {
		return engineering.RevisionKey{}, "", "", fmt.Errorf("execution does not cite a planned activity")
	}
	key, err := engineering.NewRevisionKey(plan.ArtifactID().String(), plan.RevisionID().String())
	method := strings.TrimPrefix(execution.Method().String(), Namespace+":")
	return key, activity.String(), method, err
}

// ClaimMethod returns the FeatureForge vocabulary suffix of a validated
// claim's validation method.
func (Recorder) ClaimMethod(env engineering.RecordEnvelope) (string, error) {
	if err := (Recorder{}).ValidateRecord(env); err != nil {
		return "", err
	}
	claim, err := DecodeClaim(env.Payload)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(claim.Method().String(), Namespace+":"), nil
}

func validatePayloadRoundTrip(payload []byte, digest engineering.Digest, newValue func() any) error {
	if !engineering.ComputeDigest(payload).Equal(digest) {
		return fmt.Errorf("payload digest mismatch")
	}
	value := newValue()
	if err := json.Unmarshal(payload, value); err != nil {
		return err
	}
	return validateRoundTrip(payload, value)
}

func validateRoundTrip(payload []byte, value any) error {
	reencoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(payload, reencoded) {
		return fmt.Errorf("payload is not the canonical decoded representation")
	}
	return nil
}

func validateCanonicalPayload(payload []byte, coreRevision core.ArtifactRevision, content any) error {
	var value any = coreRevision
	switch typed := content.(type) {
	case requirement.Content:
		decoded, err := DecodeRequirementRevision(payload)
		if err != nil {
			return err
		}
		value = decoded
	case validation.PlanContent:
		decoded, err := DecodePlanRevision(payload)
		if err != nil {
			return err
		}
		value = decoded
	case lifecycle.TransitionRecordContent:
		decoded, err := DecodeTransitionRecordRevision(payload)
		if err != nil {
			return err
		}
		value = decoded
	default:
		_ = typed
	}
	return validateRoundTrip(payload, value)
}

func validateProvenanceProjection(provenance core.Provenance, actor string, hasActor bool, recordedAt time.Time, hasTime bool, envelopeRecordedAt time.Time) error {
	if err := validateFeatureForgeProvenance(provenance); err != nil {
		return err
	}
	actualActor, actualHasActor := provenance.Actor()
	actualTime, actualHasTime := provenance.RecordedAt()
	if !actualHasActor || actualActor != LocalActorRef || !actualHasTime {
		return fmt.Errorf("provenance does not name the configured local actor and time")
	}
	if actualHasActor != hasActor || actualHasTime != hasTime {
		return fmt.Errorf("provenance presence projection mismatch")
	}
	if hasActor && actor != actualActor.Namespace()+":"+actualActor.Identifier() {
		return fmt.Errorf("provenance actor projection mismatch")
	}
	if hasTime && (!canonicaltime.Equal(actualTime.Time(), recordedAt) || !canonicaltime.Equal(actualTime.Time(), envelopeRecordedAt)) {
		return fmt.Errorf("provenance time projection mismatch")
	}
	return nil
}

func validateRecordTime(provenance core.Provenance, env engineering.RecordEnvelope) error {
	if err := validateFeatureForgeProvenance(provenance); err != nil {
		return err
	}
	actor, hasActor := provenance.Actor()
	if !hasActor || actor != LocalActorRef {
		return fmt.Errorf("record provenance actor is not the configured local actor")
	}
	actual, ok := provenance.RecordedAt()
	if !ok || !canonicaltime.Equal(actual.Time(), env.RecordedAt) {
		return fmt.Errorf("record provenance/recorded-at mismatch")
	}
	return nil
}

func validateFeatureForgeProvenance(provenance core.Provenance) error {
	actor, hasActor := provenance.Actor()
	_, hasTime := provenance.RecordedAt()
	_, hasSource := provenance.Source()
	_, hasMethod := provenance.Method()
	_, hasExternalSource := provenance.ExternalSourceID()
	if !hasActor || actor != LocalActorRef || !hasTime || hasSource || hasMethod || hasExternalSource || !provenance.Extension().IsZero() {
		return fmt.Errorf("provenance is not the configured local actor/time-only shape")
	}
	return nil
}

func engineeringSubjectKey(subject core.EngineeringSubjectRef) (string, error) {
	if ref, ok := subject.AsArtifact(); ok {
		return engineering.ArtifactSubjectKey(ref.ArtifactID().String()), nil
	}
	if ref, ok := subject.AsArtifactRevision(); ok {
		return engineering.ArtifactRevisionSubjectKey(ref.ArtifactID().String(), ref.RevisionID().String()), nil
	}
	return "", fmt.Errorf("unsupported engineering subject %q", subject.Kind())
}

func lifecycleSubjectKey(subject core.LifecycleSubjectRef) (string, error) {
	return engineeringSubjectKey(subject.Subject())
}
