import { useEffect, useRef, useState } from 'preact/hooks'
import { faChevronDown, faChevronUp, faLocationDot, faXmark } from '@fortawesome/free-solid-svg-icons'
import { api, ApiError, formatLocationTag, type ItemSummary, type Label } from '../api/client'
import { Header } from '../components/Header'
import { Footer } from '../components/Footer'
import { GenerateDescriptionsModal } from '../components/GenerateDescriptionsModal'
import { Icon } from '../components/Icon'
import { LabelChip } from '../components/LabelChip'
import { HoverPreview, useHoverPreview } from '../lib/hoverPreview'

interface RouteProps {
  path?: string
  default?: boolean
}

function parseLocationFilterFromURL(): { id: number; locationTag: string; description?: string } | null {
  const params = new URLSearchParams(window.location.search)
  const idStr = params.get('location_id')
  const locationTag = params.get('location_tag')
  const description = params.get('location_description')
  if (!idStr) return null
  const id = parseInt(idStr, 10)
  if (Number.isNaN(id)) return null
  return { id, locationTag: locationTag ?? '', description: description ?? undefined }
}

export function Search(_props: RouteProps) {
  const [query, setQuery] = useState('')
  const [noDescription, setNoDescription] = useState(false)
  const [noLocation, setNoLocation] = useState(false)
  const [noPhoto, setNoPhoto] = useState(false)
  const [locationFilter, setLocationFilter] = useState(() => parseLocationFilterFromURL())
  const [allLabels, setAllLabels] = useState<Label[]>([])
  const [selectedLabelIds, setSelectedLabelIds] = useState<Set<number>>(new Set())
  const [allLocationLabels, setAllLocationLabels] = useState<Label[]>([])
  const [selectedLocationLabelIds, setSelectedLocationLabelIds] = useState<Set<number>>(new Set())
  const [filtersOpen, setFiltersOpen] = useState(() => window.matchMedia('(min-width: 800px)').matches)
  const [items, setItems] = useState<ItemSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [bulkBusy, setBulkBusy] = useState(false)
  const [bulkMessage, setBulkMessage] = useState<string | null>(null)
  const [generateModalItems, setGenerateModalItems] = useState<ItemSummary[] | null>(null)
  const { preview: hoverPreview, showHoverPreview, hideHoverPreview } = useHoverPreview()
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const searchAbortRef = useRef<AbortController | null>(null)

  function runSearch() {
    searchAbortRef.current?.abort()
    const controller = new AbortController()
    searchAbortRef.current = controller
    setLoading(true)
    setError(null)
    api
      .search({
        q: query || undefined,
        noDescription,
        noLocation,
        noPhoto,
        locationId: locationFilter?.id,
        labelIds: selectedLabelIds.size > 0 ? [...selectedLabelIds] : undefined,
        locationLabelIds: selectedLocationLabelIds.size > 0 ? [...selectedLocationLabelIds] : undefined,
      }, controller.signal)
      .then((res) => {
        if (searchAbortRef.current !== controller) return
        setItems(res.items)
        setSelected(new Set())
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === 'AbortError') return
        if (searchAbortRef.current === controller) setError(err instanceof ApiError ? err.message : 'Search failed')
      })
      .finally(() => {
        if (searchAbortRef.current === controller) setLoading(false)
      })
  }

  useEffect(() => {
    api.listItemLabels().then((res) => setAllLabels(res.labels))
    api.listLocationLabels().then((res) => setAllLocationLabels(res.labels))
    api.descriptionBatchStatus()
      .then((status) => {
        if (!status.running) return
        setGenerateModalItems(
          status.items.map((item) => ({
            id: item.item_id,
            asset_tag: item.asset_tag,
            description: item.description ?? '',
            primary_image_id: item.primary_image_id,
            labels: [],
          })),
        )
      })
      .catch(() => {})
    return () => searchAbortRef.current?.abort()
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(runSearch, 250)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, noDescription, noLocation, noPhoto, locationFilter, selectedLabelIds, selectedLocationLabelIds])

  function toggleLabelFilter(labelId: number) {
    setSelectedLabelIds((prev) => {
      const next = new Set(prev)
      if (next.has(labelId)) next.delete(labelId)
      else next.add(labelId)
      return next
    })
  }

  function toggleLocationLabelFilter(labelId: number) {
    setSelectedLocationLabelIds((prev) => {
      const next = new Set(prev)
      if (next.has(labelId)) next.delete(labelId)
      else next.add(labelId)
      return next
    })
  }

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
          <div class="search-filter-row">
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
                <Icon icon={faLocationDot} />{' '}
                {locationFilter.locationTag
                  ? formatLocationTag(locationFilter.locationTag, locationFilter.description)
                  : `location #${locationFilter.id}`}
                <button type="button" class="link-button" onClick={() => setLocationFilter(null)} aria-label="Clear location filter">
                  <Icon icon={faXmark} />
                </button>
              </span>
            )}
          </div>
        </div>

        {(allLocationLabels.length > 0 || allLabels.length > 0) && (
          <div class="search-label-filters-card">
            <button
              type="button"
              class="search-label-filters-toggle"
              onClick={() => setFiltersOpen((v) => !v)}
              aria-expanded={filtersOpen}
            >
              Tag filters
              {selectedLabelIds.size + selectedLocationLabelIds.size > 0 && (
                <span class="search-label-filters-count">{selectedLabelIds.size + selectedLocationLabelIds.size}</span>
              )}
              <Icon icon={filtersOpen ? faChevronUp : faChevronDown} />
            </button>
            {filtersOpen && (
              <>
                {allLocationLabels.length > 0 && (
                  <div class="search-label-filters-section">
                    <h3 class="search-label-filters-label">Location labels</h3>
                    <div class="label-cloud search-label-filter">
                      {allLocationLabels.map((label) => (
                        <LabelChip
                          key={label.id}
                          label={label}
                          selected={selectedLocationLabelIds.has(label.id)}
                          onClick={() => toggleLocationLabelFilter(label.id)}
                        />
                      ))}
                    </div>
                  </div>
                )}
                {allLabels.length > 0 && (
                  <div class="search-label-filters-section">
                    <h3 class="search-label-filters-label">Item labels</h3>
                    <div class="label-cloud search-label-filter">
                      {allLabels.map((label) => (
                        <LabelChip key={label.id} label={label} selected={selectedLabelIds.has(label.id)} onClick={() => toggleLabelFilter(label.id)} />
                      ))}
                    </div>
                  </div>
                )}
              </>
            )}
          </div>
        )}

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
              <a class="item-card-link" href={`/items/${item.id}`}>
                <div class="item-card-thumb">
                  {item.primary_image_id ? (
                    <img
                      src={`/api/images/${item.primary_image_id}`}
                      alt={item.asset_tag}
                      onMouseEnter={(e) => showHoverPreview(`/api/images/${item.primary_image_id}`, e)}
                      onMouseMove={(e) => showHoverPreview(`/api/images/${item.primary_image_id}`, e)}
                      onMouseLeave={hideHoverPreview}
                    />
                  ) : (
                    <div class="item-card-thumb-placeholder">{item.asset_tag}</div>
                  )}
                </div>
                <div class="item-card-body">
                  <div class="item-card-info">
                    <div class="item-card-tag">{item.asset_tag}</div>
                    <div class="item-card-description">{item.description || <em>No description</em>}</div>
                    {item.location_tag && (
                      <div class="item-card-location">{formatLocationTag(item.location_tag, item.location_description)}</div>
                    )}
                  </div>
                  {item.labels.length > 0 && (
                    <div class="item-card-labels">
                      {item.labels.map((label) => (
                        <LabelChip key={label.id} label={label} />
                      ))}
                    </div>
                  )}
                </div>
              </a>
              <label class="item-card-select">
                <input type="checkbox" checked={selected.has(item.id)} onChange={() => toggleSelected(item.id)} />
              </label>
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

      <HoverPreview preview={hoverPreview} />

      <Footer />
    </div>
  )
}
