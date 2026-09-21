<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ProblemBanner from '../components/ProblemBanner.vue'
import { ApiProblem } from '../api/client'
import { useSessionStore } from '../stores/session'

const { t: $t } = useI18n()
const username = ref('')
const password = ref('')
const rememberMe = ref(false)
const showPassword = ref(false)
const throttleSeconds = ref(0)
const error = ref<unknown>()
const session = useSessionStore()
const router = useRouter()
const route = useRoute()
const secureTransport = computed(() => window.location.protocol === 'https:')
let throttleTimer: ReturnType<typeof setInterval> | undefined

function startThrottle(seconds: number) {
  if (throttleTimer) clearInterval(throttleTimer)
  throttleSeconds.value = Math.max(1, Math.ceil(seconds))
  throttleTimer = setInterval(() => {
    throttleSeconds.value = Math.max(0, throttleSeconds.value - 1)
    if (throttleSeconds.value === 0 && throttleTimer) {
      clearInterval(throttleTimer)
      throttleTimer = undefined
    }
  }, 1000)
}

onBeforeUnmount(() => {
  if (throttleTimer) clearInterval(throttleTimer)
})

async function submit() {
  if (throttleSeconds.value > 0) return
  error.value = undefined
  try {
    await session.login(username.value.trim(), password.value, rememberMe.value)
    password.value = ''
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(redirect)
  } catch (cause) {
    password.value = ''
    error.value = cause
    if (cause instanceof ApiProblem && cause.code === 'login_throttled') {
      startThrottle(cause.retryAfterSeconds ?? 1)
    }
  }
}
</script>

<template>
  <q-layout view="hHh lpR fFf">
    <q-page-container>
      <q-page class="row items-center justify-center q-pa-xs">
        <q-card flat bordered style="width: 100%; max-width: 420px">
          <q-card-section>
            <div class="text-h5 text-weight-bold">{{ $t('login.title') }}</div>
            <div class="text-body2 text-grey-7 q-mt-xs">{{ $t('login.subtitle') }}</div>
          </q-card-section>
          <q-card-section class="q-gutter-xs">
            <ProblemBanner :error="error" />
            <q-input v-model="username" outlined :label="$t('login.username')" autocomplete="username" data-cy="login-username" @keyup.enter="submit" />
            <q-input v-model="password" outlined :type="showPassword ? 'text' : 'password'" :label="$t('login.password')" autocomplete="current-password" data-cy="login-password" @keyup.enter="submit">
              <template #append>
                <q-btn
                  flat round dense
                  :icon="showPassword ? 'visibility_off' : 'visibility'"
                  :aria-label="showPassword ? $t('login.hidePassword') : $t('login.showPassword')"
                  data-cy="login-password-toggle"
                  @click="showPassword = !showPassword"
                />
              </template>
            </q-input>
            <div>
              <q-checkbox v-model="rememberMe" dense :label="$t('login.remember')" data-cy="login-remember" />
              <div class="text-caption text-grey-7 q-ml-xs">
                {{ $t('login.rememberHint') }}
              </div>
              <div v-if="rememberMe && !secureTransport" class="text-caption text-warning q-ml-xs" data-cy="login-remember-warning">
                {{ $t('login.rememberInsecureWarning') }}
              </div>
            </div>
          </q-card-section>
          <q-card-actions align="right" class="q-pa-xs">
            <q-btn color="primary" :label="throttleSeconds > 0 ? $t('login.retryIn', { seconds: throttleSeconds }) : $t('login.signIn')" data-cy="login-submit" :loading="session.loading" :disable="!username.trim() || !password || throttleSeconds > 0" @click="submit" />
          </q-card-actions>
        </q-card>
      </q-page>
    </q-page-container>
  </q-layout>
</template>
