-- FeatureForge M.5 revision subject discovery (AD-025, FF-016).
--
-- Additive only: a nullable projection column and an index. No UPDATE, no
-- backfill -- see FF-016 §7 for why a backfill cannot live in SQL (it would
-- require decoding a PEOS payload, which only internal/engineering/peos may
-- do) and why the POC does not need one (no durable database exists yet).
--
-- NULL carries the same meaning §0001 established for provenance_actor and
-- content_digest: the single presence signal for an optional projection.
-- Here NULL also means something positive in its own right (FF-016 §3.3):
-- the revision's family has no capability it is about, not merely an
-- unpopulated field.

ALTER TABLE revision_envelopes ADD COLUMN subject_key text NULL;

-- Serves ListByFamilyAndSubject, mirroring record_envelopes_kind_subject_idx.
CREATE INDEX revision_envelopes_family_subject_idx
    ON revision_envelopes(revision_family, subject_key);
