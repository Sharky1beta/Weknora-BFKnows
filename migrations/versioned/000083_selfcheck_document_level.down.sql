DROP INDEX IF EXISTS idx_selfcheck_findings_doc;
ALTER TABLE knowledge_selfcheck_findings
    DROP COLUMN IF EXISTS project_document_id,
    DROP COLUMN IF EXISTS project_document_title,
    DROP COLUMN IF EXISTS sop_document_id,
    DROP COLUMN IF EXISTS sop_document_title;

DROP TABLE IF EXISTS knowledge_selfcheck_document_mappings;
