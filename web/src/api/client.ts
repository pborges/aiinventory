export interface User {
  id: number
  username: string
  enabled: boolean
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

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: 'same-origin',
  })
  await throwIfError(res)
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

async function upload<T>(path: string, form: FormData): Promise<T> {
  const res = await fetch(path, { method: 'POST', body: form, credentials: 'same-origin' })
  await throwIfError(res)
  return (await res.json()) as T
}

export interface PromptSetting {
  override: string
  default: string
}

export interface Settings {
  gemini_model: string
  gemini_model_default: string
  prompts: Record<string, PromptSetting>
}

export interface SettingsUpdate {
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

export interface MovedItem {
  asset_tag: string
  from_location?: string
}

export interface ReconcileDiffResponse {
  has_location_code: boolean
  location_code?: string
  asset_tags: string[]
  added: string[]
  moved: MovedItem[]
  removed: string[]
}

export interface ItemSummary {
  id: number
  asset_tag: string
  description: string
  location_code?: string
  primary_image_id?: number
}

export interface SearchFilters {
  q?: string
  noDescription?: boolean
  noLocation?: boolean
  locationId?: number
}

export interface RegenerateDescriptionResult {
  item_id: number
  description?: string
  error?: string
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
  images: ItemImage[]
  activity: ActivityEntry[]
}

export const api = {
  bootstrapStatus: () => request<{ needed: boolean }>('GET', '/api/auth/bootstrap'),
  bootstrap: (username: string, password: string) =>
    request<{ user: User }>('POST', '/api/auth/bootstrap', { username, password }),
  login: (username: string, password: string) =>
    request<{ user: User }>('POST', '/api/auth/login', { username, password }),
  logout: () => request<void>('POST', '/api/auth/logout'),
  me: () => request<{ user: User }>('GET', '/api/auth/me'),
  getSettings: () => request<Settings>('GET', '/api/settings'),
  updateSettings: (update: SettingsUpdate) => request<Settings>('PUT', '/api/settings', update),
  capture: (image: Blob) => {
    const form = new FormData()
    form.set('image', image, 'capture.jpg')
    return upload<CaptureResponse>('/api/capture', form)
  },
  reconcilePreview: (image: Blob) => {
    const form = new FormData()
    form.set('image', image, 'capture.jpg')
    return upload<ReconcileDiffResponse>('/api/reconcile/preview', form)
  },
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
    if (filters.locationId != null) params.set('location_id', String(filters.locationId))
    const qs = params.toString()
    return request<{ items: ItemSummary[] }>('GET', '/api/search' + (qs ? `?${qs}` : ''))
  },
  bulkDelete: (itemIds: number[]) => request<{ deleted: number }>('POST', '/api/items/bulk-delete', { item_ids: itemIds }),
  bulkRegenerateDescription: (itemIds: number[]) =>
    request<{ results: RegenerateDescriptionResult[] }>('POST', '/api/items/bulk-regenerate-description', {
      item_ids: itemIds,
    }),
  getItem: (id: number) => request<ItemDetail>('GET', `/api/items/${id}`),
  updateItemDescription: (id: number, description: string) =>
    request<ItemDetail>('PUT', `/api/items/${id}`, { description }),
  reorderImages: (itemId: number, imageIds: number[]) =>
    request<ItemDetail>('PUT', `/api/items/${itemId}/images/order`, { image_ids: imageIds }),
}
