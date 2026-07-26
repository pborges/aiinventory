import { useEffect, useRef, useState } from 'preact/hooks'
import { api, ApiError, type ItemSummary } from '../api/client'
import { Header } from '../components/Header'

interface RouteProps {
  path?: string
  default?: boolean
}

function parseLocationFilterFromURL(): { id: number; code: string } | null {
  const params = new URLSearchParams(window.location.search)
  const idStr = params.get('location_id')
  const code = params.get('location_code')
  if (!idStr) return null
  const id = parseInt(idStr, 10)
  if (Number.isNaN(id)) return null
  return { id, code: code ?? '' }
}

export function Search(_props: RouteProps) {
  const [query, setQuery] = useState('')
  const [noDescription, setNoDescription] = useState(false)
  const [noLocation, setNoLocation] = useState(false)
  const [locationFilter, setLocationFilter] = useState(() => parseLocationFilterFromURL())
  const [items, setItems] = useState<ItemSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [bulkBusy, setBulkBusy] = useState(false)
  const [bulkMessage, setBulkMessage] = useState<string | null>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  function runSearch() {
    setLoading(true)
    setError(null)
    api
      .search({ q: query || undefined, noDescription, noLocation, locationId: locationFilter?.id })
      .then((res) => {
        setItems(res.items)
        setSelected(new Set())
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Search failed'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(runSearch, 250)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, noDescription, noLocation, locationFilter])

  function toggleSelected(id: number) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  function toggleSelectAll() {
    setSelected((prev) => (prev.size === items.length ? new Set() : new Set(items.map((it) => it.id))))
  }

  async function onBulkDelete() {
    if (selected.size === 0) return
    setBulkBusy(true)
    setBulkMessage(null)
    try {
      const { deleted } = await api.bulkDelete([...selected])
      setBulkMessage(`Deleted ${deleted} item(s).`)
      runSearch()
    } catch (err) {
      setBulkMessage(err instanceof ApiError ? err.message : 'Bulk delete failed')
    } finally {
      setBulkBusy(false)
    }
  }

  async function onBulkRegenerate() {
    if (selected.size === 0) return
    setBulkBusy(true)
    setBulkMessage(null)
    try {
      const { results } = await api.bulkRegenerateDescription([...selected])
      const failed = results.filter((r) => r.error)
      setBulkMessage(
        failed.length === 0
          ? `Regenerated ${results.length} description(s).`
          : `Regenerated ${results.length - failed.length} of ${results.length}; ${failed.length} failed.`,
      )
      runSearch()
    } catch (err) {
      setBulkMessage(err instanceof ApiError ? err.message : 'Bulk regenerate failed')
    } finally {
      setBulkBusy(false)
    }
  }

  const allSelected = items.length > 0 && selected.size === items.length

  return (
    <div class="search-view">
      <Header active="search" />

      <main class="search-body">
        <div class="search-controls">
          <input
            type="search"
            class="search-input"
            placeholder="Search items…"
            value={query}
            onInput={(e) => setQuery((e.target as HTMLInputElement).value)}
            autoFocus
          />
          <label class="search-filter">
            <input
              type="checkbox"
              checked={noDescription}
              onChange={(e) => setNoDescription((e.target as HTMLInputElement).checked)}
            />
            No description
          </label>
          <label class="search-filter">
            <input
              type="checkbox"
              checked={noLocation}
              onChange={(e) => setNoLocation((e.target as HTMLInputElement).checked)}
            />
            No location
          </label>
          {locationFilter && (
            <span class="search-location-chip">
              📍 {locationFilter.code || `location #${locationFilter.id}`}
              <button type="button" class="link-button" onClick={() => setLocationFilter(null)}>
                ×
              </button>
            </span>
          )}
        </div>

        <div class="search-bulk-bar">
          <label class="search-filter">
            <input type="checkbox" checked={allSelected} onChange={toggleSelectAll} disabled={items.length === 0} />
            Select all
          </label>
          <button type="button" onClick={onBulkDelete} disabled={selected.size === 0 || bulkBusy}>
            Delete
          </button>
          <button type="button" onClick={onBulkRegenerate} disabled={selected.size === 0 || bulkBusy}>
            Regenerate description
          </button>
          {bulkMessage && <span class="search-bulk-message">{bulkMessage}</span>}
        </div>

        {error && <p class="capture-feedback capture-feedback-error">{error}</p>}
        {loading && <p>Searching…</p>}
        {!loading && items.length === 0 && !error && <p>No items found.</p>}

        <ul class="item-card-list">
          {items.map((item) => (
            <li class="item-card" key={item.id}>
              <label class="item-card-select">
                <input type="checkbox" checked={selected.has(item.id)} onChange={() => toggleSelected(item.id)} />
              </label>
              <a class="item-card-link" href={`/items/${item.id}`}>
                <div class="item-card-thumb">
                  {item.primary_image_id ? (
                    <img src={`/api/images/${item.primary_image_id}`} alt={item.asset_tag} />
                  ) : (
                    <div class="item-card-thumb-placeholder">{item.asset_tag}</div>
                  )}
                </div>
                <div class="item-card-info">
                  <div class="item-card-tag">{item.asset_tag}</div>
                  <div class="item-card-description">{item.description || <em>No description</em>}</div>
                  {item.location_code && <div class="item-card-location">{item.location_code}</div>}
                </div>
              </a>
            </li>
          ))}
        </ul>
      </main>
    </div>
  )
}
