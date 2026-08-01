package application

import (
	"reflect"

	"github.com/aleka7sk/featureforge/internal/engineering"
)

// proposalInspectionMemo is scoped to one UnitOfWork callback. Proposal
// integrity walks the same immutable envelopes through several overlapping
// graph proofs; decoding and canonical round-trip validation each time is
// needlessly quadratic. The memo caches successful inspector results only when
// the exact envelope (and content, where applicable) is equal. It never caches
// errors and never treats identity equality as value equality, so a repository
// that returns contradictory values for one key still fails through the
// underlying inspector or the surrounding aggregate checks.
type proposalInspectionMemo struct {
	ProposalReplayInspector

	artifacts           map[engineering.ArtifactKey]engineering.ArtifactEnvelope
	capabilityArtifacts map[engineering.ArtifactKey]engineering.ArtifactEnvelope
	evidenceArtifacts   map[engineering.ArtifactKey]engineering.ArtifactEnvelope
	artifactFamilies    map[engineering.ArtifactKey]proposalArtifactFamilyMemo
	revisions           map[engineering.RevisionKey]engineering.RevisionEnvelope
	records             map[engineering.RecordKey]engineering.RecordEnvelope
	contents            map[engineering.RevisionKey]proposalContentMemo
	plans               map[engineering.RevisionKey]proposalPlanMemo
	assignments         map[engineering.RecordKey]proposalAssignmentMemo
	executions          map[engineering.RecordKey]proposalExecutionMemo
	claimMethods        map[engineering.RecordKey]proposalClaimMethodMemo
	aiRevisions         map[engineering.RevisionKey]proposalAIRevisionMemo
}

type proposalArtifactFamilyMemo struct {
	envelope engineering.ArtifactEnvelope
	family   engineering.RevisionFamily
}

type proposalContentMemo struct {
	revision engineering.RevisionEnvelope
	content  engineering.CapabilitySpecificationContent
}

type proposalPlanMemo struct {
	revision     engineering.RevisionEnvelope
	scope        string
	activityKeys []string
	methods      []string
	subjects     []engineering.RevisionKey
	requirements []engineering.RevisionKey
}

type proposalAssignmentMemo struct {
	envelope    engineering.RecordEnvelope
	established engineering.RevisionKey
}

type proposalExecutionMemo struct {
	envelope    engineering.RecordEnvelope
	plan        engineering.RevisionKey
	activityKey string
	method      string
}

type proposalClaimMethodMemo struct {
	envelope engineering.RecordEnvelope
	method   string
}

type proposalAIRevisionMemo struct {
	envelope       engineering.RevisionEnvelope
	proposalDigest engineering.Digest
	contextDigest  engineering.Digest
	sources        []string
	found          bool
}

func newProposalInspectionMemo(inspector ProposalReplayInspector) ProposalReplayInspector {
	if inspector == nil {
		return nil
	}
	return &proposalInspectionMemo{
		ProposalReplayInspector: inspector,
		artifacts:               make(map[engineering.ArtifactKey]engineering.ArtifactEnvelope),
		capabilityArtifacts:     make(map[engineering.ArtifactKey]engineering.ArtifactEnvelope),
		evidenceArtifacts:       make(map[engineering.ArtifactKey]engineering.ArtifactEnvelope),
		artifactFamilies:        make(map[engineering.ArtifactKey]proposalArtifactFamilyMemo),
		revisions:               make(map[engineering.RevisionKey]engineering.RevisionEnvelope),
		records:                 make(map[engineering.RecordKey]engineering.RecordEnvelope),
		contents:                make(map[engineering.RevisionKey]proposalContentMemo),
		plans:                   make(map[engineering.RevisionKey]proposalPlanMemo),
		assignments:             make(map[engineering.RecordKey]proposalAssignmentMemo),
		executions:              make(map[engineering.RecordKey]proposalExecutionMemo),
		claimMethods:            make(map[engineering.RecordKey]proposalClaimMethodMemo),
		aiRevisions:             make(map[engineering.RevisionKey]proposalAIRevisionMemo),
	}
}

func (m *proposalInspectionMemo) ValidateArtifact(envelope engineering.ArtifactEnvelope) error {
	if cached, ok := m.artifacts[envelope.Key]; ok && reflect.DeepEqual(cached, envelope) {
		return nil
	}
	if err := m.ProposalReplayInspector.ValidateArtifact(envelope); err != nil {
		return err
	}
	m.artifacts[envelope.Key] = envelope
	return nil
}

func (m *proposalInspectionMemo) ArtifactFamily(envelope engineering.ArtifactEnvelope) (engineering.RevisionFamily, error) {
	if cached, ok := m.artifactFamilies[envelope.Key]; ok && reflect.DeepEqual(cached.envelope, envelope) {
		return cached.family, nil
	}
	family, err := m.ProposalReplayInspector.ArtifactFamily(envelope)
	if err != nil {
		return "", err
	}
	m.artifactFamilies[envelope.Key] = proposalArtifactFamilyMemo{envelope: envelope, family: family}
	return family, nil
}

func (m *proposalInspectionMemo) ValidateCapabilityArtifact(envelope engineering.ArtifactEnvelope) error {
	if cached, ok := m.capabilityArtifacts[envelope.Key]; ok && reflect.DeepEqual(cached, envelope) {
		return nil
	}
	if err := m.ProposalReplayInspector.ValidateCapabilityArtifact(envelope); err != nil {
		return err
	}
	m.capabilityArtifacts[envelope.Key] = envelope
	return nil
}

func (m *proposalInspectionMemo) ValidateEvidenceArtifact(envelope engineering.ArtifactEnvelope) error {
	if cached, ok := m.evidenceArtifacts[envelope.Key]; ok && reflect.DeepEqual(cached, envelope) {
		return nil
	}
	if err := m.ProposalReplayInspector.ValidateEvidenceArtifact(envelope); err != nil {
		return err
	}
	m.evidenceArtifacts[envelope.Key] = envelope
	return nil
}

func (m *proposalInspectionMemo) ValidateRevision(envelope engineering.RevisionEnvelope) error {
	if cached, ok := m.revisions[envelope.Key]; ok && reflect.DeepEqual(cached, envelope) {
		return nil
	}
	if err := m.ProposalReplayInspector.ValidateRevision(envelope); err != nil {
		return err
	}
	m.revisions[envelope.Key] = envelope
	return nil
}

func (m *proposalInspectionMemo) ValidateRecord(envelope engineering.RecordEnvelope) error {
	if cached, ok := m.records[envelope.Key]; ok && reflect.DeepEqual(cached, envelope) {
		return nil
	}
	if err := m.ProposalReplayInspector.ValidateRecord(envelope); err != nil {
		return err
	}
	m.records[envelope.Key] = envelope
	return nil
}

func (m *proposalInspectionMemo) ValidateCapabilityContent(
	revision engineering.RevisionEnvelope,
	content engineering.CapabilitySpecificationContent,
) error {
	if cached, ok := m.contents[revision.Key]; ok && reflect.DeepEqual(cached.revision, revision) && reflect.DeepEqual(cached.content, content) {
		return nil
	}
	if err := m.ProposalReplayInspector.ValidateCapabilityContent(revision, content); err != nil {
		return err
	}
	m.contents[revision.Key] = proposalContentMemo{revision: revision, content: content}
	return nil
}

func (m *proposalInspectionMemo) ValidationPlanReferences(
	revision engineering.RevisionEnvelope,
) (string, []string, []string, []engineering.RevisionKey, []engineering.RevisionKey, error) {
	if cached, ok := m.plans[revision.Key]; ok && reflect.DeepEqual(cached.revision, revision) {
		return cached.scope,
			append([]string(nil), cached.activityKeys...),
			append([]string(nil), cached.methods...),
			append([]engineering.RevisionKey(nil), cached.subjects...),
			append([]engineering.RevisionKey(nil), cached.requirements...), nil
	}
	scope, activityKeys, methods, subjects, requirements, err := m.ProposalReplayInspector.ValidationPlanReferences(revision)
	if err != nil {
		return "", nil, nil, nil, nil, err
	}
	m.plans[revision.Key] = proposalPlanMemo{
		revision: revision, scope: scope,
		activityKeys: append([]string(nil), activityKeys...),
		methods:      append([]string(nil), methods...),
		subjects:     append([]engineering.RevisionKey(nil), subjects...),
		requirements: append([]engineering.RevisionKey(nil), requirements...),
	}
	return scope, activityKeys, methods, subjects, requirements, nil
}

func (m *proposalInspectionMemo) StateAssignmentEstablishedBy(envelope engineering.RecordEnvelope) (engineering.RevisionKey, error) {
	if cached, ok := m.assignments[envelope.Key]; ok && reflect.DeepEqual(cached.envelope, envelope) {
		return cached.established, nil
	}
	established, err := m.ProposalReplayInspector.StateAssignmentEstablishedBy(envelope)
	if err != nil {
		return engineering.RevisionKey{}, err
	}
	m.assignments[envelope.Key] = proposalAssignmentMemo{envelope: envelope, established: established}
	return established, nil
}

func (m *proposalInspectionMemo) ExecutionPlanActivity(envelope engineering.RecordEnvelope) (engineering.RevisionKey, string, string, error) {
	if cached, ok := m.executions[envelope.Key]; ok && reflect.DeepEqual(cached.envelope, envelope) {
		return cached.plan, cached.activityKey, cached.method, nil
	}
	plan, activityKey, method, err := m.ProposalReplayInspector.ExecutionPlanActivity(envelope)
	if err != nil {
		return engineering.RevisionKey{}, "", "", err
	}
	m.executions[envelope.Key] = proposalExecutionMemo{
		envelope: envelope, plan: plan, activityKey: activityKey, method: method,
	}
	return plan, activityKey, method, nil
}

func (m *proposalInspectionMemo) ClaimMethod(envelope engineering.RecordEnvelope) (string, error) {
	if cached, ok := m.claimMethods[envelope.Key]; ok && reflect.DeepEqual(cached.envelope, envelope) {
		return cached.method, nil
	}
	method, err := m.ProposalReplayInspector.ClaimMethod(envelope)
	if err != nil {
		return "", err
	}
	m.claimMethods[envelope.Key] = proposalClaimMethodMemo{envelope: envelope, method: method}
	return method, nil
}

func (m *proposalInspectionMemo) InspectAIAssistedCapabilityRevision(
	envelope engineering.RevisionEnvelope,
) (engineering.Digest, engineering.Digest, []string, bool, error) {
	if cached, ok := m.aiRevisions[envelope.Key]; ok && reflect.DeepEqual(cached.envelope, envelope) {
		return cached.proposalDigest, cached.contextDigest, append([]string(nil), cached.sources...), cached.found, nil
	}
	proposalDigest, contextDigest, sources, found, err := m.ProposalReplayInspector.InspectAIAssistedCapabilityRevision(envelope)
	if err != nil {
		return engineering.Digest{}, engineering.Digest{}, nil, false, err
	}
	m.aiRevisions[envelope.Key] = proposalAIRevisionMemo{
		envelope: envelope, proposalDigest: proposalDigest, contextDigest: contextDigest,
		sources: append([]string(nil), sources...), found: found,
	}
	return proposalDigest, contextDigest, sources, found, nil
}
