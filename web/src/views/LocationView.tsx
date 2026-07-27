import { useEffect, useState } from 'preact/hooks'
import { api, ApiError, type ActivityEntry, type Location, type LocationItem } from '../api/client'
import { Header } from '../components/Header'
import { Footer } from '../components/Footer'
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
  const { preview: hoverPreview, showHoverPreview, hideHoverPreview } = useHoverPreview()

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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function selectLocation(loc: Location) {
    setSelected(loc)
    setLoading(true)
    setError(null)
    Promise.all([api.getLocationItems(loc.id), api.getLocationActivity(loc.id)])
      .then(([itemsRes, activityRes]) => {
        setItems(itemsRes.items)
        setActivity(activityRes.activity)
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load location'))
      .finally(() => setLoading(false))
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

  return (
    <div class="location-view">
      <Header active="locations" />

      <div class="location-layout">
        <aside class="location-sidebar">
          <h2>Locations</h2>
          <ul>
            {locations.map((loc) => (
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
                {loc.code}
              </li>
            ))}
            {locations.length === 0 && <li class="location-sidebar-empty">No locations yet.</li>}
          </ul>
        </aside>

        <main class="location-main">
          {error && <p class="capture-feedback capture-feedback-error">{error}</p>}
          {loading && <p>Loading…</p>}
          {!loading && selected && (
            <>
              <h1>{selected.code}</h1>
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
          <h3>Activity — {selected.code}</h3>
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
