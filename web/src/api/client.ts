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

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
    credentials: 'same-origin',
  })

  if (!res.ok) {
    let message = res.statusText
    try {
      const data = await res.json()
      if (data?.error) message = data.error
    } catch {
      // response body wasn't JSON; fall back to statusText
    }
    throw new ApiError(res.status, message)
  }

  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const api = {
  bootstrapStatus: () => request<{ needed: boolean }>('GET', '/api/auth/bootstrap'),
  bootstrap: (username: string, password: string) =>
    request<{ user: User }>('POST', '/api/auth/bootstrap', { username, password }),
  login: (username: string, password: string) =>
    request<{ user: User }>('POST', '/api/auth/login', { username, password }),
  logout: () => request<void>('POST', '/api/auth/logout'),
  me: () => request<{ user: User }>('GET', '/api/auth/me'),
}
