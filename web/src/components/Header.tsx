import { faBox, faCamera, faMagnifyingGlass, faMap, faClone, faGear, faRightFromBracket } from '@fortawesome/free-solid-svg-icons'
import type { IconDefinition } from '@fortawesome/fontawesome-svg-core'
import { logout } from '../state/auth'
import { Icon } from './Icon'

export type ActiveView = 'capture' | 'search' | 'locations' | 'duplicates' | 'settings' | 'item'

interface NavItem {
  view: ActiveView
  href: string
  icon: IconDefinition
  label: string
}

const NAV_ITEMS: NavItem[] = [
  { view: 'capture', href: '/capture', icon: faCamera, label: 'Capture' },
  { view: 'search', href: '/search', icon: faMagnifyingGlass, label: 'Search' },
  { view: 'locations', href: '/locations', icon: faMap, label: 'Locations' },
  { view: 'duplicates', href: '/duplicates', icon: faClone, label: 'Duplicates' },
  { view: 'settings', href: '/settings', icon: faGear, label: 'Settings' },
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
        <Icon icon={faBox} />
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
            <Icon icon={item.icon} />
          </a>
        ))}
        <button type="button" class="app-nav-item app-nav-button" aria-label="Sign out" title="Sign out" onClick={() => logout()}>
          <Icon icon={faRightFromBracket} />
        </button>
      </nav>
    </header>
  )
}
