<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { apiFetch } from '../api/client'
import ProblemBanner from './ProblemBanner.vue'

const props = defineProps<{ modelValue: boolean; csrfToken?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; changed: [] }>()
const { t: $t } = useI18n()
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const showPasswords = ref(false)
const saving = ref(false)
const error = ref<unknown>()

const confirmationMatches = computed(() => !confirmPassword.value || confirmPassword.value === newPassword.value)
const canSubmit = computed(() => Boolean(props.csrfToken && currentPassword.value && newPassword.value.length >= 12 && newPassword.value.length <= 128 && confirmPassword.value === newPassword.value))

watch(() => props.modelValue, open => { if (!open) reset() })

function reset() {
  currentPassword.value = ''
  newPassword.value = ''
  confirmPassword.value = ''
  showPasswords.value = false
  error.value = undefined
}

function close() {
  if (!saving.value) emit('update:modelValue', false)
}

async function submit() {
  if (!canSubmit.value || !props.csrfToken) return
  saving.value = true
  error.value = undefined
  try {
    await apiFetch<void>('/api/admin/v1/session:change-password', {
      method: 'POST',
      body: JSON.stringify({ currentPassword: currentPassword.value, newPassword: newPassword.value, confirmPassword: confirmPassword.value }),
    }, props.csrfToken)
    reset()
    emit('changed')
  } catch (cause) {
    error.value = cause
    currentPassword.value = ''
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="value => emit('update:modelValue', value)">
    <q-card class="app-dialog app-dialog--sm" data-cy="change-password-dialog">
      <q-card-section>
        <div class="text-h6">{{ $t('account.changePassword') }}</div>
        <div class="text-caption text-grey-7">{{ $t('account.changePasswordHint') }}</div>
      </q-card-section>
      <q-separator />
      <q-card-section class="app-dialog__body q-gutter-xs">
        <ProblemBanner :error="error" />
        <q-input v-model="currentPassword" outlined dense :type="showPasswords ? 'text' : 'password'" :label="$t('account.currentPassword')" autocomplete="current-password" data-cy="current-password" />
        <q-input v-model="newPassword" outlined dense :type="showPasswords ? 'text' : 'password'" :label="$t('account.newPassword')" :hint="$t('account.passwordRule')" autocomplete="new-password" data-cy="new-password" />
        <q-input v-model="confirmPassword" outlined dense :type="showPasswords ? 'text' : 'password'" :label="$t('account.confirmPassword')" autocomplete="new-password" :error="!confirmationMatches" :error-message="$t('account.passwordMismatch')" data-cy="confirm-password" @keyup.enter="submit" />
        <q-checkbox v-model="showPasswords" dense :label="$t('account.showPasswords')" />
        <q-banner dense class="bg-purple-1 text-primary rounded-borders">{{ $t('account.sessionNotice') }}</q-banner>
      </q-card-section>
      <q-separator />
      <q-card-actions align="right">
        <q-btn flat dense :label="$t('common.cancel')" :disable="saving" @click="close" />
        <q-btn color="primary" dense :label="$t('account.confirmChange')" :loading="saving" :disable="!canSubmit" data-cy="change-password-submit" @click="submit" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>
