import { useEffect, useRef, useState } from 'preact/hooks'
import { route } from 'preact-router'
import { api, ApiError, type ItemDetail as ItemDetailData } from '../api/client'
import { Header } from '../components/Header'
import { Footer } from '../components/Footer'

interface RouteProps {
  path?: string
  default?: boolean
  id?: string
}

/**
 * The item detail/edit view (README flow #6, "mostly desktop but works on
 * mobile"): a drag-to-reorder image carousel (first image = primary, no
 * separate "select primary" step), the consolidated description (editable
 * by hand or regenerated from the photos' notes via Gemini), a shadowbox
 * for each photo's local per-image notes (with a delete action), a
 * clickable location badge, and the per-item activity log.
 */
export function ItemDetail({ id }: RouteProps) {
  const itemId = id ? parseInt(id, 10) : NaN
  const [detail, setDetail] = useState<ItemDetailData | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [description, setDescription] = useState('')
  const [saving, setSaving] = useState(false)
  const [regenerating, setRegenerating] = useState(false)
  const [focusedImageId, setFocusedImageId] = useState<number | null>(null)
  const [deletingImageId, setDeletingImageId] = useState<number | null>(null)
  const [confirmingDeleteItem, setConfirmingDeleteItem] = useState(false)
  const [deletingItem, setDeletingItem] = useState(false)
  const dragIdRef = useRef<number | null>(null)
  const dialogRef = useRef<HTMLDialogElement>(null)

  useEffect(() => {
    if (!itemId || Number.isNaN(itemId)) return
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [itemId])

  function load() {
    setLoading(true)
    setError(null)
    api
      .getItem(itemId)
      .then((d) => {
        setDetail(d)
        setDescription(d.description)
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Failed to load item'))
      .finally(() => setLoading(false))
  }

  async function onSaveDescription() {
    if (!detail) return
    setSaving(true)
    try {
      const updated = await api.updateItemDescription(detail.id, description)
      setDetail(updated)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Save failed')
    } finally {
      setSaving(false)
    }
  }

  async function onGenerateDescription() {
    if (!detail) return
    setRegenerating(true)
    setError(null)
    try {
      const updated = await api.regenerateItemDescription(detail.id)
      setDetail(updated)
      setDescription(updated.description)
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Description generation failed')
    } finally {
      setRegenerating(false)
    }
  }

  function onDrop(targetId: number) {
    if (!detail || dragIdRef.current === null || dragIdRef.current === targetId) return
    const ids = detail.images.map((img) => img.id)
    const from = ids.indexOf(dragIdRef.current)
    const to = ids.indexOf(targetId)
    dragIdRef.current = null
    if (from === -1 || to === -1) return
    ids.splice(to, 0, ids.splice(from, 1)[0])

    // optimistic reorder, then persist
    const byId = new Map(detail.images.map((img) => [img.id, img]))
    setDetail({ ...detail, images: ids.map((imgId) => byId.get(imgId)!) })
    api
      .reorderImages(detail.id, ids)
      .then(setDetail)
      .catch((err) => setError(err instanceof ApiError ? err.message : 'Reorder failed'))
  }

  async function onDeleteImage(imageId: number) {
    if (!detail) return
    setDeletingImageId(imageId)
    setError(null)
    try {
      const updated = await api.deleteImage(detail.id, imageId)
      setDetail(updated)
      if (focusedImageId === imageId) {
        dialogRef.current?.close()
        setFocusedImageId(null)
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Delete failed')
    } finally {
      setDeletingImageId(null)
    }
  }

  async function onDeleteItem() {
    if (!detail) return
    setDeletingItem(true)
    setError(null)
    try {
      await api.bulkDelete([detail.id])
      route('/search')
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Delete failed')
      setDeletingItem(false)
    }
  }

  function openShadowbox(imageId: number) {
    setFocusedImageId(imageId)
    dialogRef.current?.showModal()
  }

  const focusedImage = detail?.images.find((img) => img.id === focusedImageId)

  return (
    <div class="item-detail-view">
      <Header active="item" />

      <div class="item-detail-content">
        {loading && <p>Loading…</p>}
        {!loading && error && !detail && <p class="capture-feedback capture-feedback-error">{error}</p>}

        {detail && (
          <>
            <div class="item-detail-header">
              <h1 class="item-detail-tag">{detail.asset_tag}</h1>
              {detail.location_code && (
                <a
                  class="item-detail-location"
                  href={`/search?location_id=${detail.location_id}&location_code=${encodeURIComponent(detail.location_code)}`}
                >
                  📍 {detail.location_code}
                </a>
              )}

              <div class="item-detail-header-spacer" />

              {!confirmingDeleteItem ? (
                <button type="button" class="item-delete-button" onClick={() => setConfirmingDeleteItem(true)}>
                  Delete item
                </button>
              ) : (
                <span class="item-delete-confirm">
                  Delete this item and all its photos? This can't be undone.
                  <button type="button" onClick={() => setConfirmingDeleteItem(false)} disabled={deletingItem}>
                    Cancel
                  </button>
                  <button type="button" class="item-delete-button" onClick={onDeleteItem} disabled={deletingItem}>
                    {deletingItem ? 'Deleting…' : 'Confirm delete'}
                  </button>
                </span>
              )}
            </div>

            <div class="item-carousel">
              {detail.images.length === 0 && <p>No photos yet.</p>}
              {detail.images.map((img, i) => (
                <div class="item-carousel-item" key={img.id}>
                  <img
                    src={`/api/images/${img.id}`}
                    alt=""
                    class={`item-carousel-thumb${i === 0 ? ' item-carousel-primary' : ''}`}
                    draggable
                    onDragStart={() => (dragIdRef.current = img.id)}
                    onDragOver={(e) => e.preventDefault()}
                    onDrop={() => onDrop(img.id)}
                    onClick={() => openShadowbox(img.id)}
                  />
                  <button
                    type="button"
                    class="item-carousel-delete"
                    aria-label="Delete photo"
                    onClick={(e) => {
                      e.stopPropagation()
                      onDeleteImage(img.id)
                    }}
                    disabled={deletingImageId === img.id}
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>

            <div class="item-description-editor">
              <div class="item-description-label-row">
                <label for="item-description-field">Description</label>
                <button
                  type="button"
                  class="link-button"
                  onClick={onGenerateDescription}
                  disabled={regenerating || detail.images.length === 0}
                >
                  {regenerating ? 'Generating…' : 'Generate description'}
                </button>
              </div>
              <textarea
                id="item-description-field"
                rows={4}
                value={description}
                onInput={(e) => setDescription((e.target as HTMLTextAreaElement).value)}
              />
              <button type="button" onClick={onSaveDescription} disabled={saving}>
                {saving ? 'Saving…' : 'Save'}
              </button>
            </div>

            {error && <p class="capture-feedback capture-feedback-error">{error}</p>}

            <h2>Activity</h2>
            <ul class="activity-log">
              {detail.activity.length === 0 && <li>No activity yet.</li>}
              {detail.activity.map((a, i) => (
                <li key={i}>
                  <strong>{a.username}</strong> {a.action.replace(/_/g, ' ')}
                  {a.detail ? ` — ${a.detail}` : ''}
                  <span class="activity-time"> · {new Date(a.created_at).toLocaleString()}</span>
                </li>
              ))}
            </ul>
          </>
        )}
      </div>

      <dialog ref={dialogRef} class="prompt-shadowbox">
        {focusedImage && (
          <>
            <img src={`/api/images/${focusedImage.id}`} alt="" class="shadowbox-image" />
            <p>{focusedImage.description || <em>No notes for this photo.</em>}</p>
            <div class="shadowbox-actions">
              <button
                type="button"
                class="shadowbox-delete"
                onClick={() => onDeleteImage(focusedImage.id)}
                disabled={deletingImageId === focusedImage.id}
              >
                {deletingImageId === focusedImage.id ? 'Deleting…' : 'Delete photo'}
              </button>
              <button type="button" onClick={() => dialogRef.current?.close()}>
                Close
              </button>
            </div>
          </>
        )}
      </dialog>

      <Footer />
    </div>
  )
}
