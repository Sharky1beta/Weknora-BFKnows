import { get, post, put } from '@/utils/request'

export interface QualityChannelSummary {
  total: number
  hits: number
  misses: number
  hit_rate: number
}

export interface QualitySummary {
  from: string
  to: string
  total: number
  hits: number
  misses: number
  hit_rate: number
  average_response_time_ms: number
  p95_response_time_ms: number
  average_first_response_time_ms: number
  p95_first_response_time_ms: number
  latency_samples: number
  by_channel: Record<string, QualityChannelSummary>
  metric_definition: Record<string, string>
}

export interface QualityMiss {
  message_id: string
  session_id: string
  request_id: string
  question: string
  answer?: string
  channel: string
  agent_id?: string
  occurred_at: string
  response_time_ms?: number
  first_response_time_ms?: number
  reference_count: number
  reason?: 'fallback' | 'no_knowledge_reference' | string
  hit: boolean
}

export interface QualityMissList {
  items: QualityMiss[]
  total: number
  page: number
  page_size: number
  from: string
  to: string
}

export interface QualityQuery {
  from?: string
  to?: string
  agent_id?: string
  channel?: string
  page?: number
  page_size?: number
}

export interface QualityReportConfig {
  tenant_id: number
  enabled: boolean
  window_days: number
  model_id: string
  schedule: 'weekly_monday_0900' | string
  timezone: string
  im_channel_id: string
  target_chat_id: string
  target_chat_name: string
  last_run_at?: string
}

export interface QualityReportAnalysis {
  executive_summary: string
  key_findings: string[]
  recommendations: string[]
  further_questions: string[]
  caveats: string[]
}

export interface QualityReportPayload {
  title: string
  generated_at: string
  current: QualitySummary
  previous: QualitySummary
  hit_rate_change: number
  frequent_questions: Array<{ question: string; count: number; hits: number; misses: number }>
  gap_by_reason: Array<{ name: string; count: number }>
  gap_by_channel: Array<{ name: string; count: number }>
  analysis: QualityReportAnalysis
  model_id: string
}

export interface QualityReportRecord {
  id: string
  tenant_id: number
  window_from: string
  window_to: string
  status: 'generated' | 'sent' | 'send_failed' | string
  model_id: string
  report: QualityReportPayload
  sent_at?: string
  send_error?: string
  created_at: string
}

function toQueryString(params: QualityQuery) {
  const query = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value !== undefined && value !== null && value !== '') {
      query.set(key, String(value))
    }
  })
  const value = query.toString()
  return value ? `?${value}` : ''
}

export function getQualitySummary(params: QualityQuery) {
  return get<{ success: boolean; data: QualitySummary }>(
    `/api/v1/quality-monitor/summary${toQueryString(params)}`,
  )
}

export function getQualityMisses(params: QualityQuery) {
  return get<{ success: boolean; data: QualityMissList }>(
    `/api/v1/quality-monitor/misses${toQueryString(params)}`,
  )
}

export function getQualityReportConfig() {
  return get<{ success: boolean; data: QualityReportConfig }>('/api/v1/quality-monitor/report-config')
}

export function updateQualityReportConfig(data: Partial<QualityReportConfig>) {
  return put<{ success: boolean; data: QualityReportConfig }>('/api/v1/quality-monitor/report-config', data)
}

export function generateQualityReport(data: { window_days: number; model_id?: string }) {
  return post<{ success: boolean; data: { record: QualityReportRecord; report: QualityReportPayload } }>(
    '/api/v1/quality-monitor/reports/generate', data,
  )
}

export function listQualityReports() {
  return get<{ success: boolean; data: QualityReportRecord[] }>('/api/v1/quality-monitor/reports')
}

export function sendQualityReport(id: string) {
  return post<{ success: boolean; data: QualityReportRecord }>(`/api/v1/quality-monitor/reports/${id}/send`, {})
}
