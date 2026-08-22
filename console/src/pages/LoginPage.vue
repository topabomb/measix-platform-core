<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import ProblemBanner from '../components/ProblemBanner.vue'
import { useSessionStore } from '../stores/session'

const { t: $t } = useI18n()
const username = ref('')
const password = ref('')
const error = ref<unknown>()
const session = useSessionStore()
const router = useRouter()
const route = useRoute()

async function submit() {
  error.value = undefined
  try {
    await session.login(username.value.trim(), password.value)
    password.value = ''
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/'
    await router.replace(redirect)
  } catch (cause) {
    password.value = ''
    error.value = cause
  }
}
</script>

<template>
  <q-layout view="hHh lpR fFf">
    <q-page-container>
      <q-page class="row items-center justify-center bg-grey-1 q-pa-md">
        <q-card flat bordered style="width: 100%; max-width: 420px">
          <q-card-section>
            <div class="text-h5 text-weight-bold">{{ $t('login.title') }}</div>
            <div class="text-body2 text-grey-7 q-mt-xs">{{ $t('login.subtitle') }}</div>
          </q-card-section>
          <q-card-section class="q-gutter-md">
            <ProblemBanner :error="error" />
            <q-input v-model="username" outlined :label="$t('login.username')" autocomplete="username" data-cy="login-username" @keyup.enter="submit" />
            <q-input v-model="password" outlined type="password" :label="$t('login.password')" autocomplete="current-password" data-cy="login-password" @keyup.enter="submit" />
          </q-card-section>
          <q-card-actions align="right" class="q-pa-md">
            <q-btn color="primary" :label="$t('login.signIn')" data-cy="login-submit" :loading="session.loading" :disable="!username.trim() || !password" @click="submit" />
          </q-card-actions>
        </q-card>
      </q-page>
    </q-page-container>
  </q-layout>
</template>
