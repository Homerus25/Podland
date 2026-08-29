import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { createSecret, listSecrets } from '@features/secrets/services/secretsApi'
import { server } from '@/test/server'

describe('secretsApi', () => {
  it('lists a machine\u2019s secrets from the machine-scoped endpoint', async () => {
    server.use(
      http.get('*/api/machines/m-1/secrets', () =>
        HttpResponse.json([
          {
            id: '3f2e1d0c',
            name: 'db-password',
            driver: 'file',
            createdAt: '2026-08-20T10:00:00Z',
            updatedAt: '2026-08-20T10:00:00Z',
          },
        ]),
      ),
    )

    const secrets = await listSecrets('m-1')

    expect(secrets).toEqual([
      {
        id: '3f2e1d0c',
        name: 'db-password',
        driver: 'file',
        createdAt: '2026-08-20T10:00:00Z',
        updatedAt: '2026-08-20T10:00:00Z',
      },
    ])
  })

  // The DOM strips newlines from single-line inputs (HTML value
  // sanitization), so a newline cannot travel through the masked input in
  // a browser; this pins the transport half of the no-trim rule — whatever
  // value the form produces reaches the backend byte-identical, embedded
  // newlines included (JSON round-trips them).
  it('posts the create payload byte-identical, preserving whitespace and newlines', async () => {
    let received: unknown
    server.use(
      http.post('*/api/machines/m-1/secrets', async ({ request }) => {
        received = await request.json()
        return HttpResponse.json({ id: 'fresh' }, { status: 201 })
      }),
    )

    const created = await createSecret('m-1', {
      name: 'pem-key',
      value: '  line-one\nline-two=rsa...\n',
    })

    expect(created).toEqual({ id: 'fresh' })
    expect(received).toEqual({
      name: 'pem-key',
      value: '  line-one\nline-two=rsa...\n',
    })
  })
})
