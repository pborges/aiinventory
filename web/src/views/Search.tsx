import { useEffect, useRef, useState } from 'preact/hooks'
import { faLocationDot, faXmark } from '@fortawesome/free-solid-svg-icons'
import { api, ApiError, type ItemSummary } from '../api/client'
import { Header } from '../components/Header'
import { Footer } from '../components/Footer'
import { GenerateDescriptionsModal } from '../components/GenerateDescriptionsModal'
import { Icon } from '../components/Icon'

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
  const [noPhoto, setNoPhoto] = useState(false)
  const [locationFilter, setLocationFilter] = useState(() => parseLocationFilterFromURL())
  const [items, setItems] = useState<ItemSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [bulkBusy, setBulkBusy] = useState(false)
  const [bulkMessage, setBulkMessage] = useState<string | null>(null)
  const [generateModalItems, setGenerateModalItems] = useState<ItemSummary[] | null>(null)
  const [hoverPreview, setHoverPreview] = useState<{ src: string; x: number; y: number } | null>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  function runSearch() {
    setLoading(true)
    setError(null)
    api
      .search({ q: query || undefined, noDescription, noLocation, noPhoto, locationId: locationFilter?.id })
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
  }, [query, noDescription, noLocation, noPhoto, locationFilter])

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

  function onBulkRegenerate() {
    if (selected.size === 0) return
    setGenerateModalItems(items.filter((it) => selected.has(it.id)))
  }

  const allSelected = items.length > 0 && selected.size === items.length

  // Follows the cursor rather than growing the thumbnail in place — an
  // in-place hover-zoom (tried first) covers the row's own tag/description
  // text, since the thumb sits directly beside them in the same flex row.
  const HOVER_PREVIEW_SIZE = 320
  const HOVER_PREVIEW_MARGIN = 16

  function showHoverPreview(src: string, e: { clientX: number; clientY: number }) {
    let x = e.clientX + HOVER_PREVIEW_MARGIN
    let y = e.clientY + HOVER_PREVIEW_MARGIN
    if (x + HOVER_PREVIEW_SIZE > window.innerWidth) x = e.clientX - HOVER_PREVIEW_SIZE - HOVER_PREVIEW_MARGIN
    if (y + HOVER_PREVIEW_SIZE > window.innerHeight) y = window.innerHeight - HOVER_PREVIEW_SIZE - HOVER_PREVIEW_MARGIN
    if (y < HOVER_PREVIEW_MARGIN) y = HOVER_PREVIEW_MARGIN
    setHoverPreview({ src, x, y })
  }

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
          <label class="search-filter">
            <input
              type="checkbox"
              checked={noPhoto}
              onChange={(e) => setNoPhoto((e.target as HTMLInputElement).checked)}
            />
            No photo
          </label>
          {locationFilter && (
            <span class="search-location-chip">
              <Icon icon={faLocationDot} /> {locationFilter.code || `location #${locationFilter.id}`}
              <button type="button" class="link-button" onClick={() => setLocationFilter(null)} aria-label="Clear location filter">
                <Icon icon={faXmark} />
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
                    <img
                      src={`/api/images/${item.primary_image_id}`}
                      alt={item.asset_tag}
                      onMouseEnter={(e) => showHoverPreview(`/api/images/${item.primary_image_id}`, e)}
                      onMouseMove={(e) => showHoverPreview(`/api/images/${item.primary_image_id}`, e)}
                      onMouseLeave={() => setHoverPreview(null)}
                    />
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

      {generateModalItems && (
        <GenerateDescriptionsModal
          items={generateModalItems}
          onClose={() => setGenerateModalItems(null)}
          onComplete={runSearch}
        />
      )}

      {hoverPreview && (
        <img
          src={hoverPreview.src}
          alt=""
          class="search-hover-preview"
          style={{ left: `${hoverPreview.x}px`, top: `${hoverPreview.y}px` }}
        />
      )}

      <Footer />
    </div>
  )
}
