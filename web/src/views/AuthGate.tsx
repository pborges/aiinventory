import { useState } from 'preact/hooks'
import { authError, bootstrapNeeded, bootstrap, login } from '../state/auth'
import { ApiError } from '../api/client'
import { Footer } from '../components/Footer'

export function AuthGate() {
  const isBootstrap = bootstrapNeeded.value
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)

  async function onSubmit(e: Event) {
    e.preventDefault()
    setError(null)
    authError.value = null
    setSubmitting(true)
    try {
      if (isBootstrap) {
        await bootstrap(username, password)
      } else {
        await login(username, password)
      }
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.status === 401 ? 'Unauthorized' : err.message)
      } else {
        setError('Something went wrong')
      }
    } finally {
      setSubmitting(false)
    }
  }

  const displayedError = error ?? authError.value

  return (
    <div class="auth-gate">
      <div class="auth-gate-body">
        <form class="auth-form" onSubmit={onSubmit}>
          <h1>aiinventory</h1>
          <p class="auth-subtitle">
            {isBootstrap ? 'Create the first account to get started.' : 'Sign in'}
          </p>

          <label>
            Username
            <input
              value={username}
              onInput={(e) => setUsername((e.target as HTMLInputElement).value)}
              autoFocus
              required
            />
          </label>

          <label>
            Password
            <input
              type="password"
              value={password}
              onInput={(e) => setPassword((e.target as HTMLInputElement).value)}
              minLength={8}
              required
            />
          </label>

          {displayedError && <p class="auth-error">{displayedError}</p>}

          <button type="submit" class="btn-primary" disabled={submitting}>
            {isBootstrap ? 'Create account' : 'Sign in'}
          </button>
        </form>
      </div>

      <Footer />
    </div>
  )
}
