import { currentUser, logout } from '../state/auth'

// preact-router passes route-matching props (path, default, etc.) to routed
// components; accepted here and ignored since Search doesn't use them yet.
interface RouteProps {
  path?: string
  default?: boolean
}

export function Search(_props: RouteProps) {
  return (
    <div class="search-view">
      <header class="app-header">
        <span class="app-title">aiinventory</span>
        <span class="app-header-user">
          {currentUser.value?.username}
          <button type="button" class="link-button" onClick={() => logout()}>
            Sign out
          </button>
        </span>
      </header>
      <main class="search-body">
        <p>Search — coming in a later phase.</p>
      </main>
    </div>
  )
}
