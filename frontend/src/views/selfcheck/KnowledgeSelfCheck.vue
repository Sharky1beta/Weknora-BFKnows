<template>
  <main class="selfcheck-page">
    <header>
      <div><p class="eyebrow">KNOWLEDGE SELF-CHECK</p><h1>知识库自检</h1><p>为项目文档直接指定对应 SOP，并自动检查交付物完整性。</p></div>
      <div class="header-actions">
        <div class="model-picker">
          <span>自检模型</span>
          <t-select v-model="selectedModelId" :options="modelOptions" :loading="modelsLoading" placeholder="选择自检模型" @change="persistSelfCheckModel" />
        </div>
        <t-button theme="primary" :loading="loading" @click="load">刷新数据</t-button>
      </div>
    </header>
    <div v-if="error" class="error">{{ error }}</div>

    <section class="panel">
      <div class="panel-title"><div><h2>1. 配置知识库</h2><p>将知识库标记为项目材料或企业 SOP；项目只需直接选择对应 SOP。</p></div></div>
      <div class="profile-list">
        <article v-for="kb in overview.knowledge_bases" :key="kb.id" class="profile-card" :class="{ 'project-profile': profile(kb.id).purpose === 'project' }">
          <div class="kb-title"><strong>{{ kb.name }}</strong><span>{{ kb.knowledge_count || 0 }} 个文档</span></div>
          <div class="fields">
            <t-select v-model="profile(kb.id).purpose" :options="purposeOptions" />
            <template v-if="profile(kb.id).purpose === 'project'">
              <t-select v-model="profile(kb.id).sop_knowledge_base_id" :options="sopOptions" clearable placeholder="选择对应 SOP" />
              <div class="owner-field"><t-select v-model="profile(kb.id).owner_open_id" :options="ownerOptions" :disabled="!ownerOptions.length" filterable clearable placeholder="选择负责人飞书成员（可选）" @change="syncOwnerName(profile(kb.id))" /><small v-if="ownerMemberError">{{ ownerMemberError }}</small></div>
              <t-select v-model="profile(kb.id).im_channel_id" :options="imChannelOptions" clearable placeholder="通知机器人（可选）" />
              <t-select v-model="profile(kb.id).daily_schedule" :options="dailyScheduleOptions" placeholder="每日自检时间" />
            </template>
          </div>
          <div class="card-actions"><t-switch v-model="profile(kb.id).enabled" :disabled="profile(kb.id).purpose !== 'project'" /><span>启用自检</span><t-button size="small" variant="outline" @click="saveProfile(kb.id)">保存</t-button></div>
        </article>
      </div>
    </section>

    <section class="panel">
      <div class="panel-title"><div><h2>2. 执行自检</h2><p>系统会自动识别入口项目文档并匹配 SOP；如已手动绑定，则手动绑定优先。后台按配置的每日时间自动运行，并私聊通知负责人。</p></div></div>
      <div v-for="project in enabledProjects" :key="`binding-${project.knowledge_base_id}`" class="binding-list">
        <div v-if="entryMappings(project.knowledge_base_id).length" class="binding-title">
          <span>{{ kbName(project.knowledge_base_id) }}：入口文档绑定</span>
          <t-button size="small" theme="primary" variant="outline" :loading="savingMappings[project.knowledge_base_id]" :disabled="!changedMappings(project.knowledge_base_id).length" @click="saveProjectDocuments(project.knowledge_base_id)">保存本组绑定</t-button>
        </div>
        <div v-for="item in entryMappings(project.knowledge_base_id)" :key="item.project_document_id" class="binding-row">
          <div><strong>{{ item.project_document_name }}</strong><p>{{ item.sop_document_name ? `当前 SOP：${item.sop_document_name}` : '暂未匹配到 SOP' }}</p></div>
          <t-select v-model="item.sop_document_id" :options="(documentMappings[project.knowledge_base_id]?.sop_documents || []).map(x => ({ label: x.name, value: x.id }))" clearable placeholder="选择对应 SOP" />
        </div>
      </div>
      <div class="run-list">
        <article v-for="item in enabledProjects" :key="item.knowledge_base_id" class="run-card">
          <div><strong>{{ kbName(item.knowledge_base_id) }}</strong><p>对应 SOP：{{ kbName(item.sop_knowledge_base_id) }}</p></div>
          <t-button theme="primary" :loading="runningId === item.knowledge_base_id" :disabled="!item.sop_knowledge_base_id" @click="execute(item.knowledge_base_id)">立即自检</t-button>
        </article>
        <div v-if="!enabledProjects.length" class="empty">尚未配置并启用项目知识库。</div>
      </div>
    </section>

    <section class="panel">
      <div class="panel-title">
        <div><h2>3. 最近检查结果</h2><p>缺失和待确认项会通知负责人；已存在项保留为审计依据。</p></div>
        <div class="panel-actions">
          <t-tooltip content="刷新结果">
            <t-button shape="circle" variant="outline" :loading="loading" @click="load">
              <template #icon><t-icon name="refresh" /></template>
            </t-button>
          </t-tooltip>
          <t-tooltip v-if="overview.runs.length" content="清空记录">
            <t-button shape="circle" variant="outline" theme="danger" @click="clearRuns">
              <template #icon><t-icon name="delete" /></template>
            </t-button>
          </t-tooltip>
        </div>
      </div>
      <article v-for="run in overview.runs" :key="run.id" class="result-card">
        <div class="result-head"><div><strong>{{ run.project_kb_name }}</strong><span>依据 {{ run.sop_kb_name }} {{ run.sop_version }}</span></div><time>{{ formatTime(run.completed_at || run.started_at) }}</time></div>
        <div v-if="run.status === 'failed'" class="run-failed">本次自检未完成：{{ runError(run) }}</div>
        <div v-else class="run-results">
          <table class="document-summary"><thead><tr><th>项目文档</th><th>对应 SOP</th><th>当前状态</th><th>缺失/待确认交付物</th></tr></thead><tbody>
            <tr v-for="row in documentRows(run.id)" :key="row.document"><td>{{ row.document }}</td><td>{{ row.sop }}</td><td><span :class="['status', `status-${row.state}`]">{{ stateLabel(row.state) }}</span></td><td>{{ row.missing || '-' }}</td></tr>
          </tbody></table>
          <div v-if="!documentRows(run.id).length" class="no-findings">本次检查未产生明细：未找到可核对的交付要求或模型未返回结果。</div>
        </div>
      </article>
      <div v-if="!overview.runs.length" class="empty">尚未执行知识库自检。</div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { MessagePlugin, DialogPlugin } from 'tdesign-vue-next'
import { listAllIMChannels, type IMChannelOverview } from '@/api/agent'
import { clearSelfCheckRuns, getSelfCheckOverview, getSelfCheckDocumentMappings, listSelfCheckFeishuMembers, runSelfCheck, saveSelfCheckDocumentMapping, updateSelfCheckProfile, type SelfCheckOverview, type SelfCheckProfile, type SelfCheckFinding, type SelfCheckDocumentMapping } from '@/api/knowledge-selfcheck'
import { listModels, type ModelConfig } from '@/api/model'

const emptyOverview = (): SelfCheckOverview => ({ knowledge_bases: [], profiles: [], mappings: [], runs: [], findings: [] })
const overview = reactive<SelfCheckOverview>(emptyOverview())
const drafts = reactive<Record<string, SelfCheckProfile>>({})
const documentMappings = reactive<Record<string, { items: SelfCheckDocumentMapping[]; sop_documents: { id: string; name: string }[] }>>({})
const savedDocumentBindings = reactive<Record<string, Record<string, string>>>({})
const savingMappings = reactive<Record<string, boolean>>({})
const loading = ref(false), runningId = ref(''), error = ref(''), imChannels = ref<IMChannelOverview[]>([]), ownerMembers = ref<{ open_id: string; name: string }[]>([]), ownerMemberError = ref('')
const models = ref<ModelConfig[]>([]), modelsLoading = ref(false), selectedModelId = ref(localStorage.getItem('weknora:selfcheck:model_id') || '')
const purposeOptions = [{ label: '项目材料', value: 'project' }, { label: '企业 SOP', value: 'sop' }, { label: '不参与自检', value: 'ignore' }]
const dailyScheduleOptions = Array.from({ length: 24 }, (_, hour) => ({ label: `每天 ${String(hour).padStart(2, '0')}:00`, value: `daily_${String(hour).padStart(2, '0')}00` }))
const activeKBIds = computed(() => new Set(overview.knowledge_bases.map(x => x.id)))
const activeProfiles = computed(() => overview.knowledge_bases.filter(x => activeKBIds.value.has(x.id)).map(x => profile(x.id)))
const sopOptions = computed(() => activeProfiles.value.filter(x => x.purpose === 'sop').map(x => ({ label: kbName(x.knowledge_base_id), value: x.knowledge_base_id })))
const modelOptions = computed(() => models.value.map(x => ({ label: x.display_name || x.name || x.id || '未命名模型', value: x.id || '' })).filter(x => x.value))
const enabledProjects = computed(() => activeProfiles.value.filter(x => x.purpose === 'project' && x.enabled))
const projectProfiles = computed(() => activeProfiles.value.filter(x => x.purpose === 'project'))
const imChannelOptions = computed(() => imChannels.value.filter(x => x.enabled && ['feishu', 'lark'].includes(x.platform)).map(x => ({ label: (x.name || '飞书') + ' · ' + (x.agent_name || '智能体'), value: x.id })))
const ownerOptions = computed(() => ownerMembers.value.map(x => ({ label: x.name, value: x.open_id })))
function profile(kbId: string) { if (!drafts[kbId]) drafts[kbId] = { tenant_id: 0, knowledge_base_id: kbId, purpose: 'ignore', sop_knowledge_base_id: '', owner_name: '', owner_email: '', owner_open_id: '', im_channel_id: '', daily_schedule: 'daily_0900', enabled: false }; return drafts[kbId] }
function syncOwnerName(item: SelfCheckProfile) { item.owner_name = ownerMembers.value.find(x => x.open_id === item.owner_open_id)?.name || '' }
function kbName(id: string) { return id ? (overview.knowledge_bases.find(x => x.id === id)?.name || id) : '未选择' }
function findings(runId: string) { return overview.findings.filter(x => x.run_id === runId) }
function groupedFindings(runId: string) {
  const items = findings(runId)
  const map: Record<string, SelfCheckFinding[]> = {}
  for (const item of items) {
    const key = item.project_document_title || '通用项目文档'
    if (!map[key]) map[key] = []
    map[key].push(item)
  }
  return map
}
function documentRows(runId: string) {
  const groups = groupedFindings(runId)
  return Object.entries(groups).map(([document, items]) => {
    const states = items.map(x => x.state)
    const hasUnmatchedSOP = items.some(x => x.state === 'unmatched' || !x.sop_document_id || ['未检测到', '未匹配', '未找到'].some(word => (x.sop_document_title || '').includes(word)))
    const state = hasUnmatchedSOP ? 'unmatched' : states.includes('sop_unreadable') ? 'sop_unreadable' : states.includes('analysis_error') ? 'analysis_error' : states.includes('missing') ? 'missing' : (states.includes('partial') || states.includes('incomplete')) ? 'partial' : states.includes('uncertain') ? 'uncertain' : states.every(x => x === 'not_applicable') ? 'not_applicable' : (states.includes('present') || states.includes('complete')) ? 'present' : 'uncertain'
    const missing = items.filter(x => ['missing', 'partial', 'incomplete', 'uncertain'].includes(x.state)).map(x => x.requirement).filter(Boolean).join('、') || (state === 'unmatched' ? '未检测到适配的 SOP' : state === 'sop_unreadable' ? '无法提取该 SOP 的交付物清单' : state === 'analysis_error' ? '自动核对失败，需要重试' : '')
    return { document, sop: items[0]?.sop_document_title || '未检测到', state, missing }
  })
}
function runError(run: any) {
  return run.summary?.error || run.summary?.doc_summaries?.find((x: any) => x?.error)?.error || '模型或数据读取异常，请重试。'
}
function stateLabel(state: string) { return ({ present: '已完成', complete: '已完成', missing: '缺失', partial: '不完整', incomplete: '不完整', uncertain: '证据不足', unmatched: '未检测到 SOP', sop_unreadable: 'SOP 交付物未提取', analysis_error: '核对失败', not_applicable: '不适用' } as Record<string, string>)[state] || state }
function formatTime(value?: string) { return value ? new Date(value).toLocaleString('zh-CN') : '-' }
function entryMappings(kbId: string) { return (documentMappings[kbId]?.items || []).filter(isEntryDocument) }
function changedMappings(kbId: string) { return entryMappings(kbId).filter(x => x.sop_document_id && savedDocumentBindings[kbId]?.[x.project_document_id] !== x.sop_document_id) }
function isEntryDocument(item: SelfCheckDocumentMapping) {
  const title = (item.project_document_name || '').toLowerCase()
  return ['prd', '产品方案', '方案设计', '项目方案', '需求说明', '项目启动', '项目规划', '服务交接', '服务方案', '运营方案', '发布方案', '部署方案'].some(word => title.includes(word.toLowerCase()))
}
async function load() { loading.value = true; error.value = ''; ownerMemberError.value = ''; try { const [res, channels] = await Promise.all([getSelfCheckOverview(), listAllIMChannels()]); Object.assign(overview, emptyOverview(), res.data); imChannels.value = channels.data || []; overview.profiles.forEach(x => drafts[x.knowledge_base_id] = { ...x }); await loadMappings(); try { ownerMembers.value = (await listSelfCheckFeishuMembers()).data || [] } catch { ownerMembers.value = []; ownerMemberError.value = '当前工作区尚未配置可用的飞书数据源，暂不能选择负责人。' } } catch (e: any) { error.value = e?.message || '加载失败' } finally { loading.value = false } }
async function loadModels() {
  modelsLoading.value = true
  try {
    models.value = (await listModels('KnowledgeQA')).filter(x => x.status !== 'deleted')
    const preferred = models.value.find(x => [x.name, x.display_name, x.id].some(v => (v || '').includes('sensenova-6.7-flash-lite')))
    const activeSelected = selectedModelId.value && models.value.some(x => x.id === selectedModelId.value)
    if (!activeSelected) {
      selectedModelId.value = preferred?.id || models.value.find(x => x.is_default)?.id || models.value[0]?.id || ''
      persistSelfCheckModel()
    }
  } catch (e: any) {
    MessagePlugin.warning(e?.message || '自检模型列表加载失败')
  } finally {
    modelsLoading.value = false
  }
}
function persistSelfCheckModel() {
  if (selectedModelId.value) localStorage.setItem('weknora:selfcheck:model_id', selectedModelId.value)
}
async function saveProfile(kbId: string) { try { const draft = { ...profile(kbId) }; await updateSelfCheckProfile(kbId, draft); const idx = overview.profiles.findIndex(x => x.knowledge_base_id === kbId); if (idx >= 0) overview.profiles[idx] = { ...overview.profiles[idx], ...draft }; else overview.profiles.push(draft); drafts[kbId] = { ...draft }; if (draft.purpose === 'project' && draft.sop_knowledge_base_id) await loadMappings(kbId); else { delete documentMappings[kbId]; delete savedDocumentBindings[kbId] }; MessagePlugin.success('知识库配置已保存') } catch (e: any) { MessagePlugin.error(e?.message || '保存失败') } }
async function execute(kbId: string) { runningId.value = kbId; try { await runSelfCheck(kbId, selectedModelId.value); MessagePlugin.success('自检已启动，稍后刷新结果'); await load() } catch (e: any) { MessagePlugin.error(e?.message || '自检启动失败') } finally { runningId.value = '' } }
async function loadMappings(onlyProjectKBID = '') {
  for (const project of projectProfiles.value.filter(x => x.sop_knowledge_base_id && (!onlyProjectKBID || x.knowledge_base_id === onlyProjectKBID))) {
    try {
      const data = (await getSelfCheckDocumentMappings(project.knowledge_base_id)).data
      documentMappings[project.knowledge_base_id] = data
      savedDocumentBindings[project.knowledge_base_id] = Object.fromEntries((data.items || []).map(x => [x.project_document_id, x.sop_document_id || '']))
    } catch {
      documentMappings[project.knowledge_base_id] = { items: [], sop_documents: [] }
      savedDocumentBindings[project.knowledge_base_id] = {}
    }
  }
}
async function saveProjectDocuments(projectKBID: string) {
  const project = overview.profiles.find(x => x.knowledge_base_id === projectKBID)
  const changes = changedMappings(projectKBID)
  if (!project || !changes.length) return
  savingMappings[projectKBID] = true
  try {
    for (const item of changes) {
      await saveSelfCheckDocumentMapping({ project_knowledge_base_id: projectKBID, project_document_id: item.project_document_id, sop_knowledge_base_id: project.sop_knowledge_base_id, sop_document_id: item.sop_document_id, enabled: true })
    }
    MessagePlugin.success(`已保存 ${changes.length} 个文档绑定`)
    await loadMappings()
  } catch (e: any) {
    MessagePlugin.error(e?.message || '保存失败')
  } finally {
    savingMappings[projectKBID] = false
  }
}
async function clearRuns() {
  const confirmDialog = DialogPlugin.confirm({
    header: '二次确认',
    body: '清空自检记录将删除当前工作区所有历史审计结果，确定清空吗？',
    confirmBtn: '确定清空',
    cancelBtn: '取消',
    onConfirm: async () => {
      try {
        await clearSelfCheckRuns()
        overview.runs = []
        overview.findings = []
        MessagePlugin.success('已清空自检记录')
        confirmDialog.destroy()
      } catch (e: any) {
        MessagePlugin.error(e?.message || '清空失败')
      }
    },
  })
}
void loadModels()
load()
</script>

<style scoped>
.selfcheck-page{height:100%;overflow-y:auto;padding:34px 44px 70px;background:linear-gradient(135deg,#f5f7f7,#eef8f4);color:#25332f}.selfcheck-page>header{display:flex;justify-content:space-between;align-items:center;gap:16px;margin-bottom:20px}.header-actions{display:flex;align-items:flex-end;gap:12px}.model-picker{display:grid;gap:6px;min-width:280px}.model-picker span{color:#5f6f69;font-size:12px;font-weight:600}.eyebrow{margin:0;color:#00a870;font-size:12px;font-weight:800;letter-spacing:2px}h1{margin:5px 0;font-size:32px}header p,.panel-title p{margin:0;color:#77827e}.panel{margin:18px 0;padding:24px;background:#fff;border-radius:14px;box-shadow:0 3px 16px #1f49370d}.panel-title{display:flex;justify-content:space-between;align-items:center;margin-bottom:18px}.panel-actions{display:flex;align-items:center;gap:8px}h2{margin:0 0 5px;font-size:20px}.profile-list{display:grid;grid-template-columns:repeat(auto-fit,minmax(320px,1fr));gap:12px;align-items:stretch}.profile-card,.run-card,.result-card,.binding-row{border:1px solid #e5ece9;border-radius:10px;padding:16px}.profile-card{display:flex;flex-direction:column;min-height:140px}.kb-title,.card-actions,.run-card,.result-head,.binding-row{display:flex;align-items:center;justify-content:space-between;gap:12px}.kb-title span,.run-card p,.result-head span{color:#84908b;font-size:12px}.fields{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:9px;margin:12px 0}.profile-card.project-profile .fields{grid-template-columns:repeat(auto-fit,minmax(180px,1fr))}.owner-field small{display:block;margin-top:4px;color:#a66a10;font-size:12px;line-height:1.35}.card-actions{justify-content:flex-end;gap:8px;margin-top:auto}.run-list,.binding-list{display:grid;gap:10px}.binding-row p{margin:4px 0 0;color:#77827e}.binding-list{margin-bottom:12px}.binding-title{display:flex;align-items:center;justify-content:space-between;gap:12px;margin:2px 0 0;font-weight:700;color:#1d3830}.binding-row{background:#fffaf0}.binding-row .t-select{max-width:520px}.run-card p{margin:4px 0 0}.result-card{margin-top:12px}.result-head span{margin-left:10px}.result-head time{color:#84908b;font-size:12px}.doc-group{margin-top:14px;border:1px solid #e1e9e5;border-radius:8px;padding:12px;background:#fafcfb}.doc-group-header{display:flex;justify-content:space-between;align-items:center;margin-bottom:8px;font-weight:600;font-size:13px;color:#1d3830}.p-doc-title{color:#087958}.s-doc-title{color:#2b6cb0;font-size:12px}table{width:100%;margin-top:8px;border-collapse:collapse;font-size:13px}th{text-align:left;background:#173f35;color:white;padding:9px}td{padding:10px;border-bottom:1px solid #edf0ef;vertical-align:top}td small{display:block;margin-top:5px;color:#7d8984}.status{display:inline-block;padding:2px 8px;border-radius:12px;white-space:nowrap}.status-present,.status-complete{background:#dff6ec;color:#087958}.status-missing{background:#ffebe6;color:#c93521}.status-partial,.status-incomplete{background:#fff2d8;color:#9b6000}.status-uncertain{background:#e8efff;color:#315ba6}.status-not_applicable{background:#eef0f2;color:#687078}.run-failed{margin-top:14px;padding:12px;border-radius:8px;background:#fff0ed;color:#c73722;white-space:pre-wrap}.empty{padding:24px;text-align:center;color:#8b9692}.error{padding:12px;border-radius:8px;background:#fff0ed;color:#c73722}@media(max-width:850px){.selfcheck-page{padding:20px}.selfcheck-page>header,.header-actions,.binding-row,.binding-title{align-items:stretch;flex-direction:column}.model-picker{min-width:0}.profile-list,.fields,.profile-card.project-profile .fields{grid-template-columns:1fr}}
</style>




