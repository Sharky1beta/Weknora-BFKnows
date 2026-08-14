CREATE TABLE IF NOT EXISTS knowledge_selfcheck_document_mappings (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    project_kb_id VARCHAR(36) NOT NULL,
    project_document_id VARCHAR(36) NOT NULL,
    project_document_title VARCHAR(255) NOT NULL DEFAULT '',
    sop_kb_id VARCHAR(36) NOT NULL,
    sop_document_id VARCHAR(36) NOT NULL DEFAULT '',
    sop_document_title VARCHAR(255) NOT NULL DEFAULT '',
    match_method VARCHAR(32) NOT NULL DEFAULT 'title',
    match_confidence NUMERIC(5,2) NOT NULL DEFAULT 0.00,
    match_status VARCHAR(24) NOT NULL DEFAULT 'matched',
    project_content_hash VARCHAR(64) NOT NULL DEFAULT '',
    sop_content_hash VARCHAR(64) NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_selfcheck_doc_map ON knowledge_selfcheck_document_mappings (tenant_id, project_kb_id, project_document_id);

ALTER TABLE knowledge_selfcheck_findings
    ADD COLUMN IF NOT EXISTS project_document_id VARCHAR(36) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS project_document_title VARCHAR(255) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sop_document_id VARCHAR(36) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS sop_document_title VARCHAR(255) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_selfcheck_findings_doc ON knowledge_selfcheck_findings (tenant_id, run_id, project_document_id);
