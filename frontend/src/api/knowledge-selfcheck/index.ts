import { del, get, post, put } from '@/utils/request'

export interface SelfCheckKnowledgeBase { id: string; name: string; description?: string; knowledge_count?: number }
export interface SelfCheckFeishuMember { open_id: string; name: string }
export interface SelfCheckProfile { tenant_id: number; knowledge_base_id: string; purpose: 'project' | 'sop' | 'ignore'; sop_knowledge_base_id: string; owner_name: string; owner_email: string; owner_open_id: string; im_channel_id: string; daily_schedule?: string; last_run_at?: string; enabled: boolean }
export interface SelfCheckMapping { id: string; business_line: string; project_type: string; sop_knowledge_base_id: string; sop_version: string; active: boolean }
export interface SelfCheckRun { id: string; project_kb_id: string; project_kb_name: string; sop_kb_id: string; sop_kb_name: string; sop_version: string; status: string; summary?: { error?: string; [key: string]: unknown }; model_id: string; started_at: string; completed_at?: string }
export interface SelfCheckFinding {
  id: string
  tenant_id: number
  run_id: string
  project_document_id?: string
  project_document_title?: string
  sop_document_id?: string
  sop_document_title?: string
  requirement: string
  material: string
  state: 'complete' | 'missing' | 'incomplete' | 'uncertain' | 'not_applicable'
  evidence: string
  sop_clause: string
  suggestion: string
  created_at: string
  updated_at: string
}
export interface SelfCheckOverview { knowledge_bases: SelfCheckKnowledgeBase[]; profiles: SelfCheckProfile[]; mappings: SelfCheckMapping[]; runs: SelfCheckRun[]; findings: SelfCheckFinding[] }
export interface SelfCheckDocumentMapping { project_document_id: string; project_document_name: string; sop_document_id: string; sop_document_name: string; sop_knowledge_base_id: string; enabled: boolean }
export interface SelfCheckDocumentMappingData { items: SelfCheckDocumentMapping[]; sop_documents: { id: string; name: string }[] }

export const getSelfCheckOverview = () => get<{ success: boolean; data: SelfCheckOverview }>('/api/v1/knowledge-selfcheck/overview')
export const clearSelfCheckRuns = () => del('/api/v1/knowledge-selfcheck/runs')
export const listSelfCheckFeishuMembers = () => get<{ success: boolean; data: SelfCheckFeishuMember[] }>('/api/v1/knowledge-selfcheck/feishu-members')
export const updateSelfCheckProfile = (kbId: string, data: Partial<SelfCheckProfile>) => put(`/api/v1/knowledge-selfcheck/profiles/${kbId}`, data)
export const saveSelfCheckMapping = (data: Partial<SelfCheckMapping>) => post('/api/v1/knowledge-selfcheck/mappings', data)
export const runSelfCheck = (projectKnowledgeBaseId: string, modelId = '') => post('/api/v1/knowledge-selfcheck/runs', { project_knowledge_base_id: projectKnowledgeBaseId, model_id: modelId })
export const getSelfCheckDocumentMappings = (projectKnowledgeBaseId: string) => get<{ success: boolean; data: SelfCheckDocumentMappingData }>(`/api/v1/knowledge-selfcheck/document-mappings?project_knowledge_base_id=${encodeURIComponent(projectKnowledgeBaseId)}`)
export const saveSelfCheckDocumentMapping = (data: { project_knowledge_base_id: string; project_document_id: string; sop_knowledge_base_id: string; sop_document_id: string; enabled: boolean }) => put('/api/v1/knowledge-selfcheck/document-mappings', data)
