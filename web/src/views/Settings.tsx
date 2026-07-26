import { useEffect, useRef, useState } from 'preact/hooks'
import { api, ApiError, type Settings as SettingsData } from '../api/client'

interface RouteProps {
  path?: string
  default?: boolean
}

const PROMPT_TYPES: { key: string; label: string }[] = [
  { key: 'tag_capture', label: 'Tag capture' },
  { key: 'location_reconciliation', label: 'Location reconciliation' },
  { key: 'description_regeneration', label: 'Description regeneration' },
  { key: 'duplicate_detection', label: 'Duplicate detection' },
]

const MODEL_SUGGESTIONS = ['gemini-2.5-flash', 'gemini-2.5-pro', 'gemini-2.5-flash-lite']

export function Settings(_props: RouteProps) {
  const [data, setData] = useState<SettingsData | null>(null)
  const [model, setModel] = useState('')
  const [overrides, setOverrides] = useState<Record<string, string>>({})
  const [status, setStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [errorMsg, setErrorMsg] = useState<string | null>(null)
  const dialogRef = useRef<HTMLDialogElement>(null)
  const [shadowboxText, setShadowboxText] = useState('')

  useEffect(() => {
    api.getSettings().then((s) => {
      setData(s)
      setModel(s.gemini_model)
      const o: Record<string, string> = {}
      for (const { key } of PROMPT_TYPES) o[key] = s.prompts[key]?.override ?? ''
      setOverrides(o)
    })
  }, [])

  function showDefault(key: string) {
    setShadowboxText(data?.prompts[key]?.default ?? '')
    dialogRef.current?.showModal()
  }

  async function onSave(e: Event) {
    e.preventDefault()
    setStatus('saving')
    setErrorMsg(null)
    try {
      const updated = await api.updateSettings({ gemini_model: model, prompts: overrides })
      setData(updated)
      setStatus('saved')
    } catch (err) {
      setErrorMsg(err instanceof ApiError ? err.message : 'Something went wrong')
      setStatus('error')
    }
  }

  if (!data) {
    return <p>Loading…</p>
  }

  return (
    <div class="settings-view">
      <a href="/capture" class="settings-back-link">
        ← Back
      </a>
      <h1>Settings</h1>

      <form class="settings-form" onSubmit={onSave}>
        <section>
          <h2>Gemini configuration</h2>
          <label class="settings-field">
            Model
            <input
              list="model-suggestions"
              value={model}
              placeholder={data.gemini_model_default}
              onInput={(e) => setModel((e.target as HTMLInputElement).value)}
            />
            <datalist id="model-suggestions">
              {MODEL_SUGGESTIONS.map((m) => (
                <option value={m} key={m} />
              ))}
            </datalist>
          </label>

          {PROMPT_TYPES.map(({ key, label }) => (
            <div class="settings-field" key={key}>
              <label>
                {label} prompt override
                <textarea
                  rows={4}
                  value={overrides[key] ?? ''}
                  placeholder="Leave empty to use the built-in default"
                  onInput={(e) => setOverrides({ ...overrides, [key]: (e.target as HTMLTextAreaElement).value })}
                />
              </label>
              <button type="button" class="link-button" onClick={() => showDefault(key)}>
                View default prompt
              </button>
            </div>
          ))}
        </section>

        <div class="settings-actions">
          <button type="submit" disabled={status === 'saving'}>
            Save
          </button>
          {status === 'saved' && <span class="settings-status">Saved</span>}
          {errorMsg && <span class="settings-status settings-status-error">{errorMsg}</span>}
        </div>
      </form>

      <dialog ref={dialogRef} class="prompt-shadowbox">
        <pre>{shadowboxText}</pre>
        <button type="button" onClick={() => dialogRef.current?.close()}>
          Close
        </button>
      </dialog>
    </div>
  )
}
