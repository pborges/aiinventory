export interface User {
  id: number
  username: string
  enabled: boolean
}

export interface UserListItem extends User {
  created_at: string
}

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function throwIfError(res: Response): Promise<void> {
  if (res.ok) return
  let message = res.statusText
  try {
    const data = await res.json()
    if (data?.error) message = data.error
  } catch {
    // response body wasn't JSON; fall back to statusText
  }
  throw new ApiError(res.status, message)
}

// Some endpoints (e.g. the batch-kickoff 202 Accepted responses) return no
// body at all — parsing JSON unconditionally on any non-204 status throws a
// plain SyntaxError on the empty string, which isn't an ApiError and reads
// as a mysterious "Failed to start" even though the request succeeded. Read
// as text first and only parse if there's actually something there,
// regardless of which status code came back.
async function parseBody<T>(res: Response): Promise<T> {
  const text = await res.text()
  return (text ? JSON.parse(text) : undefined) as T
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: 'same-origin',
  })
  await throwIfError(res)
  return parseBody<T>(res)
}

async function upload<T>(path: string, form: FormData): Promise<T> {
  const res = await fetch(path, { method: 'POST', body: form, credentials: 'same-origin' })
  await throwIfError(res)
  return parseBody<T>(res)
}

export interface PromptSetting {
  override: string
  default: string
}

export interface Settings {
  gemini_api_key_set: boolean
  gemini_model: string
  gemini_model_default: string
  prompts: Record<string, PromptSetting>
  location_dual_read_enabled: boolean
}


export interface SettingsUpdate {
  gemini_api_key?: string
  gemini_model?: string
  prompts?: Record<string, string>
  location_dual_read_enabled?: boolean
}

export interface CaptureResponse {
  has_asset_tag: boolean
  asset_tag?: string
  item_id?: number
  item_was_new?: boolean
  item_guess?: string
  image_description?: string
}

export interface CapturePreviewResponse {
  has_asset_tag: boolean
  asset_tag?: string
  raw_asset_tag?: string
  corrected?: boolean
  needs_resolution?: boolean
  candidates?: string[]
  item_guess?: string
  image_description?: string
  item_will_be_new?: boolean
}

export interface MovedItem {
  asset_tag: string
  from_location?: string
}

export interface TagResolution {
  raw: string
  status: 'exact' | 'corrected' | 'ambiguous' | 'no_match'
  candidates?: string[]
}

export interface ReconcileDiffResponse {
  has_location_tag: boolean
  location_tag?: string
  // Only set on preview responses — what Gemini actually read, alongside
  // has_location_tag. The deterministic registry check runs on top of this.
  raw_location_tag?: string
  // True when there's a single, confident distance-1 registry candidate for
  // raw_location_tag — never auto-applied, only a pre-selected suggestion
  // (location_tag_candidates[0]) the operator must still confirm.
  location_tag_corrected?: boolean
  // True whenever the read isn't an exact registry match. The diff above is
  // still computed against the raw read, so there's something to show while
  // the operator resolves it.
  location_tag_needs_resolution?: boolean
  location_tag_candidates?: string[]
  asset_tags: string[]
  new: string[]
  added: string[]
  moved: MovedItem[]
  removed: string[]
  // Only set on the first (straight) preview response of the locate flow's
  // dual-read cross-check — which way to rotate the same frame for the
  // second, corroborating read.
  suggested_rotation?: 'clockwise' | 'counterclockwise'
  // Only set on preview responses — one entry per raw tag Gemini read
  // (before shape-invalid ones are rejected outright).
  tag_resolutions?: TagResolution[]
}

export interface Label {
  id: number
  name: string
  color: string
}

export interface ItemSummary {
  id: number
  asset_tag: string
  description: string
  location_tag?: string
  location_description?: string
  primary_image_id?: number
  labels: Label[]
}

export interface SearchFilters {
  q?: string
  noDescription?: boolean
  noLocation?: boolean
  noPhoto?: boolean
  locationId?: number
  labelIds?: number[]
  locationLabelIds?: number[]
}

export interface ItemImage {
  id: number
  description: string
  sort_order: number
}

export interface ActivityEntry {
  username: string
  action: string
  detail?: string
  created_at: string
}

export interface ItemDetail {
  id: number
  asset_tag: string
  description: string
  location_id?: number
  location_tag?: string
  location_description?: string
  images: ItemImage[]
  activity: ActivityEntry[]
  labels: Label[]
}

export interface Location {
  id: number
  location_tag: string
  description?: string
  labels: Label[]
}

// formatLocationTag renders a location's tag with its optional description
// appended, e.g. "@XYZ (Break room shelf)" — the "@XYZ (description)" shape
// used everywhere a location tag is displayed.
export function formatLocationTag(locationTag: string, description?: string): string {
  return description ? `${locationTag} (${description})` : locationTag
}

export interface LocationItem {
  id: number
  asset_tag: string
  description: string
  images: ItemImage[]
}

export interface DuplicateStatus {
  running: boolean
  started_at?: string
}

export interface DuplicateGroupMember {
  item_id: number
  asset_tag: string
}

export interface DuplicateGroup {
  id: number
  items: DuplicateGroupMember[]
  reasoning: string
  created_at: string
}

// RegisteredTagEntry is one row in the asset-tag or location-tag allow-list
// registry — the Settings registry section's list view. Create/bulk-create/
// list/delete only; entries are never edited.
export interface RegisteredTagEntry {
  id: number
  tag: string
  created_at: string
}

export interface UploadRegisteredTagsResponse {
  added: number
  skipped: number
}

export const api = {
  version: () => request<{ version: string }>('GET', '/api/version'),
  bootstrapStatus: () => request<{ needed: boolean }>('GET', '/api/auth/bootstrap'),
  bootstrap: (username: string, password: string) =>
    request<{ user: User }>('POST', '/api/auth/bootstrap', { username, password }),
  login: (username: string, password: string) =>
    request<{ user: User }>('POST', '/api/auth/login', { username, password }),
  logout: () => request<void>('POST', '/api/auth/logout'),
  me: () => request<{ user: User }>('GET', '/api/auth/me'),
  getSettings: () => request<Settings>('GET', '/api/settings'),
  updateSettings: (update: SettingsUpdate) => request<Settings>('PUT', '/api/settings', update),
  capturePreview: (image: Blob) => {
    const form = new FormData()
    form.set('image', image, 'capture.jpg')
    return upload<CapturePreviewResponse>('/api/capture/preview', form)
  },
  captureApply: (image: Blob, assetTag: string, description: string) => {
    const form = new FormData()
    form.set('image', image, 'capture.jpg')
    form.set('asset_tag', assetTag)
    form.set('description', description)
    return upload<CaptureResponse>('/api/capture/apply', form)
  },
  reconcilePreview: (image: Blob) => {
    const form = new FormData()
    form.set('image', image, 'capture.jpg')
    return upload<ReconcileDiffResponse>('/api/reconcile/preview', form)
  },
  reconcileDiff: (locationTag: string, assetTags: string[]) =>
    request<ReconcileDiffResponse>('POST', '/api/reconcile/diff', {
      location_tag: locationTag,
      asset_tags: assetTags,
    }),
  reconcileApply: (locationTag: string, assetTags: string[]) =>
    request<ReconcileDiffResponse>('POST', '/api/reconcile/apply', {
      location_tag: locationTag,
      asset_tags: assetTags,
    }),
  search: (filters: SearchFilters) => {
    const params = new URLSearchParams()
    if (filters.q) params.set('q', filters.q)
    if (filters.noDescription) params.set('no_description', '1')
    if (filters.noLocation) params.set('no_location', '1')
    if (filters.noPhoto) params.set('no_photo', '1')
    if (filters.locationId != null) params.set('location_id', String(filters.locationId))
    for (const labelId of filters.labelIds ?? []) params.append('label_id', String(labelId))
    for (const labelId of filters.locationLabelIds ?? []) params.append('location_label_id', String(labelId))
    const qs = params.toString()
    return request<{ items: ItemSummary[] }>('GET', '/api/search' + (qs ? `?${qs}` : ''))
  },
  bulkDelete: (itemIds: number[]) => request<{ deleted: number }>('POST', '/api/items/bulk-delete', { item_ids: itemIds }),
  getItem: (id: number) => request<ItemDetail>('GET', `/api/items/${id}`),
  updateItemDescription: (id: number, description: string) =>
    request<ItemDetail>('PUT', `/api/items/${id}`, { description }),
  reorderImages: (itemId: number, imageIds: number[]) =>
    request<ItemDetail>('PUT', `/api/items/${itemId}/images/order`, { image_ids: imageIds }),
  deleteImage: (itemId: number, imageId: number) =>
    request<ItemDetail>('DELETE', `/api/items/${itemId}/images/${imageId}`),
  regenerateItemDescription: (itemId: number, hint?: string) =>
    request<ItemDetail>('POST', `/api/items/${itemId}/regenerate-description`, hint ? { hint } : undefined),
  listLocations: () => request<{ locations: Location[] }>('GET', '/api/locations'),
  updateLocation: (id: number, description: string) =>
    request<{ location: Location }>('PUT', `/api/locations/${id}`, { description }),
  getLocationItems: (id: number) => request<{ items: LocationItem[] }>('GET', `/api/locations/${id}/items`),
  getLocationActivity: (id: number) => request<{ activity: ActivityEntry[] }>('GET', `/api/locations/${id}/activity`),
  moveItemToLocation: (locationId: number, itemId: number) =>
    request<{ item_id: number; location_id: number }>('POST', `/api/locations/${locationId}/move-item`, { item_id: itemId }),
  setLocationLabels: (locationId: number, labelIds: number[]) =>
    request<{ location: Location }>('PUT', `/api/locations/${locationId}/labels`, { label_ids: labelIds }),
  duplicatesStatus: () => request<DuplicateStatus>('GET', '/api/duplicates/status'),
  startDuplicateRun: () => request<void>('POST', '/api/duplicates/run'),
  listDuplicateGroups: () => request<{ groups: DuplicateGroup[] }>('GET', '/api/duplicates/groups'),
  dismissDuplicateGroup: (id: number) => request<void>('POST', `/api/duplicates/groups/${id}/dismiss`),
  mergeDuplicateGroup: (id: number, survivorItemId: number, locationId: number | null) =>
    request<void>('POST', `/api/duplicates/groups/${id}/merge`, {
      survivor_item_id: survivorItemId,
      location_id: locationId,
    }),
  listUsers: () => request<{ users: UserListItem[] }>('GET', '/api/users'),
  createUser: (username: string, password: string) =>
    request<{ user: User }>('POST', '/api/users', { username, password }),
  setUserEnabled: (id: number, enabled: boolean) => request<void>('PUT', `/api/users/${id}`, { enabled }),
  listItemLabels: () => request<{ labels: Label[] }>('GET', '/api/labels'),
  createItemLabel: (name: string, color: string) => request<{ label: Label }>('POST', '/api/labels', { name, color }),
  updateItemLabel: (id: number, name: string, color: string) =>
    request<{ label: Label }>('PUT', `/api/labels/${id}`, { name, color }),
  deleteItemLabel: (id: number) => request<void>('DELETE', `/api/labels/${id}`),
  setItemLabels: (itemId: number, labelIds: number[]) =>
    request<ItemDetail>('PUT', `/api/items/${itemId}/labels`, { label_ids: labelIds }),
  listLocationLabels: () => request<{ labels: Label[] }>('GET', '/api/location-labels'),
  createLocationLabel: (name: string, color: string) => request<{ label: Label }>('POST', '/api/location-labels', { name, color }),
  updateLocationLabel: (id: number, name: string, color: string) =>
    request<{ label: Label }>('PUT', `/api/location-labels/${id}`, { name, color }),
  deleteLocationLabel: (id: number) => request<void>('DELETE', `/api/location-labels/${id}`),
  listRegisteredAssetTags: () => request<{ tags: RegisteredTagEntry[] }>('GET', '/api/tags'),
  createRegisteredAssetTag: (tag: string) => request<{ tag: RegisteredTagEntry }>('POST', '/api/tags', { tag }),
  deleteRegisteredAssetTag: (id: number) => request<void>('DELETE', `/api/tags/${id}`),
  uploadRegisteredAssetTags: (file: File) => {
    const form = new FormData()
    form.set('file', file)
    return upload<UploadRegisteredTagsResponse>('/api/tags/upload', form)
  },
  listRegisteredLocationTags: () => request<{ tags: RegisteredTagEntry[] }>('GET', '/api/location-tags'),
  createRegisteredLocationTag: (tag: string) => request<{ tag: RegisteredTagEntry }>('POST', '/api/location-tags', { tag }),
  deleteRegisteredLocationTag: (id: number) => request<void>('DELETE', `/api/location-tags/${id}`),
  uploadRegisteredLocationTags: (file: File) => {
    const form = new FormData()
    form.set('file', file)
    return upload<UploadRegisteredTagsResponse>('/api/location-tags/upload', form)
  },
}
