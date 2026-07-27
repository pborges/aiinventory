import { useEffect, useState } from 'preact/hooks'
import { ApiError, type Tag } from '../api/client'
import { TagChip } from './TagChip'
import { TagColorPicker } from './TagColorPicker'

interface Props {
  title: string
  /** Noun used in the delete-confirmation sentence, e.g. "item" or "location". */
  deleteWarningTarget: string
  list: () => Promise<{ tags: Tag[] }>
  create: (name: string, color: string) => Promise<{ tag: Tag }>
  update: (id: number, name: string, color: string) => Promise<{ tag: Tag }>
  remove: (id: number) => Promise<void>
}

/** Full CRUD list (name + color, add/edit/delete) for one independent tag
 * pool — Settings uses this for both the item-tag and location-tag
 * sections, which are otherwise identical UIs over two separate tables. */
export function TagManagerSection({ title, deleteWarningTarget, list, create, update, remove }: Props) {
  const [tags, setTags] = useState<Tag[] | null>(null)
  const [newName, setNewName] = useState('')
  const [newColor, setNewColor] = useState('#a6e22e')
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editingName, setEditingName] = useState('')
  const [editingColor, setEditingColor] = useState('')
  const [confirmingDeleteId, setConfirmingDeleteId] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

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
      await create(newName, newColor)
      setNewName('')
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to create tag')
    } finally {
      setBusy(false)
    }
  }

  function startEditing(tag: Tag) {
    setEditingId(tag.id)
    setEditingName(tag.name)
    setEditingColor(tag.color)
  }

  async function onSaveEdit(e: Event) {
    e.preventDefault()
    if (editingId == null) return
    setBusy(true)
    setError(null)
    try {
      await update(editingId, editingName, editingColor)
      setEditingId(null)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to update tag')
    } finally {
      setBusy(false)
    }
  }

  async function onDelete(tag: Tag) {
    setBusy(true)
    setError(null)
    try {
      await remove(tag.id)
      setConfirmingDeleteId(null)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete tag')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section class="settings-tags">
      <h2>{title}</h2>

      <ul class="settings-tag-list">
        {tags === null && <li>Loading…</li>}
        {tags?.map((tag) =>
          editingId === tag.id ? (
            <li class="settings-tag-row" key={tag.id}>
              <form class="settings-new-tag-form" onSubmit={onSaveEdit}>
                <input
                  type="text"
                  class="settings-tag-row-name-input"
                  value={editingName}
                  onInput={(e) => setEditingName((e.target as HTMLInputElement).value)}
                  required
                />
                <TagColorPicker value={editingColor} onChange={setEditingColor} />
                <button type="submit" class="btn-primary" disabled={busy}>
                  Save
                </button>
                <button type="button" onClick={() => setEditingId(null)} disabled={busy}>
                  Cancel
                </button>
              </form>
            </li>
          ) : (
            <li class="settings-tag-row" key={tag.id}>
              <TagChip tag={tag} />
              {confirmingDeleteId === tag.id ? (
                <span class="settings-tag-row-actions">
                  Delete “{tag.name}” and remove it from every {deleteWarningTarget}?
                  <button type="button" onClick={() => setConfirmingDeleteId(null)} disabled={busy}>
                    Cancel
                  </button>
                  <button type="button" class="btn-danger" onClick={() => onDelete(tag)} disabled={busy}>
                    {busy ? 'Deleting…' : 'Confirm delete'}
                  </button>
                </span>
              ) : (
                <span class="settings-tag-row-actions">
                  <button type="button" onClick={() => startEditing(tag)} disabled={busy}>
                    Edit
                  </button>
                  <button type="button" onClick={() => setConfirmingDeleteId(tag.id)} disabled={busy}>
                    Delete
                  </button>
                </span>
              )}
            </li>
          ),
        )}
      </ul>

      <form class="settings-new-tag-form" onSubmit={onCreate}>
        <input
          type="text"
          placeholder="Tag name"
          value={newName}
          onInput={(e) => setNewName((e.target as HTMLInputElement).value)}
          required
        />
        <TagColorPicker value={newColor} onChange={setNewColor} />
        <button type="submit" class="btn-primary" disabled={busy}>
          Add tag
        </button>
      </form>
      {error && <p class="settings-status settings-status-error">{error}</p>}
    </section>
  )
}
