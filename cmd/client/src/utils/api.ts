// APIError is a refused request: code is the scheduler's, invalid, not_found or internal, or the web client's,
// unavailable or timeout when the scheduler cannot be reached.
export class APIError extends Error {
  code: string

  constructor(code: string, message: string) {
    super(message)
    this.code = code
  }
}

const call = async <T>(path: string, init?: RequestInit): Promise<T> => {
  const res = await fetch(path, init)
  const body = await res.json().catch(() => ({ error: { code: 'internal', message: `${res.status} ${res.statusText}` } }))
  if (body.error) {
    throw new APIError(body.error.code, body.error.message)
  }
  return body as T
}

// get returns the reply to a GET of path.
export const get = <T>(path: string): Promise<T> => call<T>(path)

// post returns the reply to a POST of body, as JSON, to path.
export const post = <T>(path: string, body: unknown): Promise<T> =>
  call<T>(path, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  })
