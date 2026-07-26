import { useEffect, useRef, useState } from 'preact/hooks'
import { api, ApiError, type Settings as SettingsData, type UserListItem } from '../api/client'
import { currentUser } from '../state/auth'
import { Header } from '../components/Header'

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

// A starting-point list, not exhaustive — this field is free text specifically
// because model names move fast; check ai.google.dev/gemini-api/docs/models
// for the current lineup if these look stale.
const MODEL_SUGGESTIONS = [
  'gemini-3.6-flash',
  'gemini-3.5-flash',
  'gemini-3.5-flash-lite',
  'gemini-3.1-pro-preview',
  'gemini-2.5-pro',
]

export function Settings(_props: RouteProps) {
  const [data, setData] = useState<SettingsData | null>(null)
  const [model, setModel] = useState('')
  const [overrides, setOverrides] = useState<Record<string, string>>({})
  const [status, setStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [errorMsg, setErrorMsg] = useState<string | null>(null)
  const dialogRef = useRef<HTMLDialogElement>(null)
  const [shadowboxText, setShadowboxText] = useState('')

  const [users, setUsers] = useState<UserListItem[] | null>(null)
  const [newUsername, setNewUsername] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [userError, setUserError] = useState<string | null>(null)
  const [userBusy, setUserBusy] = useState(false)

  useEffect(() => {
    api.getSettings().then((s) => {
      setData(s)
      setModel(s.gemini_model)
      const o: Record<string, string> = {}
      for (const { key } of PROMPT_TYPES) o[key] = s.prompts[key]?.override ?? ''
      setOverrides(o)
    })
    loadUsers()
  }, [])

  function loadUsers() {
    api
      .listUsers()
      .then((res) => setUsers(res.users))
      .catch((err) => setUserError(err instanceof ApiError ? err.message : 'Failed to load users'))
  }

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

  async function onCreateUser(e: Event) {
    e.preventDefault()
    setUserBusy(true)
    setUserError(null)
    try {
      await api.createUser(newUsername, newPassword)
      setNewUsername('')
      setNewPassword('')
      loadUsers()
    } catch (err) {
      setUserError(err instanceof ApiError ? err.message : 'Failed to create user')
    } finally {
      setUserBusy(false)
    }
  }

  async function onToggleEnabled(user: UserListItem) {
    setUserBusy(true)
    setUserError(null)
    try {
      await api.setUserEnabled(user.id, !user.enabled)
      loadUsers()
    } catch (err) {
      setUserError(err instanceof ApiError ? err.message : 'Failed to update user')
    } finally {
      setUserBusy(false)
    }
  }

  return (
    <div class="settings-view">
      <Header active="settings" />

      <div class="settings-content">
        {!data ? (
          <p>Loading…</p>
        ) : (
          <>
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

            <section class="settings-users">
              <h2>Users</h2>
              <p class="settings-users-note">
                No admin/non-admin distinction yet — any logged-in, enabled user can manage every account.
              </p>

              <ul class="settings-user-list">
                {users === null && <li>Loading…</li>}
                {users?.map((u) => (
                  <li class="settings-user-row" key={u.id}>
                    <span class="settings-user-name">
                      {u.username}
                      {u.id === currentUser.value?.id && ' (you)'}
                    </span>
                    <span class={u.enabled ? 'settings-user-status-enabled' : 'settings-user-status-disabled'}>
                      {u.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                    <button type="button" onClick={() => onToggleEnabled(u)} disabled={userBusy}>
                      {u.enabled ? 'Disable' : 'Enable'}
                    </button>
                  </li>
                ))}
              </ul>

              <form class="settings-new-user-form" onSubmit={onCreateUser}>
                <input
                  placeholder="Username"
                  value={newUsername}
                  onInput={(e) => setNewUsername((e.target as HTMLInputElement).value)}
                  required
                />
                <input
                  type="password"
                  placeholder="Password"
                  value={newPassword}
                  onInput={(e) => setNewPassword((e.target as HTMLInputElement).value)}
                  minLength={8}
                  required
                />
                <button type="submit" disabled={userBusy}>
                  Add user
                </button>
              </form>
              {userError && <p class="settings-status settings-status-error">{userError}</p>}
            </section>
          </>
        )}
      </div>

      <dialog ref={dialogRef} class="prompt-shadowbox">
        <pre>{shadowboxText}</pre>
        <button type="button" onClick={() => dialogRef.current?.close()}>
          Close
        </button>
      </dialog>
    </div>
  )
}
