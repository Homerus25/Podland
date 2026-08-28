import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import { SecretList } from '@features/secrets/components/SecretList'
import type { SecretSummary } from '@features/secrets/services/secretsApi'
import { server } from '@/test/server'

function secretFixture(overrides: Partial<SecretSummary> = {}): SecretSummary {
  return {
    id: '3f2e1d0c',
    name: 'db-password',
    driver: 'file',
    createdAt: '2026-08-20T10:00:00Z',
    updatedAt: '2026-08-20T10:00:00Z',
    ...overrides,
  }
}

function mockSecrets(secrets: SecretSummary[]) {
  server.use(
    http.get('*/api/machines/m-1/secrets', () => HttpResponse.json(secrets)),
  )
}

function renderSecretList(permission: 'read' | 'write' | 'admin' = 'admin') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const view = render(
    <QueryClientProvider client={queryClient}>
      <SecretList machineId="m-1" permission={permission} />
    </QueryClientProvider>,
  )
  return { queryClient, ...view }
}

describe('SecretList permission gating', () => {
  it('hides write controls from read users and explains the access level instead', async () => {
    mockSecrets([secretFixture()])
    renderSecretList('read')

    expect(await screen.findByTestId('secret-row')).toBeInTheDocument()
    expect(screen.queryByTestId('create-secret-button')).not.toBeInTheDocument()
    const hint = screen.getByTestId('secrets-permission-hint')
    expect(hint).toHaveAttribute('data-permission', 'read')
    expect(hint.textContent).toMatch(/read-only/i)
  })

  it.each(['write', 'admin'] as const)(
    'shows the create affordance area to %s users',
    async (permission) => {
      mockSecrets([secretFixture()])
      renderSecretList(permission)

      expect(await screen.findByTestId('create-secret-button')).toBeInTheDocument()
      expect(screen.queryByTestId('secrets-permission-hint')).not.toBeInTheDocument()
    },
  )
})

describe('SecretList', () => {
  it('renders one row per secret with name, driver, and created date', async () => {
    mockSecrets([
      secretFixture(),
      secretFixture({
        id: '9a8b7c6d',
        name: 'api-token',
        driver: 'file',
        createdAt: '2026-08-22T12:00:00Z',
        updatedAt: '2026-08-22T12:00:00Z',
      }),
    ])
    renderSecretList()

    await waitFor(() => {
      expect(screen.queryAllByTestId('secret-row')).toHaveLength(2)
    })
    expect(screen.getByText('db-password')).toBeInTheDocument()
    expect(screen.getByText('api-token')).toBeInTheDocument()
    expect(screen.getAllByText('file')).toHaveLength(2)
  })

  it('caches secrets under the per-machine query key hierarchy', async () => {
    mockSecrets([secretFixture()])
    const { queryClient } = renderSecretList()

    await screen.findByText('db-password')
    const cached = queryClient.getQueryData<SecretSummary[]>(['machines', 'm-1', 'secrets'])
    expect(cached).toHaveLength(1)
    expect(cached?.[0].id).toBe('3f2e1d0c')
  })

  it('shows a loading state before secrets arrive', async () => {
    server.use(
      http.get('*/api/machines/m-1/secrets', async () => {
        await new Promise((resolve) => setTimeout(resolve, 50))
        return HttpResponse.json([secretFixture()])
      }),
    )
    renderSecretList()

    expect(screen.getByTestId('secrets-loading')).toBeInTheDocument()
  })

  it('shows an explanatory empty state when the machine has no secrets', async () => {
    mockSecrets([])
    renderSecretList()

    const empty = await screen.findByTestId('secrets-empty')
    expect(empty.textContent).toMatch(/no secrets/i)
    expect(screen.queryByTestId('secret-row')).not.toBeInTheDocument()
  })

  it('renders the backend error envelope with code and message', async () => {
    server.use(
      http.get('*/api/machines/m-1/secrets', () =>
        HttpResponse.json(
          {
            requestId: 'req-41',
            error: { message: 'Machine not found', code: 'MACHINE_NOT_FOUND' },
          },
          { status: 404 },
        ),
      ),
    )
    renderSecretList()

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('MACHINE_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('Machine not found')).toBeInTheDocument()
  })
})
