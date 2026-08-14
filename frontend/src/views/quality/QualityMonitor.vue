<template>
  <main class="quality-page">
    <header class="quality-header">
      <div>
        <p class="eyebrow">KNOWLEDGE QUALITY</p>
        <h1>质量监控</h1>
        <p class="subtitle">监控知识问答命中率、响应时间，并自动汇总知识缺口。</p>
      </div>
      <div class="header-actions">
        <span class="freshness">{{ refreshedAt ? `更新于 ${refreshedAt}` : '尚未刷新' }}</span>
        <t-button theme="primary" :loading="loading" @click="loadDashboard">刷新数据</t-button>
      </div>
    </header>

    <section class="filter-bar" aria-label="质量监控筛选条件">
      <label>
        <span>统计周期</span>
        <t-select v-model="windowDays" :options="windowOptions" style="width: 150px" @change="onFilterChange" />
      </label>
      <label>
        <span>渠道</span>
        <t-select v-model="channel" :options="channelOptions" style="width: 150px" @change="onFilterChange" />
      </label>
      <label>
        <span>智能体</span>
        <t-select v-model="agentId" :options="agentOptions" style="width: 220px" @change="onFilterChange" />
      </label>
      <div class="definition">命中：回答至少引用 1 条知识且未触发兜底</div>
    </section>

    <div v-if="errorMessage" class="error-banner">
      <span>{{ errorMessage }}</span>
      <button type="button" @click="loadDashboard">重试</button>
    </div>

    <section class="metric-grid" :aria-busy="loading">
      <article class="metric-card metric-card--primary">
        <span>知识问答命中率</span>
        <strong>{{ formatPercent(summary?.hit_rate) }}</strong>
        <div class="progress"><i :style="{ width: `${boundedHitRate}%` }" /></div>
        <small>{{ summary?.hits ?? 0 }} 次命中 / {{ summary?.total ?? 0 }} 次知识问答</small>
      </article>
      <article class="metric-card">
        <span>知识问答总数</span>
        <strong>{{ summary?.total ?? 0 }}</strong>
        <small>仅统计启用了知识检索的问答</small>
      </article>
      <article class="metric-card metric-card--warning">
        <span>未命中问题</span>
        <strong>{{ summary?.misses ?? 0 }}</strong>
        <small>兜底回答或没有知识引用</small>
      </article>
      <article class="metric-card">
        <span>平均完整响应</span>
        <strong>{{ formatDuration(summary?.average_response_time_ms) }}</strong>
        <small>P95 {{ formatDuration(summary?.p95_response_time_ms) }} · 样本 {{ summary?.latency_samples ?? 0 }}</small>
      </article>
    </section>

    <section class="quality-panel report-panel">
      <div class="panel-heading">
        <div>
          <h2>质量报告</h2>
          <p>汇总命中率、高频问题与知识缺口，并使用所选 AI 模型生成改进建议。</p>
        </div>
        <div class="report-actions">
          <t-button variant="outline" :loading="savingReportConfig" @click="saveReportConfig">保存配置</t-button>
          <t-button theme="primary" :loading="generatingReport" @click="createReport">生成报告</t-button>
        </div>
      </div>

      <div class="report-config-grid">
        <label>
          <span>统计周期</span>
          <t-select v-model="reportConfig.window_days" :options="windowOptions" />
        </label>
        <label>
          <span>报告模型</span>
          <t-select v-model="reportConfig.model_id" :options="reportModelOptions" placeholder="使用默认问答模型" />
        </label>
        <label>
          <span>发送周期</span>
          <div class="schedule-fields">
            <t-select v-model="scheduleWeekday" :options="scheduleWeekdayOptions" />
            <t-select v-model="scheduleHour" :options="scheduleHourOptions" />
          </div>
          <small>每周按所选星期和时间发送（北京时间）。</small>
        </label>
        <label>
          <span>发送机器人</span>
          <t-select v-model="reportConfig.im_channel_id" :options="imChannelOptions" clearable placeholder="从 IM 集成中选择" />
          <small>选择由哪个已配置的飞书 IM 机器人发送周报。</small>
        </label>
        <label class="target-group-field">
          <span>项目管理群</span>
          <t-input v-model="reportConfig.target_chat_id" placeholder="先留空；配置后填写飞书群 chat_id" />
        </label>
        <label class="auto-send-field">
          <span>自动发送</span>
          <t-switch v-model="reportConfig.enabled" :disabled="!reportConfig.im_channel_id || !reportConfig.target_chat_id" />
          <small>目标群为空时保持关闭，只生成报告。</small>
        </label>
      </div>

      <div v-if="latestReport" class="report-preview">
        <div v-if="latestReportRecord?.status === 'failed'" class="report-failure">
          <strong>报告未完成</strong>
          <p>{{ latestReportRecord?.send_error || latestReport.analysis.caveats?.[0] || 'AI 分析生成失败，请稍后重试。' }}</p>
        </div>
        <div class="report-summary-row">
          <div>
            <span>最新报告</span>
            <strong>{{ formatDate(latestReport.generated_at) }}</strong>
          </div>
          <div>
            <span>命中率</span>
            <strong>{{ formatPercent(latestReport.current.hit_rate) }}</strong>
          </div>
          <div>
            <span>较上期</span>
            <strong :class="latestReport.hit_rate_change < 0 ? 'negative' : 'positive'">
              {{ latestReport.hit_rate_change >= 0 ? '+' : '' }}{{ latestReport.hit_rate_change.toFixed(1) }} 个百分点
            </strong>
          </div>
          <div>
            <span>未命中</span>
            <strong>{{ latestReport.current.misses }}</strong>
          </div>
        </div>
        <h3>执行摘要</h3>
        <p>{{ latestReport.analysis.executive_summary }}</p>
        <div class="report-columns">
          <div>
            <h3>关键发现</h3>
            <ul v-if="latestReport.analysis.key_findings?.length"><li v-for="item in latestReport.analysis.key_findings" :key="item">{{ item }}</li></ul>
            <p v-else class="muted-text">暂无关键发现。</p>
          </div>
          <div>
            <h3>AI 改进建议</h3>
            <ul v-if="latestReport.analysis.recommendations?.length"><li v-for="item in latestReport.analysis.recommendations" :key="item">{{ item }}</li></ul>
            <p v-else class="muted-text">暂无 AI 改进建议。</p>
          </div>
        </div>
        <div class="report-footer">
          <span>状态：{{ reportStatusLabel(latestReportRecord?.status) }}</span>
          <t-button size="small" variant="text" :disabled="!canSendReport" :loading="sendingReport" @click="sendLatestReport">
            发送到项目管理群
          </t-button>
        </div>
      </div>
      <div v-else class="empty-state">尚未生成质量报告。生成时才会调用 AI 模型并消耗少量 Token。</div>
    </section>

    <section class="quality-panel">
      <div class="panel-heading">
        <div>
          <h2>渠道表现</h2>
          <p>快速定位 Web、飞书 IM 与 API 接入的质量差异。</p>
        </div>
      </div>
      <div v-if="channelRows.length" class="channel-list">
        <div v-for="item in channelRows" :key="item.channel" class="channel-row">
          <span class="channel-badge">{{ channelLabel(item.channel) }}</span>
          <div class="channel-progress"><i :style="{ width: `${item.hit_rate}%` }" /></div>
          <strong>{{ formatPercent(item.hit_rate) }}</strong>
          <span>{{ item.hits }} 命中</span>
          <span>{{ item.misses }} 未命中</span>
          <span>{{ item.total }} 总计</span>
        </div>
      </div>
      <div v-else class="empty-state">当前筛选范围内暂无知识问答数据。</div>
    </section>

    <section class="quality-panel misses-panel">
      <div class="panel-heading">
        <div>
          <h2>未命中问题</h2>
          <p>这些问题已随原始问答自动保存在 WeKnora，可直接作为知识补充清单。</p>
        </div>
        <span class="count-pill">{{ misses.total }} 条</span>
      </div>

      <div class="table-wrap">
        <table v-if="misses.items.length" class="miss-table">
          <thead>
            <tr>
              <th>时间</th>
              <th>问题</th>
              <th>智能体</th>
              <th>渠道</th>
              <th>原因</th>
              <th>响应时间</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in misses.items" :key="item.message_id">
              <td class="nowrap">{{ formatDate(item.occurred_at) }}</td>
              <td class="question-cell" :title="item.question">{{ item.question || '（未保存问题文本）' }}</td>
              <td>{{ agentName(item.agent_id) }}</td>
              <td><span class="channel-badge channel-badge--small">{{ channelLabel(item.channel) }}</span></td>
              <td><span class="reason-tag">{{ reasonLabel(item.reason) }}</span></td>
              <td class="nowrap">{{ formatDuration(item.response_time_ms) }}</td>
              <td><button class="link-button" type="button" @click="openSession(item.session_id)">查看会话</button></td>
            </tr>
          </tbody>
        </table>
        <div v-else class="empty-state">没有未命中问题，或当前筛选范围内尚无数据。</div>
      </div>

      <footer v-if="misses.total > pageSize" class="pagination">
        <span>第 {{ page }} / {{ totalPages }} 页</span>
        <button type="button" :disabled="page <= 1 || loading" @click="changePage(page - 1)">上一页</button>
        <button type="button" :disabled="page >= totalPages || loading" @click="changePage(page + 1)">下一页</button>
      </footer>
    </section>

    <aside class="metric-note">
      <strong>口径说明</strong>
      <span>普通闲聊不进入分母；读取本页不会调用大模型，也不会消耗额外 Token。</span>
    </aside>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { listAgents, listAllIMChannels, type CustomAgent, type IMChannelOverview } from '@/api/agent'
import { listModels, type ModelConfig } from '@/api/model'
import {
  generateQualityReport,
  getQualityReportConfig,
  getQualityMisses,
  getQualitySummary,
  listQualityReports,
  sendQualityReport,
  updateQualityReportConfig,
  type QualityMissList,
  type QualityReportConfig,
  type QualityReportPayload,
  type QualityReportRecord,
  type QualitySummary,
} from '@/api/quality-monitor'

const router = useRouter()
const loading = ref(false)
const errorMessage = ref('')
const refreshedAt = ref('')
const windowDays = ref(7)
const channel = ref('')
const agentId = ref('')
const page = ref(1)
const pageSize = 20
const agents = ref<CustomAgent[]>([])
const summary = ref<QualitySummary | null>(null)
const misses = ref<QualityMissList>({ items: [], total: 0, page: 1, page_size: pageSize, from: '', to: '' })
const reportModels = ref<ModelConfig[]>([])
const imChannels = ref<IMChannelOverview[]>([])
const reportHistory = ref<QualityReportRecord[]>([])
const generatingReport = ref(false)
const savingReportConfig = ref(false)
const sendingReport = ref(false)
const scheduleWeekday = ref('monday')
const scheduleHour = ref('09')
const reportConfig = ref<QualityReportConfig>({
  tenant_id: 0,
  enabled: false,
  window_days: 7,
  model_id: '',
  schedule: 'weekly_monday_0900',
  timezone: 'Asia/Shanghai',
  im_channel_id: '',
  target_chat_id: '',
  target_chat_name: '',
})

const windowOptions = [
  { label: '最近 7 天', value: 7 },
  { label: '最近 30 天', value: 30 },
  { label: '最近 90 天', value: 90 },
]
const scheduleWeekdayOptions = [
  { label: '周一', value: 'monday' },
  { label: '周二', value: 'tuesday' },
  { label: '周三', value: 'wednesday' },
  { label: '周四', value: 'thursday' },
  { label: '周五', value: 'friday' },
  { label: '周六', value: 'saturday' },
  { label: '周日', value: 'sunday' },
]
const scheduleHourOptions = Array.from({ length: 24 }, (_, hour) => {
  const value = String(hour).padStart(2, '0')
  return { label: `${value}:00`, value }
})
const channelOptions = [
  { label: '全部渠道', value: '' },
  { label: '网页', value: 'web' },
  { label: '飞书 IM', value: 'im' },
  { label: 'API', value: 'api' },
  { label: '未标记', value: 'unknown' },
]

const agentOptions = computed(() => [
  { label: '全部智能体', value: '' },
  ...agents.value.map(agent => ({ label: agent.name, value: agent.id })),
])
const boundedHitRate = computed(() => Math.max(0, Math.min(100, summary.value?.hit_rate ?? 0)))
const channelRows = computed(() => Object.entries(summary.value?.by_channel ?? {})
  .map(([name, value]) => ({ channel: name, ...value }))
  .sort((a, b) => b.total - a.total))
const totalPages = computed(() => Math.max(1, Math.ceil(misses.value.total / pageSize)))
const reportModelOptions = computed(() => [
  { label: '默认问答模型', value: '' },
  ...reportModels.value.map(model => ({ label: model.display_name || model.name, value: model.id || '' })),
])
const imChannelOptions = computed(() => [
  { label: '暂不发送', value: '' },
  ...imChannels.value.filter(item => item.enabled && (item.platform === 'feishu' || item.platform === 'lark'))
    .map(item => ({ label: `${item.name || '飞书'} · ${item.agent_name || '智能体'}`, value: item.id })),
])
const latestReportRecord = computed(() => reportHistory.value[0] || null)
const latestReport = computed<QualityReportPayload | null>(() => latestReportRecord.value?.report || null)
const canSendReport = computed(() => Boolean(latestReportRecord.value && reportConfig.value.im_channel_id && reportConfig.value.target_chat_id))

function applyScheduleValue(value?: string) {
  const match = /^weekly_(sunday|monday|tuesday|wednesday|thursday|friday|saturday)_(\d{2})\d{2}$/.exec(value || '')
  if (!match) return
  scheduleWeekday.value = match[1]
  scheduleHour.value = match[2]
}

function currentQuery() {
  const to = new Date()
  const from = new Date(to.getTime() - windowDays.value * 24 * 60 * 60 * 1000)
  return {
    from: from.toISOString(),
    to: to.toISOString(),
    agent_id: agentId.value,
    channel: channel.value,
  }
}

async function loadDashboard() {
  loading.value = true
  errorMessage.value = ''
  try {
    const query = currentQuery()
    const [summaryResponse, missesResponse] = await Promise.all([
      getQualitySummary(query),
      getQualityMisses({ ...query, page: page.value, page_size: pageSize }),
    ])
    summary.value = summaryResponse.data
    misses.value = missesResponse.data
    refreshedAt.value = new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', second: '2-digit' }).format(new Date())
  } catch (error: any) {
    errorMessage.value = error?.message || '质量监控数据加载失败，请稍后重试。'
  } finally {
    loading.value = false
  }
}

async function loadAgents() {
  try {
    const response = await listAgents()
    agents.value = response.data || []
  } catch {
    agents.value = []
  }
}

async function loadReportResources() {
  try {
    const [configResponse, historyResponse, modelsResponse, channelsResponse] = await Promise.all([
      getQualityReportConfig(),
      listQualityReports(),
      listModels('KnowledgeQA'),
      listAllIMChannels(),
    ])
    reportConfig.value = configResponse.data
    applyScheduleValue(reportConfig.value.schedule)
    reportHistory.value = historyResponse.data || []
    reportModels.value = modelsResponse.filter(model => model.status === 'active')
    imChannels.value = channelsResponse.data || []
  } catch (error: any) {
    errorMessage.value = error?.message || '质量报告配置加载失败。'
  }
}

async function saveReportConfig() {
  savingReportConfig.value = true
  errorMessage.value = ''
  try {
    reportConfig.value.schedule = `weekly_${scheduleWeekday.value}_${scheduleHour.value}00`
    const response = await updateQualityReportConfig(reportConfig.value)
    reportConfig.value = response.data
    applyScheduleValue(reportConfig.value.schedule)
  } catch (error: any) {
    errorMessage.value = error?.message || '质量报告配置保存失败。'
  } finally {
    savingReportConfig.value = false
  }
}

async function createReport() {
  generatingReport.value = true
  errorMessage.value = ''
  try {
    const response = await generateQualityReport({
      window_days: reportConfig.value.window_days,
      model_id: reportConfig.value.model_id,
    })
    reportHistory.value = [response.data.record, ...reportHistory.value.filter(item => item.id !== response.data.record.id)]
    if (response.data.record.status === 'failed') {
      errorMessage.value = response.data.record.send_error || '质量报告生成失败：AI 分析未完成。'
    }
  } catch (error: any) {
    errorMessage.value = error?.message || '质量报告生成失败，请检查报告模型配置。'
  } finally {
    generatingReport.value = false
  }
}

async function sendLatestReport() {
  if (!latestReportRecord.value) return
  sendingReport.value = true
  errorMessage.value = ''
  try {
    await saveReportConfig()
    const response = await sendQualityReport(latestReportRecord.value.id)
    reportHistory.value = [response.data, ...reportHistory.value.slice(1)]
  } catch (error: any) {
    errorMessage.value = error?.message || '质量报告发送失败。'
  } finally {
    sendingReport.value = false
  }
}

function reportStatusLabel(value?: string) {
  return ({ generated: '已生成', sent: '已发送', send_failed: '发送失败', failed: '生成失败' } as Record<string, string>)[value || ''] || '—'
}

function onFilterChange() {
  page.value = 1
  void loadDashboard()
}

function changePage(nextPage: number) {
  page.value = nextPage
  void loadDashboard()
}

function formatPercent(value?: number) {
  if (value === undefined || value === null) return '—'
  return `${value.toFixed(value % 1 === 0 ? 0 : 1)}%`
}

function formatDuration(value?: number) {
  if (!value || value < 0) return '—'
  if (value < 1000) return `${value} ms`
  return `${(value / 1000).toFixed(value >= 10000 ? 1 : 2)} s`
}

function formatDate(value: string) {
  if (!value) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  }).format(new Date(value))
}

function channelLabel(value: string) {
  return ({ web: '网页', im: '飞书 IM', api: 'API', unknown: '未标记' } as Record<string, string>)[value] || value
}

function reasonLabel(value?: string) {
  return ({
    fallback: '触发兜底',
    no_knowledge_reference: '无知识引用',
    explicit_no_answer: '引用无有效答案',
  } as Record<string, string>)[value || ''] || '未命中'
}

function agentName(value?: string) {
  if (!value) return '—'
  return agents.value.find(agent => agent.id === value)?.name || value.slice(0, 8)
}

function openSession(sessionId: string) {
  if (sessionId) void router.push(`/platform/chat/${sessionId}`)
}

onMounted(async () => {
  await Promise.all([loadAgents(), loadReportResources()])
  await loadDashboard()
})
</script>

<style scoped lang="less">
.quality-page {
  flex: 1 1 auto;
  width: 100%;
  height: 100%;
  min-height: 100%;
  min-width: 0;
  overflow-x: hidden;
  overflow-y: auto;
  box-sizing: border-box;
  scrollbar-gutter: stable;
  padding: 32px 40px 56px;
  color: var(--td-text-color-primary);
  background:
    radial-gradient(circle at 92% 0%, rgba(0, 168, 112, 0.10), transparent 30%),
    var(--td-bg-color-page);
}

.quality-header, .filter-bar, .panel-heading, .header-actions, .pagination, .metric-note {
  display: flex;
  align-items: center;
}

.quality-header { justify-content: space-between; gap: 24px; margin-bottom: 24px; }
.quality-header h1 { margin: 4px 0 6px; font-size: 30px; line-height: 1.2; }
.eyebrow { margin: 0; color: var(--td-brand-color); font-size: 11px; font-weight: 700; letter-spacing: 0.16em; }
.subtitle, .panel-heading p { margin: 0; color: var(--td-text-color-secondary); }
.header-actions { gap: 14px; }
.freshness { color: var(--td-text-color-placeholder); font-size: 12px; }

.filter-bar {
  gap: 18px;
  padding: 16px 18px;
  margin-bottom: 18px;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-stroke);
  border-radius: 12px;
}
.filter-bar label { display: flex; align-items: center; gap: 9px; font-size: 13px; color: var(--td-text-color-secondary); }
.definition { margin-left: auto; color: var(--td-text-color-placeholder); font-size: 12px; }

.error-banner { display: flex; justify-content: space-between; padding: 12px 16px; margin-bottom: 16px; color: var(--td-error-color); background: var(--td-error-color-1); border-radius: 8px; }
.error-banner button, .link-button { border: 0; color: var(--td-brand-color); background: transparent; cursor: pointer; }

.metric-grid { display: grid; grid-template-columns: repeat(5, minmax(150px, 1fr)); gap: 14px; margin-bottom: 18px; }
.metric-card { min-height: 132px; padding: 20px; background: var(--td-bg-color-container); border: 1px solid var(--td-component-stroke); border-radius: 12px; box-shadow: var(--td-shadow-1); }
.metric-card > span { display: block; color: var(--td-text-color-secondary); font-size: 13px; }
.metric-card strong { display: block; margin: 12px 0 9px; font-size: 30px; line-height: 1; }
.metric-card small { color: var(--td-text-color-placeholder); line-height: 1.5; }
.metric-card--primary { color: white; background: linear-gradient(135deg, #075d4a, #0aa579); border-color: transparent; }
.metric-card--primary > span, .metric-card--primary small { color: rgba(255,255,255,.76); }
.metric-card--warning strong { color: var(--td-warning-color); }
.progress, .channel-progress { overflow: hidden; background: rgba(255,255,255,.2); border-radius: 99px; }
.progress { height: 5px; margin: 0 0 10px; }
.progress i, .channel-progress i { display: block; height: 100%; background: #72f0bf; border-radius: inherit; }

.quality-panel { padding: 22px; margin-bottom: 18px; background: var(--td-bg-color-container); border: 1px solid var(--td-component-stroke); border-radius: 12px; }
.panel-heading { justify-content: space-between; gap: 16px; margin-bottom: 18px; }
.panel-heading h2 { margin: 0 0 5px; font-size: 18px; }
.panel-heading p { font-size: 13px; }
.count-pill, .reason-tag, .channel-badge { display: inline-flex; align-items: center; white-space: nowrap; border-radius: 99px; }
.count-pill { padding: 5px 10px; color: var(--td-warning-color); background: var(--td-warning-color-1); }
.channel-list { display: grid; gap: 13px; }
.channel-row { display: grid; grid-template-columns: 90px minmax(120px, 1fr) 64px 80px 90px 70px; align-items: center; gap: 14px; font-size: 13px; color: var(--td-text-color-secondary); }
.channel-row strong { color: var(--td-text-color-primary); }
.channel-progress { height: 8px; background: var(--td-bg-color-secondarycontainer); }
.channel-progress i { background: var(--td-brand-color); }
.channel-badge { width: fit-content; padding: 4px 9px; color: var(--td-brand-color); background: var(--td-brand-color-light); font-weight: 600; }
.channel-badge--small { padding: 3px 7px; font-size: 12px; }

.table-wrap { overflow-x: auto; }
.miss-table { width: 100%; border-collapse: collapse; font-size: 13px; }
.miss-table th { padding: 10px 12px; text-align: left; color: var(--td-text-color-placeholder); background: var(--td-bg-color-secondarycontainer); font-weight: 500; }
.miss-table td { padding: 14px 12px; border-bottom: 1px solid var(--td-component-stroke); color: var(--td-text-color-secondary); }
.miss-table tbody tr:hover { background: var(--td-bg-color-container-hover); }
.question-cell { max-width: 420px; color: var(--td-text-color-primary) !important; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.nowrap { white-space: nowrap; }
.reason-tag { padding: 3px 8px; color: var(--td-warning-color); background: var(--td-warning-color-1); }
.link-button { padding: 0; white-space: nowrap; }
.empty-state { padding: 38px 12px; text-align: center; color: var(--td-text-color-placeholder); }
.pagination { justify-content: flex-end; gap: 10px; padding-top: 16px; color: var(--td-text-color-secondary); font-size: 13px; }
.pagination button { padding: 6px 12px; border: 1px solid var(--td-component-border); border-radius: 6px; color: var(--td-text-color-secondary); background: var(--td-bg-color-container); cursor: pointer; }
.pagination button:disabled { opacity: .45; cursor: not-allowed; }
.metric-note { gap: 10px; padding: 13px 16px; color: var(--td-text-color-secondary); background: var(--td-bg-color-secondarycontainer); border-radius: 8px; font-size: 12px; }
.metric-note strong { color: var(--td-text-color-primary); }
.report-actions { display: flex; gap: 10px; }
.report-config-grid { display: grid; grid-template-columns: repeat(3, minmax(190px, 1fr)); gap: 16px; }
.report-config-grid label { display: grid; gap: 7px; color: var(--td-text-color-secondary); font-size: 13px; }
.schedule-fields { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 8px; }
.target-group-field { grid-column: span 2; }
.auto-send-field { display: grid !important; grid-template-columns: auto 1fr; align-items: center; }
.auto-send-field > span { grid-column: 1 / -1; }
.auto-send-field small { color: var(--td-text-color-placeholder); }
.report-preview { margin-top: 22px; padding: 20px; border: 1px solid var(--td-brand-color-light); border-radius: 10px; background: var(--td-brand-color-light); }
.report-failure { padding: 12px 14px; margin-bottom: 16px; color: var(--td-error-color); background: var(--td-error-color-1); border-radius: 8px; }
.report-failure strong { display: block; margin-bottom: 4px; }
.report-failure p { margin: 0; color: var(--td-error-color); word-break: break-word; }
.report-summary-row { display: grid; grid-template-columns: repeat(4, 1fr); gap: 12px; margin-bottom: 18px; }
.report-summary-row > div { padding: 12px; background: var(--td-bg-color-container); border-radius: 8px; }
.report-summary-row span { display: block; margin-bottom: 6px; color: var(--td-text-color-secondary); font-size: 12px; }
.report-summary-row strong { font-size: 18px; }
.report-preview h3 { margin: 16px 0 8px; font-size: 15px; }
.report-preview p, .report-preview li { color: var(--td-text-color-secondary); line-height: 1.7; }
.report-columns { display: grid; grid-template-columns: 1fr 1fr; gap: 24px; }
.report-columns ul { margin: 0; padding-left: 20px; }
.report-footer { display: flex; align-items: center; justify-content: space-between; margin-top: 18px; padding-top: 14px; border-top: 1px solid var(--td-component-stroke); color: var(--td-text-color-secondary); }
.muted-text { color: var(--td-text-color-placeholder) !important; }
.positive { color: var(--td-success-color); }
.negative { color: var(--td-error-color); }

@media (max-width: 1200px) {
  .metric-grid { grid-template-columns: repeat(3, minmax(160px, 1fr)); }
  .report-config-grid { grid-template-columns: repeat(2, minmax(190px, 1fr)); }
  .channel-row { grid-template-columns: 90px minmax(120px, 1fr) 60px 70px; }
  .channel-row span:nth-last-child(-n+2) { display: none; }
}
@media (max-width: 760px) {
  .quality-page { padding: 20px 16px 40px; }
  .quality-header, .filter-bar { align-items: flex-start; flex-direction: column; }
  .definition { margin-left: 0; }
  .metric-grid { grid-template-columns: 1fr; }
  .report-config-grid, .report-summary-row, .report-columns { grid-template-columns: 1fr; }
  .target-group-field { grid-column: auto; }
  .channel-row { grid-template-columns: 80px 1fr 55px; }
  .channel-row span:nth-last-child(-n+3) { display: none; }
}
</style>
