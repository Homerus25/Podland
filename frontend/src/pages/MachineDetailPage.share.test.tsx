import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { describe, expect, it } from 'vitest'
import type { AccessGrant, Machine, MachinePermission } from '@shared/types/api'
import { renderApp } from '@/test/auth'
import { server } from '@/test/server'

const machineFixture: Machine = {
  id: 'm-1',
  name: 'prod-web-01',
  description: 'Production web server',
  hostname: 'web01.example.com',
  port: 22,
  sshUsername: 'deploy',
  sshKeyFingerprint: 'SHA256:aaa',
  podmanSocketPath: '/run/podman/podman.sock',
  ownerId: 'u-owner',
  permission: 'admin',
  createdAt: '2026-08-01T10:00:00Z',
  updatedAt: '2026-08-01T10:00:00Z',
}

const defaultGrants: AccessGrant[] = [
  { userId: 'u-owner', permission: 'admin', grantedAt: '2026-08-01T10:00:00Z' },
  { userId: 'u-bob', permission: 'read', grantedAt: '2026-08-03T10:00:00Z' },
]

let grants: AccessGrant[] = defaultGrants

let grantPayload: Record<string, unknown> | undefined
let revokedUserId: string | undefined
let grantsRequests = 0

function resetState() {
  grants = defaultGrants
  grantPayload = undefined
  revokedUserId = undefined
  grantsRequests = 0
}

async function renderSharingSection(permission: MachinePermission = 'admin') {
  resetState()
  server.use(
    http.get('*/api/machines/m-1/access', () => {
      grantsRequests++
      return HttpResponse.json(grants)
    }),
    http.post('*/api/machines/m-1/access', async ({ request }) => {
      grantPayload = (await request.json()) as Record<string, unknown>
      const granted = {
        userId: grantPayload.userId as string,
        permission: grantPayload.permission,
        grantedAt: '2026-08-05T10:00:00Z',
      }
      grants = [...grants, granted as AccessGrant]
      return HttpResponse.json(granted, { status: 201 })
    }),
    http.delete('*/api/machines/m-1/access/:userId', ({ params }) => {
      revokedUserId = params.userId as string
      grants = grants.filter((grant) => grant.userId !== revokedUserId)
      return new HttpResponse(null, { status: 204 })
    }),
  )
  const machine = { ...machineFixture, permission }
  server.use(http.get('*/api/machines/m-1', () => HttpResponse.json(machine)))
  renderApp('/machines/m-1', {
    resolveInitialState: async () => ({ username: 'alice' }),
  })
  await screen.findByText(machine.name)
  await userEvent.click(screen.getByRole('tab', { name: /settings/i }))
}

async function grantAccess(permission: string) {
  await userEvent.type(screen.getByLabelText(/user id/i), 'u-carol')
  await userEvent.selectOptions(screen.getByLabelText(/permission/i), permission)
  await userEvent.click(screen.getByTestId('grant-access-button'))
}

describe('ShareMachineSection (machine sharing on the Settings tab)', () => {
  it('lists the current grants with user id, permission, and granted time', async () => {
    await renderSharingSection()

    expect(await screen.findByTestId('grant-row-u-bob')).toBeInTheDocument()
    expect(screen.getByTestId('grant-row-u-bob')).toHaveTextContent('u-bob')
    expect(screen.getByTestId('grant-row-u-bob')).toHaveTextContent('read')
    expect(screen.getByTestId('grant-row-u-owner')).toBeInTheDocument()
  })

  it('grants access with the selected permission and refetches the grants', async () => {
    await renderSharingSection()
    await screen.findByTestId('grant-row-u-bob')
    const requestsBefore = grantsRequests

    await grantAccess('write')

    expect(await screen.findByTestId('grant-row-u-carol')).toBeInTheDocument()
    expect(grantPayload).toMatchObject({ userId: 'u-carol', permission: 'write' })
    await waitFor(() => expect(grantsRequests).toBeGreaterThan(requestsBefore))
  })

  it('revokes a user, removing their row and refetching grants', async () => {
    await renderSharingSection()
    await screen.findByTestId('grant-row-u-bob')
    const requestsBefore = grantsRequests

    await userEvent.click(screen.getByTestId('revoke-access-u-bob'))

    await waitFor(() => expect(screen.queryByTestId('grant-row-u-bob')).not.toBeInTheDocument())
    expect(revokedUserId).toBe('u-bob')
    await waitFor(() => expect(grantsRequests).toBeGreaterThan(requestsBefore))
  })

  it('renders the error envelope when a grant fails, keeping the pending state during the request', async () => {
    await renderSharingSection()
    await screen.findByTestId('grant-row-u-bob')
    server.use(http.post('*/api/machines/m-1/access', async () => {
      await new Promise((resolve) => setTimeout(resolve, 50))
      return HttpResponse.json(
        { requestId: 'req-6', error: { message: 'User not found', code: 'USER_NOT_FOUND' } },
        { status: 404 },
      )
    }))

    await grantAccess('read')

    expect(screen.getByTestId('grant-access-button')).toBeDisabled()
    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('USER_NOT_FOUND')).toBeInTheDocument()
  })

  it('renders the error envelope when a revoke fails', async () => {
    await renderSharingSection()
    await screen.findByTestId('grant-row-u-bob')
    server.use(
      http.delete('*/api/machines/m-1/access/:userId', () =>
        HttpResponse.json(
          { requestId: 'req-7', error: { message: 'Not allowed', code: 'ACCESS_DENIED' } },
          { status: 403 },
        ),
      ),
    )

    await userEvent.click(screen.getByTestId('revoke-access-u-bob'))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('ACCESS_DENIED')).toBeInTheDocument()
    expect(screen.getByTestId('grant-row-u-bob')).toBeInTheDocument()
  })

  it.each(['write', 'read'] as const)(
    'hides the sharing section for %s-permission users',
    async (permission) => {
      await renderSharingSection(permission)

      expect(screen.queryByTestId('grant-access-button')).not.toBeInTheDocument()
      expect(screen.queryByTestId(/^grant-row-/)).not.toBeInTheDocument()
    },
  )
})
