import { useEffect } from 'preact/hooks'
import Router from 'preact-router'
import { currentUser, authLoading, initAuth } from './state/auth'
import { AuthGate } from './views/AuthGate'
import { Search } from './views/Search'

export function App() {
  useEffect(() => {
    initAuth()
  }, [])

  if (authLoading.value) {
    return <div class="app-loading">Loading…</div>
  }

  if (!currentUser.value) {
    return <AuthGate />
  }

  return (
    <Router>
      <Search path="/" />
      <Search path="/search" default />
    </Router>
  )
}
