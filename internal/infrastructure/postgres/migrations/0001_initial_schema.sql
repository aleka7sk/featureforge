-- FeatureForge M.4 initial schema (FF-014).
--
-- Every table below stores exactly what the corresponding FeatureForge value
-- carries -- no derived state, no materialized current-revision, readiness, or
-- lifecycle answer. Those remain computed queries in internal/application
-- (AD-006).
--
-- Payload columns are bytea, not jsonb: every idempotency, conflict, and
-- digest check in this codebase compares canonical JSON as exact bytes, and
-- jsonb reparses and reserializes on write with no byte-identity guarantee.
-- Nothing ever queries inside a payload -- each field a query needs is already
-- a separate projected column (AD-020).
--
-- A Has* boolean pair in the Go struct becomes one nullable column here, with
-- SQL NULL as the single presence signal, so two columns can never disagree.

-- schema_migrations is deliberately absent here: the migration runner owns it
-- and bootstraps it before applying any migration, so it is runner
-- infrastructure rather than schema content.

-- domain.Project
CREATE TABLE projects (
    project_id  text         PRIMARY KEY,
    name        text         NOT NULL,
    created_at  timestamptz  NOT NULL
);

-- domain.FeatureCard. The project reference is a plain single-column foreign
-- key, enforced here and by an explicit lookup in the in-memory adapter so
-- both reject the same input (AD-021).
CREATE TABLE feature_cards (
    feature_card_id  text         PRIMARY KEY,
    project_id       text         NOT NULL REFERENCES projects(project_id),
    title            text         NOT NULL,
    description      text         NOT NULL DEFAULT '',
    created_at       timestamptz  NOT NULL
);
CREATE INDEX feature_cards_project_idx ON feature_cards(project_id);

-- A FeatureCard's capability link, completed exactly once after the card
-- exists (FeatureCardRepository.LinkCapability). Held separately rather than
-- as a nullable column on feature_cards so the card row itself stays strictly
-- insert-only.
CREATE TABLE feature_card_capability_links (
    feature_card_id  text  PRIMARY KEY REFERENCES feature_cards(feature_card_id),
    artifact_id      text  NOT NULL
);

-- engineering.ArtifactEnvelope
CREATE TABLE artifact_envelopes (
    artifact_id     text         PRIMARY KEY,
    artifact_type   text         NOT NULL,
    payload         bytea        NOT NULL,
    payload_digest  text         NOT NULL,
    recorded_at     timestamptz  NOT NULL
);

-- engineering.RevisionEnvelope
CREATE TABLE revision_envelopes (
    artifact_id             text         NOT NULL REFERENCES artifact_envelopes(artifact_id),
    revision_id             text         NOT NULL,
    revision_family         text         NOT NULL,
    artifact_type           text         NOT NULL,
    integrity_value         text         NOT NULL,
    provenance_actor        text         NULL,  -- NULL means HasProvenanceActor is false
    provenance_recorded_at  timestamptz  NULL,  -- NULL means HasProvenanceTime is false
    content_digest          text         NULL,  -- NULL means the digest is zero
    payload                 bytea        NOT NULL,
    payload_digest          text         NOT NULL,
    recorded_at             timestamptz  NOT NULL,
    PRIMARY KEY (artifact_id, revision_id)
);

-- engineering.CapabilitySpecificationContent, stored as its canonical JSON.
-- CanonicalJSON and ParseCapabilitySpecificationContent already round-trip
-- exactly, and no query reads inside it, so no field-level columns are needed.
CREATE TABLE structured_content (
    artifact_id  text   NOT NULL,
    revision_id  text   NOT NULL,
    content      bytea  NOT NULL,
    PRIMARY KEY (artifact_id, revision_id),
    FOREIGN KEY (artifact_id, revision_id) REFERENCES revision_envelopes(artifact_id, revision_id)
);

-- engineering.RecordEnvelope: decisions, executions, claims, state assignments.
--
-- subject_key and correction_target_id are the single stored representation of
-- those relationships. They are deliberately NOT split into denormalized
-- foreign-key columns (subject_artifact_id, subject_revision_*, and so on):
-- SubjectKey is already the canonical identifier, and materializing it into
-- extra columns purely so PostgreSQL could enforce it would mean maintaining
-- two representations of one relationship with no query or performance need
-- to justify it. Existence is verified instead by recordRepo.Put, which parses
-- the key through engineering.ParseSubjectKey and SELECTs the referenced row
-- inside the same transaction as the insert -- structurally the same check the
-- in-memory adapter performs (AD-021).
--
-- criterion_keys, evidence_keys, and execution_keys are plain text[] with no
-- GIN index: every reader (query_correction.go, query_readiness.go) lists by
-- kind or by kind-and-subject first and then filters in Go, so no SQL
-- predicate ever searches inside these arrays.
CREATE TABLE record_envelopes (
    kind                  text         NOT NULL,
    id                    text         NOT NULL,
    subject_key           text         NOT NULL,
    scope                 text         NOT NULL DEFAULT '',
    occurred_at           timestamptz  NULL,  -- NULL means HasOccurredAt is false
    outcome               text         NOT NULL DEFAULT '',
    criterion_keys        text[]       NOT NULL DEFAULT '{}',
    evidence_keys         text[]       NOT NULL DEFAULT '{}',
    execution_keys        text[]       NOT NULL DEFAULT '{}',
    correction_kind       text         NOT NULL DEFAULT '',
    correction_target_id  text         NOT NULL DEFAULT '',
    state_id              text         NOT NULL DEFAULT '',
    payload               bytea        NOT NULL,
    payload_digest        text         NOT NULL,
    recorded_at           timestamptz  NOT NULL,
    PRIMARY KEY (kind, id)
);
-- Serves ListByKind (leading-column prefix scan) and ListByKindAndSubject.
CREATE INDEX record_envelopes_kind_subject_idx ON record_envelopes(kind, subject_key);

-- engineering.RevisionOrderMetadata: the product-owned sequence only, never an
-- acceptance flag (AD-015). The unique constraint is what makes two concurrent
-- revision creations for one artifact resolve to distinct sequences.
CREATE TABLE revision_order (
    artifact_id  text         NOT NULL,
    revision_id  text         NOT NULL,
    sequence     integer      NOT NULL,
    recorded_at  timestamptz  NOT NULL,
    PRIMARY KEY (artifact_id, revision_id),
    FOREIGN KEY (artifact_id, revision_id) REFERENCES revision_envelopes(artifact_id, revision_id),
    CONSTRAINT revision_order_artifact_sequence_key UNIQUE (artifact_id, sequence)
);

-- engineering.RevisionAcceptanceRecord: the append-only acceptance journal.
--
-- The primary key is a surrogate bigserial because this is an internal journal
-- rather than an externally referenceable entity: the repository exposes only
-- Append, ListByRevision, and ListByArtifact -- never a lookup by identity --
-- and no foreign key anywhere targets it.
--
-- record_id is nonetheless unique. FF-009 section 4.3 declares it a
-- product-owned unique identity, and FF-006 section 2 derives a timeline
-- event's identity from it, so two entries sharing one record_id would collide
-- two timeline events onto one event id (AD-021).
CREATE TABLE revision_acceptance (
    id            bigserial    PRIMARY KEY,
    record_id     text         NOT NULL,
    artifact_id   text         NOT NULL,
    revision_id   text         NOT NULL,
    state         text         NOT NULL,
    effective_at  timestamptz  NOT NULL,
    actor         text         NOT NULL,
    reason        text         NOT NULL DEFAULT '',
    FOREIGN KEY (artifact_id, revision_id) REFERENCES revision_envelopes(artifact_id, revision_id),
    CONSTRAINT revision_acceptance_record_id_key UNIQUE (record_id)
);
-- Both list methods order by (effective_at, record_id), the total order
-- resolveAcceptanceState relies on.
CREATE INDEX revision_acceptance_revision_idx ON revision_acceptance(artifact_id, revision_id, effective_at, record_id);
CREATE INDEX revision_acceptance_artifact_idx ON revision_acceptance(artifact_id, effective_at, record_id);
