package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aleka7sk/featureforge/internal/application"
	"github.com/aleka7sk/featureforge/internal/domain"
	"github.com/aleka7sk/featureforge/internal/engineering"
	"github.com/jackc/pgx/v5"
)

// reposFor returns an application.Repositories bundle backed by tx. The
// transaction handle travels by being closed over in each repository value,
// exactly as the in-memory adapter's reposFor does -- not through the
// context, which Do's callback signature gives it no way to reach.
func reposFor(tx pgx.Tx) application.Repositories {
	return application.Repositories{
		Projects:           projectRepo{tx: tx},
		FeatureCards:       featureCardRepo{tx: tx},
		Artifacts:          artifactRepo{tx: tx},
		Revisions:          revisionRepo{tx: tx},
		StructuredContent:  contentRepo{tx: tx},
		Records:            recordRepo{tx: tx},
		RevisionOrder:      orderRepo{tx: tx},
		RevisionAcceptance: acceptanceRepo{tx: tx},
	}
}

// exists reports whether a single-row existence query matched.
func exists(ctx context.Context, tx pgx.Tx, sql string, args ...any) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, sql, args...).Scan(&found)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return found, nil
}

// nullableString maps SQL NULL to Go's empty-string-plus-presence-flag
// convention, which the envelope types use for optional projections.
func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func stringOrEmpty(p *string) (string, bool) {
	if p == nil {
		return "", false
	}
	return *p, true
}

func nullableTime(t time.Time, present bool) *time.Time {
	if !present {
		return nil
	}
	return &t
}

func timeOrZero(p *time.Time) (time.Time, bool) {
	if p == nil {
		return time.Time{}, false
	}
	return *p, true
}

// parseDigest rebuilds a Digest from its stored hex form. An empty string is
// the zero Digest, which is how an absent content digest is stored (SQL NULL
// read back through stringOrEmpty).
func parseDigest(hex string) (engineering.Digest, error) {
	if hex == "" {
		return engineering.Digest{}, nil
	}
	return engineering.NewDigest(hex)
}

// --- Projects ---

type projectRepo struct{ tx pgx.Tx }

func (r projectRepo) Put(ctx context.Context, p domain.Project) error {
	// Create-only: an identical re-Put is a no-op and a differing one
	// conflicts. ON CONFLICT DO NOTHING plus an explicit comparison keeps the
	// common idempotent case off the error path entirely, which matters
	// because Do may replay a whole callback after a serialization failure.
	tag, err := r.tx.Exec(ctx, `
        INSERT INTO projects (project_id, name, created_at) VALUES ($1, $2, $3)
        ON CONFLICT (project_id) DO NOTHING`,
		p.ID().String(), p.Name(), p.CreatedAt().UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	existing, found, err := r.Get(ctx, p.ID())
	if err != nil {
		return err
	}
	if found && existing == p {
		return nil
	}
	return fmt.Errorf("%w: project %s", application.ErrImmutableValueConflict, p.ID())
}

func (r projectRepo) Get(ctx context.Context, id domain.ProjectID) (domain.Project, bool, error) {
	var name string
	var createdAt time.Time
	err := r.tx.QueryRow(ctx,
		`SELECT name, created_at FROM projects WHERE project_id = $1`, id.String()).
		Scan(&name, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Project{}, false, nil
	}
	if err != nil {
		return domain.Project{}, false, err
	}
	p, err := domain.NewProject(id, name, createdAt.UTC())
	if err != nil {
		return domain.Project{}, false, err
	}
	return p, true, nil
}

func (r projectRepo) List(ctx context.Context) ([]domain.Project, error) {
	rows, err := r.tx.Query(ctx, `SELECT project_id, name, created_at FROM projects ORDER BY project_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.Project{}
	for rows.Next() {
		var idStr, name string
		var createdAt time.Time
		if err := rows.Scan(&idStr, &name, &createdAt); err != nil {
			return nil, err
		}
		id, err := domain.NewProjectID(idStr)
		if err != nil {
			return nil, err
		}
		p, err := domain.NewProject(id, name, createdAt.UTC())
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- Feature cards ---

type featureCardRepo struct{ tx pgx.Tx }

func (r featureCardRepo) Put(ctx context.Context, c domain.FeatureCard) error {
	// The project reference is a real foreign key, so a missing project
	// surfaces as SQLSTATE 23503, which UnitOfWork.Do maps to
	// ErrReferencedValueMissing -- the same error the in-memory adapter
	// raises from an explicit lookup (AD-021).
	tag, err := r.tx.Exec(ctx, `
        INSERT INTO feature_cards (feature_card_id, project_id, title, description, created_at)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (feature_card_id) DO NOTHING`,
		c.ID().String(), c.ProjectID().String(), c.Title(), c.Description(), c.CreatedAt().UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	existing, found, err := r.Get(ctx, c.ID())
	if err != nil {
		return err
	}
	if found && sameCard(existing, c) {
		return nil
	}
	return fmt.Errorf("%w: feature card %s", application.ErrImmutableValueConflict, c.ID())
}

func sameCard(a, b domain.FeatureCard) bool {
	aLink, aOK := a.CapabilityArtifactID()
	bLink, bOK := b.CapabilityArtifactID()
	return a.ID() == b.ID() && a.ProjectID() == b.ProjectID() && a.Title() == b.Title() &&
		a.Description() == b.Description() && a.CreatedAt().Equal(b.CreatedAt()) &&
		aOK == bOK && aLink == bLink
}

func (r featureCardRepo) Get(ctx context.Context, id domain.FeatureCardID) (domain.FeatureCard, bool, error) {
	var projectID, title, description string
	var createdAt time.Time
	var link *string
	err := r.tx.QueryRow(ctx, `
        SELECT c.project_id, c.title, c.description, c.created_at, l.artifact_id
        FROM feature_cards c
        LEFT JOIN feature_card_capability_links l ON l.feature_card_id = c.feature_card_id
        WHERE c.feature_card_id = $1`, id.String()).
		Scan(&projectID, &title, &description, &createdAt, &link)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FeatureCard{}, false, nil
	}
	if err != nil {
		return domain.FeatureCard{}, false, err
	}
	return buildCard(id, projectID, title, description, createdAt, link)
}

func buildCard(id domain.FeatureCardID, projectID, title, description string, createdAt time.Time, link *string) (domain.FeatureCard, bool, error) {
	pid, err := domain.NewProjectID(projectID)
	if err != nil {
		return domain.FeatureCard{}, false, err
	}
	card, err := domain.NewFeatureCard(id, pid, title, description, createdAt.UTC())
	if err != nil {
		return domain.FeatureCard{}, false, err
	}
	if artifactID, linked := stringOrEmpty(link); linked {
		card, err = card.WithCapabilityArtifactID(artifactID)
		if err != nil {
			return domain.FeatureCard{}, false, err
		}
	}
	return card, true, nil
}

func (r featureCardRepo) ListByProject(ctx context.Context, projectID domain.ProjectID) ([]domain.FeatureCard, error) {
	rows, err := r.tx.Query(ctx, `
        SELECT c.feature_card_id, c.project_id, c.title, c.description, c.created_at, l.artifact_id
        FROM feature_cards c
        LEFT JOIN feature_card_capability_links l ON l.feature_card_id = c.feature_card_id
        WHERE c.project_id = $1
        ORDER BY c.feature_card_id`, projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []domain.FeatureCard{}
	for rows.Next() {
		var idStr, pidStr, title, description string
		var createdAt time.Time
		var link *string
		if err := rows.Scan(&idStr, &pidStr, &title, &description, &createdAt, &link); err != nil {
			return nil, err
		}
		id, err := domain.NewFeatureCardID(idStr)
		if err != nil {
			return nil, err
		}
		card, _, err := buildCard(id, pidStr, title, description, createdAt, link)
		if err != nil {
			return nil, err
		}
		out = append(out, card)
	}
	return out, rows.Err()
}

func (r featureCardRepo) LinkCapability(ctx context.Context, id domain.FeatureCardID, artifactID string) error {
	tag, err := r.tx.Exec(ctx, `
        INSERT INTO feature_card_capability_links (feature_card_id, artifact_id) VALUES ($1, $2)
        ON CONFLICT (feature_card_id) DO NOTHING`, id.String(), artifactID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	var existing string
	if err := r.tx.QueryRow(ctx,
		`SELECT artifact_id FROM feature_card_capability_links WHERE feature_card_id = $1`, id.String()).
		Scan(&existing); err != nil {
		return err
	}
	if existing == artifactID {
		return nil
	}
	return fmt.Errorf("%w: feature card %s already linked to %s", application.ErrCapabilityAlreadyLinked, id, existing)
}

// --- Artifact envelopes ---

type artifactRepo struct{ tx pgx.Tx }

func (r artifactRepo) Put(ctx context.Context, env engineering.ArtifactEnvelope) error {
	tag, err := r.tx.Exec(ctx, `
        INSERT INTO artifact_envelopes (artifact_id, artifact_type, payload, payload_digest, recorded_at)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (artifact_id) DO NOTHING`,
		env.Key.ArtifactID, env.ArtifactType, env.Payload, env.PayloadDigest.String(), env.RecordedAt.UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	existing, found, err := r.Get(ctx, env.Key)
	if err != nil {
		return err
	}
	if found && existing.Equal(env) {
		return nil
	}
	return fmt.Errorf("%w: artifact %s", application.ErrImmutableValueConflict, env.Key)
}

func (r artifactRepo) Get(ctx context.Context, key engineering.ArtifactKey) (engineering.ArtifactEnvelope, bool, error) {
	var artifactType, digest string
	var payload []byte
	var recordedAt time.Time
	err := r.tx.QueryRow(ctx, `
        SELECT artifact_type, payload, payload_digest, recorded_at
        FROM artifact_envelopes WHERE artifact_id = $1`, key.ArtifactID).
		Scan(&artifactType, &payload, &digest, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return engineering.ArtifactEnvelope{}, false, nil
	}
	if err != nil {
		return engineering.ArtifactEnvelope{}, false, err
	}
	payloadDigest, err := parseDigest(digest)
	if err != nil {
		return engineering.ArtifactEnvelope{}, false, err
	}
	env, err := engineering.NewArtifactEnvelope(key, artifactType, payload, payloadDigest, recordedAt.UTC())
	if err != nil {
		return engineering.ArtifactEnvelope{}, false, err
	}
	return env, true, nil
}

// --- Revision envelopes ---

type revisionRepo struct{ tx pgx.Tx }

func (r revisionRepo) Put(ctx context.Context, env engineering.RevisionEnvelope) error {
	contentDigest := env.ContentDigest.String()
	tag, err := r.tx.Exec(ctx, `
        INSERT INTO revision_envelopes (
            artifact_id, revision_id, revision_family, artifact_type, integrity_value,
            provenance_actor, provenance_recorded_at, content_digest,
            payload, payload_digest, recorded_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
        ON CONFLICT (artifact_id, revision_id) DO NOTHING`,
		env.Key.ArtifactID, env.Key.RevisionID, string(env.RevisionFamily), env.ArtifactType, env.IntegrityValue,
		nullableString(actorIfPresent(env)), nullableTime(env.ProvenanceRecordedAt, env.HasProvenanceTime),
		nullableString(contentDigest),
		env.Payload, env.PayloadDigest.String(), env.RecordedAt.UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	existing, found, err := r.Get(ctx, env.Key)
	if err != nil {
		return err
	}
	if found && existing.Equal(env) {
		return nil
	}
	return fmt.Errorf("%w: revision %s", application.ErrImmutableValueConflict, env.Key)
}

func actorIfPresent(env engineering.RevisionEnvelope) string {
	if !env.HasProvenanceActor {
		return ""
	}
	return env.ProvenanceActor
}

func (r revisionRepo) Get(ctx context.Context, key engineering.RevisionKey) (engineering.RevisionEnvelope, bool, error) {
	rows, err := r.tx.Query(ctx, `
        SELECT artifact_id, revision_id, revision_family, artifact_type, integrity_value,
               provenance_actor, provenance_recorded_at, content_digest,
               payload, payload_digest, recorded_at
        FROM revision_envelopes WHERE artifact_id = $1 AND revision_id = $2`,
		key.ArtifactID, key.RevisionID)
	if err != nil {
		return engineering.RevisionEnvelope{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return engineering.RevisionEnvelope{}, false, rows.Err()
	}
	env, err := scanRevision(rows)
	if err != nil {
		return engineering.RevisionEnvelope{}, false, err
	}
	return env, true, rows.Err()
}

func (r revisionRepo) ListByArtifact(ctx context.Context, artifactID string) ([]engineering.RevisionEnvelope, error) {
	rows, err := r.tx.Query(ctx, `
        SELECT artifact_id, revision_id, revision_family, artifact_type, integrity_value,
               provenance_actor, provenance_recorded_at, content_digest,
               payload, payload_digest, recorded_at
        FROM revision_envelopes WHERE artifact_id = $1
        ORDER BY artifact_id, revision_id`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []engineering.RevisionEnvelope{}
	for rows.Next() {
		env, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, env)
	}
	return out, rows.Err()
}

func scanRevision(rows pgx.Rows) (engineering.RevisionEnvelope, error) {
	var artifactID, revisionID, family, artifactType, integrity, digest string
	var actor, contentDigest *string
	var provenanceAt *time.Time
	var payload []byte
	var recordedAt time.Time
	if err := rows.Scan(&artifactID, &revisionID, &family, &artifactType, &integrity,
		&actor, &provenanceAt, &contentDigest, &payload, &digest, &recordedAt); err != nil {
		return engineering.RevisionEnvelope{}, err
	}
	key, err := engineering.NewRevisionKey(artifactID, revisionID)
	if err != nil {
		return engineering.RevisionEnvelope{}, err
	}
	actorValue, hasActor := stringOrEmpty(actor)
	provenanceValue, hasProvenance := timeOrZero(provenanceAt)
	contentValue, _ := stringOrEmpty(contentDigest)
	contentParsed, err := parseDigest(contentValue)
	if err != nil {
		return engineering.RevisionEnvelope{}, err
	}
	payloadDigest, err := parseDigest(digest)
	if err != nil {
		return engineering.RevisionEnvelope{}, err
	}
	return engineering.NewRevisionEnvelope(engineering.RevisionEnvelopeInput{
		Key:                  key,
		RevisionFamily:       engineering.RevisionFamily(family),
		ArtifactType:         artifactType,
		IntegrityValue:       integrity,
		ProvenanceActor:      actorValue,
		HasProvenanceActor:   hasActor,
		ProvenanceRecordedAt: provenanceValue.UTC(),
		HasProvenanceTime:    hasProvenance,
		ContentDigest:        contentParsed,
		Payload:              payload,
		PayloadDigest:        payloadDigest,
		RecordedAt:           recordedAt.UTC(),
	})
}

// --- Structured content ---

type contentRepo struct{ tx pgx.Tx }

func (r contentRepo) Put(ctx context.Context, key engineering.RevisionKey, content engineering.CapabilitySpecificationContent) error {
	encoded, err := content.CanonicalJSON()
	if err != nil {
		return err
	}
	tag, err := r.tx.Exec(ctx, `
        INSERT INTO structured_content (artifact_id, revision_id, content) VALUES ($1, $2, $3)
        ON CONFLICT (artifact_id, revision_id) DO NOTHING`,
		key.ArtifactID, key.RevisionID, encoded)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	existing, found, err := r.Get(ctx, key)
	if err != nil {
		return err
	}
	if found && existing.Equal(content) {
		return nil
	}
	return fmt.Errorf("%w: content for revision %s", application.ErrImmutableValueConflict, key)
}

func (r contentRepo) Get(ctx context.Context, key engineering.RevisionKey) (engineering.CapabilitySpecificationContent, bool, error) {
	var encoded []byte
	err := r.tx.QueryRow(ctx,
		`SELECT content FROM structured_content WHERE artifact_id = $1 AND revision_id = $2`,
		key.ArtifactID, key.RevisionID).Scan(&encoded)
	if errors.Is(err, pgx.ErrNoRows) {
		return engineering.CapabilitySpecificationContent{}, false, nil
	}
	if err != nil {
		return engineering.CapabilitySpecificationContent{}, false, err
	}
	content, err := engineering.ParseCapabilitySpecificationContent(encoded)
	if err != nil {
		return engineering.CapabilitySpecificationContent{}, false, err
	}
	return content, true, nil
}

// --- Record envelopes ---

type recordRepo struct{ tx pgx.Tx }

func (r recordRepo) Put(ctx context.Context, env engineering.RecordEnvelope) error {
	if err := r.verifySubject(ctx, env); err != nil {
		return err
	}
	tag, err := r.tx.Exec(ctx, `
        INSERT INTO record_envelopes (
            kind, id, subject_key, scope, occurred_at, outcome,
            criterion_keys, evidence_keys, execution_keys,
            correction_kind, correction_target_id, state_id,
            payload, payload_digest, recorded_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
        ON CONFLICT (kind, id) DO NOTHING`,
		string(env.Key.Kind), env.Key.ID, env.SubjectKey, env.Scope,
		nullableTime(env.OccurredAt, env.HasOccurredAt), env.Outcome,
		nonNilStrings(env.CriterionKeys), nonNilStrings(env.EvidenceKeys), nonNilStrings(env.ExecutionKeys),
		env.CorrectionKind, env.CorrectionTargetID, env.StateID,
		env.Payload, env.PayloadDigest.String(), env.RecordedAt.UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	existing, found, err := r.Get(ctx, env.Key)
	if err != nil {
		return err
	}
	if found && existing.Equal(env) {
		return nil
	}
	return fmt.Errorf("%w: record %s", application.ErrImmutableValueConflict, env.Key)
}

// verifySubject resolves env.SubjectKey through the shared parser and
// confirms the artifact or revision it names exists, inside the same
// transaction as the insert (AD-021).
//
// SubjectKey is stored as one column rather than being split into
// foreign-key-able parts: it is already the canonical identifier, and
// denormalizing it purely so PostgreSQL could enforce it would mean keeping
// two representations of one relationship in agreement. This check is
// structurally identical to the in-memory adapter's, which is what keeps the
// two adapters rejecting the same inputs.
//
// CorrectionTargetID is deliberately not verified. AD-017 places that check
// on the write side in CorrectValidationClaimCommand, which reports the more
// specific ErrCorrectionTargetMissing, and requires ResolveCurrentClaim to
// stay total over any stored graph -- dangling, self-referential, or cyclic
// included. Enforcing it here too would duplicate the command-layer check and
// make that read-side guarantee unreachable.
func (r recordRepo) verifySubject(ctx context.Context, env engineering.RecordEnvelope) error {
	kind, artifactID, revisionID, err := engineering.ParseSubjectKey(env.SubjectKey)
	if err != nil {
		return fmt.Errorf("record %s: %w", env.Key, err)
	}
	switch kind {
	case engineering.SubjectKindArtifact:
		found, err := exists(ctx, r.tx,
			`SELECT true FROM artifact_envelopes WHERE artifact_id = $1`, artifactID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: record %s names artifact %s, which does not exist", application.ErrReferencedValueMissing, env.Key, artifactID)
		}
	case engineering.SubjectKindArtifactRevision:
		found, err := exists(ctx, r.tx,
			`SELECT true FROM revision_envelopes WHERE artifact_id = $1 AND revision_id = $2`, artifactID, revisionID)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: record %s names revision %s/%s, which does not exist", application.ErrReferencedValueMissing, env.Key, artifactID, revisionID)
		}
	}
	return nil
}

// nonNilStrings normalizes a nil slice to an empty one so a NOT NULL text[]
// column never receives NULL.
func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func (r recordRepo) Get(ctx context.Context, key engineering.RecordKey) (engineering.RecordEnvelope, bool, error) {
	rows, err := r.tx.Query(ctx, recordSelect+` WHERE kind = $1 AND id = $2`, string(key.Kind), key.ID)
	if err != nil {
		return engineering.RecordEnvelope{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return engineering.RecordEnvelope{}, false, rows.Err()
	}
	env, err := scanRecord(rows)
	if err != nil {
		return engineering.RecordEnvelope{}, false, err
	}
	return env, true, rows.Err()
}

func (r recordRepo) ListByKind(ctx context.Context, kind engineering.RecordKind) ([]engineering.RecordEnvelope, error) {
	rows, err := r.tx.Query(ctx, recordSelect+` WHERE kind = $1 ORDER BY kind, id`, string(kind))
	if err != nil {
		return nil, err
	}
	return collectRecords(rows)
}

func (r recordRepo) ListByKindAndSubject(ctx context.Context, kind engineering.RecordKind, subjectKey string) ([]engineering.RecordEnvelope, error) {
	rows, err := r.tx.Query(ctx,
		recordSelect+` WHERE kind = $1 AND subject_key = $2 ORDER BY kind, id`, string(kind), subjectKey)
	if err != nil {
		return nil, err
	}
	return collectRecords(rows)
}

const recordSelect = `
    SELECT kind, id, subject_key, scope, occurred_at, outcome,
           criterion_keys, evidence_keys, execution_keys,
           correction_kind, correction_target_id, state_id,
           payload, payload_digest, recorded_at
    FROM record_envelopes`

func collectRecords(rows pgx.Rows) ([]engineering.RecordEnvelope, error) {
	defer rows.Close()
	out := []engineering.RecordEnvelope{}
	for rows.Next() {
		env, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, env)
	}
	return out, rows.Err()
}

func scanRecord(rows pgx.Rows) (engineering.RecordEnvelope, error) {
	var kind, id, subjectKey, scope, outcome, correctionKind, correctionTarget, stateID, digest string
	var criteria, evidence, executions []string
	var occurredAt *time.Time
	var payload []byte
	var recordedAt time.Time
	if err := rows.Scan(&kind, &id, &subjectKey, &scope, &occurredAt, &outcome,
		&criteria, &evidence, &executions, &correctionKind, &correctionTarget, &stateID,
		&payload, &digest, &recordedAt); err != nil {
		return engineering.RecordEnvelope{}, err
	}
	key, err := engineering.NewRecordKey(engineering.RecordKind(kind), id)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	occurred, hasOccurred := timeOrZero(occurredAt)
	payloadDigest, err := parseDigest(digest)
	if err != nil {
		return engineering.RecordEnvelope{}, err
	}
	return engineering.NewRecordEnvelope(engineering.RecordEnvelopeInput{
		Key:                key,
		SubjectKey:         subjectKey,
		Scope:              scope,
		OccurredAt:         occurred.UTC(),
		HasOccurredAt:      hasOccurred,
		Outcome:            outcome,
		CriterionKeys:      criteria,
		EvidenceKeys:       evidence,
		ExecutionKeys:      executions,
		CorrectionKind:     correctionKind,
		CorrectionTargetID: correctionTarget,
		StateID:            stateID,
		Payload:            payload,
		PayloadDigest:      payloadDigest,
		RecordedAt:         recordedAt.UTC(),
	})
}

// --- Revision order metadata ---

type orderRepo struct{ tx pgx.Tx }

func (r orderRepo) Put(ctx context.Context, order engineering.RevisionOrderMetadata) error {
	tag, err := r.tx.Exec(ctx, `
        INSERT INTO revision_order (artifact_id, revision_id, sequence, recorded_at)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (artifact_id, revision_id) DO NOTHING`,
		order.Key.ArtifactID, order.Key.RevisionID, order.Sequence, order.RecordedAt.UTC())
	if err != nil {
		// A duplicate (artifact_id, sequence) is a sequence conflict, not a
		// key conflict: two different revisions claimed the same position.
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	existing, found, err := r.Get(ctx, order.Key)
	if err != nil {
		return err
	}
	if found && existing.Key == order.Key && existing.Sequence == order.Sequence &&
		existing.RecordedAt.Equal(order.RecordedAt) {
		return nil
	}
	return fmt.Errorf("%w: order metadata for revision %s", application.ErrImmutableValueConflict, order.Key)
}

func (r orderRepo) Get(ctx context.Context, key engineering.RevisionKey) (engineering.RevisionOrderMetadata, bool, error) {
	var sequence int
	var recordedAt time.Time
	err := r.tx.QueryRow(ctx,
		`SELECT sequence, recorded_at FROM revision_order WHERE artifact_id = $1 AND revision_id = $2`,
		key.ArtifactID, key.RevisionID).Scan(&sequence, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return engineering.RevisionOrderMetadata{}, false, nil
	}
	if err != nil {
		return engineering.RevisionOrderMetadata{}, false, err
	}
	order, err := engineering.NewRevisionOrderMetadata(key, sequence, recordedAt.UTC())
	if err != nil {
		return engineering.RevisionOrderMetadata{}, false, err
	}
	return order, true, nil
}

func (r orderRepo) ListByArtifact(ctx context.Context, artifactID string) ([]engineering.RevisionOrderMetadata, error) {
	rows, err := r.tx.Query(ctx, `
        SELECT artifact_id, revision_id, sequence, recorded_at
        FROM revision_order WHERE artifact_id = $1
        ORDER BY artifact_id, revision_id`, artifactID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []engineering.RevisionOrderMetadata{}
	for rows.Next() {
		var aID, rID string
		var sequence int
		var recordedAt time.Time
		if err := rows.Scan(&aID, &rID, &sequence, &recordedAt); err != nil {
			return nil, err
		}
		key, err := engineering.NewRevisionKey(aID, rID)
		if err != nil {
			return nil, err
		}
		order, err := engineering.NewRevisionOrderMetadata(key, sequence, recordedAt.UTC())
		if err != nil {
			return nil, err
		}
		out = append(out, order)
	}
	return out, rows.Err()
}

// --- Revision acceptance journal ---

type acceptanceRepo struct{ tx pgx.Tx }

func (r acceptanceRepo) Append(ctx context.Context, record engineering.RevisionAcceptanceRecord) error {
	// Create-only on RecordID (AD-021): FF-009 §4.3 declares it unique and
	// FF-006 §2 derives a timeline event's identity from it.
	tag, err := r.tx.Exec(ctx, `
        INSERT INTO revision_acceptance (record_id, artifact_id, revision_id, state, effective_at, actor, reason)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
        ON CONFLICT (record_id) DO NOTHING`,
		record.RecordID, record.Key.ArtifactID, record.Key.RevisionID,
		string(record.State), record.EffectiveAt.UTC(), record.Actor, record.Reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 1 {
		return nil
	}
	existing, found, err := r.getByRecordID(ctx, record.RecordID)
	if err != nil {
		return err
	}
	if found && sameAcceptance(existing, record) {
		return nil
	}
	return fmt.Errorf("%w: acceptance record id %s", application.ErrImmutableValueConflict, record.RecordID)
}

func sameAcceptance(a, b engineering.RevisionAcceptanceRecord) bool {
	return a.RecordID == b.RecordID && a.Key == b.Key && a.State == b.State &&
		a.EffectiveAt.Equal(b.EffectiveAt) && a.Actor == b.Actor && a.Reason == b.Reason
}

func (r acceptanceRepo) getByRecordID(ctx context.Context, recordID string) (engineering.RevisionAcceptanceRecord, bool, error) {
	rows, err := r.tx.Query(ctx, acceptanceSelect+` WHERE record_id = $1`, recordID)
	if err != nil {
		return engineering.RevisionAcceptanceRecord{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return engineering.RevisionAcceptanceRecord{}, false, rows.Err()
	}
	rec, err := scanAcceptance(rows)
	if err != nil {
		return engineering.RevisionAcceptanceRecord{}, false, err
	}
	return rec, true, rows.Err()
}

func (r acceptanceRepo) ListByRevision(ctx context.Context, key engineering.RevisionKey) ([]engineering.RevisionAcceptanceRecord, error) {
	rows, err := r.tx.Query(ctx, acceptanceSelect+`
        WHERE artifact_id = $1 AND revision_id = $2
        ORDER BY effective_at, record_id`, key.ArtifactID, key.RevisionID)
	if err != nil {
		return nil, err
	}
	return collectAcceptance(rows)
}

func (r acceptanceRepo) ListByArtifact(ctx context.Context, artifactID string) ([]engineering.RevisionAcceptanceRecord, error) {
	rows, err := r.tx.Query(ctx, acceptanceSelect+`
        WHERE artifact_id = $1
        ORDER BY effective_at, record_id`, artifactID)
	if err != nil {
		return nil, err
	}
	return collectAcceptance(rows)
}

const acceptanceSelect = `
    SELECT record_id, artifact_id, revision_id, state, effective_at, actor, reason
    FROM revision_acceptance`

func collectAcceptance(rows pgx.Rows) ([]engineering.RevisionAcceptanceRecord, error) {
	defer rows.Close()
	var out []engineering.RevisionAcceptanceRecord
	for rows.Next() {
		rec, err := scanAcceptance(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func scanAcceptance(rows pgx.Rows) (engineering.RevisionAcceptanceRecord, error) {
	var recordID, artifactID, revisionID, state, actor, reason string
	var effectiveAt time.Time
	if err := rows.Scan(&recordID, &artifactID, &revisionID, &state, &effectiveAt, &actor, &reason); err != nil {
		return engineering.RevisionAcceptanceRecord{}, err
	}
	key, err := engineering.NewRevisionKey(artifactID, revisionID)
	if err != nil {
		return engineering.RevisionAcceptanceRecord{}, err
	}
	return engineering.NewRevisionAcceptanceRecord(
		recordID, key, engineering.AcceptanceState(state), effectiveAt.UTC(), actor, reason)
}
