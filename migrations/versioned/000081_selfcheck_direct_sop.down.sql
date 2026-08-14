DROP INDEX IF EXISTS idx_selfcheck_profile_sop;
ALTER TABLE knowledge_selfcheck_profiles
    DROP COLUMN IF EXISTS sop_version,
    DROP COLUMN IF EXISTS sop_knowledge_base_id;
