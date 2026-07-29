import { useEffect, useRef, useState } from 'preact/hooks'
import { ApiError, type RegisteredTagEntry, type UploadRegisteredTagsResponse } from '../api/client'

interface Props {
  title: string
  pattern: RegExp
  placeholder: string
  list: () => Promise<{ tags: RegisteredTagEntry[] }>
  create: (tag: string) => Promise<{ tag: RegisteredTagEntry }>
  remove: (id: number) => Promise<void>
  upload: (file: File) => Promise<UploadRegisteredTagsResponse>
}

/** Simple registry CRUD — create, bulk-create (.txt upload), list, delete;
 * no edit — for one tag allow-list, backing the deterministic OCR-correction
 * system in internal/inventory.ResolveTag. Settings uses this for both the
 * asset-tag and location-tag registries, which are otherwise identical UIs
 * over two separate tables. Delete is immediate/non-confirming, same as
 * ItemDetail's per-photo delete — a registry entry's blast radius is just
 * its own membership row, trivially reversible by re-adding it. */
export function RegisteredTagsSection({ title, pattern, placeholder, list, create, remove, upload }: Props) {
  const [tags, setTags] = useState<RegisteredTagEntry[] | null>(null)
  const [newTag, setNewTag] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const [uploadStatus, setUploadStatus] = useState<string | null>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  function load() {
    list()
      .then((res) => setTags(res.tags))
      .catch((err) => setError(err instanceof ApiError ? err.message : `Failed to load ${title.toLowerCase()}`))
  }

  async function onCreate(e: Event) {
    e.preventDefault()
    setBusy(true)
    setError(null)
    try {
      await create(newTag)
      setNewTag('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to add tag')
    } finally {
      setBusy(false)
    }
  }

  async function onDelete(entry: RegisteredTagEntry) {
    setBusy(true)
    setError(null)
    try {
      await remove(entry.id)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete tag')
    } finally {
      setBusy(false)
    }
  }

  async function onFileChange(e: Event) {
    const file = (e.target as HTMLInputElement).files?.[0]
    if (!file) return
    setBusy(true)
    setError(null)
    setUploadStatus(null)
    try {
      const res = await upload(file)
      setUploadStatus(`Added ${res.added}, skipped ${res.skipped}.`)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Upload failed')
    } finally {
      setBusy(false)
      if (fileInputRef.current) fileInputRef.current.value = ''
    }
  }

  return (
    <section class="settings-registry">
      <h2>{title}</h2>

      <ul class="settings-registry-list">
        {tags === null && <li>Loading…</li>}
        {tags?.length === 0 && <li class="settings-registry-empty">No tags registered yet.</li>}
        {tags?.map((entry) => (
          <li class="settings-registry-row" key={entry.id}>
            <span class="settings-registry-tag">{entry.tag}</span>
            <button type="button" onClick={() => onDelete(entry)} disabled={busy}>
              Delete
            </button>
          </li>
        ))}
      </ul>

      <form class="settings-new-registry-form" onSubmit={onCreate}>
        <input
          type="text"
          placeholder={placeholder}
          value={newTag}
          onInput={(e) => setNewTag((e.target as HTMLInputElement).value.toUpperCase())}
          pattern={pattern.source}
          required
        />
        <button type="submit" class="btn-primary" disabled={busy}>
          Add
        </button>
      </form>

      <label class="settings-registry-upload">
        Bulk import from a .txt file (one tag per line)
        <input ref={fileInputRef} type="file" accept=".txt" onChange={onFileChange} disabled={busy} />
      </label>
      {uploadStatus && <p class="settings-status">{uploadStatus}</p>}
      {error && <p class="settings-status settings-status-error">{error}</p>}
    </section>
  )
}
