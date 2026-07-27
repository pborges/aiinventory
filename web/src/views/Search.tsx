import { useEffect, useRef, useState } from 'preact/hooks'
import { faLocationDot, faXmark } from '@fortawesome/free-solid-svg-icons'
import { api, ApiError, formatLocationCode, type ItemSummary, type Tag } from '../api/client'
import { Header } from '../components/Header'
import { Footer } from '../components/Footer'
import { GenerateDescriptionsModal } from '../components/GenerateDescriptionsModal'
import { Icon } from '../components/Icon'
import { TagChip } from '../components/TagChip'
import { HoverPreview, useHoverPreview } from '../lib/hoverPreview'

interface RouteProps {
  path?: string
  default?: boolean
}

function parseLocationFilterFromURL(): { id: number; code: string; description?: string } | null {
  const params = new URLSearchParams(window.location.search)
  const idStr = params.get('location_id')
  const code = params.get('location_code')
  const description = params.get('location_description')
  if (!idStr) return null
  const id = parseInt(idStr, 10)
  if (Number.isNaN(id)) return null
  return { id, code: code ?? '', description: description ?? undefined }
}

export function Search(_props: RouteProps) {
  const [query, setQuery] = useState('')
  const [noDescription, setNoDescription] = useState(false)
  const [noLocation, setNoLocation] = useState(false)
  const [noPhoto, setNoPhoto] = useState(false)
  const [locationFilter, setLocationFilter] = useState(() => parseLocationFilterFromURL())
  const [allTags, setAllTags] = useState<Tag[]>([])
  const [selectedTagIds, setSelectedTagIds] = useState<Set<number>>(new Set())
  const [items, setItems] = useState<ItemSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [bulkBusy, setBulkBusy] = useState(false)
  const [bulkMessage, setBulkMessage] = useState<string | null>(null)
  const [generateModalItems, setGenerateModalItems] = useState<ItemSummary[] | null>(null)
  const { preview: hoverPreview, showHoverPreview, hideHoverPreview } = useHoverPreview()
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  function runSearch() {
    setLoading(true)
    setError(null)
    api
      .search({
        q: query || undefined,
        noDescription,
        noLocation,
        noPhoto,
        locationId: locationFilter?.id,
        tagIds: selectedTagIds.size > 0 ? [...selectedTagIds] : undefined,
      })
      .then((res) => {
        setItems(res.items)
        setSelected(new Set())
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Search failed'))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    api.listTags().then((res) => setAllTags(res.tags))
  }, [])

  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(runSearch, 250)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query, noDescription, noLocation, noPhoto, locationFilter, selectedTagIds])

  function toggleTagFilter(tagId: number) {
    setSelectedTagIds((prev) => {
      const next = new Set(prev)
      if (next.has(tagId)) next.delete(tagId)
      else next.add(tagId)
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
              {locationFilter.code ? formatLocationCode(locationFilter.code, locationFilter.description) : `location #${locationFilter.id}`}
              <button type="button" class="link-button" onClick={() => setLocationFilter(null)} aria-label="Clear location filter">
                <Icon icon={faXmark} />
              </button>
            </span>
          )}
        </div>

        {allTags.length > 0 && (
          <div class="tag-cloud search-tag-filter">
            {allTags.map((tag) => (
              <TagChip key={tag.id} tag={tag} selected={selectedTagIds.has(tag.id)} onClick={() => toggleTagFilter(tag.id)} />
            ))}
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
                    {item.location_code && (
                      <div class="item-card-location">{formatLocationCode(item.location_code, item.location_description)}</div>
                    )}
                  </div>
                  {item.tags.length > 0 && (
                    <div class="item-card-tags">
                      {item.tags.map((tag) => (
                        <TagChip key={tag.id} tag={tag} />
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
