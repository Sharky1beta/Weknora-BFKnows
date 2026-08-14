CREATE TABLE IF NOT EXISTS knowledge_selfcheck_sop_deliverables (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    sop_kb_id VARCHAR(36) NOT NULL,
    sop_document_id VARCHAR(36) NOT NULL,
    sop_content_hash VARCHAR(64) NOT NULL,
    prompt_version VARCHAR(64) NOT NULL,
    status VARCHAR(24) NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    requirements JSONB NOT NULL DEFAULT '[]'::JSONB,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_selfcheck_sop_deliverable_cache
    ON knowledge_selfcheck_sop_deliverables (tenant_id, sop_kb_id, sop_document_id, sop_content_hash, prompt_version);
