import { signal } from '@preact/signals'
import { api, ApiError, type User } from '../api/client'

export const currentUser = signal<User | null>(null)
export const authLoading = signal(true)
export const bootstrapNeeded = signal(false)

export async function initAuth() {
  authLoading.value = true
  try {
    const { user } = await api.me()
    currentUser.value = user
  } catch (err) {
    currentUser.value = null
    if (err instanceof ApiError && err.status === 401) {
      try {
        bootstrapNeeded.value = (await api.bootstrapStatus()).needed
      } catch {
        bootstrapNeeded.value = false
      }
    }
  } finally {
    authLoading.value = false
  }
}

export async function login(username: string, password: string) {
  const { user } = await api.login(username, password)
  currentUser.value = user
}

export async function bootstrap(username: string, password: string) {
  const { user } = await api.bootstrap(username, password)
  currentUser.value = user
  bootstrapNeeded.value = false
}

export async function logout() {
  await api.logout()
  currentUser.value = null
  bootstrapNeeded.value = false
}
