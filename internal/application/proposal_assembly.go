package application

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aleka7sk/featureforge/internal/engineering"
	"github.com/aleka7sk/featureforge/internal/proposal"
)

// proposalAssembly is the application-private, already-inspected input used
// to construct proposal.ContextPack. It deliberately retains the existing
// engineering/application values instead of copying PEOS shapes into this
// package. Conversion to the pure proposal value happens at the boundary.
type proposalAssembly struct {
	current      CurrentRevisionResult
	content      engineering.CapabilitySpecificationContent
	historySize  int
	requirements []EffectiveRequirement
	claims       map[engineering.RevisionKey]proposalClaimProjection
	decisions    []proposalDecisionProjection
}

type proposalClaimProjection struct {
	record    engineering.RecordEnvelope
	reasoning string
}

type proposalDecisionProjection struct {
	record engineering.RecordEnvelope
	detail engineering.DecisionDetail
}

// collectProposalAssembly performs the authoritative read side of FF-024
// against repositories already scoped to one UnitOfWork callback. It is kept
// separate from AssembleProposalContext so acceptance can run the identical
// assembly in its write transaction without opening a nested UnitOfWork.
func collectProposalAssembly(
	ctx context.Context,
	repos Repositories,
	projector EngineeringProjector,
	inspector EngineeringReplayInspector,
	artifactID string,
) (proposalAssembly, error) {
	if projector == nil {
		return proposalAssembly{}, integrityError("engineering projector is unavailable", nil)
	}
	artifact, found, err := repos.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: artifactID})
	if err != nil {
		return proposalAssembly{}, err
	}
	if !found && !artifact.Key.IsZero() {
		return proposalAssembly{}, integrityError("proposal Artifact lookup has contradictory presence", nil)
	}
	if !found {
		if err := validateAbsentArtifactHistory(ctx, repos, inspector, artifactID); err != nil {
			return proposalAssembly{}, err
		}
		return proposalAssembly{}, fmt.Errorf("%w: capability artifact %s", ErrNotFound, artifactID)
	}
	if artifact.Key.ArtifactID != artifactID {
		return proposalAssembly{}, integrityError("proposal Artifact lookup returned another identity", nil)
	}
	if err := inspectArtifact(inspector, artifact); err != nil {
		return proposalAssembly{}, err
	}
	family, err := inspector.ArtifactFamily(artifact)
	if err != nil {
		return proposalAssembly{}, integrityError("proposal Artifact family is unreadable", err)
	}
	if family != engineering.RevisionFamilyCapability {
		if err := validateForeignCapabilityCommandArtifact(ctx, repos, inspector, artifact); err != nil {
			return proposalAssembly{}, err
		}
		return proposalAssembly{}, immutableConflict("proposal Artifact identity belongs to another family")
	}
	historySize, err := validateManagedHistory(ctx, repos, inspector, artifactID, engineering.RevisionFamilyCapability, false)
	if err != nil {
		return proposalAssembly{}, err
	}

	current, err := ResolveCurrentRevision(ctx, repos, artifactID)
	if err != nil {
		return proposalAssembly{}, err
	}
	if !current.Found {
		return proposalAssembly{}, fmt.Errorf("%w: capability %s", ErrNoAcceptedRevision, artifactID)
	}
	if err := inspectRevision(inspector, current.Revision); err != nil {
		return proposalAssembly{}, err
	}
	if current.Revision.RevisionFamily != engineering.RevisionFamilyCapability {
		return proposalAssembly{}, integrityError("current capability resolution selected another revision family", nil)
	}
	content, found, err := repos.StructuredContent.Get(ctx, current.Revision.Key)
	if err != nil {
		return proposalAssembly{}, err
	}
	if !found {
		return proposalAssembly{}, integrityError("current capability revision has no structured content", nil)
	}
	if err := inspector.ValidateCapabilityContent(current.Revision, content); err != nil {
		return proposalAssembly{}, integrityError("current capability content disagrees with its revision", err)
	}

	requirementIDs, err := DiscoverRequirementArtifactIDs(ctx, repos, inspector, artifactID)
	if err != nil {
		return proposalAssembly{}, err
	}
	requirements, err := ResolveEffectiveRequirements(ctx, repos, inspector, requirementIDs)
	if err != nil {
		return proposalAssembly{}, err
	}
	for index := range requirements {
		revision, found, err := repos.Revisions.Get(ctx, requirements[index].RevisionKey)
		if err != nil {
			return proposalAssembly{}, err
		}
		if !found {
			return proposalAssembly{}, integrityError("effective requirement revision disappeared during context assembly", nil)
		}
		if revision.Key != requirements[index].RevisionKey {
			return proposalAssembly{}, integrityError("effective requirement lookup returned another revision identity", nil)
		}
		if err := inspectRevision(inspector, revision); err != nil {
			return proposalAssembly{}, err
		}
		statement, err := projector.ProjectRequirementStatement(revision.Payload)
		if err != nil {
			return proposalAssembly{}, fmt.Errorf("%w: requirement %s: %w", ErrStoredPayloadUnreadable, revision.Key, err)
		}
		requirements[index].Statement = statement
	}
	sort.Slice(requirements, func(i, j int) bool {
		return compareProposalRevisionKey(requirements[i].RevisionKey, requirements[j].RevisionKey) < 0
	})

	claims := make(map[engineering.RevisionKey]proposalClaimProjection, len(requirements))
	currentSubject := engineering.ArtifactRevisionSubjectKey(current.Revision.Key.ArtifactID, current.Revision.Key.RevisionID)
	scope := "featureforge:capability|" + artifactID
	for _, requirement := range requirements {
		criterionKey, err := engineering.RequirementCriterionKey(requirement.RevisionKey)
		if err != nil {
			return proposalAssembly{}, integrityError("effective requirement criterion identity is invalid", err)
		}
		resolved, err := ResolveCurrentClaim(ctx, repos, inspector, currentSubject, scope, []string{criterionKey})
		if err != nil {
			return proposalAssembly{}, err
		}
		if !resolved.Found {
			continue
		}
		if err := validateStoredClaimReferences(ctx, repos, inspector, resolved.Claim); err != nil {
			return proposalAssembly{}, err
		}
		reasoning, err := projector.ProjectClaimReasoning(resolved.Claim.Payload)
		if err != nil {
			return proposalAssembly{}, fmt.Errorf("%w: claim %s: %w", ErrStoredPayloadUnreadable, resolved.Claim.Key, err)
		}
		claims[requirement.RevisionKey] = proposalClaimProjection{record: resolved.Claim, reasoning: reasoning}
	}

	decisions, err := collectApplicableProposalDecisions(ctx, repos, projector, inspector, artifactID)
	if err != nil {
		return proposalAssembly{}, err
	}

	return proposalAssembly{
		current:      current,
		content:      content,
		historySize:  historySize,
		requirements: requirements,
		claims:       claims,
		decisions:    decisions,
	}, nil
}

type proposalContextValues struct {
	capability        proposal.CapabilityContext
	requirements      []proposal.RequirementContext
	claims            []proposal.ClaimContext
	decisions         []proposal.DecisionContext
	openQuestions     []proposal.OpenQuestionContext
	uncoveredCriteria []proposal.UncoveredCriterion
	findings          []proposal.ValidationFinding
}

// AssembleProposalContext reads and constructs one complete ContextPack in
// exactly one read-only UnitOfWork. The returned value contains no repository
// or callback authority and is safe to pass to proposal.Generator only after
// Do has returned.
func AssembleProposalContext(
	ctx context.Context,
	uow UnitOfWork,
	projector EngineeringProjector,
	inspector EngineeringReplayInspector,
	artifactID string,
) (proposal.ContextPack, error) {
	if err := requireIdentity("artifact id", artifactID); err != nil {
		return proposal.ContextPack{}, err
	}
	var pack proposal.ContextPack
	err := uow.Do(ctx, func(repos Repositories) error {
		var err error
		pack, err = assembleProposalContext(ctx, repos, projector, inspector, artifactID)
		return err
	})
	if err != nil {
		return proposal.ContextPack{}, err
	}
	return pack, nil
}

// assembleProposalContext is the callback-scoped form reused by proposal
// acceptance, where freshness and the three writes must share one transaction.
func assembleProposalContext(
	ctx context.Context,
	repos Repositories,
	projector EngineeringProjector,
	inspector EngineeringReplayInspector,
	artifactID string,
) (proposal.ContextPack, error) {
	assembly, err := collectProposalAssembly(ctx, repos, projector, inspector, artifactID)
	if err != nil {
		return proposal.ContextPack{}, err
	}
	return proposalContextPackFromAssembly(assembly)
}

func proposalContextPackFromAssembly(assembly proposalAssembly) (proposal.ContextPack, error) {
	values, err := materializeProposalContextValues(assembly)
	if err != nil {
		return proposal.ContextPack{}, err
	}
	pack, err := proposal.NewContextPack(
		values.capability,
		values.requirements,
		values.claims,
		values.decisions,
		values.openQuestions,
		values.uncoveredCriteria,
		values.findings,
	)
	if err != nil {
		return proposal.ContextPack{}, integrityError("authoritative state cannot form a proposal context", err)
	}
	return pack, nil
}

// materializeProposalContextValues converts the already authoritative
// assembly into proposal's immutable PEOS-free values and computes the exact
// current-criterion coverage/finding sections. The final ContextPack
// constructor remains responsible for deriving its canonical source union.
func materializeProposalContextValues(assembly proposalAssembly) (proposalContextValues, error) {
	capability, err := proposal.NewCapabilityContext(assembly.current.Revision.Key, assembly.current.Sequence, assembly.content)
	if err != nil {
		return proposalContextValues{}, integrityError("current capability cannot form proposal context", err)
	}

	values := proposalContextValues{
		capability:        capability,
		requirements:      make([]proposal.RequirementContext, 0, len(assembly.requirements)),
		claims:            make([]proposal.ClaimContext, 0, len(assembly.claims)),
		decisions:         make([]proposal.DecisionContext, 0, len(assembly.decisions)),
		openQuestions:     make([]proposal.OpenQuestionContext, 0, len(assembly.content.OpenQuestions())),
		uncoveredCriteria: []proposal.UncoveredCriterion{},
		findings:          []proposal.ValidationFinding{},
	}

	for _, requirement := range assembly.requirements {
		value, err := proposal.NewRequirementContext(
			requirement.RevisionKey,
			requirement.Sequence,
			requirement.Statement,
			requirement.SourceCapabilityRevision,
			requirement.SourceAcceptanceCriterion,
		)
		if err != nil {
			return proposalContextValues{}, integrityError("effective requirement cannot form proposal context", err)
		}
		values.requirements = append(values.requirements, value)

		claim, found := assembly.claims[requirement.RevisionKey]
		if !found {
			continue
		}
		claimValue, err := newProposalClaimContext(assembly.current.Revision.Key, assembly.current.Revision.Key.ArtifactID, requirement.RevisionKey, claim)
		if err != nil {
			return proposalContextValues{}, err
		}
		values.claims = append(values.claims, claimValue)
	}

	for index, decision := range assembly.decisions {
		subject, err := newProposalDecisionSubject(decision.record.SubjectKey)
		if err != nil {
			return proposalContextValues{}, err
		}
		value, err := proposal.NewDecisionContext(decision.record.Key.ID, subject, decision.detail.OutcomeStatement, index+1)
		if err != nil {
			return proposalContextValues{}, integrityError("applicable decision cannot form proposal context", err)
		}
		values.decisions = append(values.decisions, value)
	}

	for index, question := range assembly.content.OpenQuestions() {
		value, err := proposal.NewOpenQuestionContext(assembly.current.Revision.Key, index+1, question)
		if err != nil {
			return proposalContextValues{}, integrityError("open question cannot form proposal context", err)
		}
		values.openQuestions = append(values.openQuestions, value)
	}

	mappedByCriterion := make(map[string][]EffectiveRequirement)
	for _, requirement := range assembly.requirements {
		if requirement.SourceCapabilityRevision == assembly.current.Revision.Key {
			mappedByCriterion[requirement.SourceAcceptanceCriterion] = append(mappedByCriterion[requirement.SourceAcceptanceCriterion], requirement)
		}
	}
	for _, criterion := range assembly.content.AcceptanceCriteria() {
		mapped := mappedByCriterion[criterion.Key()]
		if len(mapped) == 0 {
			value, err := proposal.NewUncoveredCriterion(
				assembly.current.Revision.Key,
				criterion.Key(),
				criterion.Text(),
				proposal.UncoveredNoRequirementTrace,
				nil,
			)
			if err != nil {
				return proposalContextValues{}, integrityError("uncovered criterion cannot form proposal context", err)
			}
			values.uncoveredCriteria = append(values.uncoveredCriteria, value)
			continue
		}

		missing := make([]engineering.RevisionKey, 0, len(mapped))
		for _, requirement := range mapped {
			claim, found := assembly.claims[requirement.RevisionKey]
			if !found {
				missing = append(missing, requirement.RevisionKey)
				continue
			}
			outcome, finding := proposalFindingOutcome(claim.record.Outcome)
			if !finding {
				continue
			}
			value, err := proposal.NewValidationFinding(
				assembly.current.Revision.Key,
				criterion.Key(),
				requirement.RevisionKey,
				claim.record.Key,
				outcome,
				claim.reasoning,
			)
			if err != nil {
				return proposalContextValues{}, integrityError("validation finding cannot form proposal context", err)
			}
			values.findings = append(values.findings, value)
		}
		if len(missing) != 0 {
			sort.Slice(missing, func(i, j int) bool { return compareProposalRevisionKey(missing[i], missing[j]) < 0 })
			value, err := proposal.NewUncoveredCriterion(
				assembly.current.Revision.Key,
				criterion.Key(),
				criterion.Text(),
				proposal.UncoveredMissingCurrentClaim,
				missing,
			)
			if err != nil {
				return proposalContextValues{}, integrityError("uncovered criterion cannot form proposal context", err)
			}
			values.uncoveredCriteria = append(values.uncoveredCriteria, value)
		}
	}
	return values, nil
}

func newProposalClaimContext(
	capabilityRevision engineering.RevisionKey,
	scopeArtifactID string,
	requirementRevision engineering.RevisionKey,
	claim proposalClaimProjection,
) (proposal.ClaimContext, error) {
	executions := make([]engineering.RecordKey, 0, len(claim.record.ExecutionKeys))
	for _, reference := range claim.record.ExecutionKeys {
		executionID, found := strings.CutPrefix(reference, "execution:")
		if !found || executionID == "" {
			return proposal.ClaimContext{}, integrityError("current claim has a malformed execution projection", nil)
		}
		key, err := engineering.NewRecordKey(engineering.RecordKindExecution, executionID)
		if err != nil {
			return proposal.ClaimContext{}, integrityError("current claim has a malformed execution projection", err)
		}
		executions = append(executions, key)
	}
	evidence := make([]engineering.RevisionKey, 0, len(claim.record.EvidenceKeys))
	for _, reference := range claim.record.EvidenceKeys {
		artifactID, revisionID, err := engineering.ParseEvidenceKey(reference)
		if err != nil {
			return proposal.ClaimContext{}, integrityError("current claim has a malformed evidence projection", err)
		}
		key, err := engineering.NewRevisionKey(artifactID, revisionID)
		if err != nil {
			return proposal.ClaimContext{}, integrityError("current claim has a malformed evidence projection", err)
		}
		evidence = append(evidence, key)
	}
	outcome, ok := proposalClaimOutcome(claim.record.Outcome)
	if !ok {
		return proposal.ClaimContext{}, integrityError("current claim has an unsupported outcome", nil)
	}
	value, err := proposal.NewClaimContext(
		claim.record.Key,
		requirementRevision,
		capabilityRevision,
		scopeArtifactID,
		claim.record.CriterionKeys,
		outcome,
		claim.reasoning,
		executions,
		evidence,
	)
	if err != nil {
		return proposal.ClaimContext{}, integrityError("current claim cannot form proposal context", err)
	}
	return value, nil
}

func newProposalDecisionSubject(subjectKey string) (proposal.DecisionSubject, error) {
	kind, artifactID, revisionID, err := engineering.ParseSubjectKey(subjectKey)
	if err != nil {
		return proposal.DecisionSubject{}, integrityError("applicable decision subject is malformed", err)
	}
	switch kind {
	case engineering.SubjectKindArtifact:
		subject, err := proposal.NewArtifactDecisionSubject(artifactID)
		if err != nil {
			return proposal.DecisionSubject{}, integrityError("applicable decision Artifact subject is invalid", err)
		}
		return subject, nil
	case engineering.SubjectKindArtifactRevision:
		key, err := engineering.NewRevisionKey(artifactID, revisionID)
		if err != nil {
			return proposal.DecisionSubject{}, integrityError("applicable decision Revision subject is invalid", err)
		}
		subject, err := proposal.NewRevisionDecisionSubject(key)
		if err != nil {
			return proposal.DecisionSubject{}, integrityError("applicable decision Revision subject is invalid", err)
		}
		return subject, nil
	default:
		return proposal.DecisionSubject{}, integrityError("applicable decision subject kind is unsupported", nil)
	}
}

func proposalClaimOutcome(stored string) (string, bool) {
	switch stored {
	case "peos:satisfied":
		return "satisfied", true
	case "peos:not-satisfied":
		return "not-satisfied", true
	case "peos:inconclusive":
		return "inconclusive", true
	default:
		return "", false
	}
}

func proposalFindingOutcome(stored string) (proposal.FindingOutcome, bool) {
	switch stored {
	case "peos:not-satisfied":
		return proposal.FindingNotSatisfied, true
	case "peos:inconclusive":
		return proposal.FindingInconclusive, true
	default:
		return "", false
	}
}

func compareProposalRevisionKey(left, right engineering.RevisionKey) int {
	if left.ArtifactID < right.ArtifactID {
		return -1
	}
	if left.ArtifactID > right.ArtifactID {
		return 1
	}
	if left.RevisionID < right.RevisionID {
		return -1
	}
	if left.RevisionID > right.RevisionID {
		return 1
	}
	return 0
}

// collectApplicableProposalDecisions implements FF-004 §3.3 without using
// DiscoverDecisionIDs' historical revision-only projection. Both a capability
// Artifact subject and any exact Revision subject in its validated history are
// applicable. All Record payloads are inspected before their Kind/SubjectKey
// projections are used as filters.
func collectApplicableProposalDecisions(
	ctx context.Context,
	repos Repositories,
	projector EngineeringProjector,
	inspector EngineeringReplayInspector,
	artifactID string,
) ([]proposalDecisionProjection, error) {
	revisions, err := listValidatedRevisions(ctx, repos, inspector)
	if err != nil {
		return nil, err
	}
	subjects := map[string]struct{}{engineering.ArtifactSubjectKey(artifactID): {}}
	for _, revision := range revisions {
		if revision.Key.ArtifactID == artifactID {
			subjects[engineering.ArtifactRevisionSubjectKey(revision.Key.ArtifactID, revision.Key.RevisionID)] = struct{}{}
		}
	}
	records, err := listValidatedRecords(ctx, repos, inspector)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	result := make([]proposalDecisionProjection, 0)
	for _, record := range records {
		if record.Kind != engineering.RecordKindDecision {
			continue
		}
		if _, applicable := subjects[record.SubjectKey]; !applicable {
			continue
		}
		if _, duplicate := seen[record.Key.ID]; duplicate {
			return nil, integrityError("applicable decision population contains a duplicate identity", nil)
		}
		if err := validateProposalDecisionReferences(ctx, repos, inspector, record, artifactID); err != nil {
			return nil, err
		}
		detail, err := projector.ProjectDecisionDetail(record.Payload)
		if err != nil {
			return nil, fmt.Errorf("%w: decision %s: %w", ErrStoredPayloadUnreadable, record.Key, err)
		}
		seen[record.Key.ID] = struct{}{}
		result = append(result, proposalDecisionProjection{record: record, detail: detail})
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := result[i].record, result[j].record
		if canonicalTimeEqual(left.RecordedAt, right.RecordedAt) {
			return left.Key.ID < right.Key.ID
		}
		return left.RecordedAt.Before(right.RecordedAt)
	})
	return result, nil
}

func validateProposalDecisionReferences(
	ctx context.Context,
	repos Repositories,
	inspector EngineeringReplayInspector,
	decision engineering.RecordEnvelope,
	capabilityArtifactID string,
) error {
	if len(decision.EvidenceKeys) != 1 {
		return integrityError("decision must project exactly one evidence revision", nil)
	}
	if _, _, err := engineering.ParseEvidenceKey(decision.EvidenceKeys[0]); err != nil {
		return integrityError("decision evidence projection", err)
	}
	kind, artifactID, revisionID, err := engineering.ParseSubjectKey(decision.SubjectKey)
	if err != nil || artifactID != capabilityArtifactID {
		return integrityError("applicable decision subject projection is contradictory", err)
	}
	switch kind {
	case engineering.SubjectKindArtifact:
		artifact, found, err := repos.Artifacts.Get(ctx, engineering.ArtifactKey{ArtifactID: artifactID})
		if err != nil {
			return err
		}
		if !found {
			return integrityError("decision Artifact subject is dangling", nil)
		}
		if artifact.Key.ArtifactID != artifactID {
			return integrityError("decision Artifact subject lookup returned another identity", nil)
		}
		if err := inspectArtifact(inspector, artifact); err != nil {
			return err
		}
		if err := inspector.ValidateCapabilityArtifact(artifact); err != nil {
			return integrityError("decision Artifact subject names another family", err)
		}
	case engineering.SubjectKindArtifactRevision:
		key, err := engineering.NewRevisionKey(artifactID, revisionID)
		if err != nil {
			return integrityError("decision Revision subject is malformed", err)
		}
		revision, found, err := repos.Revisions.Get(ctx, key)
		if err != nil {
			return err
		}
		if !found {
			return integrityError("decision Revision subject is dangling", nil)
		}
		if revision.Key != key {
			return integrityError("decision Revision subject lookup returned another identity", nil)
		}
		if err := inspectRevision(inspector, revision); err != nil {
			return err
		}
		if revision.RevisionFamily != engineering.RevisionFamilyCapability {
			return integrityError("decision Revision subject names another family", nil)
		}
	default:
		return integrityError("decision subject uses an unsupported kind", nil)
	}
	_, err = validateManagedHistory(ctx, repos, inspector, artifactID, engineering.RevisionFamilyCapability, false)
	return err
}
