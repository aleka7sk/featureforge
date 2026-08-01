package application

import (
	"context"
	"fmt"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// LifecycleHistoryAssignment joins one validated assignment to the exact
// Transition Record Revision that established it.
type LifecycleHistoryAssignment struct {
	Assignment       engineering.RecordEnvelope
	Detail           engineering.LifecycleAssignmentDetail
	Transition       engineering.RevisionEnvelope
	TransitionDetail engineering.LifecycleTransitionDetail
}

// LifecycleHistory is the one validated causal chain for a capability.
type LifecycleHistory struct {
	Found          bool
	Policy         engineering.LifecyclePolicy
	RootArtifactID string
	Assignments    []LifecycleHistoryAssignment
	Head           LifecycleHistoryAssignment
}

// ResolveLifecycleHistory validates persisted configuration and every
// lifecycle member observable for one capability. Current state is the unique
// head of the predecessor chain, never a timestamp maximum.
func ResolveLifecycleHistory(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector, capabilityArtifactID string) (LifecycleHistory, error) {
	policy, err := loadLifecyclePolicy(ctx, repos, inspector)
	if err != nil {
		return LifecycleHistory{}, err
	}
	subjectKey := engineering.ArtifactSubjectKey(capabilityArtifactID)
	allRecords, err := listValidatedRecords(ctx, repos, inspector)
	if err != nil {
		return LifecycleHistory{}, err
	}
	allRevisions, err := listValidatedRevisions(ctx, repos, inspector)
	if err != nil {
		return LifecycleHistory{}, err
	}
	allAssignments := make([]engineering.RecordEnvelope, 0)
	for _, record := range allRecords {
		if record.Kind == engineering.RecordKindStateAssignment {
			allAssignments = append(allAssignments, record)
		}
	}
	allTransitionRevisions := make([]engineering.RevisionEnvelope, 0)
	for _, revision := range allRevisions {
		if revision.RevisionFamily == engineering.RevisionFamilyTransitionRecord {
			allTransitionRevisions = append(allTransitionRevisions, revision)
		}
	}
	assignments := make([]engineering.RecordEnvelope, 0, len(allAssignments))
	assignmentDetails := make(map[engineering.RecordKey]engineering.LifecycleAssignmentDetail, len(allAssignments))
	for _, assignment := range allAssignments {
		detail, inspectErr := inspector.InspectLifecycleAssignment(assignment)
		if inspectErr != nil {
			return LifecycleHistory{}, integrityError("lifecycle assignment is unreadable", inspectErr)
		}
		if detail.SubjectArtifactID != capabilityArtifactID && assignment.SubjectKey != subjectKey {
			continue
		}
		if detail.SubjectArtifactID != capabilityArtifactID || assignment.SubjectKey != subjectKey {
			return LifecycleHistory{}, integrityError("lifecycle assignment subject projection mismatch", nil)
		}
		assignments = append(assignments, assignment)
		assignmentDetails[assignment.Key] = detail
	}
	transitionRevisions := make([]engineering.RevisionEnvelope, 0, len(allTransitionRevisions))
	transitionDetails := make(map[engineering.RevisionKey]engineering.LifecycleTransitionDetail, len(allTransitionRevisions))
	for _, revision := range allTransitionRevisions {
		detail, inspectErr := inspector.InspectLifecycleTransition(revision)
		if inspectErr != nil {
			return LifecycleHistory{}, integrityError("lifecycle transition revision is unreadable", inspectErr)
		}
		if detail.SubjectArtifactID != capabilityArtifactID && revision.SubjectKey != subjectKey {
			continue
		}
		if detail.SubjectArtifactID != capabilityArtifactID || revision.SubjectKey != subjectKey {
			return LifecycleHistory{}, integrityError("lifecycle transition subject projection mismatch", nil)
		}
		transitionRevisions = append(transitionRevisions, revision)
		transitionDetails[revision.Key] = detail
	}

	assignmentByID := make(map[string]engineering.RecordEnvelope, len(assignments))
	detailByID := make(map[string]engineering.LifecycleAssignmentDetail, len(assignments))
	revisionByKey := make(map[engineering.RevisionKey]engineering.RevisionEnvelope, len(transitionRevisions)+len(assignments))
	for _, revision := range transitionRevisions {
		if _, duplicate := revisionByKey[revision.Key]; duplicate {
			return LifecycleHistory{}, integrityError("lifecycle history contains a duplicate transition revision", nil)
		}
		revisionByKey[revision.Key] = revision
	}
	for _, assignment := range assignments {
		detail := assignmentDetails[assignment.Key]
		if detail.SubjectArtifactID != capabilityArtifactID || assignment.SubjectKey != subjectKey {
			return LifecycleHistory{}, integrityError("lifecycle assignment subject projection mismatch", nil)
		}
		if _, duplicate := assignmentByID[detail.AssignmentID]; duplicate {
			return LifecycleHistory{}, integrityError("lifecycle history contains a duplicate assignment identity", nil)
		}
		assignmentByID[detail.AssignmentID] = assignment
		detailByID[detail.AssignmentID] = detail
		if _, found := revisionByKey[detail.EstablishedBy]; !found {
			revision, revisionFound, lookupErr := repos.Revisions.Get(ctx, detail.EstablishedBy)
			if lookupErr != nil {
				return LifecycleHistory{}, lookupErr
			}
			if !revisionFound {
				return LifecycleHistory{}, integrityError("lifecycle assignment has a dangling establishing revision", nil)
			}
			revisionByKey[detail.EstablishedBy] = revision
		}
	}

	if len(assignmentByID) == 0 && len(revisionByKey) == 0 {
		return LifecycleHistory{Policy: policy}, nil
	}
	if len(assignmentByID) == 0 || len(revisionByKey) == 0 {
		return LifecycleHistory{}, integrityError("lifecycle history is partially occupied", nil)
	}

	transitionDetailByKey := make(map[engineering.RevisionKey]engineering.LifecycleTransitionDetail, len(revisionByKey))
	rootIDs := make(map[string]bool)
	for key, revision := range revisionByKey {
		if revision.RevisionFamily != engineering.RevisionFamilyTransitionRecord {
			return LifecycleHistory{}, integrityError("lifecycle assignment is established by another revision family", nil)
		}
		detail := transitionDetails[key]
		if detail.SubjectArtifactID != capabilityArtifactID || revision.SubjectKey != subjectKey {
			return LifecycleHistory{}, integrityError("lifecycle transition subject projection mismatch", nil)
		}
		transitionDetailByKey[key] = detail
		rootIDs[key.ArtifactID] = true
	}
	// Validate every selected root's complete occupancy before accepting the
	// subject-local chain. Filtering revisions by family and subject above is
	// only candidate discovery: a mixed-family revision or a sibling lifecycle
	// for another subject under the same root must remain observable corruption,
	// not disappear behind that filter.
	for rootID := range rootIDs {
		root, found, err := repos.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: rootID})
		if err != nil {
			return LifecycleHistory{}, err
		}
		if !found {
			return LifecycleHistory{}, integrityError("lifecycle transition-record root is missing", nil)
		}
		if err := inspectArtifact(inspector, root); err != nil {
			return LifecycleHistory{}, err
		}
		rootFamily, err := inspector.ArtifactFamily(root)
		if err != nil || rootFamily != engineering.RevisionFamilyTransitionRecord {
			return LifecycleHistory{}, integrityError("lifecycle root has the wrong artifact family", err)
		}
		if err := validateForeignArtifactOccupancy(ctx, repos, inspector, root); err != nil {
			return LifecycleHistory{}, err
		}
	}
	if len(rootIDs) != 1 {
		return LifecycleHistory{}, integrityError("lifecycle history must use exactly one transition-record root", nil)
	}
	var rootID string
	for id := range rootIDs {
		rootID = id
	}

	ownerByRevision := make(map[engineering.RevisionKey]string, len(assignments))
	for assignmentID, detail := range detailByID {
		if other, duplicate := ownerByRevision[detail.EstablishedBy]; duplicate {
			return LifecycleHistory{}, integrityError(fmt.Sprintf("transition revision establishes both %s and %s", other, assignmentID), nil)
		}
		ownerByRevision[detail.EstablishedBy] = assignmentID
	}
	if len(ownerByRevision) != len(revisionByKey) {
		return LifecycleHistory{}, integrityError("a transition revision has no resulting state assignment", nil)
	}

	var entryID string
	nextByAssignment := make(map[string]string, len(assignments)-1)
	nodeByID := make(map[string]LifecycleHistoryAssignment, len(assignments))
	configuredKey := engineering.LifecycleDefinitionVersionKey{DefinitionID: policy.DefinitionID, VersionID: policy.VersionID}
	for revisionKey, transitionDetail := range transitionDetailByKey {
		assignmentID, owned := ownerByRevision[revisionKey]
		if !owned {
			return LifecycleHistory{}, integrityError("transition revision has no assignment owner", nil)
		}
		assignmentDetail := detailByID[assignmentID]
		assignment := assignmentByID[assignmentID]
		transition := revisionByKey[revisionKey]
		if assignmentDetail.DefinitionVersion != configuredKey {
			return LifecycleHistory{}, integrityError("state assignment names another lifecycle definition version", nil)
		}
		if assignmentDetail.EstablishedBy != revisionKey || !canonicalTimeEqual(assignmentDetail.RecordedAt, transitionDetail.RecordedAt) || !canonicalTimeEqual(assignment.RecordedAt, transition.RecordedAt) {
			return LifecycleHistory{}, integrityError("assignment and establishing transition disagree", nil)
		}
		if assignmentDetail.EffectiveAt.After(assignmentDetail.RecordedAt) {
			return LifecycleHistory{}, integrityError("state assignment is effective after it was recorded", nil)
		}

		if transitionDetail.Entry {
			if entryID != "" || !policy.HasInitialState(assignmentDetail.StateID) {
				return LifecycleHistory{}, integrityError("lifecycle history has an invalid or duplicate entry", nil)
			}
			entryID = assignmentID
		} else {
			if transitionDetail.DefinitionVersion != configuredKey || transitionDetail.ResultingAssignmentID != assignmentID || transitionDetail.TargetStateID != assignmentDetail.StateID {
				return LifecycleHistory{}, integrityError("transition and resulting assignment disagree", nil)
			}
			sourceDetail, sourceFound := detailByID[transitionDetail.FromAssignmentID]
			if !sourceFound {
				return LifecycleHistory{}, integrityError("lifecycle transition has a dangling predecessor", nil)
			}
			if !policy.Permits(transitionDetail.TransitionID, sourceDetail.StateID, assignmentDetail.StateID) {
				return LifecycleHistory{}, integrityError("stored lifecycle transition violates configured policy", nil)
			}
			if !sourceDetail.EffectiveAt.Before(assignmentDetail.EffectiveAt) || sourceDetail.RecordedAt.After(transitionDetail.AttemptedAt) || transitionDetail.AttemptedAt.After(transitionDetail.CompletedAt) || transitionDetail.CompletedAt.After(assignmentDetail.EffectiveAt) {
				return LifecycleHistory{}, integrityError("stored lifecycle transition violates canonical time order", nil)
			}
			if _, branched := nextByAssignment[transitionDetail.FromAssignmentID]; branched {
				return LifecycleHistory{}, integrityError("lifecycle history branches from one predecessor", nil)
			}
			nextByAssignment[transitionDetail.FromAssignmentID] = assignmentID
		}
		nodeByID[assignmentID] = LifecycleHistoryAssignment{
			Assignment: assignment, Detail: assignmentDetail,
			Transition: transition, TransitionDetail: transitionDetail,
		}
	}
	if entryID == "" {
		return LifecycleHistory{}, integrityError("lifecycle history has no entry assignment", nil)
	}

	ordered := make([]LifecycleHistoryAssignment, 0, len(assignments))
	seen := make(map[string]bool, len(assignments))
	current := entryID
	for current != "" {
		if seen[current] {
			return LifecycleHistory{}, integrityError("lifecycle history contains a cycle", nil)
		}
		seen[current] = true
		node, found := nodeByID[current]
		if !found {
			return LifecycleHistory{}, integrityError("lifecycle history contains a disconnected assignment", nil)
		}
		ordered = append(ordered, node)
		current = nextByAssignment[current]
	}
	if len(ordered) != len(assignments) {
		return LifecycleHistory{}, integrityError("lifecycle history contains a disconnected assignment", nil)
	}
	return LifecycleHistory{
		Found: true, Policy: policy, RootArtifactID: rootID,
		Assignments: ordered, Head: ordered[len(ordered)-1],
	}, nil
}

func loadLifecyclePolicy(ctx context.Context, repos Repositories, inspector EngineeringReplayInspector) (engineering.LifecyclePolicy, error) {
	configuredKey := inspector.ConfiguredLifecycleVersionKey()
	definitions, err := repos.LifecycleDefinitions.ListDefinitions(ctx)
	if err != nil {
		return engineering.LifecyclePolicy{}, err
	}
	definition, definitionFound, err := repos.LifecycleDefinitions.GetDefinition(ctx, configuredKey.DefinitionID)
	if err != nil {
		return engineering.LifecyclePolicy{}, err
	}
	version, versionFound, err := repos.LifecycleDefinitions.GetVersion(ctx, configuredKey)
	if err != nil {
		return engineering.LifecyclePolicy{}, err
	}
	versions, err := repos.LifecycleDefinitions.ListVersions(ctx, configuredKey.DefinitionID)
	if err != nil {
		return engineering.LifecyclePolicy{}, err
	}
	if !definitionFound || !versionFound || len(definitions) != 1 || definitions[0].DefinitionID != configuredKey.DefinitionID || len(versions) != 1 || versions[0].Key != configuredKey {
		return engineering.LifecyclePolicy{}, integrityError("lifecycle configuration is absent, partial, or contradictory", nil)
	}
	policy, err := inspector.InspectLifecycleConfiguration(definition, version)
	if err != nil {
		return engineering.LifecyclePolicy{}, integrityError("lifecycle configuration is unreadable", err)
	}
	return policy, nil
}
