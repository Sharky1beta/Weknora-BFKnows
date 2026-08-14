ALTER TABLE knowledge_selfcheck_profiles
    ADD COLUMN IF NOT EXISTS sop_knowledge_base_id VARCHAR(36) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sop_version VARCHAR(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_selfcheck_profile_sop
    ON knowledge_selfcheck_profiles (tenant_id, sop_knowledge_base_id);
