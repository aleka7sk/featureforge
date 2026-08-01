package memory

import (
	"context"
	"fmt"
	"maps"
	"sort"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
)

// reposFor returns an application.Repositories bundle backed by txn. ctx is
// accepted for interface conformance and forward-compatibility (e.g. a
// future PostgreSQL adapter's use of it for cancellation); the in-memory
// adapter does not need it.
func reposFor(ctx context.Context, txn *transaction) application.Repositories {
	return application.Repositories{
		Projects:             projectRepo{ctx: ctx, txn: txn},
		FeatureCards:         featureCardRepo{ctx: ctx, txn: txn},
		Artifacts:            artifactRepo{ctx: ctx, txn: txn},
		Revisions:            revisionRepo{ctx: ctx, txn: txn},
		StructuredContent:    contentRepo{ctx: ctx, txn: txn},
		Records:              recordRepo{ctx: ctx, txn: txn},
		RevisionOrder:        orderRepo{ctx: ctx, txn: txn},
		RevisionAcceptance:   acceptanceRepo{ctx: ctx, txn: txn},
		RequirementTraces:    requirementTraceRepo{ctx: ctx, txn: txn},
		LifecycleDefinitions: lifecycleDefinitionRepo{ctx: ctx, txn: txn},
	}
}

// lookup returns the overlay's entry for key if present, else the
// committed entry, else the zero value and false.
func lookup[K comparable, V any](overlay, committed map[K]V, key K) (V, bool) {
	if v, ok := overlay[key]; ok {
		return v, true
	}
	v, ok := committed[key]
	return v, ok
}

// putCreateOnly enforces FF-009 §5's create-only Put semantics for a
// comparable value type: identical payload is a no-op (idempotent),
// differing payload for an existing key conflicts, and a new key writes to
// the overlay. equal compares two values for the idempotency check.
func putCreateOnly[K comparable, V any](overlay, committed map[K]V, key K, value V, equal func(a, b V) bool) error {
	if existing, ok := lookup(overlay, committed, key); ok {
		if equal(existing, value) {
			return nil
		}
		return fmt.Errorf("%w: %v", application.ErrImmutableValueConflict, key)
	}
	overlay[key] = value
	return nil
}

// --- Projects ---

type projectRepo struct {
	ctx context.Context
	txn *transaction
}

func (r projectRepo) Put(_ context.Context, p domain.Project) error {
	if err := r.txn.store.countWrite("project"); err != nil {
		return err
	}
	return putCreateOnly(r.txn.overlay.projects, r.txn.store.committed.projects, p.ID(), p,
		func(a, b domain.Project) bool { return a == b })
}

func (r projectRepo) Get(_ context.Context, id domain.ProjectID) (domain.Project, bool, error) {
	v, ok := lookup(r.txn.overlay.projects, r.txn.store.committed.projects, id)
	return v, ok, nil
}

func (r projectRepo) List(_ context.Context) ([]domain.Project, error) {
	seen := map[domain.ProjectID]domain.Project{}
	maps.Copy(seen, r.txn.store.committed.projects)
	maps.Copy(seen, r.txn.overlay.projects)
	out := make([]domain.Project, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID().String() < out[j].ID().String() })
	return out, nil
}

// --- Feature cards ---

type featureCardRepo struct {
	ctx context.Context
	txn *transaction
}

func (r featureCardRepo) Put(_ context.Context, c domain.FeatureCard) error {
	if err := r.txn.store.countWrite("featurecard"); err != nil {
		return err
	}
	if _, ok := lookup(r.txn.overlay.projects, r.txn.store.committed.projects, c.ProjectID()); !ok {
		return fmt.Errorf("%w: feature card %s names project %s, which does not exist", application.ErrReferencedValueMissing, c.ID(), c.ProjectID())
	}
	// LinkCapability is the sole persistence path for the monotonic link. A
	// materialized FeatureCard may be passed back to Put, so strip that read
	// projection before storing the base establishment value.
	base, err := domain.NewFeatureCard(c.ID(), c.ProjectID(), c.Title(), c.Description(), c.CreatedAt())
	if err != nil {
		return err
	}
	return putCreateOnly(r.txn.overlay.featureCards, r.txn.store.committed.featureCards, base.ID(), base,
		func(a, b domain.FeatureCard) bool { return a.SameEstablishment(b) })
}

func (r featureCardRepo) Get(_ context.Context, id domain.FeatureCardID) (domain.FeatureCard, bool, error) {
	v, ok := lookup(r.txn.overlay.featureCards, r.txn.store.committed.featureCards, id)
	if !ok {
		return domain.FeatureCard{}, false, nil
	}
	if link, linked := lookup(r.txn.overlay.capabilityLinks, r.txn.store.committed.capabilityLinks, id); linked {
		withLink, err := v.WithCapabilityArtifactID(link)
		if err != nil {
			return domain.FeatureCard{}, false, err
		}
		v = withLink
	}
	return v, true, nil
}

func (r featureCardRepo) ListByProject(_ context.Context, projectID domain.ProjectID) ([]domain.FeatureCard, error) {
	seen := map[domain.FeatureCardID]domain.FeatureCard{}
	maps.Copy(seen, r.txn.store.committed.featureCards)
	maps.Copy(seen, r.txn.overlay.featureCards)
	out := make([]domain.FeatureCard, 0)
	for id, v := range seen {
		if v.ProjectID() != projectID {
			continue
		}
		if link, linked := lookup(r.txn.overlay.capabilityLinks, r.txn.store.committed.capabilityLinks, id); linked {
			withLink, err := v.WithCapabilityArtifactID(link)
			if err != nil {
				return nil, err
			}
			v = withLink
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID().String() < out[j].ID().String() })
	return out, nil
}

func (r featureCardRepo) LinkCapability(_ context.Context, id domain.FeatureCardID, artifactID string) error {
	if _, found := lookup(r.txn.overlay.featureCards, r.txn.store.committed.featureCards, id); !found {
		return fmt.Errorf("%w: feature card %s", application.ErrReferencedValueMissing, id)
	}
	if existing, ok := lookup(r.txn.overlay.capabilityLinks, r.txn.store.committed.capabilityLinks, id); ok {
		if existing == artifactID {
			return nil
		}
		return fmt.Errorf("%w: feature card %s already linked to %s", application.ErrCapabilityAlreadyLinked, id, existing)
	}
	if err := r.txn.store.countWrite("capabilitylink"); err != nil {
		return err
	}
	r.txn.overlay.capabilityLinks[id] = artifactID
	return nil
}

// --- Artifact envelopes ---

type artifactRepo struct {
	ctx context.Context
	txn *transaction
}

func cloneArtifactEnvelope(env engineering.ArtifactEnvelope) engineering.ArtifactEnvelope {
	env.Payload = append([]byte(nil), env.Payload...)
	return env
}

func cloneRevisionEnvelope(env engineering.RevisionEnvelope) engineering.RevisionEnvelope {
	env.Payload = append([]byte(nil), env.Payload...)
	return env
}

func cloneRecordEnvelope(env engineering.RecordEnvelope) engineering.RecordEnvelope {
	env.Payload = append([]byte(nil), env.Payload...)
	env.CriterionKeys = append([]string(nil), env.CriterionKeys...)
	env.EvidenceKeys = append([]string(nil), env.EvidenceKeys...)
	env.ExecutionKeys = append([]string(nil), env.ExecutionKeys...)
	return env
}

func (r artifactRepo) Put(_ context.Context, env engineering.ArtifactEnvelope) error {
	if err := r.txn.store.countWrite("artifact"); err != nil {
		return err
	}
	env = cloneArtifactEnvelope(env)
	return putCreateOnly(r.txn.overlay.artifacts, r.txn.store.committed.artifacts, env.Key, env,
		func(a, b engineering.ArtifactEnvelope) bool { return a.Equal(b) })
}

func (r artifactRepo) Get(_ context.Context, key engineering.ArtifactKey) (engineering.ArtifactEnvelope, bool, error) {
	v, ok := lookup(r.txn.overlay.artifacts, r.txn.store.committed.artifacts, key)
	return cloneArtifactEnvelope(v), ok, nil
}

// --- Revision envelopes ---

type revisionRepo struct {
	ctx context.Context
	txn *transaction
}

func (r revisionRepo) Put(_ context.Context, env engineering.RevisionEnvelope) error {
	if err := r.txn.store.countWrite("revision"); err != nil {
		return err
	}
	if _, ok := lookup(r.txn.overlay.artifacts, r.txn.store.committed.artifacts, mustArtifactKey(env.Key)); !ok {
		return fmt.Errorf("%w: revision %s names artifact %s, which does not exist", application.ErrReferencedValueMissing, env.Key, env.Key.ArtifactID)
	}
	env = cloneRevisionEnvelope(env)
	return putCreateOnly(r.txn.overlay.revisions, r.txn.store.committed.revisions, env.Key, env,
		func(a, b engineering.RevisionEnvelope) bool { return a.Equal(b) })
}

func (r revisionRepo) Get(_ context.Context, key engineering.RevisionKey) (engineering.RevisionEnvelope, bool, error) {
	v, ok := lookup(r.txn.overlay.revisions, r.txn.store.committed.revisions, key)
	return cloneRevisionEnvelope(v), ok, nil
}

func (r revisionRepo) ListAll(_ context.Context) ([]engineering.RevisionEnvelope, error) {
	seen := map[engineering.RevisionKey]engineering.RevisionEnvelope{}
	for key, env := range r.txn.store.committed.revisions {
		seen[key] = env
	}
	for key, env := range r.txn.overlay.revisions {
		seen[key] = env
	}
	out := make([]engineering.RevisionEnvelope, 0, len(seen))
	for _, env := range seen {
		out = append(out, cloneRevisionEnvelope(env))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.String() < out[j].Key.String() })
	return out, nil
}

func (r revisionRepo) ListByArtifact(_ context.Context, artifactID string) ([]engineering.RevisionEnvelope, error) {
	seen := map[engineering.RevisionKey]engineering.RevisionEnvelope{}
	for k, v := range r.txn.store.committed.revisions {
		if k.ArtifactID == artifactID {
			seen[k] = v
		}
	}
	for k, v := range r.txn.overlay.revisions {
		if k.ArtifactID == artifactID {
			seen[k] = v
		}
	}
	out := make([]engineering.RevisionEnvelope, 0, len(seen))
	for _, v := range seen {
		out = append(out, cloneRevisionEnvelope(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.String() < out[j].Key.String() })
	return out, nil
}

// ListByFamilyAndSubject scans committed and overlay revisions directly.
// An empty subjectKey matches nothing, matching the interface
// contract that empty means "no subject queried" rather than "list every
// subject-less revision".
func (r revisionRepo) ListByFamilyAndSubject(_ context.Context, family engineering.RevisionFamily, subjectKey string) ([]engineering.RevisionEnvelope, error) {
	if subjectKey == "" {
		return []engineering.RevisionEnvelope{}, nil
	}
	seen := map[engineering.RevisionKey]engineering.RevisionEnvelope{}
	for k, v := range r.txn.store.committed.revisions {
		if v.RevisionFamily == family && v.SubjectKey == subjectKey {
			seen[k] = v
		}
	}
	for k, v := range r.txn.overlay.revisions {
		if v.RevisionFamily == family && v.SubjectKey == subjectKey {
			seen[k] = v
		}
	}
	out := make([]engineering.RevisionEnvelope, 0, len(seen))
	for _, v := range seen {
		out = append(out, cloneRevisionEnvelope(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.String() < out[j].Key.String() })
	return out, nil
}

func mustArtifactKey(rk engineering.RevisionKey) engineering.ArtifactKey {
	return engineering.ArtifactKey{ArtifactID: rk.ArtifactID}
}

// --- Structured content ---

type contentRepo struct {
	ctx context.Context
	txn *transaction
}

func (r contentRepo) Put(_ context.Context, key engineering.RevisionKey, content engineering.CapabilitySpecificationContent) error {
	if err := r.txn.store.countWrite("content"); err != nil {
		return err
	}
	if _, ok := lookup(r.txn.overlay.revisions, r.txn.store.committed.revisions, key); !ok {
		return fmt.Errorf("%w: content for revision %s references a revision that does not exist", application.ErrReferencedValueMissing, key)
	}
	return putCreateOnly(r.txn.overlay.content, r.txn.store.committed.content, key, content,
		func(a, b engineering.CapabilitySpecificationContent) bool { return a.Equal(b) })
}

func (r contentRepo) Get(_ context.Context, key engineering.RevisionKey) (engineering.CapabilitySpecificationContent, bool, error) {
	v, ok := lookup(r.txn.overlay.content, r.txn.store.committed.content, key)
	return v, ok, nil
}

// --- Record envelopes ---

type recordRepo struct {
	ctx context.Context
	txn *transaction
}

func (r recordRepo) Put(_ context.Context, env engineering.RecordEnvelope) error {
	if err := r.txn.store.countWrite("record"); err != nil {
		return err
	}
	if err := r.verifySubject(env); err != nil {
		return err
	}
	env = cloneRecordEnvelope(env)
	return putCreateOnly(r.txn.overlay.records, r.txn.store.committed.records, env.Key, env,
		func(a, b engineering.RecordEnvelope) bool { return a.Equal(b) })
}

// verifySubject resolves env.SubjectKey through the shared parser and
// confirms the artifact or revision it names exists (AD-021). The PostgreSQL
// adapter performs the structurally identical check with a SELECT.
//
// CorrectionTargetID is deliberately NOT verified here. AD-017 already places
// that check on the write side in CorrectValidationClaimCommand, which
// returns the more specific ErrCorrectionTargetMissing, and requires the read
// side -- ResolveCurrentClaim -- to remain total over any stored graph,
// including a dangling, self-referential, or cyclic one, so that resolution
// cannot be poisoned by a payload written by some other means. A third check
// at this layer would duplicate the command-layer one and make the read-side
// guarantee both unreachable and untestable.
func (r recordRepo) verifySubject(env engineering.RecordEnvelope) error {
	kind, artifactID, revisionID, err := engineering.ParseSubjectKey(env.SubjectKey)
	if err != nil {
		return fmt.Errorf("record %s: %w", env.Key, err)
	}
	switch kind {
	case engineering.SubjectKindArtifact:
		if _, ok := lookup(r.txn.overlay.artifacts, r.txn.store.committed.artifacts, engineering.ArtifactKey{ArtifactID: artifactID}); !ok {
			return fmt.Errorf("%w: record %s names artifact %s, which does not exist", application.ErrReferencedValueMissing, env.Key, artifactID)
		}
	case engineering.SubjectKindArtifactRevision:
		key := engineering.RevisionKey{ArtifactID: artifactID, RevisionID: revisionID}
		if _, ok := lookup(r.txn.overlay.revisions, r.txn.store.committed.revisions, key); !ok {
			return fmt.Errorf("%w: record %s names revision %s, which does not exist", application.ErrReferencedValueMissing, env.Key, key)
		}
	}
	return nil
}

func (r recordRepo) Get(_ context.Context, key engineering.RecordKey) (engineering.RecordEnvelope, bool, error) {
	v, ok := lookup(r.txn.overlay.records, r.txn.store.committed.records, key)
	return cloneRecordEnvelope(v), ok, nil
}

func (r recordRepo) ListAll(_ context.Context) ([]engineering.RecordEnvelope, error) {
	seen := map[engineering.RecordKey]engineering.RecordEnvelope{}
	for key, env := range r.txn.store.committed.records {
		seen[key] = env
	}
	for key, env := range r.txn.overlay.records {
		seen[key] = env
	}
	out := make([]engineering.RecordEnvelope, 0, len(seen))
	for _, env := range seen {
		out = append(out, cloneRecordEnvelope(env))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.String() < out[j].Key.String() })
	return out, nil
}

func (r recordRepo) ListByKind(_ context.Context, kind engineering.RecordKind) ([]engineering.RecordEnvelope, error) {
	seen := map[engineering.RecordKey]engineering.RecordEnvelope{}
	for k, v := range r.txn.store.committed.records {
		if k.Kind == kind {
			seen[k] = v
		}
	}
	for k, v := range r.txn.overlay.records {
		if k.Kind == kind {
			seen[k] = v
		}
	}
	out := make([]engineering.RecordEnvelope, 0, len(seen))
	for _, v := range seen {
		out = append(out, cloneRecordEnvelope(v))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.String() < out[j].Key.String() })
	return out, nil
}

func (r recordRepo) ListByKindAndSubject(_ context.Context, kind engineering.RecordKind, subjectKey string) ([]engineering.RecordEnvelope, error) {
	all, err := r.ListByKind(r.ctx, kind)
	if err != nil {
		return nil, err
	}
	out := make([]engineering.RecordEnvelope, 0, len(all))
	for _, v := range all {
		if v.SubjectKey == subjectKey {
			out = append(out, v)
		}
	}
	return out, nil
}

// --- Revision order metadata ---

type orderRepo struct {
	ctx context.Context
	txn *transaction
}

func (r orderRepo) Put(_ context.Context, order engineering.RevisionOrderMetadata) error {
	if err := r.txn.store.countWrite("order"); err != nil {
		return err
	}
	if _, ok := lookup(r.txn.overlay.revisions, r.txn.store.committed.revisions, order.Key); !ok {
		return fmt.Errorf("%w: order metadata for revision %s references a revision that does not exist", application.ErrReferencedValueMissing, order.Key)
	}
	// Sequence uniqueness within the artifact is enforced at write time
	// (FF-010 §3.2): a different RevisionKey claiming the same
	// (ArtifactID, Sequence) pair is a conflict, distinct from the
	// generic same-key conflict rule.
	existingByArtifact, err := r.ListByArtifact(r.ctx, order.Key.ArtifactID)
	if err != nil {
		return err
	}
	for _, existing := range existingByArtifact {
		if existing.Key == order.Key {
			continue
		}
		if existing.Sequence == order.Sequence {
			return fmt.Errorf("%w: sequence %d already assigned to %s within artifact %s",
				application.ErrRevisionSequenceConflict, order.Sequence, existing.Key, order.Key.ArtifactID)
		}
	}
	return putCreateOnly(r.txn.overlay.order, r.txn.store.committed.order, order.Key, order,
		func(a, b engineering.RevisionOrderMetadata) bool { return a == b })
}

func (r orderRepo) Get(_ context.Context, key engineering.RevisionKey) (engineering.RevisionOrderMetadata, bool, error) {
	v, ok := lookup(r.txn.overlay.order, r.txn.store.committed.order, key)
	return v, ok, nil
}

func (r orderRepo) ListByArtifact(_ context.Context, artifactID string) ([]engineering.RevisionOrderMetadata, error) {
	seen := map[engineering.RevisionKey]engineering.RevisionOrderMetadata{}
	for k, v := range r.txn.store.committed.order {
		if k.ArtifactID == artifactID {
			seen[k] = v
		}
	}
	for k, v := range r.txn.overlay.order {
		if k.ArtifactID == artifactID {
			seen[k] = v
		}
	}
	out := make([]engineering.RevisionOrderMetadata, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Sequence < out[j].Sequence })
	return out, nil
}

// --- Requirement-to-criterion traces ---

type requirementTraceRepo struct {
	ctx context.Context
	txn *transaction
}

func (r requirementTraceRepo) Put(_ context.Context, trace engineering.RequirementCriterionTrace) error {
	if err := r.txn.store.countWrite("requirement-trace"); err != nil {
		return err
	}
	if _, ok := lookup(r.txn.overlay.revisions, r.txn.store.committed.revisions, trace.RequirementRevision); !ok {
		return fmt.Errorf("%w: requirement trace references missing requirement revision %s", application.ErrReferencedValueMissing, trace.RequirementRevision)
	}
	if _, ok := lookup(r.txn.overlay.revisions, r.txn.store.committed.revisions, trace.CapabilityRevision); !ok {
		return fmt.Errorf("%w: requirement trace references missing capability revision %s", application.ErrReferencedValueMissing, trace.CapabilityRevision)
	}
	return putCreateOnly(r.txn.overlay.traces, r.txn.store.committed.traces, trace.RequirementRevision, trace,
		func(a, b engineering.RequirementCriterionTrace) bool { return a.Equal(b) })
}

func (r requirementTraceRepo) Get(_ context.Context, key engineering.RevisionKey) (engineering.RequirementCriterionTrace, bool, error) {
	v, ok := lookup(r.txn.overlay.traces, r.txn.store.committed.traces, key)
	return v, ok, nil
}

// --- Lifecycle Definition and Definition Version configuration ---

type lifecycleDefinitionRepo struct {
	ctx context.Context
	txn *transaction
}

func cloneLifecycleDefinition(env engineering.LifecycleDefinitionEnvelope) engineering.LifecycleDefinitionEnvelope {
	env.Payload = append([]byte(nil), env.Payload...)
	return env
}

func cloneLifecycleVersion(env engineering.LifecycleDefinitionVersionEnvelope) engineering.LifecycleDefinitionVersionEnvelope {
	env.Payload = append([]byte(nil), env.Payload...)
	return env
}

func (r lifecycleDefinitionRepo) PutDefinition(_ context.Context, env engineering.LifecycleDefinitionEnvelope) error {
	if err := r.txn.store.countWrite("lifecycle-definition"); err != nil {
		return err
	}
	env = cloneLifecycleDefinition(env)
	return putCreateOnly(r.txn.overlay.lifecycleDefinitions, r.txn.store.committed.lifecycleDefinitions, env.DefinitionID, env,
		func(a, b engineering.LifecycleDefinitionEnvelope) bool { return a.Equal(b) })
}

func (r lifecycleDefinitionRepo) GetDefinition(_ context.Context, definitionID string) (engineering.LifecycleDefinitionEnvelope, bool, error) {
	env, found := lookup(r.txn.overlay.lifecycleDefinitions, r.txn.store.committed.lifecycleDefinitions, definitionID)
	return cloneLifecycleDefinition(env), found, nil
}

func (r lifecycleDefinitionRepo) ListDefinitions(_ context.Context) ([]engineering.LifecycleDefinitionEnvelope, error) {
	seen := make(map[string]engineering.LifecycleDefinitionEnvelope)
	maps.Copy(seen, r.txn.store.committed.lifecycleDefinitions)
	maps.Copy(seen, r.txn.overlay.lifecycleDefinitions)
	out := make([]engineering.LifecycleDefinitionEnvelope, 0, len(seen))
	for _, env := range seen {
		out = append(out, cloneLifecycleDefinition(env))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DefinitionID < out[j].DefinitionID })
	return out, nil
}

func (r lifecycleDefinitionRepo) PutVersion(_ context.Context, env engineering.LifecycleDefinitionVersionEnvelope) error {
	if err := r.txn.store.countWrite("lifecycle-definition-version"); err != nil {
		return err
	}
	if _, found := lookup(r.txn.overlay.lifecycleDefinitions, r.txn.store.committed.lifecycleDefinitions, env.Key.DefinitionID); !found {
		return fmt.Errorf("%w: lifecycle definition version %s names definition %s, which does not exist", application.ErrReferencedValueMissing, env.Key, env.Key.DefinitionID)
	}
	env = cloneLifecycleVersion(env)
	return putCreateOnly(r.txn.overlay.lifecycleVersions, r.txn.store.committed.lifecycleVersions, env.Key, env,
		func(a, b engineering.LifecycleDefinitionVersionEnvelope) bool {
			return a.Equal(b) && a.RecordedAt.Equal(b.RecordedAt)
		})
}

func (r lifecycleDefinitionRepo) GetVersion(_ context.Context, key engineering.LifecycleDefinitionVersionKey) (engineering.LifecycleDefinitionVersionEnvelope, bool, error) {
	env, found := lookup(r.txn.overlay.lifecycleVersions, r.txn.store.committed.lifecycleVersions, key)
	return cloneLifecycleVersion(env), found, nil
}

func (r lifecycleDefinitionRepo) ListVersions(_ context.Context, definitionID string) ([]engineering.LifecycleDefinitionVersionEnvelope, error) {
	seen := make(map[engineering.LifecycleDefinitionVersionKey]engineering.LifecycleDefinitionVersionEnvelope)
	for key, env := range r.txn.store.committed.lifecycleVersions {
		if key.DefinitionID == definitionID {
			seen[key] = env
		}
	}
	for key, env := range r.txn.overlay.lifecycleVersions {
		if key.DefinitionID == definitionID {
			seen[key] = env
		}
	}
	out := make([]engineering.LifecycleDefinitionVersionEnvelope, 0, len(seen))
	for _, env := range seen {
		out = append(out, cloneLifecycleVersion(env))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key.String() < out[j].Key.String() })
	return out, nil
}

// --- Revision acceptance journal ---

type acceptanceRepo struct {
	ctx context.Context
	txn *transaction
}

func (r acceptanceRepo) Append(_ context.Context, record engineering.RevisionAcceptanceRecord) error {
	if err := r.txn.store.countWrite("acceptance"); err != nil {
		return err
	}
	if _, ok := lookup(r.txn.overlay.revisions, r.txn.store.committed.revisions, record.Key); !ok {
		return fmt.Errorf("%w: acceptance record for revision %s references a revision that does not exist", application.ErrReferencedValueMissing, record.Key)
	}
	existing, found, err := r.GetByRecordID(r.ctx, record.RecordID)
	if err != nil {
		return err
	}
	if found {
		if existing == record {
			return nil
		}
		return fmt.Errorf("%w: acceptance record id %s", application.ErrImmutableValueConflict, record.RecordID)
	}
	r.txn.overlay.acceptance[record.Key] = append(r.txn.overlay.acceptance[record.Key], record)
	return nil
}

// GetByRecordID searches the whole journal for an entry with this RecordID.
// FF-009 §4.3 declares RecordID unique and FF-006 §2 derives a timeline
// event's identity from it. More than one match is therefore stored-state
// corruption rather than an arbitrary choice of one record.
func (r acceptanceRepo) GetByRecordID(_ context.Context, recordID string) (engineering.RevisionAcceptanceRecord, bool, error) {
	var match engineering.RevisionAcceptanceRecord
	matches := 0
	for _, entries := range r.txn.store.committed.acceptance {
		for _, e := range entries {
			if e.RecordID == recordID {
				match = e
				matches++
			}
		}
	}
	for _, entries := range r.txn.overlay.acceptance {
		for _, e := range entries {
			if e.RecordID == recordID {
				match = e
				matches++
			}
		}
	}
	switch matches {
	case 0:
		return engineering.RevisionAcceptanceRecord{}, false, nil
	case 1:
		return match, true, nil
	default:
		return engineering.RevisionAcceptanceRecord{}, false,
			fmt.Errorf("%w: acceptance record id %q appears more than once", application.ErrStoredStateIntegrity, recordID)
	}
}

func (r acceptanceRepo) ListByRevision(_ context.Context, key engineering.RevisionKey) ([]engineering.RevisionAcceptanceRecord, error) {
	out := append([]engineering.RevisionAcceptanceRecord(nil), r.txn.store.committed.acceptance[key]...)
	out = append(out, r.txn.overlay.acceptance[key]...)
	sortAcceptance(out)
	return out, nil
}

func (r acceptanceRepo) ListByArtifact(_ context.Context, artifactID string) ([]engineering.RevisionAcceptanceRecord, error) {
	var out []engineering.RevisionAcceptanceRecord
	for k, entries := range r.txn.store.committed.acceptance {
		if k.ArtifactID == artifactID {
			out = append(out, entries...)
		}
	}
	for k, entries := range r.txn.overlay.acceptance {
		if k.ArtifactID == artifactID {
			out = append(out, entries...)
		}
	}
	sortAcceptance(out)
	return out, nil
}

func sortAcceptance(entries []engineering.RevisionAcceptanceRecord) {
	sort.Slice(entries, func(i, j int) bool {
		if !entries[i].EffectiveAt.Equal(entries[j].EffectiveAt) {
			return entries[i].EffectiveAt.Before(entries[j].EffectiveAt)
		}
		return entries[i].RecordID < entries[j].RecordID
	})
}
