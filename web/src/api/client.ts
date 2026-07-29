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
}

export interface SettingsUpdate {
  gemini_api_key?: string
  gemini_model?: string
  prompts?: Record<string, string>
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
  item_guess?: string
  image_description?: string
  item_will_be_new?: boolean
}

export interface MovedItem {
  asset_tag: string
  from_location?: string
}

export interface ReconcileDiffResponse {
  has_location_code: boolean
  location_code?: string
  asset_tags: string[]
  new: string[]
  added: string[]
  moved: MovedItem[]
  removed: string[]
  // Only set on the first (straight) preview response of the locate flow's
  // dual-read cross-check — which way to rotate the same frame for the
  // second, corroborating read.
  suggested_rotation?: 'clockwise' | 'counterclockwise'
}

export interface Tag {
  id: number
  name: string
  color: string
}

export interface ItemSummary {
  id: number
  asset_tag: string
  description: string
  location_code?: string
  location_description?: string
  primary_image_id?: number
  tags: Tag[]
}

export interface SearchFilters {
  q?: string
  noDescription?: boolean
  noLocation?: boolean
  noPhoto?: boolean
  locationId?: number
  tagIds?: number[]
  locationTagIds?: number[]
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
  location_code?: string
  location_description?: string
  images: ItemImage[]
  activity: ActivityEntry[]
  tags: Tag[]
}

export interface Location {
  id: number
  code: string
  description?: string
  tags: Tag[]
}

// formatLocationCode renders a location's code with its optional description
// appended, e.g. "@XYZ (Break room shelf)" — the "@XYZ (description)" shape
// used everywhere a location code is displayed.
export function formatLocationCode(code: string, description?: string): string {
  return description ? `${code} (${description})` : code
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
  reconcileDiff: (locationCode: string, assetTags: string[]) =>
    request<ReconcileDiffResponse>('POST', '/api/reconcile/diff', {
      location_code: locationCode,
      asset_tags: assetTags,
    }),
  reconcileApply: (locationCode: string, assetTags: string[]) =>
    request<ReconcileDiffResponse>('POST', '/api/reconcile/apply', {
      location_code: locationCode,
      asset_tags: assetTags,
    }),
  search: (filters: SearchFilters) => {
    const params = new URLSearchParams()
    if (filters.q) params.set('q', filters.q)
    if (filters.noDescription) params.set('no_description', '1')
    if (filters.noLocation) params.set('no_location', '1')
    if (filters.noPhoto) params.set('no_photo', '1')
    if (filters.locationId != null) params.set('location_id', String(filters.locationId))
    for (const tagId of filters.tagIds ?? []) params.append('tag_id', String(tagId))
    for (const tagId of filters.locationTagIds ?? []) params.append('location_tag_id', String(tagId))
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
  setLocationTags: (locationId: number, tagIds: number[]) =>
    request<{ location: Location }>('PUT', `/api/locations/${locationId}/tags`, { tag_ids: tagIds }),
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
  listTags: () => request<{ tags: Tag[] }>('GET', '/api/tags'),
  createTag: (name: string, color: string) => request<{ tag: Tag }>('POST', '/api/tags', { name, color }),
  updateTag: (id: number, name: string, color: string) =>
    request<{ tag: Tag }>('PUT', `/api/tags/${id}`, { name, color }),
  deleteTag: (id: number) => request<void>('DELETE', `/api/tags/${id}`),
  setItemTags: (itemId: number, tagIds: number[]) =>
    request<ItemDetail>('PUT', `/api/items/${itemId}/tags`, { tag_ids: tagIds }),
  listLocationTags: () => request<{ tags: Tag[] }>('GET', '/api/location-tags'),
  createLocationTag: (name: string, color: string) => request<{ tag: Tag }>('POST', '/api/location-tags', { name, color }),
  updateLocationTag: (id: number, name: string, color: string) =>
    request<{ tag: Tag }>('PUT', `/api/location-tags/${id}`, { name, color }),
  deleteLocationTag: (id: number) => request<void>('DELETE', `/api/location-tags/${id}`),
}
