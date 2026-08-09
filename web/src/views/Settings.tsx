import { useEffect, useRef, useState } from 'preact/hooks'
import { api, ApiError, type Settings as SettingsData, type UserListItem } from '../api/client'
import { currentUser } from '../state/auth'
import { Header } from '../components/Header'
import { Footer } from '../components/Footer'
import { LabelManagerSection } from '../components/LabelManagerSection'
import { RegisteredTagsSection } from '../components/RegisteredTagsSection'
import { GenerateTagsSection } from '../components/GenerateTagsSection'

interface RouteProps {
  path?: string
  default?: boolean
}

type SettingsSection =
  | 'gemini'
  | 'users'
  | 'asset-tags'
  | 'generate-asset-tags'
  | 'location-tags'
  | 'generate-location-tags'

const SETTINGS_SECTIONS: { key: SettingsSection; label: string }[] = [
  { key: 'gemini', label: 'Gemini configuration' },
  { key: 'users', label: 'Users' },
  { key: 'asset-tags', label: 'Asset Tags' },
  { key: 'generate-asset-tags', label: 'Generate Asset Tags' },
  { key: 'location-tags', label: 'Location Tags' },
  { key: 'generate-location-tags', label: 'Generate Location Tags' },
]

const ASSET_TAG_PATTERN = /^[A-Z]{4}$/
const LOCATION_TAG_PATTERN = /^@[A-Z]{3}$/

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
  const [section, setSection] = useState<SettingsSection>('gemini')
  const [data, setData] = useState<SettingsData | null>(null)
  const [apiKey, setApiKey] = useState('')
  const [model, setModel] = useState('')
  const [overrides, setOverrides] = useState<Record<string, string>>({})
  const [dualReadEnabled, setDualReadEnabled] = useState(true)
  const [status, setStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')
  const [errorMsg, setErrorMsg] = useState<string | null>(null)
  const dialogRef = useRef<HTMLDialogElement>(null)
  const [shadowboxText, setShadowboxText] = useState('')

  const [users, setUsers] = useState<UserListItem[] | null>(null)
  const [newUsername, setNewUsername] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [userError, setUserError] = useState<string | null>(null)
  const [userBusy, setUserBusy] = useState(false)

  // Bumped after a Generate-tags batch registration so each pane's
  // RegisteredTagsSection reloads its list — see refreshKey on that component.
  const [assetTagsRefreshKey, setAssetTagsRefreshKey] = useState(0)
  const [locationTagsRefreshKey, setLocationTagsRefreshKey] = useState(0)

  useEffect(() => {
    api.getSettings().then((s) => {
      setData(s)
      setModel(s.gemini_model)
      const o: Record<string, string> = {}
      for (const { key } of PROMPT_TYPES) o[key] = s.prompts[key]?.override ?? ''
      setOverrides(o)
      setDualReadEnabled(s.location_dual_read_enabled)
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
      const updated = await api.updateSettings({
        gemini_model: model,
        prompts: overrides,
        location_dual_read_enabled: dualReadEnabled,
        ...(apiKey ? { gemini_api_key: apiKey } : {}),
      })
      setData(updated)
      setApiKey('')
      setStatus('saved')
    } catch (err) {
      setErrorMsg(err instanceof ApiError ? err.message : 'Something went wrong')
      setStatus('error')
    }
  }

  async function onClearApiKey() {
    setStatus('saving')
    setErrorMsg(null)
    try {
      const updated = await api.updateSettings({ gemini_api_key: '' })
      setData(updated)
      setApiKey('')
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

      {!data ? (
        <p class="settings-loading">Loading…</p>
      ) : (
        <div class="sidebar-page-layout">
          <aside class="sidebar-page-sidebar">
            <h2>Settings</h2>
            <ul>
              {SETTINGS_SECTIONS.map(({ key, label }) => (
                <li
                  key={key}
                  class={'settings-nav-item' + (section === key ? ' settings-nav-item-active' : '')}
                  onClick={() => setSection(key)}
                >
                  {label}
                </li>
              ))}
            </ul>
          </aside>

          <main class="sidebar-page-main">
            <div class="settings-content">
              {section === 'gemini' && (
                <form class="settings-form" onSubmit={onSave}>
                  <section>
                    <h2>Gemini configuration</h2>
                    <label class="settings-field">
                      API key
                      <input
                        type="password"
                        autocomplete="off"
                        value={apiKey}
                        placeholder={data.gemini_api_key_set ? 'Configured — enter a new key to replace it' : 'Not configured'}
                        onInput={(e) => setApiKey((e.target as HTMLInputElement).value)}
                      />
                    </label>
                    {data.gemini_api_key_set && (
                      <button type="button" class="link-button" onClick={onClearApiKey} disabled={status === 'saving'}>
                        Clear API key
                      </button>
                    )}

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

                    <label class="settings-checkbox-field">
                      <input
                        type="checkbox"
                        checked={dualReadEnabled}
                        onChange={(e) => setDualReadEnabled((e.target as HTMLInputElement).checked)}
                      />
                      Dual-read location tag cross-check
                    </label>
                    <p class="settings-field-hint">
                      When enabled, a "Locate items" capture analyzes each frame twice — straight and rotated 90° — and
                      cross-checks the two reads before trusting a tag. Disabling this halves the Gemini calls per locate
                      scan but drops that extra safety check.
                    </p>

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
                    <button type="submit" class="btn-primary" disabled={status === 'saving'}>
                      Save
                    </button>
                    {status === 'saved' && <span class="settings-status">Saved</span>}
                    {errorMsg && <span class="settings-status settings-status-error">{errorMsg}</span>}
                  </div>
                </form>
              )}

              {section === 'users' && (
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
                    <button type="submit" class="btn-primary" disabled={userBusy}>
                      Add user
                    </button>
                  </form>
                  {userError && <p class="settings-status settings-status-error">{userError}</p>}
                </section>
              )}

              {section === 'asset-tags' && (
                <>
                  <LabelManagerSection
                    title="Labels"
                    deleteWarningTarget="item"
                    list={api.listItemLabels}
                    create={api.createItemLabel}
                    update={api.updateItemLabel}
                    remove={api.deleteItemLabel}
                  />
                  <RegisteredTagsSection
                    title="Registered Tags"
                    pattern={ASSET_TAG_PATTERN}
                    placeholder="ABCD"
                    list={api.listRegisteredAssetTags}
                    create={api.createRegisteredAssetTag}
                    remove={api.deleteRegisteredAssetTag}
                    upload={api.uploadRegisteredAssetTags}
                    refreshKey={assetTagsRefreshKey}
                  />
                </>
              )}

              {section === 'generate-asset-tags' && (
                <GenerateTagsSection
                  title="Generate Asset Tags"
                  generate={api.generateAssetTagSheet}
                  register={api.registerAssetTagSheet}
                  getSettings={api.getAssetTagSheetSettings}
                  saveSettings={api.saveAssetTagSheetSettings}
                  resetSettings={api.resetAssetTagSheetSettings}
                  fileBaseName="asset-tags"
                  onRegistered={() => setAssetTagsRefreshKey((k) => k + 1)}
                />
              )}

              {section === 'location-tags' && (
                <>
                  <LabelManagerSection
                    title="Labels"
                    deleteWarningTarget="location"
                    list={api.listLocationLabels}
                    create={api.createLocationLabel}
                    update={api.updateLocationLabel}
                    remove={api.deleteLocationLabel}
                  />
                  <RegisteredTagsSection
                    title="Registered Tags"
                    pattern={LOCATION_TAG_PATTERN}
                    placeholder="@ABC"
                    list={api.listRegisteredLocationTags}
                    create={api.createRegisteredLocationTag}
                    remove={api.deleteRegisteredLocationTag}
                    upload={api.uploadRegisteredLocationTags}
                    refreshKey={locationTagsRefreshKey}
                  />
                </>
              )}

              {section === 'generate-location-tags' && (
                <GenerateTagsSection
                  title="Generate Location Tags"
                  generate={api.generateLocationTagSheet}
                  register={api.registerLocationTagSheet}
                  getSettings={api.getLocationTagSheetSettings}
                  saveSettings={api.saveLocationTagSheetSettings}
                  resetSettings={api.resetLocationTagSheetSettings}
                  fileBaseName="location-tags"
                  onRegistered={() => setLocationTagsRefreshKey((k) => k + 1)}
                />
              )}
            </div>
          </main>
        </div>
      )}

      <dialog ref={dialogRef} class="prompt-shadowbox">
        <pre>{shadowboxText}</pre>
        <button type="button" onClick={() => dialogRef.current?.close()}>
          Close
        </button>
      </dialog>

      <Footer />
    </div>
  )
}
