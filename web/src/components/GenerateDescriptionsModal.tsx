import { useEffect, useRef, useState } from 'preact/hooks'
import { api, ApiError, type DescriptionBatchItem, type ItemSummary } from '../api/client'

const POLL_INTERVAL_MS = 1000

interface Row {
  itemId: number
  assetTag: string
  primaryImageId?: number
  status: DescriptionBatchItem['status']
  description: string
  error: string
}

interface Props {
  items: ItemSummary[]
  onClose: () => void
  onComplete: () => void
}

/**
 * Live-progress viewer for the Search view's bulk "Generate description"
 * action. Opening the modal kicks off a detached, server-side batch (so it
 * survives this modal closing or the page refreshing — see
 * internal/inventory.DescriptionBatch) and polls its status until every
 * item is done or errored. Each row also has its own hint box and
 * "Regenerate" button for redoing just that one item — those go through
 * the single-item endpoint directly, independent of the batch.
 */
export function GenerateDescriptionsModal({ items, onClose, onComplete }: Props) {
  const [rows, setRows] = useState<Row[]>(
    items.map((it) => ({
      itemId: it.id,
      assetTag: it.asset_tag,
      primaryImageId: it.primary_image_id,
      status: 'pending',
      description: '',
      error: '',
    })),
  )
  const [hints, setHints] = useState<Record<number, string>>({})
  const [individuallyBusy, setIndividuallyBusy] = useState<Set<number>>(new Set())
  const [startNotice, setStartNotice] = useState<string | null>(null)
  const [running, setRunning] = useState(true)
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const wasRunningRef = useRef(false)

  useEffect(() => {
    api
      .startBulkRegenerateDescription(items.map((it) => ({ item_id: it.id, hint: '' })))
      .catch((err) => {
        if (err instanceof ApiError && err.status === 409) {
          setStartNotice('A description batch was already running — showing its progress instead.')
        } else {
          setStartNotice(err instanceof ApiError ? err.message : 'Failed to start')
        }
      })
      .finally(poll)

    pollRef.current = setInterval(poll, POLL_INTERVAL_MS)
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function poll() {
    api.bulkRegenerateDescriptionStatus().then((status) => {
      setRunning(status.running)
      if (status.running) wasRunningRef.current = true

      const byID = new Map(status.items.map((it) => [it.item_id, it]))
      setRows((prev) =>
        prev.map((row) => {
          const match = byID.get(row.itemId)
          if (!match) return row
          return {
            ...row,
            assetTag: match.asset_tag || row.assetTag,
            status: match.status,
            description: match.description ?? row.description,
            error: match.error ?? '',
          }
        }),
      )

      if (!status.running) {
        if (pollRef.current) {
          clearInterval(pollRef.current)
          pollRef.current = null
        }
        if (wasRunningRef.current) onComplete()
      }
    })
  }

  async function onRegenerateOne(itemId: number) {
    setIndividuallyBusy((prev) => new Set(prev).add(itemId))
    setRows((prev) => prev.map((r) => (r.itemId === itemId ? { ...r, status: 'generating', error: '' } : r)))
    try {
      const updated = await api.regenerateItemDescription(itemId, hints[itemId] || undefined)
      setRows((prev) =>
        prev.map((r) => (r.itemId === itemId ? { ...r, status: 'done', description: updated.description, error: '' } : r)),
      )
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed'
      setRows((prev) => prev.map((r) => (r.itemId === itemId ? { ...r, status: 'error', error: message } : r)))
    } finally {
      setIndividuallyBusy((prev) => {
        const next = new Set(prev)
        next.delete(itemId)
        return next
      })
    }
  }

  const doneCount = rows.filter((r) => r.status === 'done').length
  const errorCount = rows.filter((r) => r.status === 'error').length

  return (
    <div class="modal-overlay">
      <div class="modal-panel generate-descriptions-modal">
        <h2>Generate descriptions</h2>
        <p class="generate-descriptions-progress">
          {running ? 'Working…' : 'Finished.'} {doneCount}/{rows.length} done
          {errorCount > 0 && `, ${errorCount} failed`}.
        </p>
        {startNotice && <p class="capture-feedback capture-feedback-warning">{startNotice}</p>}

        <ul class="generate-descriptions-list">
          {rows.map((row) => (
            <li class="generate-descriptions-row" key={row.itemId}>
              <div class="generate-descriptions-thumb">
                {row.primaryImageId ? (
                  <img src={`/api/images/${row.primaryImageId}`} alt={row.assetTag} />
                ) : (
                  <div class="item-card-thumb-placeholder">{row.assetTag}</div>
                )}
              </div>
              <div class="generate-descriptions-row-body">
                <div class="generate-descriptions-row-header">
                  <span class="generate-descriptions-tag">{row.assetTag}</span>
                  <span class={`generate-descriptions-status generate-descriptions-status-${row.status}`}>
                    {row.status}
                  </span>
                </div>
                <p class="generate-descriptions-description">
                  {row.status === 'error' ? row.error : row.description || <em>—</em>}
                </p>
                <div class="generate-descriptions-row-actions">
                  <input
                    type="text"
                    class="generate-descriptions-hint"
                    placeholder={'Optional hint (e.g. "blue enclosure")'}
                    value={hints[row.itemId] ?? ''}
                    onInput={(e) => setHints({ ...hints, [row.itemId]: (e.target as HTMLInputElement).value })}
                  />
                  <button
                    type="button"
                    onClick={() => onRegenerateOne(row.itemId)}
                    disabled={individuallyBusy.has(row.itemId)}
                  >
                    {individuallyBusy.has(row.itemId) ? 'Regenerating…' : 'Regenerate'}
                  </button>
                </div>
              </div>
            </li>
          ))}
        </ul>

        <div class="modal-actions">
          <button type="button" onClick={onClose}>
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
