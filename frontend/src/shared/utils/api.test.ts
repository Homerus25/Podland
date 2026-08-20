import { describe, expect, it } from 'vitest'
import { AxiosError, type AxiosResponse } from 'axios'
import { api, getApiError } from '@shared/utils/api'

function responseOf(status: number, data: unknown): AxiosResponse {
  return { status, data, statusText: '', headers: {}, config: {} } as AxiosResponse
}

describe('api client', () => {
  it('returns OK from /api/health through the shared API client', async () => {
    const response = await api.get('/health')

    expect(response.status).toBe(200)
    expect(response.data).toBe('OK')
  })
})

describe('getApiError', () => {
  it('passes through the envelope code and message for client errors', () => {
    const error = new AxiosError('Request failed', 'ERR_BAD_REQUEST', undefined, undefined, responseOf(401, {
      requestId: 'req-1',
      error: { code: 'INVALID_CREDENTIALS', message: 'Invalid username or password' },
    }))

    expect(getApiError(error)).toEqual({
      code: 'INVALID_CREDENTIALS',
      message: 'Invalid username or password',
    })
  })

  it('collapses server errors to the generic message regardless of the body', () => {
    const error = new AxiosError('Server error', 'ERR_BAD_RESPONSE', undefined, undefined, responseOf(503, {
      requestId: 'req-2',
      error: { code: 'SERVICE_UNAVAILABLE', message: 'Service temporarily unavailable' },
    }))

    expect(getApiError(error)).toEqual({
      code: 'INTERNAL_ERROR',
      message: 'An unexpected error occurred',
    })
  })

  it('collapses network failures without a response to the generic message', () => {
    expect(getApiError(new AxiosError('Network Error'))).toEqual({
      code: 'INTERNAL_ERROR',
      message: 'An unexpected error occurred',
    })
  })

  it('collapses non-envelope error bodies to the generic message', () => {
    const error = new AxiosError('Bad request', 'ERR_BAD_REQUEST', undefined, undefined, responseOf(400, 'Bad Request'))

    expect(getApiError(error)).toEqual({
      code: 'INTERNAL_ERROR',
      message: 'An unexpected error occurred',
    })
  })

  it('collapses internal error codes to the generic message even on a 4xx status', () => {
    const error = new AxiosError('Bad request', 'ERR_BAD_REQUEST', undefined, undefined, responseOf(400, {
      requestId: 'req-3',
      error: { code: 'INTERNAL_ERROR', message: 'Invalid username or password' },
    }))

    expect(getApiError(error)).toEqual({
      code: 'INTERNAL_ERROR',
      message: 'An unexpected error occurred',
    })
  })
})
