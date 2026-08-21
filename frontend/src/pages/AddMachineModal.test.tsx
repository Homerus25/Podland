import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'
import { renderApp } from '@/test/auth'
import { server } from '@/test/server'
import type { Machine } from '@shared/types/api'

const existingMachine: Machine = {
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

const createdMachine: Machine = {
  ...existingMachine,
  id: 'm-2',
  name: 'ci-runner',
}

function mockMachines(body: Machine[]) {
  return http.get('*/api/machines', () => HttpResponse.json(body))
}

let createRequests: Array<Record<string, unknown>> = []

function mockCreate(status: number, body: Record<string, unknown>, delayMs = 0) {
  return http.post('*/api/machines', async ({ request }) => {
    const payload = (await request.json()) as Record<string, unknown>
    createRequests.push(payload)
    if (delayMs > 0) {
      await new Promise((resolve) => setTimeout(resolve, delayMs))
    }
    return HttpResponse.json(body, { status })
  })
}

function keyFile() {
  return new File(['-----BEGIN OPENSSH PRIVATE KEY-----\nabc\n-----END OPENSSH PRIVATE KEY-----'], 'id_ed25519', {
    type: 'text/plain',
  })
}

async function renderAndOpenModal(machines: Machine[] = [existingMachine]) {
  server.use(mockMachines(machines))
  renderApp('/machines', { resolveInitialState: async () => ({ username: 'alice' }) })
  await screen.findByTestId('add-machine-button')
  await userEvent.click(screen.getByTestId('add-machine-button'))
  expect(await screen.findByTestId('modal')).toBeInTheDocument()
}

async function fillValidForm() {
  await userEvent.type(screen.getByLabelText(/^Name/), 'ci-runner')
  await userEvent.type(screen.getByLabelText(/^Hostname or IP/), 'ci.internal')
  await userEvent.clear(screen.getByLabelText(/^SSH port/))
  await userEvent.type(screen.getByLabelText(/^SSH port/), '2222')
  await userEvent.type(screen.getByLabelText(/^SSH username/), 'ci')
  await userEvent.upload(screen.getByLabelText(/^SSH private key/), keyFile())
}

describe('Add Machine modal', () => {
  beforeEach(() => {
    createRequests = []
  })

  it('opens a modal with the form from the Add Machine button', async () => {
    await renderAndOpenModal()
    expect(screen.getByLabelText(/^Name/)).toBeInTheDocument()
    expect(screen.getByLabelText(/^Hostname or IP/)).toBeInTheDocument()
    expect(screen.getByLabelText(/^SSH port/)).toBeInTheDocument()
    expect(screen.getByLabelText(/^SSH username/)).toBeInTheDocument()
    expect(screen.getByLabelText(/^SSH private key/)).toBeInTheDocument()
    expect(screen.getByLabelText(/^Description/)).toBeInTheDocument()
    expect(screen.getByLabelText(/^Podman socket path/)).toBeInTheDocument()
  })

  it('shows inline errors for empty required fields on submit', async () => {
    await renderAndOpenModal()
    await userEvent.clear(screen.getByLabelText(/ssh port/i))
    await userEvent.click(screen.getByTestId('machine-form-submit'))

    expect(await screen.findByText('Name is required')).toBeInTheDocument()
    expect(screen.getByText('Hostname is required')).toBeInTheDocument()
    expect(screen.getByText('Port is required')).toBeInTheDocument()
    expect(screen.getByText('SSH username is required')).toBeInTheDocument()
    expect(screen.getByText('SSH private key file is required')).toBeInTheDocument()
    expect(createRequests).toHaveLength(0)
  })

  it('shows an inline error when the port is outside the numeric range', async () => {
    await renderAndOpenModal()
    await fillValidForm()
    await userEvent.clear(screen.getByLabelText(/^SSH port/))
    await userEvent.type(screen.getByLabelText(/^SSH port/), '99999')
    await userEvent.click(screen.getByTestId('machine-form-submit'))

    expect(await screen.findByText('Port must be a number between 1 and 65535')).toBeInTheDocument()
    expect(createRequests).toHaveLength(0)
  })

  it('creates a machine, closes the modal, and refetches the list', async () => {
    let machines: Machine[] = [existingMachine]
    await renderAndOpenModal()
    // Registered after the helper's static handler so msw prefers this one.
    server.use(
      http.get('*/api/machines', () => HttpResponse.json(machines)),
      http.post('*/api/machines', async ({ request }) => {
        const body = (await request.json()) as Record<string, unknown>
    createRequests.push(body)
        machines = [existingMachine, createdMachine]
        return HttpResponse.json(createdMachine, { status: 201 })
      }),
    )
    await fillValidForm()
    await userEvent.click(screen.getByTestId('machine-form-submit'))

    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())
    expect(await screen.findByText('ci-runner')).toBeInTheDocument()
    expect(createRequests).toHaveLength(1)
    expect(createRequests[0]).toMatchObject({
      name: 'ci-runner',
      hostname: 'ci.internal',
      port: 2222,
      sshUsername: 'ci',
      sshPrivateKey: expect.stringContaining('BEGIN OPENSSH PRIVATE KEY'),
    })
  })

  it('submits optional fields empty', async () => {
    server.use(mockMachines([]), mockCreate(201, createdMachine as unknown as Record<string, unknown>))
    await renderAndOpenModal([])
    await fillValidForm()
    await userEvent.click(screen.getByTestId('machine-form-submit'))

    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())
    expect(createRequests[0]).toMatchObject({ description: '', podmanSocketPath: '' })
  })

  it('renders the error envelope inside the form when creation fails', async () => {
    server.use(
      mockMachines([existingMachine]),
      http.post('*/api/machines', () =>
        HttpResponse.json(
          { requestId: 'req-1', error: { message: 'SSH handshake failed', code: 'MACHINE_UNREACHABLE' } },
          { status: 422 },
        ),
      ),
    )
    await renderAndOpenModal()
    await fillValidForm()
    await userEvent.click(screen.getByTestId('machine-form-submit'))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('MACHINE_UNREACHABLE')).toBeInTheDocument()
    expect(screen.getByText('SSH handshake failed')).toBeInTheDocument()
    expect(screen.getByTestId('modal')).toBeInTheDocument()
  })

  it('clears a failed error when the modal is reopened', async () => {
    server.use(
      mockMachines([existingMachine]),
      http.post('*/api/machines', () =>
        HttpResponse.json(
          { requestId: 'req-1', error: { message: 'SSH handshake failed', code: 'MACHINE_UNREACHABLE' } },
          { status: 422 },
        ),
      ),
    )
    await renderAndOpenModal()
    await fillValidForm()
    await userEvent.click(screen.getByTestId('machine-form-submit'))
    expect(await screen.findByRole('alert')).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /close dialog/i }))
    await userEvent.click(screen.getByTestId('add-machine-button'))
    expect(await screen.findByTestId('modal')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('shows a pending state on the submit button and prevents double submits', async () => {
    server.use(
      mockMachines([existingMachine]),
      mockCreate(201, createdMachine as unknown as Record<string, unknown>, 100),
    )
    await renderAndOpenModal()
    await fillValidForm()
    await userEvent.click(screen.getByTestId('machine-form-submit'))
    const submit = screen.getByTestId('machine-form-submit') as HTMLButtonElement
    await waitFor(() => expect(submit.disabled).toBe(true))
    await userEvent.click(submit)
    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())
    expect(createRequests).toHaveLength(1)
  })
})
