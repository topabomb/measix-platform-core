<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { components } from '../api/generated'
import { ApiProblem, apiFetch, commandResultUncertain } from '../api/client'
import ProblemBanner from './ProblemBanner.vue'

type User = components['schemas']['User']
const props = defineProps<{ modelValue: boolean; user: User; mode: 'password' | 'role'; csrfToken?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean]; completed: [userId: string, user?: User]; conflict: [userId: string, error: unknown] }>()
const { t } = useI18n()
const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const saving = ref(false)
const error = ref<unknown>()
const uncertain = ref(false)
const targetRole = computed(() => props.user.role === 'ADMIN' ? 'MEMBER' : 'ADMIN')
const needsPassword = computed(() => props.mode === 'password' || targetRole.value === 'ADMIN' && !props.user.passwordConfigured)
const title = computed(() => t(props.mode === 'password' ? 'users.resetPassword' : targetRole.value === 'ADMIN' ? 'users.grantAdmin' : 'users.removeAdmin'))
const validPassword = computed(() => { const count = Array.from(newPassword.value).length; return count >= 12 && count <= 128 })
const canSubmit = computed(() => Boolean(props.csrfToken && currentPassword.value && (!needsPassword.value || validPassword.value && newPassword.value === confirmPassword.value)))

function clearPasswords() { currentPassword.value = ''; newPassword.value = ''; confirmPassword.value = '' }
watch(() => props.modelValue, () => { clearPasswords(); error.value = undefined; uncertain.value = false })
onBeforeUnmount(clearPasswords)
function close() { if (!saving.value) { clearPasswords(); emit('update:modelValue', false) } }

async function submit() {
  if (!canSubmit.value || saving.value || !props.csrfToken) return
  const target = props.user
  const mode = props.mode
  saving.value = true
  error.value = undefined
  uncertain.value = false
  try {
    let user: User | undefined
    if (mode === 'password') {
      const body: components['schemas']['SetPasswordRequest'] = { currentPassword: currentPassword.value, newPassword: newPassword.value, confirmPassword: confirmPassword.value }
      await apiFetch<void>(`/api/admin/v1/users/${encodeURIComponent(target.userId)}:set-password`, { method: 'POST', body: JSON.stringify(body) }, props.csrfToken)
    } else {
      const body: components['schemas']['SetUserRoleRequest'] = { role: targetRole.value, expectedRole: target.role, currentPassword: currentPassword.value }
      if (needsPassword.value) { body.newPassword = newPassword.value; body.confirmPassword = confirmPassword.value }
      user = await apiFetch<User>(`/api/admin/v1/users/${encodeURIComponent(target.userId)}:set-role`, { method: 'POST', body: JSON.stringify(body) }, props.csrfToken)
    }
    clearPasswords()
    emit('update:modelValue', false)
    emit('completed', target.userId, user)
  } catch (cause) {
    clearPasswords()
    error.value = cause
    uncertain.value = commandResultUncertain(cause)
    if (cause instanceof ApiProblem && cause.code === 'user_role_conflict') {
      emit('update:modelValue', false)
      emit('conflict', target.userId, cause)
    }
  } finally { saving.value = false }
}
</script>

<template>
  <q-dialog :model-value="modelValue" persistent @update:model-value="close">
    <q-card class="app-dialog app-dialog--sm" data-cy="admin-account-dialog">
      <q-card-section><div class="text-h6">{{ title }}</div><div class="text-caption">{{ user.displayName }} · {{ user.username }}</div></q-card-section>
      <q-separator />
      <q-card-section class="app-dialog__body q-gutter-xs">
        <q-banner v-if="uncertain" dense class="bg-orange-1" data-cy="account-result-uncertain">{{ t('users.resultUncertain') }}</q-banner>
        <ProblemBanner v-else :error="error" />
        <div class="text-body2">{{ t(mode === 'password' ? 'users.passwordHint' : 'users.roleHint') }}</div>
        <div v-if="mode === 'role' && user.status === 'DISABLED'" class="text-caption">{{ t('users.disabledRoleHint') }}</div>
        <q-input v-model="currentPassword" type="password" outlined dense :label="t('users.actorPassword')" autocomplete="current-password" :disable="saving" data-cy="account-current-password" />
        <template v-if="needsPassword">
          <q-input v-model="newPassword" type="password" outlined dense :label="t('account.newPassword')" :hint="t('account.passwordRule')" autocomplete="new-password" :disable="saving" data-cy="account-new-password" />
          <q-input v-model="confirmPassword" type="password" outlined dense :label="t('account.confirmPassword')" autocomplete="new-password" :disable="saving" :error="Boolean(confirmPassword && confirmPassword !== newPassword)" :error-message="t('account.passwordMismatch')" data-cy="account-confirm-password" />
        </template>
        <div v-else-if="mode === 'role' && targetRole === 'ADMIN'" class="text-caption">{{ t('users.keepPasswordHint') }}</div>
        <q-banner dense class="bg-purple-1 text-primary">{{ t('users.webSessionsNotice') }}</q-banner>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn flat :label="t('common.cancel')" :disable="saving" @click="close" />
        <q-btn :color="mode === 'role' && targetRole === 'MEMBER' ? 'negative' : 'primary'" :label="title" :disable="!canSubmit" :loading="saving" data-cy="account-submit" @click="submit" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>
