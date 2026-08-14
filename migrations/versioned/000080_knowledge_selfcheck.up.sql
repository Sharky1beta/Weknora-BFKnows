CREATE TABLE IF NOT EXISTS knowledge_selfcheck_profiles (
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    purpose VARCHAR(24) NOT NULL DEFAULT 'ignore',
    business_line VARCHAR(255) NOT NULL DEFAULT '',
    project_type VARCHAR(255) NOT NULL DEFAULT '',
    project_stage VARCHAR(255) NOT NULL DEFAULT '',
    owner_name VARCHAR(255) NOT NULL DEFAULT '',
    owner_open_id VARCHAR(255) NOT NULL DEFAULT '',
    im_channel_id VARCHAR(36) NOT NULL DEFAULT '',
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, knowledge_base_id)
);

CREATE TABLE IF NOT EXISTS knowledge_selfcheck_sop_mappings (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    business_line VARCHAR(255) NOT NULL,
    project_type VARCHAR(255) NOT NULL,
    sop_knowledge_base_id VARCHAR(36) NOT NULL,
    sop_version VARCHAR(64) NOT NULL DEFAULT '',
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_selfcheck_mapping_lookup ON knowledge_selfcheck_sop_mappings (tenant_id, business_line, project_type, active);

CREATE TABLE IF NOT EXISTS knowledge_selfcheck_runs (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    project_kb_id VARCHAR(36) NOT NULL,
    project_kb_name VARCHAR(255) NOT NULL DEFAULT '',
    sop_kb_id VARCHAR(36) NOT NULL,
    sop_kb_name VARCHAR(255) NOT NULL DEFAULT '',
    sop_version VARCHAR(64) NOT NULL DEFAULT '',
    status VARCHAR(24) NOT NULL,
    summary JSONB NOT NULL DEFAULT '{}'::JSONB,
    model_id VARCHAR(255) NOT NULL DEFAULT '',
    started_at TIMESTAMP WITH TIME ZONE NOT NULL,
    completed_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_selfcheck_runs_project ON knowledge_selfcheck_runs (tenant_id, project_kb_id, created_at DESC);

CREATE TABLE IF NOT EXISTS knowledge_selfcheck_findings (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    run_id VARCHAR(36) NOT NULL,
    requirement TEXT NOT NULL,
    material TEXT NOT NULL DEFAULT '',
    state VARCHAR(24) NOT NULL,
    evidence TEXT NOT NULL DEFAULT '',
    sop_clause TEXT NOT NULL DEFAULT '',
    suggestion TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_selfcheck_findings_run ON knowledge_selfcheck_findings (tenant_id, run_id, state);
