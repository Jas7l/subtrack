import type { ErrorBody } from './types'

const BASE = '/api/v1'
const TOKEN_KEY = 'subtrack.token'

// The token lives in sessionStorage: it survives a reload, and every tab has
// its own session (three roles in three tabs for the demo).
export const tokenStore = {
  get: (): string | null => sessionStorage.getItem(TOKEN_KEY),
  set: (token: string) => sessionStorage.setItem(TOKEN_KEY, token),
  clear: () => sessionStorage.removeItem(TOKEN_KEY),
}

// ApiError carries the HTTP status and the server's English `description`;
// lib/errors.ts turns it into Russian text. Status 0 means no response.
export class ApiError extends Error {
  readonly status: number
  readonly description: string

  constructor(status: number, description: string) {
    super(description || `HTTP ${status}`)
    this.name = 'ApiError'
    this.status = status
    this.description = description
  }
}

export type Query = Record<string, string | number | boolean | null | undefined>

interface Options {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  query?: Query
  body?: unknown
}

function url(path: string, query?: Query): string {
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(query ?? {})) {
    if (v !== undefined && v !== null && v !== '') params.set(k, String(v))
  }
  const qs = params.toString()
  return BASE + path + (qs ? `?${qs}` : '')
}

// A 401 on a request that carried a token means the session is over (expired
// or the user was blocked): drop the token and go to the login page, which
// returns the user here afterwards. A 401 without a token is a failed login.
function onUnauthorized() {
  tokenStore.clear()
  const { pathname, search } = window.location
  if (pathname === '/login') return
  window.location.assign(`/login?next=${encodeURIComponent(pathname + search)}`)
}

async function send(path: string, opts: Options): Promise<Response> {
  const token = tokenStore.get()
  const headers: Record<string, string> = {}
  if (token) headers.Authorization = `Bearer ${token}`
  if (opts.body !== undefined) headers['Content-Type'] = 'application/json'

  let res: Response
  try {
    res = await fetch(url(path, opts.query), {
      method: opts.method ?? 'GET',
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
    })
  } catch {
    throw new ApiError(0, '')
  }
  if (res.ok) return res

  if (res.status === 401 && token) onUnauthorized()
  let description = ''
  try {
    description = ((await res.json()) as ErrorBody).description ?? ''
  } catch {
    // not JSON (e.g. the proxy could not reach the server)
  }
  throw new ApiError(res.status, description)
}

export async function request<T>(path: string, opts: Options = {}): Promise<T> {
  const res = await send(path, opts)
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

// download fetches a file (Excel export) with the auth header and saves it
// under the name from Content-Disposition: a plain <a href> cannot send the
// token.
export async function download(path: string, query?: Query): Promise<void> {
  const res = await send(path, { query })
  const blob = await res.blob()
  const link = document.createElement('a')
  link.href = URL.createObjectURL(blob)
  link.download = fileName(res.headers.get('Content-Disposition')) ?? 'export.xlsx'
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(link.href)
}

function fileName(disposition: string | null): string | null {
  if (!disposition) return null
  const star = /filename\*=UTF-8''([^;]+)/i.exec(disposition)
  if (star) return decodeURIComponent(star[1])
  const plain = /filename="?([^";]+)"?/i.exec(disposition)
  return plain ? plain[1] : null
}
