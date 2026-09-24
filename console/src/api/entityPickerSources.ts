import type { components } from './generated'
import { apiFetch } from './client'
import { cursorPath } from './pagination'
import type { EntityPickerOption, EntityPickerPage } from '../components/PagedEntityPicker.vue'

type User = components['schemas']['User']
type UserPage = components['schemas']['UserPage']
type Upstream = components['schemas']['Upstream']
type UpstreamPage = components['schemas']['UpstreamPage']
type Secret = components['schemas']['Secret']
type SecretPage = components['schemas']['SecretPage']
type BudgetTemplate = components['schemas']['BudgetTemplate']
type BudgetTemplatePage = components['schemas']['BudgetTemplatePage']

function userOption(user: User): EntityPickerOption {
  return {
    value: user.userId,
    label: user.displayName,
    caption: user.username,
    status: user.status,
  }
}

export async function fetchUserPickerPage(query: string, cursor?: string): Promise<EntityPickerPage> {
  const params = new URLSearchParams({ limit: '50' })
  if (query) params.set('query', query)
  const path = `/api/admin/v1/users?${params.toString()}`
  const page = await apiFetch<UserPage>(cursor ? cursorPath(path, cursor) : path)
  return { items: page.items.map(userOption), nextCursor: page.nextCursor }
}

export async function resolveUserPickerOption(userId: string): Promise<EntityPickerOption | undefined> {
  const user = await apiFetch<User>(`/api/admin/v1/users/${encodeURIComponent(userId)}`)
  return userOption(user)
}

function upstreamOption(upstream: Upstream): EntityPickerOption {
  return {
    value: upstream.upstreamId,
    label: upstream.name,
    caption: upstream.config?.baseUrl,
    status: upstream.status,
  }
}

export async function fetchUpstreamPickerPage(query: string, cursor?: string): Promise<EntityPickerPage> {
  const params = new URLSearchParams({ limit: '50' })
  if (query) params.set('query', query)
  const path = `/api/admin/v1/upstreams?${params.toString()}`
  const page = await apiFetch<UpstreamPage>(cursor ? cursorPath(path, cursor) : path)
  return { items: page.items.map(upstreamOption), nextCursor: page.nextCursor }
}

export async function resolveUpstreamPickerOption(upstreamId: string): Promise<EntityPickerOption | undefined> {
  return upstreamOption(await apiFetch<Upstream>(`/api/admin/v1/upstreams/${encodeURIComponent(upstreamId)}`))
}

function secretOption(secret: Secret): EntityPickerOption {
  return {
    value: secret.secretId,
    label: secret.name,
    caption: `v${secret.secretVersion}`,
    metadata: { secretVersion: secret.secretVersion },
  }
}

export async function fetchSecretPickerPage(query: string, cursor?: string): Promise<EntityPickerPage> {
  const params = new URLSearchParams({ limit: '50' })
  if (query) params.set('query', query)
  const path = `/api/admin/v1/secrets?${params.toString()}`
  const page = await apiFetch<SecretPage>(cursor ? cursorPath(path, cursor) : path)
  return { items: page.items.map(secretOption), nextCursor: page.nextCursor }
}

export async function resolveSecretPickerOption(secretId: string): Promise<EntityPickerOption | undefined> {
  return secretOption(await apiFetch<Secret>(`/api/admin/v1/secrets/${encodeURIComponent(secretId)}`))
}

function budgetTemplateOption(template: BudgetTemplate): EntityPickerOption {
  return {
    value: template.budgetTemplateId,
    label: template.name,
    caption: template.description || `v${template.revision}`,
    metadata: { revision: template.revision, assignedUserCount: template.assignedUserCount },
  }
}

export async function fetchBudgetTemplatePickerPage(query: string, cursor?: string): Promise<EntityPickerPage> {
  const params = new URLSearchParams({ limit: '50' })
  if (query) params.set('query', query)
  const path = `/api/admin/v1/budget-templates?${params.toString()}`
  const page = await apiFetch<BudgetTemplatePage>(cursor ? cursorPath(path, cursor) : path)
  return { items: page.items.map(budgetTemplateOption), nextCursor: page.nextCursor }
}

export async function resolveBudgetTemplatePickerOption(budgetTemplateId: string): Promise<EntityPickerOption | undefined> {
  return budgetTemplateOption(await apiFetch<BudgetTemplate>(`/api/admin/v1/budget-templates/${encodeURIComponent(budgetTemplateId)}`))
}
