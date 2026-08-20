import axios from 'axios'
import type { ApiError, ApiErrorEnvelope } from '@shared/types/api'

export const api = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api',
})

// The access token is held only in this module variable for the lifetime of
// the page — never localStorage/sessionStorage — so XSS cannot exfiltrate a
// persisted bearer credential. The request interceptor attaches it below.
let bearerToken: string | null = null

export function setBearerToken(token: string | null) {
  bearerToken = token
}

api.interceptors.request.use((config) => {
  if (bearerToken) {
    config.headers.Authorization = `Bearer ${bearerToken}`
  }
  return config
})

const genericError: ApiError = { code: 'INTERNAL_ERROR', message: 'An unexpected error occurred' }

// Error codes that indicate server-internal failures. The backend already
// rewrites 5xx responses to these, but the code is what the spec keys on:
// internal codes never reach the user, whatever status they arrived with.
const internalCodePrefixes = ['INTERNAL_', 'SERVICE_']

function isInternalCode(code: string) {
  return internalCodePrefixes.some((prefix) => code.startsWith(prefix))
}

// Extracts the backend error envelope for display. Client errors (4xx) with
// a non-internal code pass their code and message through; server errors
// (5xx), internal error codes, non-envelope bodies, and network failures are
// collapsed to the generic message so internals are never leaked to users.
export function getApiError(error: unknown): ApiError {
  if (axios.isAxiosError(error)) {
    const status = error.response?.status
    const envelope = error.response?.data as ApiErrorEnvelope | undefined
    if (
      status !== undefined &&
      status < 500 &&
      envelope?.error?.code &&
      envelope?.error?.message &&
      !isInternalCode(envelope.error.code)
    ) {
      return { code: envelope.error.code, message: envelope.error.message }
    }
  }
  return { ...genericError }
}
