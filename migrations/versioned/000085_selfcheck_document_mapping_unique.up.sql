-- Keep the first authoritative binding for each tenant/document pair before
-- enforcing the invariant at the database level. This only removes duplicate
-- binding rows; unrelated mappings are preserved.
DELETE FROM knowledge_selfcheck_document_mappings a
USING knowledge_selfcheck_document_mappings b
WHERE a.tenant_id = b.tenant_id
  AND a.project_document_id = b.project_document_id
  AND (a.created_at > b.created_at OR (a.created_at = b.created_at AND a.id > b.id));

CREATE UNIQUE INDEX IF NOT EXISTS uq_selfcheck_document_mapping_tenant_doc
ON knowledge_selfcheck_document_mappings (tenant_id, project_document_id);
