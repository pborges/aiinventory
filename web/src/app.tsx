import { useEffect } from 'preact/hooks'
import Router from 'preact-router'
import { currentUser, authLoading, initAuth } from './state/auth'
import { AuthGate } from './views/AuthGate'
import { Capture } from './views/Capture'
import { Search } from './views/Search'
import { Settings } from './views/Settings'
import { ItemDetail } from './views/ItemDetail'
import { LocationView } from './views/LocationView'
import { Duplicates } from './views/Duplicates'

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
      <Capture path="/capture" />
      <Settings path="/settings" />
      <ItemDetail path="/items/:id" />
      <LocationView path="/locations" />
      <Duplicates path="/duplicates" />
    </Router>
  )
}
