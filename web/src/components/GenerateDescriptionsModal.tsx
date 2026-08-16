import { useEffect, useRef, useState } from 'preact/hooks'
import {
  api,
  ApiError,
  type DescriptionBatchStatus,
  type ItemSummary,
} from '../api/client'

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

const POLL_MS = 750

/** Live progress for the server-owned description batch. The job survives
 * closing or refreshing the tab; mounting the modal reconnects to a running
 * job and polling only observes its state. */
export function GenerateDescriptionsModal({ items, onClose, onComplete }: Props) {
  const [rows, setRows] = useState<Row[]>(
    items.map((item) => ({
      itemId: item.id,
      assetTag: item.asset_tag,
      primaryImageId: item.primary_image_id,
      status: 'pending',
      description: '',
      error: '',
    })),
  )
  const [hints, setHints] = useState<Record<number, string>>({})
  const [individuallyBusy, setIndividuallyBusy] = useState<Set<number>>(new Set())
  const [started, setStarted] = useState(false)
  const [starting, setStarting] = useState(false)
  const [running, setRunning] = useState(false)
  const [startError, setStartError] = useState<string | null>(null)
  const completionNotifiedRef = useRef(false)
  const manualStartRef = useRef(false)
  const startLockRef = useRef(false)
  const onCompleteRef = useRef(onComplete)
  onCompleteRef.current = onComplete

  function applyStatus(status: DescriptionBatchStatus) {
    setRows(status.items.map((item) => ({
      itemId: item.item_id,
      assetTag: item.asset_tag,
      primaryImageId: item.primary_image_id,
      status: item.status,
      description: item.description ?? '',
      error: item.error ?? '',
    })))
    setStarted(status.exists)
    setRunning(status.running)
  }

  function notifyComplete() {
    if (completionNotifiedRef.current) return
    completionNotifiedRef.current = true
    onCompleteRef.current()
  }

  useEffect(() => {
    let cancelled = false
    api.descriptionBatchStatus()
      .then((status) => {
        if (cancelled || manualStartRef.current || !status.running) return
        applyStatus(status)
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Exactly one polling chain exists while a server batch is running. The
  // effect cleanup cancels its timer whenever running changes or the modal
  // unmounts, and onComplete is read through a ref so it is never stale.
  useEffect(() => {
    if (!running) return
    let cancelled = false
    let timer: ReturnType<typeof setTimeout> | null = null
    async function poll() {
      try {
        const status = await api.descriptionBatchStatus()
        if (cancelled) return
        applyStatus(status)
        if (!status.running) {
          notifyComplete()
          return
        }
      } catch {
        // A transient status failure should not abandon a server-side job.
      }
      if (!cancelled) timer = setTimeout(poll, POLL_MS)
    }
    timer = setTimeout(poll, POLL_MS)
    return () => {
      cancelled = true
      if (timer) clearTimeout(timer)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [running])

  async function onGenerateAll() {
    if (startLockRef.current) return
    startLockRef.current = true
    manualStartRef.current = true
    setStartError(null)
    setStarted(true)
    setStarting(true)
    completionNotifiedRef.current = false
    try {
      const status = await api.startDescriptionBatch(
        items.map((item) => ({ item_id: item.id, hint: hints[item.id]?.trim() || undefined })),
      )
      applyStatus(status)
      if (!status.running) notifyComplete()
    } catch (err) {
      setStarted(false)
      setRunning(false)
      setStartError(err instanceof ApiError ? err.message : 'Failed to start description generation')
      startLockRef.current = false
      manualStartRef.current = false
    } finally {
      setStarting(false)
    }
  }

  async function onRegenerateOne(itemId: number) {
    setIndividuallyBusy((previous) => new Set(previous).add(itemId))
    setRows((previous) => previous.map((row) => (row.itemId === itemId ? { ...row, status: 'generating', error: '' } : row)))
    try {
      const updated = await api.regenerateItemDescription(itemId, hints[itemId] || undefined)
      setRows((previous) =>
        previous.map((row) =>
          row.itemId === itemId ? { ...row, status: 'done', description: updated.description, error: '' } : row,
        ),
      )
      onCompleteRef.current()
    } catch (err) {
      const message = err instanceof ApiError ? err.message : 'Failed'
      setRows((previous) =>
        previous.map((row) => (row.itemId === itemId ? { ...row, status: 'error', error: message } : row)),
      )
    } finally {
      setIndividuallyBusy((previous) => {
        const next = new Set(previous)
        next.delete(itemId)
        return next
      })
    }
  }

  const doneCount = rows.filter((row) => row.status === 'done').length
  const errorCount = rows.filter((row) => row.status === 'error').length

  return (
    <div class="modal-overlay">
      <div class="modal-panel generate-descriptions-modal">
        <h2>Generate descriptions</h2>
        <p class="generate-descriptions-progress">
          {started
            ? `${starting || running ? 'Working… You can safely close this window.' : 'Finished.'} ${doneCount}/${rows.length} done${errorCount > 0 ? `, ${errorCount} failed` : ''}.`
            : 'Add an optional hint per item, then Generate.'}
        </p>
        {startError && <p class="capture-feedback capture-feedback-error">{startError}</p>}

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
                  <span class={`generate-descriptions-status generate-descriptions-status-${row.status}`}>{row.status}</span>
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
                    disabled={starting || running}
                    onInput={(event) => setHints({ ...hints, [row.itemId]: (event.target as HTMLInputElement).value })}
                  />
                  <button
                    type="button"
                    onClick={() => onRegenerateOne(row.itemId)}
                    disabled={starting || running || individuallyBusy.has(row.itemId)}
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
            <button type="button" onClick={onClose}>Close</button>
          ) : (
            <>
              <button type="button" class="btn-primary" onClick={onGenerateAll}>Generate</button>
              <button type="button" onClick={onClose}>Cancel</button>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
