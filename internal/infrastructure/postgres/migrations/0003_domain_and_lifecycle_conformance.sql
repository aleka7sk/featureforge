-- FF-023 domain/lifecycle conformance migration.
--
-- This first section implements AD-033's structured, product-owned exact
-- source trace for Requirement Revisions. Lifecycle configuration tables are
-- intentionally added to this same migration version by the AD-032 work so
-- the migration loader observes one authoritative version 0003.

CREATE TABLE requirement_criterion_traces (
    requirement_artifact_id     text         NOT NULL,
    requirement_revision_id     text         NOT NULL,
    capability_artifact_id      text         NOT NULL,
    capability_revision_id      text         NOT NULL,
    acceptance_criterion_key    text         NOT NULL,
    recorded_at                 timestamptz  NOT NULL,
    PRIMARY KEY (requirement_artifact_id, requirement_revision_id),
    FOREIGN KEY (requirement_artifact_id, requirement_revision_id)
        REFERENCES revision_envelopes(artifact_id, revision_id),
    FOREIGN KEY (capability_artifact_id, capability_revision_id)
        REFERENCES revision_envelopes(artifact_id, revision_id)
);

CREATE TABLE lifecycle_definitions (
    definition_id  text   PRIMARY KEY,
    payload        bytea  NOT NULL,
    payload_digest text   NOT NULL
);

CREATE TABLE lifecycle_definition_versions (
    definition_id  text         NOT NULL REFERENCES lifecycle_definitions(definition_id),
    version_id     text         NOT NULL,
    payload        bytea        NOT NULL,
    payload_digest text         NOT NULL,
    recorded_at    timestamptz  NOT NULL,
    PRIMARY KEY (definition_id, version_id)
);
