import { describe, expect, it } from 'vitest'
import { api } from '@shared/utils/api'

describe('api client', () => {
  it('returns OK from /api/health through the shared API client', async () => {
    const response = await api.get('/health')

    expect(response.status).toBe(200)
    expect(response.data).toBe('OK')
  })
})
