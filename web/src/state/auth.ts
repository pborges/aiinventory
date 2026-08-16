import { signal } from '@preact/signals'
import { api, ApiError, setUnauthorizedHandler, type User } from '../api/client'

export const currentUser = signal<User | null>(null)
export const authLoading = signal(true)
export const bootstrapNeeded = signal(false)
export const authError = signal<string | null>(null)

setUnauthorizedHandler(() => {
  currentUser.value = null
  bootstrapNeeded.value = false
  authError.value = 'Unauthorized'
})

export async function initAuth() {
  authLoading.value = true
  authError.value = null
  try {
    const { user } = await api.me()
    currentUser.value = user
  } catch (err) {
    currentUser.value = null
    if (err instanceof ApiError && err.status === 401) {
      // A missing session on initial page load is the normal logged-out state,
      // not an error the login form needs to report.
      authError.value = null
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
  authError.value = null
  const { user } = await api.login(username, password)
  currentUser.value = user
  authError.value = null
}

export async function bootstrap(username: string, password: string) {
  authError.value = null
  const { user } = await api.bootstrap(username, password)
  currentUser.value = user
  bootstrapNeeded.value = false
  authError.value = null
}

export async function logout() {
  await api.logout()
  currentUser.value = null
  bootstrapNeeded.value = false
  authError.value = null
}
