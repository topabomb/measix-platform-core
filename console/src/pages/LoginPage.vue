<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ProblemBanner from '../components/ProblemBanner.vue'
import { useSessionStore } from '../stores/session'

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
            <div class="text-h5 text-weight-bold">MEASIX Admin</div>
            <div class="text-body2 text-grey-7 q-mt-xs">Control Hub administration</div>
          </q-card-section>
          <q-card-section class="q-gutter-md">
            <ProblemBanner :error="error" />
            <q-input v-model="username" outlined label="Username" autocomplete="username" @keyup.enter="submit" />
            <q-input v-model="password" outlined type="password" label="Password" autocomplete="current-password" @keyup.enter="submit" />
          </q-card-section>
          <q-card-actions align="right" class="q-pa-md">
            <q-btn color="primary" label="Sign in" :loading="session.loading" :disable="!username.trim() || !password" @click="submit" />
          </q-card-actions>
        </q-card>
      </q-page>
    </q-page-container>
  </q-layout>
</template>
