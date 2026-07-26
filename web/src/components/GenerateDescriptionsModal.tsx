import { useState } from 'preact/hooks'
import { api, ApiError, type ItemSummary } from '../api/client'

// How many /api/items/{id}/regenerate-description requests are in flight at
// once — a small worker pool over the item list, not one request per item
// at a time and not all of them at once.
const CONCURRENCY = 3

type RowStatus = 'pending' | 'generating' | 'done' | 'error'

interface Row {
  itemId: number
  assetTag: string
  primaryImageId?: number
  status: RowStatus
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
 * action. Opens with a hint box per item and waits for "Generate" before
 * doing anything — that click dispatches a small worker pool of individual
 * POST /api/items/{id}/regenerate-description requests (CONCURRENCY at a
 * time), each request's own response updating that row directly. No
 * server-side batch/job to poll: this all lives in the browser tab for as
 * long as the modal stays open. Each row also has its own "Regenerate"
 * button for redoing just that one item after the fact, via the same helper.
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
  const [started, setStarted] = useState(false)
  const [running, setRunning] = useState(false)

  async function runItem(itemId: number) {
    setRows((prev) => prev.map((r) => (r.itemId === itemId ? { ...r, status: 'generating', error: '' } : r)))
    try {
      const updated = await api.regenerateItemDescription(itemId, hints[itemId] || undefined)
      setRows((prev) =>
        prev.map((r) => (r.itemId === itemId ? { ...r, status: 'done', description: updated.description, error: '' } : r)),
      )
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed'
      setRows((prev) => prev.map((r) => (r.itemId === itemId ? { ...r, status: 'error', error: message } : r)))
    }
  }

  async function onGenerateAll() {
    setStarted(true)
    setRunning(true)

    let nextIndex = 0
    async function worker() {
      while (nextIndex < items.length) {
        const item = items[nextIndex++]
        await runItem(item.id)
      }
    }
    await Promise.all(Array.from({ length: Math.min(CONCURRENCY, items.length) }, worker))

    setRunning(false)
    onComplete()
  }

  async function onRegenerateOne(itemId: number) {
    setIndividuallyBusy((prev) => new Set(prev).add(itemId))
    try {
      await runItem(itemId)
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
          {started
            ? `${running ? 'Working…' : 'Finished.'} ${doneCount}/${rows.length} done${errorCount > 0 ? `, ${errorCount} failed` : ''}.`
            : 'Add an optional hint per item, then Generate.'}
        </p>

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
          {started ? (
            <button type="button" onClick={onClose}>
              Close
            </button>
          ) : (
            <>
              <button type="button" onClick={onClose}>
                Cancel
              </button>
              <button type="button" onClick={onGenerateAll}>
                Generate
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
