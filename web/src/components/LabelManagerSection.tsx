import { useEffect, useState } from 'preact/hooks'
import { ApiError, type Label } from '../api/client'
import { LabelChip } from './LabelChip'
import { LabelColorPicker } from './LabelColorPicker'

interface Props {
  title: string
  /** Noun used in the delete-confirmation sentence, e.g. "item" or "location". */
  deleteWarningTarget: string
  list: () => Promise<{ labels: Label[] }>
  create: (name: string, color: string) => Promise<{ label: Label }>
  update: (id: number, name: string, color: string) => Promise<{ label: Label }>
  remove: (id: number) => Promise<void>
}

/** Full CRUD list (name + color, add/edit/delete) for one independent label
 * pool — Settings uses this for both the item-label and location-label
 * sections, which are otherwise identical UIs over two separate tables. */
export function LabelManagerSection({ title, deleteWarningTarget, list, create, update, remove }: Props) {
  const [labels, setLabels] = useState<Label[] | null>(null)
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
      .then((res) => setLabels(res.labels))
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
      setError(err instanceof ApiError ? err.message : 'Failed to create label')
    } finally {
      setBusy(false)
    }
  }

  function startEditing(label: Label) {
    setEditingId(label.id)
    setEditingName(label.name)
    setEditingColor(label.color)
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
      setError(err instanceof ApiError ? err.message : 'Failed to update label')
    } finally {
      setBusy(false)
    }
  }

  async function onDelete(label: Label) {
    setBusy(true)
    setError(null)
    try {
      await remove(label.id)
      setConfirmingDeleteId(null)
      load()
    } catch (err) {
      setError(err instanceof ApiError ? err.message : 'Failed to delete label')
    } finally {
      setBusy(false)
    }
  }

  return (
    <section class="settings-labels">
      <h2>{title}</h2>

      <ul class="settings-label-list">
        {labels === null && <li>Loading…</li>}
        {labels?.map((label) =>
          editingId === label.id ? (
            <li class="settings-label-row" key={label.id}>
              <form class="settings-new-label-form" onSubmit={onSaveEdit}>
                <input
                  type="text"
                  class="settings-label-row-name-input"
                  value={editingName}
                  onInput={(e) => setEditingName((e.target as HTMLInputElement).value)}
                  required
                />
                <LabelColorPicker value={editingColor} onChange={setEditingColor} />
                <button type="submit" class="btn-primary" disabled={busy}>
                  Save
                </button>
                <button type="button" onClick={() => setEditingId(null)} disabled={busy}>
                  Cancel
                </button>
              </form>
            </li>
          ) : (
            <li class="settings-label-row" key={label.id}>
              <LabelChip label={label} />
              {confirmingDeleteId === label.id ? (
                <span class="settings-label-row-actions">
                  Delete “{label.name}” and remove it from every {deleteWarningTarget}?
                  <button type="button" onClick={() => setConfirmingDeleteId(null)} disabled={busy}>
                    Cancel
                  </button>
                  <button type="button" class="btn-danger" onClick={() => onDelete(label)} disabled={busy}>
                    {busy ? 'Deleting…' : 'Confirm delete'}
                  </button>
                </span>
              ) : (
                <span class="settings-label-row-actions">
                  <button type="button" onClick={() => startEditing(label)} disabled={busy}>
                    Edit
                  </button>
                  <button type="button" onClick={() => setConfirmingDeleteId(label.id)} disabled={busy}>
                    Delete
                  </button>
                </span>
              )}
            </li>
          ),
        )}
      </ul>

      <form class="settings-new-label-form" onSubmit={onCreate}>
        <input
          type="text"
          placeholder="Label name"
          value={newName}
          onInput={(e) => setNewName((e.target as HTMLInputElement).value)}
          required
        />
        <LabelColorPicker value={newColor} onChange={setNewColor} />
        <button type="submit" class="btn-primary" disabled={busy}>
          Add label
        </button>
      </form>
      {error && <p class="settings-status settings-status-error">{error}</p>}
    </section>
  )
}
