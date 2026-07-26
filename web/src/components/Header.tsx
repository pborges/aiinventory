import { logout } from '../state/auth'

export type ActiveView = 'capture' | 'search' | 'locations' | 'duplicates' | 'settings' | 'item'

interface NavItem {
  view: ActiveView
  href: string
  icon: string
  label: string
}

const NAV_ITEMS: NavItem[] = [
  { view: 'capture', href: '/capture', icon: '📷', label: 'Capture' },
  { view: 'search', href: '/search', icon: '🔍', label: 'Search' },
  { view: 'locations', href: '/locations', icon: '🗺️', label: 'Locations' },
  { view: 'duplicates', href: '/duplicates', icon: '🧬', label: 'Duplicates' },
  { view: 'settings', href: '/settings', icon: '⚙️', label: 'Settings' },
]

interface Props {
  active: ActiveView
}

/** Shared app header: a brand icon (home/search) plus finger-friendly icon
 * nav buttons — no text hyperlinks, so it works as a one-handed mobile
 * toolbar as well as a desktop nav bar. */
export function Header({ active }: Props) {
  return (
    <header class="app-header">
      <a href="/search" class="app-brand" aria-label="aiinventory home">
        📦
      </a>
      <nav class="app-nav">
        {NAV_ITEMS.map((item) => (
          <a
            key={item.view}
            href={item.href}
            class={'app-nav-item' + (active === item.view ? ' app-nav-item-active' : '')}
            aria-label={item.label}
            title={item.label}
          >
            {item.icon}
          </a>
        ))}
        <button type="button" class="app-nav-item app-nav-button" aria-label="Sign out" title="Sign out" onClick={() => logout()}>
          🚪
        </button>
      </nav>
    </header>
  )
}
