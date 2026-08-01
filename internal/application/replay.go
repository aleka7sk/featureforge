package application

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

func inspectArtifact(inspector EngineeringReplayInspector, env engineering.ArtifactEnvelope) error {
	if inspector == nil {
		return integrityError("engineering replay inspector is unavailable", nil)
	}
	if err := inspector.ValidateArtifact(env); err != nil {
		return integrityError("artifact payload/projection disagreement", err)
	}
	return nil
}

func inspectRevision(inspector EngineeringReplayInspector, env engineering.RevisionEnvelope) error {
	if inspector == nil {
		return integrityError("engineering replay inspector is unavailable", nil)
	}
	if err := inspector.ValidateRevision(env); err != nil {
		return integrityError("revision payload/projection disagreement", err)
	}
	return nil
}

func inspectRecord(inspector EngineeringReplayInspector, env engineering.RecordEnvelope) error {
	if inspector == nil {
		return integrityError("engineering replay inspector is unavailable", nil)
	}
	if err := inspector.ValidateRecord(env); err != nil {
		return integrityError("record payload/projection disagreement", err)
	}
	return nil
}

func validateAcceptanceHistory(key engineering.RevisionKey, journal []engineering.RevisionAcceptanceRecord) error {
	ordered := append([]engineering.RevisionAcceptanceRecord(nil), journal...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].EffectiveAt.Equal(ordered[j].EffectiveAt) {
			return ordered[i].RecordID < ordered[j].RecordID
		}
		return ordered[i].EffectiveAt.Before(ordered[j].EffectiveAt)
	})
	var state engineering.AcceptanceState
	seen := make(map[string]struct{}, len(ordered))
	for _, record := range ordered {
		if record.Key != key || record.RecordID == "" || record.Actor == "" || !record.State.IsValid() || record.EffectiveAt.IsZero() {
			return integrityError("acceptance journal contains an invalid record", nil)
		}
		if _, duplicate := seen[record.RecordID]; duplicate {
			return integrityError("acceptance journal contains a duplicate identity", nil)
		}
		seen[record.RecordID] = struct{}{}
		if !engineering.ValidTransition(state, record.State) {
			return integrityError("acceptance journal contains an invalid transition", nil)
		}
		state = record.State
	}
	return nil
}

func semanticAcceptanceMember(key engineering.RevisionKey, journal []engineering.RevisionAcceptanceRecord) (engineering.RevisionAcceptanceRecord, error) {
	if err := validateAcceptanceHistory(key, journal); err != nil {
		return engineering.RevisionAcceptanceRecord{}, err
	}
	var member engineering.RevisionAcceptanceRecord
	count := 0
	for _, record := range journal {
		if record.State == engineering.AcceptanceStateAccepted {
			member = record
			count++
		}
	}
	if count != 1 {
		return engineering.RevisionAcceptanceRecord{}, integrityError("completed aggregate must contain exactly one accepted semantic member", nil)
	}
	return member, nil
}

// validateManagedHistory proves the dense order/journal invariants of one
// managed Artifact.  Requiring an accepted member is the explicit C7/C9
// establishment contract; capability revisions may remain draft by absence.
func validateManagedHistory(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifactID string, family engineering.RevisionFamily, requireMember bool) (int, error) {
	artifact, found, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: artifactID})
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, integrityError("managed revision history has no owning artifact", nil)
	}
	if err := inspectArtifact(inspector, artifact); err != nil {
		return 0, err
	}
	if err := validateNoExecutionEvidenceOwners(ctx, r, inspector, artifactID, "managed artifact is named as produced execution evidence"); err != nil {
		return 0, err
	}
	if err := validateNoStateAssignmentParentOwners(ctx, r, inspector, artifactID, "managed artifact is named as a state-assignment parent"); err != nil {
		return 0, err
	}
	revisions, err := r.Revisions.ListByArtifact(ctx, artifactID)
	if err != nil {
		return 0, err
	}
	orders, err := r.RevisionOrder.ListByArtifact(ctx, artifactID)
	if err != nil {
		return 0, err
	}
	artifactJournal, err := r.RevisionAcceptance.ListByArtifact(ctx, artifactID)
	if err != nil {
		return 0, err
	}
	if len(revisions) == 0 || len(orders) != len(revisions) {
		return 0, integrityError("managed artifact has partial revision-order history", nil)
	}
	orderByKey := make(map[engineering.RevisionKey]engineering.RevisionOrderMetadata, len(orders))
	sequenceSeen := make(map[int]struct{}, len(orders))
	for _, order := range orders {
		if order.Key.ArtifactID != artifactID || order.Sequence < 1 || order.RecordedAt.IsZero() {
			return 0, integrityError("revision order metadata is invalid", nil)
		}
		if _, exists := orderByKey[order.Key]; exists {
			return 0, integrityError("duplicate revision order metadata", nil)
		}
		if _, exists := sequenceSeen[order.Sequence]; exists {
			return 0, integrityError("duplicate revision sequence", nil)
		}
		orderByKey[order.Key] = order
		sequenceSeen[order.Sequence] = struct{}{}
	}
	for sequence := 1; sequence <= len(orders); sequence++ {
		if _, exists := sequenceSeen[sequence]; !exists {
			return 0, integrityError("revision sequence history is not dense", nil)
		}
	}
	revisionByKey := make(map[engineering.RevisionKey]struct{}, len(revisions))
	for _, revision := range revisions {
		revisionByKey[revision.Key] = struct{}{}
	}
	journalByKey := make(map[engineering.RevisionKey]map[string]engineering.RevisionAcceptanceRecord, len(revisions))
	for _, record := range artifactJournal {
		if record.Key.ArtifactID != artifactID {
			return 0, integrityError("artifact acceptance listing contains a foreign record", nil)
		}
		if _, exists := revisionByKey[record.Key]; !exists {
			return 0, integrityError("acceptance journal names a missing revision", nil)
		}
		indexed, found, err := r.RevisionAcceptance.GetByRecordID(ctx, record.RecordID)
		if err != nil {
			return 0, integrityError("acceptance identity is not globally unique", err)
		}
		if !found || indexed != record {
			return 0, integrityError("acceptance identity index disagrees with artifact journal", nil)
		}
		byID := journalByKey[record.Key]
		if byID == nil {
			byID = make(map[string]engineering.RevisionAcceptanceRecord)
			journalByKey[record.Key] = byID
		}
		if _, duplicate := byID[record.RecordID]; duplicate {
			return 0, integrityError("artifact acceptance journal contains a duplicate identity", nil)
		}
		byID[record.RecordID] = record
	}
	stableSubjectKey := ""
	for _, revision := range revisions {
		if revision.Key.ArtifactID != artifactID || revision.RevisionFamily != family {
			return 0, integrityError("managed artifact mixes revision families", nil)
		}
		if err := inspectRevision(inspector, revision); err != nil {
			return 0, err
		}
		if family == engineering.RevisionFamilyRequirement || family == engineering.RevisionFamilyValidationPlan {
			if revision.SubjectKey == "" {
				return 0, integrityError("managed artifact revision has no governed subject", nil)
			}
			if stableSubjectKey == "" {
				stableSubjectKey = revision.SubjectKey
			} else if revision.SubjectKey != stableSubjectKey {
				return 0, integrityError("managed artifact revisions disagree on their stable subject", nil)
			}
		}
		if artifact.ArtifactType != revision.ArtifactType {
			return 0, integrityError("managed artifact type disagrees with its revision family", nil)
		}
		order, exists := orderByKey[revision.Key]
		if !exists {
			return 0, integrityError("revision has no order metadata", nil)
		}
		if !canonicalTimeEqual(order.RecordedAt, revision.RecordedAt) {
			return 0, integrityError("revision and order metadata disagree on recorded time", nil)
		}
		if order.Sequence == 1 && !canonicalTimeEqual(artifact.RecordedAt, revision.RecordedAt) {
			return 0, integrityError("managed artifact and founding revision disagree on recorded time", nil)
		}
		if family == engineering.RevisionFamilyCapability {
			content, found, err := r.StructuredContent.Get(ctx, revision.Key)
			if err != nil {
				return 0, err
			}
			if !found {
				return 0, integrityError("capability revision has no structured content", nil)
			}
			if err := inspector.ValidateCapabilityContent(revision, content); err != nil {
				return 0, integrityError("capability structured content disagrees with its revision", err)
			}
		} else {
			if content, found, err := r.StructuredContent.Get(ctx, revision.Key); err != nil {
				return 0, err
			} else if found || !content.IsZero() {
				return 0, integrityError("non-capability revision has unexpected structured content", nil)
			}
		}
		trace, traceFound, err := r.RequirementTraces.Get(ctx, revision.Key)
		if err != nil {
			return 0, err
		}
		if family == engineering.RevisionFamilyRequirement {
			if !traceFound {
				return 0, integrityError("requirement revision has no criterion trace", nil)
			}
			if err := validateRequirementCriterionTrace(ctx, r, inspector, revision, order, trace); err != nil {
				return 0, err
			}
		} else if traceFound || !trace.IsZero() {
			return 0, integrityError("non-requirement revision has unexpected criterion trace", nil)
		}
		journal, err := r.RevisionAcceptance.ListByRevision(ctx, revision.Key)
		if err != nil {
			return 0, err
		}
		artifactRows := journalByKey[revision.Key]
		if len(journal) != len(artifactRows) {
			return 0, integrityError("acceptance revision and artifact listings disagree", nil)
		}
		for _, record := range journal {
			if indexed, exists := artifactRows[record.RecordID]; !exists || indexed != record {
				return 0, integrityError("acceptance revision and artifact listings disagree", nil)
			}
		}
		if requireMember {
			member, err := semanticAcceptanceMember(revision.Key, journal)
			if err != nil {
				return 0, err
			}
			if err := validateSemanticMemberIdentity(ctx, r, member); err != nil {
				return 0, err
			}
			if family == engineering.RevisionFamilyValidationPlan &&
				(member.Actor != "featureforge:local-user" || member.Reason != "validation plan established" || !canonicalTimeEqual(member.EffectiveAt, revision.RecordedAt)) {
				return 0, integrityError("validation-plan semantic member has invalid fixed representation", nil)
			}
		} else if err := validateAcceptanceHistory(revision.Key, journal); err != nil {
			return 0, err
		}
		if err := validateManagedRevisionReferences(ctx, r, inspector, revision); err != nil {
			return 0, err
		}
	}
	if family == engineering.RevisionFamilyCapability {
		if _, err := capabilityFeatureOwner(ctx, r, artifactID); err != nil {
			return 0, err
		}
	} else if err := validateNoCapabilityFeatureOwners(ctx, r, artifactID, "non-capability artifact is linked from a FeatureCard"); err != nil {
		return 0, err
	}
	return len(revisions), nil
}

// validateRequirementCriterionTrace proves the product-owned trace member of
// one persisted C7 act. It deliberately validates an exact historical source,
// not whether that source remains current after later capability revisions.
func validateRequirementCriterionTrace(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, revision engineering.RevisionEnvelope, order engineering.RevisionOrderMetadata, trace engineering.RequirementCriterionTrace) error {
	if trace.RequirementRevision != revision.Key {
		return integrityError("requirement criterion trace identity disagrees with its revision", nil)
	}
	if !canonicalTimeEqual(trace.RecordedAt, revision.RecordedAt) || !canonicalTimeEqual(trace.RecordedAt, order.RecordedAt) {
		return integrityError("requirement criterion trace disagrees on recorded time", nil)
	}
	kind, subjectArtifactID, _, err := engineering.ParseSubjectKey(revision.SubjectKey)
	if err != nil || kind != engineering.SubjectKindArtifact {
		return integrityError("requirement criterion trace has an invalid requirement subject", err)
	}
	if trace.CapabilityRevision.ArtifactID != subjectArtifactID {
		return integrityError("requirement criterion trace names a capability outside the requirement subject", nil)
	}
	source, found, err := r.Revisions.Get(ctx, trace.CapabilityRevision)
	if err != nil {
		return err
	}
	if !found {
		return integrityError("requirement criterion trace source revision is dangling", nil)
	}
	if err := inspectRevision(inspector, source); err != nil {
		return err
	}
	if source.RevisionFamily != engineering.RevisionFamilyCapability {
		return integrityError("requirement criterion trace source names another revision family", nil)
	}
	if _, err := validateManagedHistory(ctx, r, inspector, source.Key.ArtifactID, engineering.RevisionFamilyCapability, false); err != nil {
		return err
	}
	content, found, err := r.StructuredContent.Get(ctx, source.Key)
	if err != nil {
		return err
	}
	if !found {
		return integrityError("requirement criterion trace source has no structured content", nil)
	}
	if err := inspector.ValidateCapabilityContent(source, content); err != nil {
		return integrityError("requirement criterion trace source content is invalid", err)
	}
	if !capabilityContentHasCriterion(content, trace.AcceptanceCriterionKey) {
		return integrityError("requirement criterion trace names a missing acceptance criterion", nil)
	}
	journal, err := r.RevisionAcceptance.ListByRevision(ctx, source.Key)
	if err != nil {
		return err
	}
	if _, err := semanticAcceptanceMember(source.Key, journal); err != nil {
		return integrityError("requirement criterion trace source was never an accepted capability revision", err)
	}
	return nil
}

func capabilityContentHasCriterion(content engineering.CapabilitySpecificationContent, key string) bool {
	count := 0
	for _, criterion := range content.AcceptanceCriteria() {
		if criterion.Key() == key {
			count++
		}
	}
	return count == 1
}

// validateRequestedManagedSubject is called only after validateManagedHistory
// has proved that an existing Requirement or Validation Plan history is
// complete and internally coherent. Those artifact families are scoped for
// their entire lifetime: later revisions may change content or activities,
// but cannot silently retarget the shared owning Artifact to another
// capability.
func validateRequestedManagedSubject(ctx context.Context, r Repositories, artifactID, requestedSubjectKey, label string) error {
	revisions, err := r.Revisions.ListByArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	if len(revisions) == 0 {
		return integrityError(label+" has no managed revisions", nil)
	}
	for _, revision := range revisions {
		if revision.SubjectKey != requestedSubjectKey {
			return immutableConflict("%s subject is immutable across revisions", label)
		}
	}
	return nil
}

func validateSemanticMemberIdentity(ctx context.Context, r Repositories, member engineering.RevisionAcceptanceRecord) error {
	stored, found, err := r.RevisionAcceptance.GetByRecordID(ctx, member.RecordID)
	if err != nil {
		return integrityError("semantic member identity is not globally unique", err)
	}
	if !found || stored != member {
		return integrityError("semantic member identity lookup disagrees with its journal", nil)
	}
	return nil
}

func validateManagedRevisionReferences(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, revision engineering.RevisionEnvelope) error {
	switch revision.RevisionFamily {
	case engineering.RevisionFamilyCapability:
		return nil
	case engineering.RevisionFamilyRequirement:
		kind, artifactID, _, err := engineering.ParseSubjectKey(revision.SubjectKey)
		if err != nil || kind != engineering.SubjectKindArtifact {
			return integrityError("requirement subject projection is invalid", err)
		}
		return validateCapabilityArtifactByID(ctx, r, inspector, artifactID, "requirement subject")
	case engineering.RevisionFamilyValidationPlan:
		scope, _, _, subjects, requirements, err := inspector.ValidationPlanReferences(revision)
		if err != nil {
			return integrityError("validation-plan references are unreadable", err)
		}
		if err := validateCapabilityArtifactByID(ctx, r, inspector, scope, "validation-plan scope"); err != nil {
			return err
		}
		for _, subject := range subjects {
			stored, found, err := r.Revisions.Get(ctx, subject)
			if err != nil {
				return err
			}
			if !found {
				return integrityError("validation-plan activity subject is dangling", nil)
			}
			if err := inspectRevision(inspector, stored); err != nil {
				return err
			}
			if stored.RevisionFamily != engineering.RevisionFamilyCapability {
				return integrityError("validation-plan activity subject names another revision family", nil)
			}
			if _, err := validateManagedHistory(ctx, r, inspector, subject.ArtifactID, engineering.RevisionFamilyCapability, false); err != nil {
				return err
			}
		}
		for _, requirement := range requirements {
			stored, found, err := r.Revisions.Get(ctx, requirement)
			if err != nil {
				return err
			}
			if !found {
				return integrityError("validation-plan requirement reference is dangling", nil)
			}
			if err := inspectRevision(inspector, stored); err != nil {
				return err
			}
			if stored.RevisionFamily != engineering.RevisionFamilyRequirement {
				return integrityError("validation-plan criterion names another revision family", nil)
			}
			if _, err := validateManagedHistory(ctx, r, inspector, requirement.ArtifactID, engineering.RevisionFamilyRequirement, true); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func validateCapabilityArtifactByID(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifactID, label string) error {
	artifact, found, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: artifactID})
	if err != nil {
		return err
	}
	if !found {
		return integrityError(label+" artifact is dangling", nil)
	}
	if err := inspectArtifact(inspector, artifact); err != nil {
		return err
	}
	if err := inspector.ValidateCapabilityArtifact(artifact); err != nil {
		return integrityError(label+" does not resolve to a capability artifact", err)
	}
	_, err = validateManagedHistory(ctx, r, inspector, artifactID, engineering.RevisionFamilyCapability, false)
	return err
}

func validateStateAssignmentAct(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, assignment engineering.RecordEnvelope) error {
	return validateStateAssignmentActFrom(ctx, r, inspector, assignment, make(map[engineering.RecordKey]struct{}))
}

func validateStateAssignmentRoot(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, assignment engineering.RecordEnvelope) error {
	if err := validateStateAssignmentAct(ctx, r, inspector, assignment); err != nil {
		return err
	}
	parent, err := inspector.StateAssignmentEstablishedBy(assignment)
	if err != nil {
		return integrityError("state assignment parent projection is unreadable", err)
	}
	artifact, found, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: parent.ArtifactID})
	if err != nil {
		return err
	}
	if !found {
		return integrityError("state assignment parent has no transition artifact", nil)
	}
	if err := inspectArtifact(inspector, artifact); err != nil {
		return err
	}
	return validateForeignArtifactOccupancy(ctx, r, inspector, artifact)
}

func validateStateAssignmentActFrom(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, assignment engineering.RecordEnvelope, visiting map[engineering.RecordKey]struct{}) error {
	if _, cycle := visiting[assignment.Key]; cycle {
		return integrityError("lifecycle predecessor chain contains a cycle", nil)
	}
	visiting[assignment.Key] = struct{}{}
	defer delete(visiting, assignment.Key)
	if err := inspectRecord(inspector, assignment); err != nil {
		return err
	}
	parentKey, err := inspector.StateAssignmentEstablishedBy(assignment)
	if err != nil {
		return integrityError("state assignment parent projection is unreadable", err)
	}
	parent, found, err := r.Revisions.Get(ctx, parentKey)
	if err != nil {
		return err
	}
	if !found {
		return integrityError("state assignment parent transition is dangling", nil)
	}
	if err := inspectRevision(inspector, parent); err != nil {
		return err
	}
	if parent.RevisionFamily != engineering.RevisionFamilyTransitionRecord {
		return integrityError("state assignment parent is not a transition revision", nil)
	}
	if err := validateUnmanagedRevisionMetadata(ctx, r, parent.Key); err != nil {
		return err
	}
	artifact, found, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: parentKey.ArtifactID})
	if err != nil {
		return err
	}
	if !found {
		return integrityError("state assignment parent has no owning artifact", nil)
	}
	if err := inspectArtifact(inspector, artifact); err != nil {
		return err
	}
	if artifact.ArtifactType != parent.ArtifactType {
		return integrityError("transition artifact and revision types disagree", nil)
	}
	if parent.SubjectKey != assignment.SubjectKey {
		return integrityError("transition revision and assignment name different lifecycle subjects", nil)
	}
	assignments, err := listValidatedRecordsByKind(ctx, r, inspector, engineering.RecordKindStateAssignment)
	if err != nil {
		return err
	}
	ownerCount := 0
	for _, candidate := range assignments {
		if err := inspectRecord(inspector, candidate); err != nil {
			return err
		}
		candidateParent, err := inspector.StateAssignmentEstablishedBy(candidate)
		if err != nil {
			return integrityError("state assignment ownership is unreadable", err)
		}
		if candidateParent == parentKey {
			ownerCount++
		}
	}
	if ownerCount != 1 {
		return integrityError("transition revision must establish exactly one state assignment", nil)
	}
	resultingID, hasResulting, err := inspector.TransitionResultingAssignment(parent)
	if err != nil {
		return integrityError("transition resulting assignment is unreadable", err)
	}
	if hasResulting && resultingID != assignment.Key.ID {
		return integrityError("transition names a different resulting assignment", nil)
	}
	targetState, hasTargetState, err := inspector.TransitionTargetState(parent)
	if err != nil {
		return integrityError("transition target state is unreadable", err)
	}
	if hasTargetState != hasResulting {
		return integrityError("transition target/resulting-assignment shape is contradictory", nil)
	}
	if hasTargetState && targetState != assignment.StateID {
		return integrityError("transition target and resulting assignment state disagree", nil)
	}
	if !hasResulting && assignment.StateID != "featureforge:drafting" {
		return integrityError("entry transition establishes a non-drafting assignment", nil)
	}
	if !canonicalTimeEqual(parent.RecordedAt, assignment.RecordedAt) {
		return integrityError("transition revision and assignment disagree on recorded time", nil)
	}
	predecessorID, hasPredecessor, err := inspector.TransitionPredecessor(parent)
	if err != nil {
		return integrityError("transition predecessor is unreadable", err)
	}
	if hasPredecessor != hasResulting {
		return integrityError("transition/assignment act has contradictory entry or resulting shape", nil)
	}
	if hasPredecessor {
		predecessorKey, err := engineering.NewRecordKey(engineering.RecordKindStateAssignment, predecessorID)
		if err != nil {
			return integrityError("transition predecessor identity is invalid", err)
		}
		predecessor, found, err := r.Records.Get(ctx, predecessorKey)
		if err != nil {
			return err
		}
		if !found {
			return integrityError("transition predecessor is dangling", nil)
		}
		if predecessor.SubjectKey != assignment.SubjectKey {
			return integrityError("transition predecessor belongs to another lifecycle subject", nil)
		}
		if err := validateStateAssignmentActFrom(ctx, r, inspector, predecessor, visiting); err != nil {
			return err
		}
		predecessorParent, err := inspector.StateAssignmentEstablishedBy(predecessor)
		if err != nil {
			return integrityError("transition predecessor parent is unreadable", err)
		}
		if predecessorParent.ArtifactID != parentKey.ArtifactID {
			return integrityError("lifecycle predecessor belongs to another transition-record root", nil)
		}
	}
	kind, subjectArtifactID, _, err := engineering.ParseSubjectKey(assignment.SubjectKey)
	if err != nil || kind != engineering.SubjectKindArtifact {
		return integrityError("state assignment subject projection is invalid", err)
	}
	return validateCapabilityArtifactByID(ctx, r, inspector, subjectArtifactID, "state assignment subject")
}

// validateManagedForeignOccupant proves that a pair occupied by another
// FeatureForge-managed revision family is a complete coherent act before the
// caller classifies it as an immutable conflict. A merely readable envelope is
// not enough: missing content, order, or semantic acceptance membership is
// stored-state corruption.
func validateManagedForeignOccupant(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifact engineering.ArtifactEnvelope, revision engineering.RevisionEnvelope, orderFound bool) error {
	if artifact.ArtifactType != revision.ArtifactType {
		return integrityError("foreign artifact and revision families disagree", nil)
	}
	requireMember := false
	switch revision.RevisionFamily {
	case engineering.RevisionFamilyCapability:
	case engineering.RevisionFamilyRequirement, engineering.RevisionFamilyValidationPlan:
		requireMember = true
	case engineering.RevisionFamilyEvidence, engineering.RevisionFamilyTransitionRecord:
		if orderFound {
			return integrityError("foreign unmanaged revision unexpectedly has order metadata", nil)
		}
		return validateForeignArtifactOccupancy(ctx, r, inspector, artifact)
	default:
		return integrityError("foreign revision occupancy is not a complete managed revision act", nil)
	}
	if !orderFound {
		return integrityError("foreign managed revision has no order metadata", nil)
	}
	if _, err := validateManagedHistory(ctx, r, inspector, revision.Key.ArtifactID, revision.RevisionFamily, requireMember); err != nil {
		return err
	}
	if revision.RevisionFamily == engineering.RevisionFamilyCapability {
		_, err := capabilityFeatureOwner(ctx, r, revision.Key.ArtifactID)
		return err
	}
	return nil
}

func validateForeignArtifactOccupancy(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifact engineering.ArtifactEnvelope) error {
	family, err := inspector.ArtifactFamily(artifact)
	if err != nil {
		return integrityError("foreign artifact family is unreadable", err)
	}
	if family != engineering.RevisionFamilyCapability {
		if err := validateNoCapabilityFeatureOwners(ctx, r, artifact.Key.ArtifactID, "non-capability artifact is linked from a FeatureCard"); err != nil {
			return err
		}
	}
	switch family {
	case engineering.RevisionFamilyCapability:
		if _, err = validateManagedHistory(ctx, r, inspector, artifact.Key.ArtifactID, family, false); err != nil {
			return err
		}
		_, err = capabilityFeatureOwner(ctx, r, artifact.Key.ArtifactID)
		return err
	case engineering.RevisionFamilyRequirement, engineering.RevisionFamilyValidationPlan:
		_, err = validateManagedHistory(ctx, r, inspector, artifact.Key.ArtifactID, family, true)
		return err
	case engineering.RevisionFamilyEvidence:
		if err := validateNoStateAssignmentParentOwners(ctx, r, inspector, artifact.Key.ArtifactID, "evidence artifact is named as a state-assignment parent"); err != nil {
			return err
		}
		revisions, lookupErr := r.Revisions.ListByArtifact(ctx, artifact.Key.ArtifactID)
		if lookupErr != nil {
			return lookupErr
		}
		if len(revisions) != 1 {
			return integrityError("foreign evidence artifact must have exactly one revision", nil)
		}
		_, err = validateEvidencePairOccupancy(ctx, r, inspector, artifact, revisions[0])
		return err
	case engineering.RevisionFamilyTransitionRecord:
		if err := validateNoExecutionEvidenceOwners(ctx, r, inspector, artifact.Key.ArtifactID, "transition artifact is named as produced execution evidence"); err != nil {
			return err
		}
		revisions, lookupErr := r.Revisions.ListByArtifact(ctx, artifact.Key.ArtifactID)
		if lookupErr != nil {
			return lookupErr
		}
		if len(revisions) == 0 {
			return integrityError("foreign transition artifact has no revisions", nil)
		}
		assignments, lookupErr := listValidatedRecordsByKind(ctx, r, inspector, engineering.RecordKindStateAssignment)
		if lookupErr != nil {
			return lookupErr
		}
		revisionSet := make(map[engineering.RevisionKey]struct{}, len(revisions))
		rootSubject := ""
		entryCount := 0
		for _, revision := range revisions {
			if err := inspectRevision(inspector, revision); err != nil {
				return err
			}
			if revision.RevisionFamily != family || artifact.ArtifactType != revision.ArtifactType {
				return integrityError("foreign transition artifact/revision act is contradictory", nil)
			}
			if rootSubject == "" {
				rootSubject = revision.SubjectKey
			} else if revision.SubjectKey != rootSubject {
				return integrityError("transition-record root mixes lifecycle subjects", nil)
			}
			_, hasPredecessor, inspectErr := inspector.TransitionPredecessor(revision)
			if inspectErr != nil {
				return integrityError("transition-record predecessor shape is unreadable", inspectErr)
			}
			if !hasPredecessor {
				entryCount++
				if !canonicalTimeEqual(artifact.RecordedAt, revision.RecordedAt) {
					return integrityError("transition-record artifact and entry revision disagree on recorded time", nil)
				}
			}
			revisionSet[revision.Key] = struct{}{}
		}
		if entryCount != 1 {
			return integrityError("transition-record root must contain exactly one entry revision", nil)
		}
		owners := make(map[engineering.RevisionKey][]engineering.RecordEnvelope, len(revisions))
		for _, assignment := range assignments {
			parent, parentErr := inspector.StateAssignmentEstablishedBy(assignment)
			if parentErr != nil {
				return integrityError("foreign transition ownership is unreadable", parentErr)
			}
			if parent.ArtifactID != artifact.Key.ArtifactID {
				continue
			}
			if _, found := revisionSet[parent]; !found {
				return integrityError("transition artifact has an assignment for a missing revision", nil)
			}
			owners[parent] = append(owners[parent], assignment)
		}
		for _, revision := range revisions {
			if len(owners[revision.Key]) != 1 {
				return integrityError("foreign transition revision must have exactly one assignment", nil)
			}
			if err := validateStateAssignmentAct(ctx, r, inspector, owners[revision.Key][0]); err != nil {
				return err
			}
		}
		return nil
	default:
		return integrityError("foreign artifact family is not governed", nil)
	}
}

func immutableConflict(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrImmutableValueConflict, fmt.Sprintf(format, args...))
}

func validateUnmanagedRevisionMetadata(ctx context.Context, r Repositories, key engineering.RevisionKey) error {
	if content, found, err := r.StructuredContent.Get(ctx, key); err != nil {
		return err
	} else if found || !content.IsZero() {
		return integrityError("unmanaged revision has unexpected structured content", nil)
	}
	if order, found, err := r.RevisionOrder.Get(ctx, key); err != nil {
		return err
	} else if found || !order.Key.IsZero() {
		return integrityError("unmanaged revision has unexpected order metadata", nil)
	}
	journal, err := r.RevisionAcceptance.ListByRevision(ctx, key)
	if err != nil {
		return err
	}
	if len(journal) != 0 {
		return integrityError("unmanaged revision has an unexpected acceptance journal", nil)
	}
	if trace, found, err := r.RequirementTraces.Get(ctx, key); err != nil {
		return err
	} else if found || !trace.IsZero() {
		return integrityError("unmanaged revision has an unexpected requirement criterion trace", nil)
	}
	return nil
}

// validateAbsentArtifactHistory proves that an absent Artifact identity is
// genuinely vacant across every root-indexed engineering collection. Looking
// only at the requested RevisionKey is insufficient: a dangling sibling
// revision, order row, or acceptance entry could otherwise be hidden by a
// newly-created Artifact with the same identity.
func validateAbsentArtifactHistory(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifactID string) error {
	revisions, err := r.Revisions.ListByArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	orders, err := r.RevisionOrder.ListByArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	journal, err := r.RevisionAcceptance.ListByArtifact(ctx, artifactID)
	if err != nil {
		return err
	}
	if len(revisions) != 0 || len(orders) != 0 || len(journal) != 0 {
		return integrityError("absent artifact identity has dangling revision history", nil)
	}
	if err := validateAbsentCapabilityOwner(ctx, r, artifactID); err != nil {
		return err
	}
	if err := validateNoExecutionEvidenceOwners(ctx, r, inspector, artifactID, "absent artifact has a dangling execution-evidence owner"); err != nil {
		return err
	}
	if err := validateNoStateAssignmentParentOwners(ctx, r, inspector, artifactID, "absent artifact has a dangling state-assignment owner"); err != nil {
		return err
	}
	return nil
}

func validateNoExecutionEvidenceOwners(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifactID, reason string) error {
	owners, err := executionEvidenceOwnersByArtifact(ctx, r, inspector, artifactID)
	if err != nil {
		return err
	}
	if len(owners) != 0 {
		return integrityError(reason, nil)
	}
	return nil
}

func stateAssignmentParentOwnersByArtifact(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifactID string) ([]engineering.RecordEnvelope, error) {
	assignments, err := listValidatedRecordsByKind(ctx, r, inspector, engineering.RecordKindStateAssignment)
	if err != nil {
		return nil, err
	}
	owners := make([]engineering.RecordEnvelope, 0, 1)
	for _, assignment := range assignments {
		if err := inspectRecord(inspector, assignment); err != nil {
			return nil, err
		}
		parent, err := inspector.StateAssignmentEstablishedBy(assignment)
		if err != nil {
			return nil, integrityError("state-assignment parent projection is unreadable", err)
		}
		if parent.ArtifactID == artifactID {
			owners = append(owners, assignment)
		}
	}
	return owners, nil
}

func validateNoStateAssignmentParentOwners(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifactID, reason string) error {
	owners, err := stateAssignmentParentOwnersByArtifact(ctx, r, inspector, artifactID)
	if err != nil {
		return err
	}
	if len(owners) != 0 {
		return integrityError(reason, nil)
	}
	return nil
}

func validateAcceptanceCandidate(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, record engineering.RevisionAcceptanceRecord) error {
	revision, found, err := r.Revisions.Get(ctx, record.Key)
	if err != nil {
		return integrityError("candidate acceptance parent lookup", err)
	}
	if !found {
		return integrityError("candidate acceptance is dangling", nil)
	}
	if err := inspectRevision(inspector, revision); err != nil {
		return err
	}
	artifact, found, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: record.Key.ArtifactID})
	if err != nil {
		return integrityError("candidate acceptance owner lookup", err)
	}
	if !found {
		return integrityError("candidate acceptance has no owning artifact", nil)
	}
	if err := inspectArtifact(inspector, artifact); err != nil {
		return err
	}
	order, found, err := r.RevisionOrder.Get(ctx, record.Key)
	if err != nil {
		return integrityError("candidate acceptance order lookup", err)
	}
	if !found || order.Key != record.Key || order.Sequence < 1 {
		return integrityError("candidate acceptance has no coherent order metadata", nil)
	}
	journal, err := r.RevisionAcceptance.ListByRevision(ctx, record.Key)
	if err != nil {
		return integrityError("candidate acceptance journal lookup", err)
	}
	foundRecord := false
	for _, item := range journal {
		if item.RecordID == record.RecordID {
			foundRecord = item == record
		}
	}
	if !foundRecord {
		return integrityError("candidate acceptance lookup disagrees with its journal", nil)
	}
	if err := validateAcceptanceHistory(record.Key, journal); err != nil {
		return err
	}
	requireMember := false
	switch revision.RevisionFamily {
	case engineering.RevisionFamilyCapability:
	case engineering.RevisionFamilyRequirement, engineering.RevisionFamilyValidationPlan:
		requireMember = true
	default:
		return integrityError("acceptance candidate belongs to an unmanaged revision family", nil)
	}
	_, err = validateManagedHistory(ctx, r, inspector, record.Key.ArtifactID, revision.RevisionFamily, requireMember)
	return err
}

func validateRevisionReference(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, key engineering.RevisionKey, storedAct bool, label string) error {
	revision, found, err := r.Revisions.Get(ctx, key)
	if err != nil {
		return err
	}
	if !found {
		if storedAct {
			return integrityError(label+" reference is dangling", nil)
		}
		return fmt.Errorf("%w: %s revision %s", ErrReferencedValueMissing, label, key)
	}
	return inspectRevision(inspector, revision)
}

func validateRecordReference(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, key engineering.RecordKey, storedAct bool, label string) error {
	record, found, err := r.Records.Get(ctx, key)
	if err != nil {
		return err
	}
	if !found {
		if storedAct {
			return integrityError(label+" reference is dangling", nil)
		}
		return fmt.Errorf("%w: %s record %s", ErrReferencedValueMissing, label, key)
	}
	return inspectRecord(inspector, record)
}

func revisionKeyFromSubject(subjectKey string) (engineering.RevisionKey, error) {
	kind, artifactID, revisionID, err := engineering.ParseSubjectKey(subjectKey)
	if err != nil || kind != engineering.SubjectKindArtifactRevision {
		return engineering.RevisionKey{}, fmt.Errorf("subject does not name an artifact revision")
	}
	return engineering.NewRevisionKey(artifactID, revisionID)
}

func requirementKeyFromCriterion(value string) (engineering.RevisionKey, error) {
	const prefix = "requirement-revision:"
	rest, ok := strings.CutPrefix(value, prefix)
	if !ok {
		return engineering.RevisionKey{}, fmt.Errorf("criterion is not a requirement revision")
	}
	artifactID, revisionID, ok := strings.Cut(rest, "/")
	if !ok {
		return engineering.RevisionKey{}, fmt.Errorf("criterion requirement key is malformed")
	}
	return engineering.NewRevisionKey(artifactID, revisionID)
}

func validateStoredClaimReferences(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, claim engineering.RecordEnvelope) error {
	if len(claim.CriterionKeys) != 1 || len(claim.EvidenceKeys) != 1 || len(claim.ExecutionKeys) != 1 {
		return integrityError("claim must project exactly one criterion, evidence, and execution reference", nil)
	}
	subject, err := revisionKeyFromSubject(claim.SubjectKey)
	if err != nil {
		return integrityError("claim subject projection", err)
	}
	requirement, err := requirementKeyFromCriterion(claim.CriterionKeys[0])
	if err != nil {
		return integrityError("claim criterion projection", err)
	}
	evidenceArtifactID, evidenceRevisionID, err := engineering.ParseEvidenceKey(claim.EvidenceKeys[0])
	if err != nil {
		return integrityError("claim evidence projection", err)
	}
	if err := validateStoredCapabilityScope(ctx, r, inspector, claim.Scope, "claim scope"); err != nil {
		return err
	}
	if err := validateClaimRevisionReference(ctx, r, inspector, subject, engineering.RevisionFamilyCapability, true, "claim subject"); err != nil {
		return err
	}
	if err := validateClaimRevisionReference(ctx, r, inspector, requirement, engineering.RevisionFamilyRequirement, true, "claim requirement"); err != nil {
		return err
	}
	if _, err := validateManagedHistory(ctx, r, inspector, requirement.ArtifactID, engineering.RevisionFamilyRequirement, true); err != nil {
		return err
	}
	evidenceKey := engineering.RevisionKey{ArtifactID: evidenceArtifactID, RevisionID: evidenceRevisionID}
	executionID, ok := strings.CutPrefix(claim.ExecutionKeys[0], "execution:")
	if !ok || executionID == "" {
		return integrityError("claim execution projection is malformed", nil)
	}
	executionKey, _ := engineering.NewRecordKey(engineering.RecordKindExecution, executionID)
	execution, found, err := r.Records.Get(ctx, executionKey)
	if err != nil {
		return err
	}
	if !found {
		return integrityError("claim execution reference is dangling", nil)
	}
	_, _, producedEvidence, err := validateStoredExecutionAct(ctx, r, inspector, execution)
	if err != nil {
		return err
	}
	if producedEvidence != evidenceKey || execution.SubjectKey != claim.SubjectKey {
		return integrityError("claim references disagree with their execution act", nil)
	}
	activity, err := storedExecutionPlanActivityReference(ctx, r, inspector, execution)
	if err != nil {
		return err
	}
	if activity.requirement != requirement || claim.Scope != "featureforge:capability|"+activity.scopeArtifactID {
		return integrityError("claim criterion or scope disagrees with its executed plan activity", nil)
	}
	_, _, executionMethod, err := inspector.ExecutionPlanActivity(execution)
	if err != nil {
		return integrityError("claim execution method is unreadable", err)
	}
	claimMethod, err := inspector.ClaimMethod(claim)
	if err != nil {
		return integrityError("claim method is unreadable", err)
	}
	if executionMethod != claimMethod {
		return integrityError("claim method disagrees with its execution", nil)
	}
	artifact, artifactFound, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: evidenceKey.ArtifactID})
	if err != nil {
		return err
	}
	revision, revisionFound, err := r.Revisions.Get(ctx, evidenceKey)
	if err != nil {
		return err
	}
	occupancy, err := classifyEvidenceOccupancy(ctx, r, inspector, evidenceKey, artifact, artifactFound, revision, revisionFound)
	if err != nil {
		return err
	}
	if occupancy != evidenceOccupancyComplete {
		return integrityError("claim evidence does not resolve to a complete validation-run act", nil)
	}
	if err := inspectRecord(inspector, execution); err != nil {
		return err
	}
	if claim.HasCorrection() {
		targetKey, err := engineering.NewRecordKey(engineering.RecordKindClaim, claim.CorrectionTargetID)
		if err != nil {
			return integrityError("claim correction projection", err)
		}
		if err := validateRecordReference(ctx, r, inspector, targetKey, true, "claim correction target"); err != nil {
			return err
		}
	}
	return nil
}

func validateClaimInputReferences(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, in engineering.ClaimInput) error {
	var ordinaryDependencyError error
	deferOrdinary := func(err error) error {
		if err == nil {
			return nil
		}
		if errors.Is(err, ErrReferencedValueMissing) {
			if ordinaryDependencyError == nil {
				ordinaryDependencyError = err
			}
			return nil
		}
		return err
	}
	if err := deferOrdinary(validateCapabilityArtifactInput(ctx, r, inspector, in.ScopeArtifactID, "claim scope")); err != nil {
		return err
	}
	subject := engineering.RevisionKey{ArtifactID: in.SubjectArtifactID, RevisionID: in.SubjectRevisionID}
	if err := deferOrdinary(validateClaimRevisionReference(ctx, r, inspector, subject, engineering.RevisionFamilyCapability, false, "claim subject")); err != nil {
		return err
	}
	requirement := engineering.RevisionKey{ArtifactID: in.RequirementArtifactID, RevisionID: in.RequirementRevisionID}
	requirementErr := validateClaimRevisionReference(ctx, r, inspector, requirement, engineering.RevisionFamilyRequirement, false, "claim requirement")
	if err := deferOrdinary(requirementErr); err != nil {
		return err
	}
	if requirementErr == nil {
		if _, err := validateManagedHistory(ctx, r, inspector, requirement.ArtifactID, engineering.RevisionFamilyRequirement, true); err != nil {
			return err
		}
	}
	executionKey, err := engineering.NewRecordKey(engineering.RecordKindExecution, in.ExecutionID)
	if err != nil {
		return invalidCommand(err)
	}
	execution, found, err := r.Records.Get(ctx, executionKey)
	if err != nil {
		return err
	}
	if !found {
		if ordinaryDependencyError == nil {
			ordinaryDependencyError = fmt.Errorf("%w: claim execution %s", ErrReferencedValueMissing, executionKey)
		}
		return ordinaryDependencyError
	}
	_, _, producedEvidence, err := validateStoredExecutionAct(ctx, r, inspector, execution)
	if err != nil {
		return err
	}
	wantEvidence := engineering.RevisionKey{ArtifactID: in.EvidenceArtifactID, RevisionID: in.EvidenceRevisionID}
	if producedEvidence != wantEvidence || execution.SubjectKey != engineering.ArtifactRevisionSubjectKey(in.SubjectArtifactID, in.SubjectRevisionID) {
		if ordinaryDependencyError == nil {
			ordinaryDependencyError = fmt.Errorf("%w: claim references disagree with their execution act", ErrReferencedValueMissing)
		}
	}
	activity, err := storedExecutionPlanActivityReference(ctx, r, inspector, execution)
	if err != nil {
		return err
	}
	if activity.requirement != requirement || activity.scopeArtifactID != in.ScopeArtifactID {
		if ordinaryDependencyError == nil {
			ordinaryDependencyError = fmt.Errorf("%w: claim criterion or scope disagrees with its executed plan activity", ErrReferencedValueMissing)
		}
	}
	_, _, executionMethod, err := inspector.ExecutionPlanActivity(execution)
	if err != nil {
		return integrityError("claim execution method is unreadable", err)
	}
	if executionMethod != in.Method {
		if ordinaryDependencyError == nil {
			ordinaryDependencyError = fmt.Errorf("%w: claim method differs from its execution", ErrReferencedValueMissing)
		}
	}
	return ordinaryDependencyError
}

func storedExecutionPlanActivityReference(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, execution engineering.RecordEnvelope) (planActivityReference, error) {
	planKey, activityKey, method, err := inspector.ExecutionPlanActivity(execution)
	if err != nil {
		return planActivityReference{}, integrityError("execution plan activity is unreadable", err)
	}
	plan, found, err := r.Revisions.Get(ctx, planKey)
	if err != nil {
		return planActivityReference{}, err
	}
	if !found {
		return planActivityReference{}, integrityError("execution plan reference is dangling", nil)
	}
	if err := inspectRevision(inspector, plan); err != nil {
		return planActivityReference{}, err
	}
	if plan.RevisionFamily != engineering.RevisionFamilyValidationPlan {
		return planActivityReference{}, integrityError("execution plan reference names another revision family", nil)
	}
	return validatePlanActivityReference(ctx, r, inspector, plan, activityKey, method, mustRevisionSubjectKey(execution.SubjectKey), true)
}

func validateClaimRevisionReference(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, key engineering.RevisionKey, family engineering.RevisionFamily, storedAct bool, label string) error {
	revision, found, err := r.Revisions.Get(ctx, key)
	if err != nil {
		return err
	}
	if !found {
		if storedAct {
			return integrityError(label+" reference is dangling", nil)
		}
		return fmt.Errorf("%w: %s revision %s", ErrReferencedValueMissing, label, key)
	}
	if err := inspectRevision(inspector, revision); err != nil {
		return err
	}
	if revision.RevisionFamily != family {
		if storedAct {
			return integrityError(label+" names another revision family", nil)
		}
		return fmt.Errorf("%w: %s names another revision family", ErrReferencedValueMissing, label)
	}
	return nil
}

func validateStoredCapabilityScope(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, scope, label string) error {
	const prefix = "featureforge:capability|"
	artifactID, ok := strings.CutPrefix(scope, prefix)
	if !ok || artifactID == "" {
		return integrityError(label+" projection is invalid", nil)
	}
	return validateCapabilityArtifactByID(ctx, r, inspector, artifactID, label)
}

func validateCapabilityArtifactInput(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, artifactID, label string) error {
	artifact, found, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: artifactID})
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: %s artifact %s", ErrReferencedValueMissing, label, artifactID)
	}
	if err := inspectArtifact(inspector, artifact); err != nil {
		return err
	}
	if err := inspector.ValidateCapabilityArtifact(artifact); err != nil {
		return fmt.Errorf("%w: %s is not a capability artifact", ErrReferencedValueMissing, label)
	}
	_, err = validateManagedHistory(ctx, r, inspector, artifactID, engineering.RevisionFamilyCapability, false)
	return err
}

func validateClaimChainIntegrity(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, claim engineering.RecordEnvelope) error {
	// Command-integrity validation cannot trust the subject projection as an
	// index predicate before the stored envelope has been inspected. A corrupt
	// correction whose payload belongs to this chain but whose SubjectKey was
	// shifted would otherwise be omitted and could let another correction be
	// written beside an invalid authoritative chain.
	claims, err := listValidatedRecordsByKind(ctx, r, inspector, engineering.RecordKindClaim)
	if err != nil {
		return err
	}
	for _, candidate := range claims {
		if err := inspectRecord(inspector, candidate); err != nil {
			return err
		}
		if candidate.SubjectKey != claim.SubjectKey || candidate.Scope != claim.Scope || !sameCriteria(candidate.CriterionKeys, claim.CriterionKeys) {
			continue
		}
		if err := validateStoredClaimReferences(ctx, r, inspector, candidate); err != nil {
			return err
		}
	}
	if _, err := ResolveCurrentClaim(ctx, r, inspector, claim.SubjectKey, claim.Scope, claim.CriterionKeys); err != nil && !errors.Is(err, ErrCorrectionAmbiguous) {
		return integrityError("claim correction chain is invalid", err)
	}
	return nil
}

func validateStoredDecisionReferences(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, decision engineering.RecordEnvelope) error {
	if len(decision.EvidenceKeys) != 1 {
		return integrityError("decision must project exactly one evidence revision", nil)
	}
	subjectKind, artifactID, revisionID, err := engineering.ParseSubjectKey(decision.SubjectKey)
	if err != nil {
		return integrityError("decision subject projection", err)
	}
	if _, _, err := engineering.ParseEvidenceKey(decision.EvidenceKeys[0]); err != nil {
		return integrityError("decision evidence projection", err)
	}
	if subjectKind == engineering.SubjectKindArtifact {
		artifact, found, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: artifactID})
		if err != nil {
			return err
		}
		if !found {
			return integrityError("decision Artifact subject reference is dangling", nil)
		}
		if err := inspectArtifact(inspector, artifact); err != nil {
			return err
		}
		if err := inspector.ValidateCapabilityArtifact(artifact); err != nil {
			return integrityError("decision Artifact subject names another family", err)
		}
		if _, err := validateManagedHistory(ctx, r, inspector, artifactID, engineering.RevisionFamilyCapability, false); err != nil {
			return err
		}
		return nil
	}
	if subjectKind != engineering.SubjectKindArtifactRevision {
		return integrityError("decision subject projection uses an unsupported kind", nil)
	}
	subject := engineering.RevisionKey{ArtifactID: artifactID, RevisionID: revisionID}
	stored, found, err := r.Revisions.Get(ctx, subject)
	if err != nil {
		return err
	}
	if !found {
		return integrityError("decision subject reference is dangling", nil)
	}
	if err := inspectRevision(inspector, stored); err != nil {
		return err
	}
	if stored.RevisionFamily != engineering.RevisionFamilyCapability {
		return integrityError("decision subject names another revision family", nil)
	}
	if _, err := validateManagedHistory(ctx, r, inspector, subject.ArtifactID, engineering.RevisionFamilyCapability, false); err != nil {
		return err
	}
	return nil
}

func validateDecisionInputReferences(ctx context.Context, r Repositories, inspector EngineeringReplayInspector, in engineering.DecisionInput) error {
	if in.SubjectRevisionID == "" {
		artifact, found, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: in.SubjectArtifactID})
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: decision subject artifact %s", ErrReferencedValueMissing, in.SubjectArtifactID)
		}
		if err := inspectArtifact(inspector, artifact); err != nil {
			return err
		}
		if err := inspector.ValidateCapabilityArtifact(artifact); err != nil {
			return fmt.Errorf("%w: decision subject is not a capability artifact", ErrReferencedValueMissing)
		}
		_, err = validateManagedHistory(ctx, r, inspector, in.SubjectArtifactID, engineering.RevisionFamilyCapability, false)
		return err
	}
	subject := engineering.RevisionKey{ArtifactID: in.SubjectArtifactID, RevisionID: in.SubjectRevisionID}
	stored, found, err := r.Revisions.Get(ctx, subject)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: decision subject revision %s", ErrReferencedValueMissing, subject)
	}
	if err := inspectRevision(inspector, stored); err != nil {
		return err
	}
	if stored.RevisionFamily != engineering.RevisionFamilyCapability {
		return fmt.Errorf("%w: decision subject is not a capability revision", ErrReferencedValueMissing)
	}
	if _, err := validateManagedHistory(ctx, r, inspector, subject.ArtifactID, engineering.RevisionFamilyCapability, false); err != nil {
		return err
	}
	return nil
}

func validateCapabilityArtifactReference(ctx context.Context, r Repositories, recorder EngineeringRecorder, inspector EngineeringReplayInspector, artifactID, label string, storedAct bool) error {
	artifact, found, err := r.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: artifactID})
	if err != nil {
		return err
	}
	if !found {
		if storedAct {
			return integrityError(label+" artifact reference is dangling", nil)
		}
		return fmt.Errorf("%w: %s artifact %s", ErrReferencedValueMissing, label, artifactID)
	}
	if err := inspectArtifact(inspector, artifact); err != nil {
		return err
	}
	expected, err := recorder.RecordCapabilityArtifact(artifactID, artifact.RecordedAt)
	if err != nil {
		return invalidCommand(err)
	}
	if !artifact.Equal(expected) {
		if storedAct {
			return integrityError(label+" does not reference a capability artifact", nil)
		}
		return fmt.Errorf("%w: %s does not reference a capability artifact", ErrReferencedValueMissing, label)
	}
	_, err = validateManagedHistory(ctx, r, inspector, artifactID, engineering.RevisionFamilyCapability, false)
	return err
}
