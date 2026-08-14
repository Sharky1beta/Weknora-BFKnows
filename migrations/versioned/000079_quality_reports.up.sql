-- Migration 000079: tenant quality-report configuration and generated history.
CREATE TABLE IF NOT EXISTS quality_report_configs (
    tenant_id BIGINT PRIMARY KEY,
    enabled BOOLEAN NOT NULL DEFAULT FALSE,
    window_days INT NOT NULL DEFAULT 7,
    model_id VARCHAR(64) NOT NULL DEFAULT '',
    schedule VARCHAR(32) NOT NULL DEFAULT 'weekly_monday_0900',
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
    im_channel_id VARCHAR(36) NOT NULL DEFAULT '',
    target_chat_id VARCHAR(255) NOT NULL DEFAULT '',
    target_chat_name VARCHAR(255) NOT NULL DEFAULT '',
    last_run_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS quality_reports (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    window_from TIMESTAMP WITH TIME ZONE NOT NULL,
    window_to TIMESTAMP WITH TIME ZONE NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'generated',
    model_id VARCHAR(64) NOT NULL DEFAULT '',
    report JSONB NOT NULL DEFAULT '{}'::JSONB,
    sent_at TIMESTAMP WITH TIME ZONE,
    send_error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_quality_reports_tenant_created
    ON quality_reports (tenant_id, created_at DESC);
