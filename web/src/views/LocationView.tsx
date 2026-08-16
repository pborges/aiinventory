import { useEffect, useRef, useState } from 'preact/hooks'
import { faChevronDown, faChevronUp } from '@fortawesome/free-solid-svg-icons'
import { api, ApiError, formatLocationTag, type ActivityEntry, type Location, type LocationItem, type Label } from '../api/client'
import { Header } from '../components/Header'
import { Footer } from '../components/Footer'
import { Icon } from '../components/Icon'
import { LabelChip } from '../components/LabelChip'
import { HoverPreview, useHoverPreview } from '../lib/hoverPreview'

interface RouteProps {
  path?: string
  default?: boolean
}

/**
 * The location view (README flow #4) — a specialized, desktop-oriented
 * version of search scoped to browsing/organizing by location: a sidebar of
 * locations, richer item cards with a live image carousel per item, drag a
 * card onto a different location in the sidebar to relocate it, and a
 * footer activity log for whichever location is selected.
 */
export function LocationView(_props: RouteProps) {
  const [locations, setLocations] = useState<Location[]>([])
  const [selected, setSelected] = useState<Location | null>(null)
  const [items, setItems] = useState<LocationItem[]>([])
  const [activity, setActivity] = useState<ActivityEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [dragOverId, setDragOverId] = useState<number | null>(null)
  const [descriptionInput, setDescriptionInput] = useState('')
  const [savingDescription, setSavingDescription] = useState(false)
  const [allLocationLabels, setAllLocationLabels] = useState<Label[]>([])
  const [selectedLabelFilterIds, setSelectedLabelFilterIds] = useState<Set<number>>(new Set())
  const [labelsBusy, setLabelsBusy] = useState(false)
  const [sidebarOpen, setSidebarOpen] = useState(() => window.matchMedia('(min-width: 800px)').matches)
  const { preview: hoverPreview, showHoverPreview, hideHoverPreview } = useHoverPreview()
  const locationAbortRef = useRef<AbortController | null>(null)

  useEffect(() => {
    api
      .listLocations()
      .then((res) => {
        setLocations(res.locations)
        if (res.locations.length > 0) selectLocation(res.locations[0])
        else setLoading(false)
      })
      .catch((err) => {
        setError(err instanceof ApiError ? err.message : 'Failed to load locations')
        setLoading(false)
      })
    api.listLocationLabels().then((res) => setAllLocationLabels(res.labels))
    return () => locationAbortRef.current?.abort()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function selectLocation(loc: Location) {
    locationAbortRef.current?.abort()
    const controller = new AbortController()
    locationAbortRef.current = controller
    setSelected(loc)
    setDescriptionInput(loc.description ?? '')
    setLoading(true)
    setError(null)
    if (!window.matchMedia('(min-width: 800px)').matches) setSidebarOpen(false)
    Promise.all([api.getLocationItems(loc.id, controller.signal), api.getLocationActivity(loc.id, controller.signal)])
      .then(([itemsRes, activityRes]) => {
        if (locationAbortRef.current !== controller) return
        setItems(itemsRes.items)
        setActivity(activityRes.activity)
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === 'AbortError') return
        if (locationAbortRef.current === controller) setError(err instanceof ApiError ? err.message : 'Failed to load location')
      })
      .finally(() => {
        if (locationAbortRef.current === controller) setLoading(false)
      })
  }

  async function onSaveDescription() {
    if (!selected) return
    setSavingDescription(true)
    setError(null)
    try {
      const { location } = await api.updateLocation(selected.id, descriptionInput)
      setSelected(location)
      setLocations((prev) => prev.map((loc) => (loc.id === location.id ? location : loc)))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Save failed')
    } finally {
      setSavingDescription(false)
    }
  }

  function toggleLabelFilter(labelId: number) {
    setSelectedLabelFilterIds((prev) => {
      const next = new Set(prev)
      if (next.has(labelId)) next.delete(labelId)
      else next.add(labelId)
      return next
    })
  }

  async function onToggleLocationLabel(label: Label) {
    if (!selected) return
    const current = new Set(selected.labels.map((l) => l.id))
    if (current.has(label.id)) current.delete(label.id)
    else current.add(label.id)
    const nextIds = [...current]

    // optimistic toggle, then persist
    setSelected({ ...selected, labels: allLocationLabels.filter((l) => current.has(l.id)) })
    setLabelsBusy(true)
    try {
      const { location } = await api.setLocationLabels(selected.id, nextIds)
      setSelected(location)
      setLocations((prev) => prev.map((loc) => (loc.id === location.id ? location : loc)))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to update labels')
      selectLocation(selected)
    } finally {
      setLabelsBusy(false)
    }
  }

  function onDropOnLocation(loc: Location, e: DragEvent) {
    e.preventDefault()
    setDragOverId(null)
    const itemIdStr = e.dataTransfer?.getData('text/plain')
    if (!itemIdStr) return
    api
      .moveItemToLocation(loc.id, parseInt(itemIdStr, 10))
      .then(() => {
        if (selected) selectLocation(selected)
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Move failed'))
  }

  const visibleLocations =
    selectedLabelFilterIds.size === 0
      ? locations
      : locations.filter((loc) => loc.labels.some((l) => selectedLabelFilterIds.has(l.id)))

  return (
    <div class="location-view">
      <Header active="locations" />

      <div class="sidebar-page-layout">
        <aside class={'sidebar-page-sidebar' + (sidebarOpen ? '' : ' sidebar-page-sidebar-collapsed')}>
          <button
            type="button"
            class="sidebar-page-sidebar-toggle"
            onClick={() => setSidebarOpen((v) => !v)}
            aria-expanded={sidebarOpen}
            title="Locations"
          >
            <h2>Locations{selected ? `: ${selected.location_tag}` : ''}</h2>
            <Icon icon={sidebarOpen ? faChevronUp : faChevronDown} />
          </button>
          {sidebarOpen && (
            <>
              {allLocationLabels.length > 0 && (
                <div class="label-cloud location-sidebar-label-filter">
                  {allLocationLabels.map((label) => (
                    <LabelChip
                      key={label.id}
                      label={label}
                      selected={selectedLabelFilterIds.has(label.id)}
                      onClick={() => toggleLabelFilter(label.id)}
                    />
                  ))}
                </div>
              )}
              <ul>
                {visibleLocations.map((loc) => (
                  <li
                    key={loc.id}
                    class={
                      'location-sidebar-item' +
                      (selected?.id === loc.id ? ' location-sidebar-item-active' : '') +
                      (dragOverId === loc.id ? ' location-sidebar-item-dragover' : '')
                    }
                    onClick={() => selectLocation(loc)}
                    onDragOver={(e) => {
                      e.preventDefault()
                      setDragOverId(loc.id)
                    }}
                    onDragLeave={() => setDragOverId(null)}
                    onDrop={(e) => onDropOnLocation(loc, e)}
                  >
                    <div class="location-sidebar-tag">{loc.location_tag}</div>
                    {loc.description && <div class="location-sidebar-desc">{loc.description}</div>}
                  </li>
                ))}
                {visibleLocations.length === 0 && (
                  <li class="location-sidebar-empty">
                    {locations.length === 0 ? 'No locations yet.' : 'No locations match the selected tags.'}
                  </li>
                )}
              </ul>
            </>
          )}
        </aside>

        <main class="sidebar-page-main">
          {error && <p class="capture-feedback capture-feedback-error">{error}</p>}
          {loading && <p>Loading…</p>}
          {!loading && selected && (
            <>
              <h1>{selected.location_tag}</h1>
              <div class="location-description-editor">
                <label for="location-description-field">Description</label>
                <input
                  id="location-description-field"
                  type="text"
                  placeholder="No description set"
                  value={descriptionInput}
                  onInput={(e) => setDescriptionInput((e.target as HTMLInputElement).value)}
                />
                <button type="button" class="btn-primary" onClick={onSaveDescription} disabled={savingDescription}>
                  {savingDescription ? 'Saving…' : 'Save'}
                </button>
              </div>
              <div class="label-cloud location-labels-editor">
                {allLocationLabels.length === 0 && (
                  <p class="location-labels-empty">No location labels yet — create some in Settings.</p>
                )}
                {allLocationLabels.map((label) => (
                  <LabelChip
                    key={label.id}
                    label={label}
                    selected={selected.labels.some((l) => l.id === label.id)}
                    onClick={() => !labelsBusy && onToggleLocationLabel(label)}
                  />
                ))}
              </div>
              <ul class="location-item-list">
                {items.length === 0 && <p>No items at this location.</p>}
                {items.map((item) => (
                  <li
                    class="location-item-card"
                    key={item.id}
                    draggable
                    onDragStart={(e) => e.dataTransfer?.setData('text/plain', String(item.id))}
                  >
                    <a href={`/items/${item.id}`} class="location-item-tag">
                      {item.asset_tag}
                    </a>
                    <div class="location-item-carousel">
                      {item.images.length === 0 && <div class="item-card-thumb-placeholder">{item.asset_tag}</div>}
                      {item.images.map((img) => (
                        <img
                          key={img.id}
                          src={`/api/images/${img.id}`}
                          alt=""
                          onMouseEnter={(e) => showHoverPreview(`/api/images/${img.id}`, e)}
                          onMouseMove={(e) => showHoverPreview(`/api/images/${img.id}`, e)}
                          onMouseLeave={hideHoverPreview}
                        />
                      ))}
                    </div>
                    <p class="location-item-description">{item.description || <em>No description</em>}</p>
                  </li>
                ))}
              </ul>
            </>
          )}
        </main>
      </div>

      {selected && (
        <footer class="location-activity-footer">
          <h3>Activity — {formatLocationTag(selected.location_tag, selected.description)}</h3>
          <ul class="activity-log">
            {activity.length === 0 && <li>No activity yet.</li>}
            {activity.map((a, i) => (
              <li key={i}>
                <strong>{a.username}</strong> {a.action.replace(/_/g, ' ')}
                {a.detail ? ` — ${a.detail}` : ''}
                <span class="activity-time"> · {new Date(a.created_at).toLocaleString()}</span>
              </li>
            ))}
          </ul>
        </footer>
      )}

      <HoverPreview preview={hoverPreview} />

      <Footer />
    </div>
  )
}
