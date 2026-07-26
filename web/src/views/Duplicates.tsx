import { useEffect, useRef, useState } from 'preact/hooks'
import { api, ApiError, type DuplicateGroup, type Location } from '../api/client'
import { Header } from '../components/Header'
import { Footer } from '../components/Footer'

interface RouteProps {
  path?: string
  default?: boolean
}

const POLL_INTERVAL_MS = 1500

/**
 * The duplicate finder (README flow #5): an on-demand background scan for
 * items that look like the same physical thing tagged more than once.
 * "Is a run active" is polled from the server's in-memory Runner, not a DB
 * row — see internal/inventory.Runner.
 */
export function Duplicates(_props: RouteProps) {
  const [running, setRunning] = useState(false)
  const [groups, setGroups] = useState<DuplicateGroup[]>([])
  const [locations, setLocations] = useState<Location[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busyGroupId, setBusyGroupId] = useState<number | null>(null)
  const [survivorChoice, setSurvivorChoice] = useState<Record<number, number>>({})
  const [locationChoice, setLocationChoice] = useState<Record<number, string>>({})
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    api
      .listLocations()
      .then((res) => setLocations(res.locations))
      .catch(() => {})
    refresh()
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function refresh() {
    setLoading(true)
    setError(null)
    Promise.all([api.duplicatesStatus(), api.listDuplicateGroups()])
      .then(([status, groupsRes]) => {
        setRunning(status.running)
        setGroups(groupsRes.groups)
        if (status.running) startPolling()
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load'))
      .finally(() => setLoading(false))
  }

  function startPolling() {
    if (pollRef.current) return
    pollRef.current = setInterval(() => {
      api.duplicatesStatus().then((status) => {
        setRunning(status.running)
        if (!status.running) {
          if (pollRef.current) {
            clearInterval(pollRef.current)
            pollRef.current = null
          }
          api.listDuplicateGroups().then((res) => setGroups(res.groups))
        }
      })
    }, POLL_INTERVAL_MS)
  }

  async function onRun() {
    setError(null)
    try {
      await api.startDuplicateRun()
      setRunning(true)
      startPolling()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to start run')
    }
  }

  async function onDismiss(groupId: number) {
    setBusyGroupId(groupId)
    setError(null)
    try {
      await api.dismissDuplicateGroup(groupId)
      setGroups((prev) => prev.filter((g) => g.id !== groupId))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Dismiss failed')
    } finally {
      setBusyGroupId(null)
    }
  }

  async function onMerge(group: DuplicateGroup) {
    const survivor = survivorChoice[group.id] ?? group.items[0]?.item_id
    if (!survivor) return
    setBusyGroupId(group.id)
    setError(null)
    try {
      const locStr = locationChoice[group.id]
      const locationId = locStr ? parseInt(locStr, 10) : null
      await api.mergeDuplicateGroup(group.id, survivor, locationId)
      setGroups((prev) => prev.filter((g) => g.id !== group.id))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Merge failed')
    } finally {
      setBusyGroupId(null)
    }
  }

  return (
    <div class="duplicates-view">
      <Header active="duplicates" />

      <main class="duplicates-body">
        <div class="duplicates-toolbar">
          <button type="button" class="btn-primary" onClick={onRun} disabled={running}>
            {running ? 'Running…' : 'Find duplicates'}
          </button>
          {running && <span class="duplicates-running-indicator">Scanning all items…</span>}
        </div>

        {error && <p class="capture-feedback capture-feedback-error">{error}</p>}
        {loading && <p>Loading…</p>}
        {!loading && groups.length === 0 && !running && <p>No pending duplicate groups.</p>}

        <ul class="duplicate-group-list">
          {groups.map((group) => (
            <li class="duplicate-group-card" key={group.id}>
              <p class="duplicate-group-reasoning">{group.reasoning}</p>
              <div class="duplicate-group-members">
                {group.items.map((m) => (
                  <label class="duplicate-group-member" key={m.item_id}>
                    <input
                      type="radio"
                      name={`survivor-${group.id}`}
                      checked={(survivorChoice[group.id] ?? group.items[0]?.item_id) === m.item_id}
                      onChange={() => setSurvivorChoice({ ...survivorChoice, [group.id]: m.item_id })}
                    />
                    <a href={`/items/${m.item_id}`}>{m.asset_tag}</a>
                  </label>
                ))}
              </div>
              <label class="duplicate-group-location">
                Location for merged item
                <select
                  value={locationChoice[group.id] ?? ''}
                  onChange={(e) =>
                    setLocationChoice({ ...locationChoice, [group.id]: (e.target as HTMLSelectElement).value })
                  }
                >
                  <option value="">Keep survivor's current location</option>
                  {locations.map((loc) => (
                    <option value={String(loc.id)} key={loc.id}>
                      {loc.code}
                    </option>
                  ))}
                </select>
              </label>
              <div class="duplicate-group-actions">
                <button type="button" onClick={() => onDismiss(group.id)} disabled={busyGroupId === group.id}>
                  Not a duplicate
                </button>
                <button type="button" class="btn-primary" onClick={() => onMerge(group)} disabled={busyGroupId === group.id}>
                  Merge
                </button>
              </div>
            </li>
          ))}
        </ul>
      </main>

      <Footer />
    </div>
  )
}
