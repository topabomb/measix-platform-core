<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import { useSessionStore } from '../stores/session'

const { t: $t } = useI18n()

type PricingRule = components['schemas']['PricingRule']
type PricingSet = components['schemas']['PricingSet']
type UsageSummary = components['schemas']['UsageSummary']

const session = useSessionStore()
const loading = ref(false)
const saving = ref(false)
const error = ref<unknown>()
const revision = ref<number>()
const rules = ref<PricingRule[]>([])
const usageCost = ref<UsageSummary['cost']>()

const METER_OPTIONS = [
  { label: 'MODEL · INPUT_TOKENS', value: 'INPUT_TOKENS' },
  { label: 'MODEL · OUTPUT_TOKENS', value: 'OUTPUT_TOKENS' },
  { label: 'MODEL · CACHED_TOKENS', value: 'CACHED_TOKENS' },
  { label: 'MODEL · REQUESTS', value: 'REQUESTS' },
  { label: 'TTS · CHARACTERS', value: 'CHARACTERS' },
  { label: 'TTS · AUDIO_SECONDS', value: 'AUDIO_SECONDS' },
  { label: 'ASR · AUDIO_SECONDS', value: 'AUDIO_SECONDS' },
  { label: 'MCP · REQUESTS', value: 'REQUESTS' },
]

const METER_KIND_MAP: Record<string, string> = {
  INPUT_TOKENS: 'MODEL',
  OUTPUT_TOKENS: 'MODEL',
  CACHED_TOKENS: 'MODEL',
  REQUESTS: 'MCP',
  CHARACTERS: 'TTS',
  AUDIO_SECONDS: 'ASR',
}

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    const [set, usage] = await Promise.all([
      apiFetch<PricingSet>('/api/admin/v1/pricing'),
      apiFetch<UsageSummary>('/api/admin/v1/usage/summary').catch(() => undefined as UsageSummary | undefined),
    ])
    revision.value = set.pricingRevision
    rules.value = set.rules ?? []
    if (usage) usageCost.value = usage.cost
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

function addRule() {
  rules.value.push({
    pricingRuleId: `prc_${crypto.randomUUID()}`,
    meter: 'INPUT_TOKENS',
    unitSize: '1000',
    unitPrice: '',
    currency: 'USD',
    effectiveFrom: new Date().toISOString(),
  })
}

function removeRule(index: number) {
  rules.value.splice(index, 1)
}

async function save() {
  if (revision.value === undefined) return
  saving.value = true
  error.value = undefined
  try {
    const set = await apiFetch<PricingSet>('/api/admin/v1/pricing', {
      method: 'PUT',
      body: JSON.stringify({ expectedPricingRevision: revision.value, rules: rules.value }),
    }, session.csrfToken)
    revision.value = set.pricingRevision
    rules.value = set.rules ?? []
  } catch (cause) {
    error.value = cause
  } finally {
    saving.value = false
  }
}

function costStatusLabel(status: string): string {
  switch (status) {
    case 'KNOWN': return $t('usage.costKnown')
    case 'PARTIAL': return $t('usage.costPartial')
    default: return $t('usage.costUnknown')
  }
}

function costStatusColor(status: string): string {
  switch (status) {
    case 'KNOWN': return 'green'
    case 'PARTIAL': return 'amber'
    default: return 'grey'
  }
}

onMounted(refresh)
</script>

<template>
  <q-card flat bordered>
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-subtitle2">{{ $t('pricing.title') }}</div>
        <div class="text-caption text-grey-7">
          {{ $t('pricing.subtitle') }} {{ $t('pricing.revision') }} {{ revision ?? '—' }}
        </div>
      </div>
      <div class="q-gutter-sm">
        <q-btn flat icon="refresh" :loading="loading" @click="refresh" />
        <q-btn outline icon="add" :label="$t('pricing.addRule')" @click="addRule" data-cy="pricing-add-rule-btn" />
        <q-btn color="primary" icon="save" :label="$t('common.save')" :disable="revision === undefined" :loading="saving" @click="save" data-cy="pricing-save-btn" />
      </div>
    </q-card-section>

    <!-- Cost completeness banner -->
    <q-banner v-if="usageCost" class="bg-grey-2 q-ma-md rounded-borders">
      <div class="row items-center q-gutter-md">
        <div>
          <div class="text-caption text-grey-7">{{ $t('pricing.currentCostStatus') }}</div>
          <q-chip dense :color="costStatusColor(usageCost.status)" text-color="white" :label="costStatusLabel(usageCost.status)" />
        </div>
        <div v-if="usageCost.amount" class="text-h6">{{ usageCost.amount }} {{ usageCost.currency ?? '' }}</div>
        <div class="text-caption text-grey-7">{{ $t('pricing.costUnknownHint') }}</div>
      </div>
    </q-banner>

    <q-separator />
    <ProblemBanner :error="error" class="q-ma-md" />
    <LoadingState v-if="loading" />

    <q-list v-else separator>
      <q-item v-for="(rule, idx) in rules" :key="rule.pricingRuleId">
        <q-item-section>
          <div class="row q-col-gutter-sm items-center">
            <div class="col-12 col-md-2">
              <q-select v-model="rule.meter" dense outlined :options="METER_OPTIONS" emit-value map-options :label="$t('pricing.meter')" />
              <div class="text-caption text-grey-7">{{ $t('pricing.meterKind') }}: {{ METER_KIND_MAP[rule.meter] ?? '—' }}</div>
            </div>
            <div class="col-6 col-md-2">
              <q-input v-model="rule.resourceId" dense outlined :label="$t('pricing.resourceId')" placeholder="(global)" :hint="$t('pricing.resourceIdHint')" />
            </div>
            <div class="col-6 col-md-2">
              <q-input v-model="rule.upstreamId" dense outlined :label="$t('pricing.upstreamId')" placeholder="(all)" :hint="$t('pricing.upstreamIdHint')" />
            </div>
            <div class="col-6 col-md-1">
              <q-input v-model="rule.unitSize" dense outlined :label="$t('pricing.unitSize')" />
            </div>
            <div class="col-6 col-md-1">
              <q-input v-model="rule.unitPrice" dense outlined :label="$t('pricing.unitPrice')" data-cy="pricing-unit-price" />
            </div>
            <div class="col-6 col-md-1">
              <q-input v-model="rule.currency" dense outlined :label="$t('pricing.currency')" />
            </div>
            <div class="col-6 col-md-2">
              <q-input v-model="rule.effectiveFrom" dense outlined :label="$t('pricing.effectiveFrom')" />
            </div>
            <div class="col-12 col-md-1 text-right">
              <q-btn flat dense color="negative" icon="delete" @click="removeRule(idx)" />
            </div>
          </div>
          <div class="text-caption text-grey-7 q-mt-xs">{{ rule.pricingRuleId }}</div>
        </q-item-section>
      </q-item>
      <q-item v-if="!rules.length">
        <q-item-section class="text-grey-7">{{ $t('pricing.noRules') }}</q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>
