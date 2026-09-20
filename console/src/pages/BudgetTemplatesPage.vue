<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { ApiProblem, apiFetch } from '../api/client'
import type { BudgetCapability, BudgetLimitDefinition, BudgetMode, BudgetTemplate, BudgetTemplatePage, BudgetTemplateRule } from '../api/usageBudget'
import { budgetRuleValueKey, capabilities } from '../api/usageBudget'
import { useSessionStore } from '../stores/session'
import DetailWorkspace from '../components/DetailWorkspace.vue'
import BudgetRuleEditor from '../components/BudgetRuleEditor.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import LoadingState from '../components/LoadingState.vue'
import CursorPager from '../components/CursorPager.vue'
import { useCursorPager } from '../composables/useCursorPager'
import { cursorPath } from '../api/pagination'

type RuleState = { included: boolean; mode: BudgetMode; limits: BudgetLimitDefinition[] }
type User = components['schemas']['User']
type UserPage = components['schemas']['UserPage']
type BudgetTemplateAuditItem = components['schemas']['BudgetTemplateAuditItem']
type BudgetTemplateAuditPage = components['schemas']['BudgetTemplateAuditPage']
type BudgetTemplateAuditSnapshot = components['schemas']['BudgetTemplateAuditSnapshot']

const { t: $t } = useI18n()
const session = useSessionStore()
const listPath = ref('/api/admin/v1/budget-templates?limit=50')
const {
  items: templates,
  nextCursor,
  pageNumber,
  loading,
  error,
  reset: load,
  nextPage,
  previousPage,
} = useCursorPager<BudgetTemplate, BudgetTemplatePage>(listPath, path => apiFetch<BudgetTemplatePage>(path))
const selected = ref<BudgetTemplate>()
const saving = ref(false)
const search = ref('')
const name = ref('')
const description = ref('')
const reason = ref('')
const rules = reactive<Record<BudgetCapability, RuleState>>(emptyRules())
const creating = ref(false)
const assignedUsers = ref<User[]>()
const usersLoading = ref(false)
const assignedUsersNextCursor = ref<string>()
const auditItems = ref<BudgetTemplateAuditItem[]>()
const auditLoading = ref(false)
const auditNextCursor = ref<string>()
const saveConfirmOpen = ref(false)
const deleteConfirmOpen = ref(false)
const conflict = ref(false)
const saveFeedback = ref<'created' | 'updated'>()

const canSave = computed(() => Boolean(session.csrfToken && name.value.trim() && reason.value.trim()) && capabilities.every(capability => {
  const rule = rules[capability]
  if (!rule.included || rule.mode === 'UNLIMITED') return true
  if (!rule.limits.length) return false
  const keys = new Set<string>()
  return rule.limits.every(limit => {
    const key = `${limit.period}:${limit.meter}`
    if (keys.has(key) || !/^(0|[1-9]\d*)$/.test(limit.limit)) return false
    keys.add(key)
    return true
  })
}))

function emptyRules(): Record<BudgetCapability, RuleState> {
  return Object.fromEntries(capabilities.map(capability => [capability, { included: false, mode: 'UNLIMITED', limits: [] }])) as unknown as Record<BudgetCapability, RuleState>
}

function resetRules() {
  for (const capability of capabilities) Object.assign(rules[capability], { included: false, mode: 'UNLIMITED', limits: [] })
}

function open(template: BudgetTemplate) {
  conflict.value = false
  saveFeedback.value = undefined
  selected.value = template
  creating.value = false
  name.value = template.name
  description.value = template.description
  reason.value = ''
  assignedUsers.value = undefined
  assignedUsersNextCursor.value = undefined
  auditItems.value = undefined
  auditNextCursor.value = undefined
  resetRules()
  for (const rule of template.rules) Object.assign(rules[rule.capability], { included: true, mode: rule.mode, limits: rule.limits.map(limit => ({ ...limit })) })
}

async function loadAudit(more = false) {
  if (!selected.value || auditLoading.value) return
  auditLoading.value = true
  error.value = undefined
  try {
    const base = `/api/admin/v1/budget-templates/${encodeURIComponent(selected.value.budgetTemplateId)}/audit?limit=20`
    const page = await apiFetch<BudgetTemplateAuditPage>(more && auditNextCursor.value ? cursorPath(base, auditNextCursor.value) : base)
    auditItems.value = more ? [...(auditItems.value ?? []), ...page.items] : page.items
    auditNextCursor.value = page.nextCursor
  } catch (cause) {
    error.value = cause
  } finally {
    auditLoading.value = false
  }
}

async function loadAssignedUsers(more = false) {
  if (!selected.value || usersLoading.value) return
  usersLoading.value = true
  error.value = undefined
  try {
    const base = `/api/admin/v1/budget-templates/${encodeURIComponent(selected.value.budgetTemplateId)}/users?limit=50`
    const page = await apiFetch<UserPage>(more && assignedUsersNextCursor.value ? cursorPath(base, assignedUsersNextCursor.value) : base)
    assignedUsers.value = more ? [...(assignedUsers.value ?? []), ...page.items] : page.items
    assignedUsersNextCursor.value = page.nextCursor
  } catch (cause) {
    error.value = cause
  } finally {
    usersLoading.value = false
  }
}

function beginCreate() {
  conflict.value = false
  saveFeedback.value = undefined
  selected.value = undefined
  creating.value = true
  name.value = ''
  description.value = ''
  reason.value = ''
  resetRules()
}

function backToList() {
  selected.value = undefined
  creating.value = false
  saveConfirmOpen.value = false
  deleteConfirmOpen.value = false
  conflict.value = false
  saveFeedback.value = undefined
}

let searchTimer: ReturnType<typeof setTimeout> | undefined
watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    const query = new URLSearchParams({ limit: '50' })
    if (search.value.trim()) query.set('query', search.value.trim())
    listPath.value = `/api/admin/v1/budget-templates?${query}`
    void load()
  }, 300)
})

function requestRules() {
  return capabilities.filter(capability => rules[capability].included).map(capability => ({
    capability,
    mode: rules[capability].mode,
    limits: rules[capability].mode === 'LIMITED' ? rules[capability].limits : [],
  }))
}

const templateRuleChanges = computed(() => {
  if (!selected.value || creating.value) return []
  const nextRules = requestRules()
  return capabilities.flatMap(capability => {
    const before = selected.value!.rules.find(rule => rule.capability === capability)
    const after = nextRules.find(rule => rule.capability === capability)
    return budgetRuleValueKey(before) === budgetRuleValueKey(after) ? [] : [{ capability, before, after }]
  })
})

function requestSave() {
  if (canSave.value) saveConfirmOpen.value = true
}

function requestDelete() {
  if (selected.value && selected.value.assignedUserCount === 0 && reason.value.trim()) deleteConfirmOpen.value = true
}

async function reloadSelected() {
  if (!selected.value) {
    await load()
    return
  }
  const templateId = selected.value.budgetTemplateId
  const current = await apiFetch<BudgetTemplate>(`/api/admin/v1/budget-templates/${encodeURIComponent(templateId)}`)
  open(current)
  conflict.value = true
}

async function handleConflict(cause: unknown): Promise<boolean> {
  if (!(cause instanceof ApiProblem) || cause.status !== 409) return false
  saveConfirmOpen.value = false
  deleteConfirmOpen.value = false
  try {
    await reloadSelected()
  } catch (reloadCause) {
    error.value = reloadCause
  }
  conflict.value = true
  return true
}

function auditRule(snapshot: BudgetTemplateAuditSnapshot | undefined, capability: BudgetCapability) {
  return snapshot?.rules.find(rule => rule.capability === capability)
}

function ruleSummary(rule: BudgetTemplateRule | undefined): string {
  if (!rule) return $t('budgetTemplates.auditDefault')
  if (rule.mode === 'UNLIMITED') return $t('budgets.mode.UNLIMITED')
  return rule.limits.map(limit => `${$t(`budgets.period.${limit.period}`)} · ${$t(`usage.meters.${limit.meter}`)}: ${limit.limit}`).join('; ')
}

function auditRuleSummary(snapshot: BudgetTemplateAuditSnapshot | undefined, capability: BudgetCapability): string {
  return ruleSummary(auditRule(snapshot, capability))
}

async function save() {
  if (!session.csrfToken || !canSave.value) return
  const wasCreating = creating.value
  saving.value = true
  error.value = undefined
  try {
    const path = creating.value ? '/api/admin/v1/budget-templates' : `/api/admin/v1/budget-templates/${encodeURIComponent(selected.value!.budgetTemplateId)}`
    const body = creating.value
      ? { name: name.value.trim(), description: description.value.trim(), rules: requestRules(), reason: reason.value.trim() }
      : { expectedRevision: selected.value!.revision, name: name.value.trim(), description: description.value.trim(), rules: requestRules(), reason: reason.value.trim() }
    const saved = await apiFetch<BudgetTemplate>(path, { method: creating.value ? 'POST' : 'PUT', body: JSON.stringify(body) }, session.csrfToken)
    saveConfirmOpen.value = false
    creating.value = false
    selected.value = saved
    saveFeedback.value = wasCreating ? 'created' : 'updated'
    await load()
  } catch (cause) {
    if (!(await handleConflict(cause))) error.value = cause
  } finally {
    saving.value = false
  }
}

async function remove() {
  if (!session.csrfToken || !selected.value || selected.value.assignedUserCount > 0 || !reason.value.trim()) return
  saving.value = true
  error.value = undefined
  try {
    const query = new URLSearchParams({ expectedRevision: String(selected.value.revision), reason: reason.value.trim() })
    await apiFetch<void>(`/api/admin/v1/budget-templates/${encodeURIComponent(selected.value.budgetTemplateId)}?${query}`, { method: 'DELETE' }, session.csrfToken)
    deleteConfirmOpen.value = false
    selected.value = undefined
    creating.value = false
    await load()
  } catch (cause) {
    if (!(await handleConflict(cause))) error.value = cause
  } finally {
    saving.value = false
  }
}

onMounted(() => { void load() })
onBeforeUnmount(() => { if (searchTimer) clearTimeout(searchTimer) })
</script>

<template>
  <div class="admin-page">
    <div class="row items-center justify-between q-mb-xs">
      <div>
        <h1 class="text-h5 q-my-none">{{ $t('budgetTemplates.title') }}</h1>
        <div class="text-caption text-grey-7">{{ $t('budgetTemplates.subtitle') }}</div>
      </div>
      <q-btn color="primary" icon="add" no-caps :label="$t('budgetTemplates.create')" @click="beginCreate" />
    </div>
    <q-banner v-if="conflict" class="bg-orange-1 q-mb-xs" data-cy="budget-template-conflict">
      <div class="row items-center justify-between q-gutter-xs">
        <span>{{ $t('budgetTemplates.conflictReloaded') }}</span>
        <q-btn flat dense no-caps :label="$t('common.refresh')" @click="reloadSelected" />
      </div>
    </q-banner>
    <q-banner v-if="saveFeedback === 'updated'" class="bg-green-1 q-mb-xs" data-cy="budget-template-saved">
      {{ $t('budgetTemplates.liveLinkedSaved') }}
    </q-banner>
    <q-banner v-else-if="saveFeedback === 'created'" class="bg-green-1 q-mb-xs" data-cy="budget-template-saved">
      {{ $t('budgetTemplates.createdSaved') }}
    </q-banner>
    <ProblemBanner :error="error" class="q-mb-xs" />
    <LoadingState v-if="loading && !templates.length" />
    <DetailWorkspace v-else :detail-open="Boolean(selected) || creating">
      <template #list>
        <q-card flat bordered>
          <q-card-section class="q-pa-xs"><q-input v-model="search" dense outlined clearable :label="$t('common.search')"><template #prepend><q-icon name="search" /></template></q-input></q-card-section>
          <q-separator />
          <q-list separator>
            <q-item v-for="template in templates" :key="template.budgetTemplateId" clickable :active="selected?.budgetTemplateId === template.budgetTemplateId" data-cy="budget-template-row" @click="open(template)">
              <q-item-section>
                <q-item-label>{{ template.name }}</q-item-label>
                <q-item-label v-if="template.description" caption lines="2">{{ template.description }}</q-item-label>
                <q-item-label caption>{{ $t('budgetTemplates.updatedAt', { time: new Date(template.updatedAt).toLocaleString() }) }} · {{ $t('budgetTemplates.assignmentCount', { count: template.assignedUserCount }) }} · {{ $t('budgets.revision', { revision: template.revision }) }}</q-item-label>
              </q-item-section>
            </q-item>
            <q-item v-if="!templates.length"><q-item-section class="text-grey-7">{{ $t('budgetTemplates.empty') }}</q-item-section></q-item>
          </q-list>
          <q-separator />
          <CursorPager :page="pageNumber" :count="templates.length" :has-next="Boolean(nextCursor)" :loading="loading" @previous="previousPage" @next="nextPage" />
        </q-card>
      </template>
      <template #detail>
        <q-card flat bordered>
          <q-card-section class="q-pb-none">
            <q-btn flat dense no-caps icon="arrow_back" :label="$t('resources.backToList')" data-cy="budget-template-back" @click="backToList" />
          </q-card-section>
          <q-card-section class="row items-start justify-between">
            <div>
              <div class="text-subtitle1 text-weight-medium">{{ creating ? $t('budgetTemplates.create') : name }}</div>
              <div v-if="selected" class="text-caption text-grey-7">{{ selected.budgetTemplateId }} · {{ $t('budgets.revision', { revision: selected.revision }) }}</div>
            </div>
            <q-btn v-if="!creating" flat dense color="negative" icon="delete" :aria-label="$t('common.delete')" :disable="selected!.assignedUserCount > 0 || !reason.trim()" @click="requestDelete" />
          </q-card-section>
          <div v-if="selected && selected.assignedUserCount > 0" class="text-caption text-negative q-px-md q-pb-sm">{{ $t('budgetTemplates.deleteAssignedBlocked') }}</div>
          <q-separator />
          <q-card-section class="q-gutter-xs">
            <q-input v-model="name" outlined dense :label="$t('budgetTemplates.name')" :maxlength="100" />
            <q-input v-model="description" outlined dense type="textarea" autogrow :label="$t('budgetTemplates.description')" maxlength="500" />
          </q-card-section>
          <template v-if="selected && !creating">
            <q-separator />
            <q-card-section>
              <div class="row items-center justify-between">
                <div>
                  <div class="text-subtitle2">{{ $t('budgetTemplates.assignedUsers') }}</div>
                  <div class="text-caption text-grey-7">{{ $t('budgetTemplates.assignmentCount', { count: selected.assignedUserCount }) }}</div>
                </div>
                <q-btn flat dense no-caps icon="group" :label="$t('budgetTemplates.viewAssignedUsers')" :loading="usersLoading" @click="loadAssignedUsers()" />
              </div>
              <q-list v-if="assignedUsers" dense separator class="q-mt-xs" data-cy="budget-template-users">
                <q-item v-for="user in assignedUsers" :key="user.userId">
                  <q-item-section>
                    <q-item-label>{{ user.displayName }}</q-item-label>
                    <q-item-label caption>{{ user.username }}</q-item-label>
                  </q-item-section>
                  <q-item-section side>{{ user.status }}</q-item-section>
                </q-item>
                <q-item v-if="!assignedUsers.length"><q-item-section class="text-grey-7">{{ $t('budgetTemplates.noAssignedUsers') }}</q-item-section></q-item>
              </q-list>
              <q-btn v-if="assignedUsersNextCursor" flat dense no-caps :label="$t('common.loadMore')" :loading="usersLoading" @click="loadAssignedUsers(true)" />
            </q-card-section>
          </template>
          <q-separator />
          <q-card-section class="template-rules">
            <div v-for="capability in capabilities" :key="capability" class="template-rule" :data-cy="`template-rule-${capability}`">
              <div class="row items-center justify-between">
                <div class="text-subtitle2">{{ $t(`usage.kind.${capability}`) }}</div>
                <q-toggle v-model="rules[capability].included" :label="$t('budgetTemplates.useRule')" />
              </div>
              <BudgetRuleEditor v-if="rules[capability].included" :capability="capability" :mode="rules[capability].mode" :limits="rules[capability].limits" @update:mode="rules[capability].mode = $event" @update:limits="rules[capability].limits = $event" />
              <div v-else class="text-caption text-grey-7">{{ $t('budgetTemplates.systemDefault') }}</div>
            </div>
          </q-card-section>
          <template v-if="selected && !creating">
            <q-separator />
            <q-card-section>
              <div class="row items-center justify-between">
                <div class="text-subtitle2">{{ $t('budgetTemplates.audit') }}</div>
                <q-btn flat dense no-caps icon="history" :label="$t('budgetTemplates.viewAudit')" :loading="auditLoading" @click="loadAudit()" />
              </div>
              <q-list v-if="auditItems" dense separator class="q-mt-xs" data-cy="budget-template-audit">
                <q-item v-for="item in auditItems" :key="item.auditId">
                  <q-item-section>
                    <q-item-label>{{ $t(`budgetTemplates.actions.${item.action}`) }}<template v-if="item.userId"> · {{ item.userId }}</template></q-item-label>
                    <q-item-label caption>{{ item.changedBy }} · {{ new Date(item.createdAt).toLocaleString() }}</q-item-label>
                    <q-item-label caption>{{ item.reason }}</q-item-label>
                    <details v-if="item.before || item.after" class="audit-change text-caption q-mt-xs">
                      <summary class="cursor-pointer text-primary">{{ $t('budgetTemplates.auditChanges') }}</summary>
                      <div class="audit-row audit-heading">
                        <span>{{ $t('budgetTemplates.auditField') }}</span>
                        <span>{{ $t('budgetTemplates.auditBefore') }}</span>
                        <span>{{ $t('budgetTemplates.auditAfter') }}</span>
                      </div>
                      <div class="audit-row">
                        <span>{{ $t('budgetTemplates.name') }}</span>
                        <span>{{ item.before?.name ?? '—' }}</span>
                        <span>{{ item.after?.name ?? '—' }}</span>
                      </div>
                      <div class="audit-row">
                        <span>{{ $t('budgetTemplates.description') }}</span>
                        <span>{{ item.before?.description || '—' }}</span>
                        <span>{{ item.after?.description || '—' }}</span>
                      </div>
                      <div v-for="capability in capabilities" :key="capability" class="audit-row">
                        <span>{{ $t(`usage.kind.${capability}`) }}</span>
                        <span>{{ auditRuleSummary(item.before, capability) }}</span>
                        <span>{{ auditRuleSummary(item.after, capability) }}</span>
                      </div>
                    </details>
                  </q-item-section>
                </q-item>
                <q-item v-if="!auditItems.length"><q-item-section class="text-grey-7">{{ $t('budgetTemplates.noAudit') }}</q-item-section></q-item>
              </q-list>
              <q-btn v-if="auditNextCursor" flat dense no-caps :label="$t('common.loadMore')" :loading="auditLoading" @click="loadAudit(true)" />
            </q-card-section>
          </template>
          <q-separator />
          <q-card-section>
            <q-input v-model="reason" outlined dense :label="$t('budgets.reasonLabel')" :hint="$t('budgets.reasonHint')" maxlength="500" />
            <div class="row justify-end q-mt-sm"><q-btn color="primary" :label="$t('common.save')" :disable="!canSave" :loading="saving" @click="requestSave" /></div>
          </q-card-section>
        </q-card>
      </template>
    </DetailWorkspace>

    <q-dialog v-model="saveConfirmOpen">
      <q-card style="min-width: min(420px, 90vw)">
        <q-card-section class="text-subtitle1">{{ creating ? $t('budgetTemplates.confirmCreateTitle') : $t('budgetTemplates.confirmUpdateTitle') }}</q-card-section>
        <q-card-section class="q-pt-none text-body2">
          <template v-if="creating">{{ $t('budgetTemplates.createImpact') }}</template>
          <template v-else>
            <div>{{ $t('budgetTemplates.updateImpact', { count: selected?.assignedUserCount ?? 0 }) }}</div>
            <div class="text-caption text-grey-7 q-mt-sm">{{ $t('budgetTemplates.ruleChanges') }}</div>
            <div v-if="!templateRuleChanges.length" class="text-caption q-mt-xs">{{ $t('budgetTemplates.noRuleChanges') }}</div>
            <div v-for="change in templateRuleChanges" :key="change.capability" class="rule-change q-mt-xs" :data-cy="`budget-template-change-${change.capability}`">
              <div class="text-weight-medium">{{ $t(`usage.kind.${change.capability}`) }}</div>
              <div class="text-caption"><span class="text-grey-7">{{ $t('budgetTemplates.auditBefore') }}:</span> {{ ruleSummary(change.before) }}</div>
              <div class="text-caption"><span class="text-grey-7">{{ $t('budgetTemplates.auditAfter') }}:</span> {{ ruleSummary(change.after) }}</div>
            </div>
          </template>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps :label="$t('common.cancel')" @click="saveConfirmOpen = false" />
          <q-btn color="primary" no-caps :label="$t('common.save')" :loading="saving" data-cy="confirm-budget-template-save" @click="save" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="deleteConfirmOpen">
      <q-card style="min-width: min(420px, 90vw)">
        <q-card-section class="text-subtitle1">{{ $t('budgetTemplates.confirmDeleteTitle') }}</q-card-section>
        <q-card-section class="q-pt-none text-body2">{{ $t('budgetTemplates.deleteImpact', { name: selected?.name ?? '' }) }}</q-card-section>
        <q-card-actions align="right">
          <q-btn flat no-caps :label="$t('common.cancel')" @click="deleteConfirmOpen = false" />
          <q-btn color="negative" no-caps :label="$t('common.delete')" :loading="saving" data-cy="confirm-budget-template-delete" @click="remove" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<style scoped>
.template-rules { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 8px; }
.template-rule { border: 1px solid rgba(0, 0, 0, .12); border-radius: 4px; padding: 8px; min-width: 0; }
.audit-change { max-width: 100%; }
.audit-row { display: grid; grid-template-columns: minmax(96px, .7fr) repeat(2, minmax(120px, 1fr)); gap: 8px; padding: 4px 0; border-top: 1px solid rgba(0, 0, 0, .08); }
.audit-heading { color: rgba(0, 0, 0, .6); border-top: 0; }
@media (max-width: 700px) { .template-rules { grid-template-columns: minmax(0, 1fr); } }
</style>
