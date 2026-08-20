<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { components } from '../api/generated'
import { apiFetch } from '../api/client'
import LoadingState from '../components/LoadingState.vue'
import ProblemBanner from '../components/ProblemBanner.vue'
import { useSessionStore } from '../stores/session'

type PricingRule = components['schemas']['PricingRule']
type PricingSet = components['schemas']['PricingSet']

const session = useSessionStore()
const loading = ref(false)
const saving = ref(false)
const error = ref<unknown>()
const revision = ref<number>()
const rules = ref<PricingRule[]>([])

const METER_OPTIONS = [
  { label: 'MODEL · prompt_tokens', value: 'prompt_tokens' },
  { label: 'MODEL · output_tokens', value: 'output_tokens' },
  { label: 'MODEL · cached_tokens', value: 'cached_tokens' },
  { label: 'MODEL · requests', value: 'requests' },
  { label: 'TTS · characters', value: 'characters' },
  { label: 'TTS · audio_seconds', value: 'audio_seconds' },
  { label: 'ASR · audio_seconds', value: 'audio_seconds' },
  { label: 'MCP · requests', value: 'requests' },
]

async function refresh() {
  loading.value = true
  error.value = undefined
  try {
    const set = await apiFetch<PricingSet>('/api/admin/v1/pricing')
    revision.value = set.pricingRevision
    rules.value = set.rules ?? []
  } catch (cause) {
    error.value = cause
  } finally {
    loading.value = false
  }
}

function addRule() {
  rules.value.push({
    pricingRuleId: `prc_${crypto.randomUUID()}`,
    meter: 'prompt_tokens',
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

onMounted(refresh)
</script>

<template>
  <q-card flat bordered>
    <q-card-section class="row items-center justify-between">
      <div>
        <div class="text-subtitle2">Pricing</div>
        <div class="text-caption text-grey-7">
          Per-meter cost rules applied to usage records. Revision {{ revision ?? '—' }}
        </div>
      </div>
      <div class="q-gutter-sm">
        <q-btn flat icon="refresh" :loading="loading" @click="refresh" />
        <q-btn outline icon="add" label="Add rule" @click="addRule" />
        <q-btn color="primary" icon="save" label="Save" :disable="revision === undefined" :loading="saving" @click="save" />
      </div>
    </q-card-section>

    <q-separator />
    <ProblemBanner :error="error" class="q-ma-md" />
    <LoadingState v-if="loading" />

    <q-list v-else separator>
      <q-item v-for="(rule, idx) in rules" :key="rule.pricingRuleId">
        <q-item-section>
          <div class="row q-col-gutter-sm items-center">
            <div class="col-12 col-md-3">
              <q-select v-model="rule.meter" dense outlined :options="METER_OPTIONS" emit-value map-options label="Meter" />
            </div>
            <div class="col-6 col-md-2">
              <q-input v-model="rule.unitSize" dense outlined label="Unit size" />
            </div>
            <div class="col-6 col-md-2">
              <q-input v-model="rule.unitPrice" dense outlined label="Unit price" />
            </div>
            <div class="col-6 col-md-2">
              <q-input v-model="rule.currency" dense outlined label="Currency" />
            </div>
            <div class="col-6 col-md-2">
              <q-input v-model="rule.effectiveFrom" dense outlined label="Effective from" />
            </div>
            <div class="col-12 col-md-1 text-right">
              <q-btn flat dense color="negative" icon="delete" @click="removeRule(idx)" />
            </div>
          </div>
          <div class="text-caption text-grey-7 q-mt-xs">{{ rule.pricingRuleId }}</div>
        </q-item-section>
      </q-item>
      <q-item v-if="!rules.length">
        <q-item-section class="text-grey-7">No pricing rules yet. Add a rule to price usage.</q-item-section>
      </q-item>
    </q-list>
  </q-card>
</template>
