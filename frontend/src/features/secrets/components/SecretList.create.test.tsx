import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { http, HttpResponse } from 'msw'
import { beforeEach, describe, expect, it } from 'vitest'
import { SecretList } from '@features/secrets/components/SecretList'
import type { SecretSummary } from '@features/secrets/services/secretsApi'
import { server } from '@/test/server'

function secretFixture(overrides: Partial<SecretSummary> = {}): SecretSummary {
  return {
    id: 'existing',
    name: 'existing',
    driver: 'file',
    createdAt: '2026-08-20T10:00:00Z',
    updatedAt: '2026-08-20T10:00:00Z',
    ...overrides,
  }
}

const existing = secretFixture()

let createRequests: { count: number; body: unknown } = { count: 0, body: null }

function resetCounters() {
  createRequests = { count: 0, body: null }
}

function secretsHandler(secrets: () => SecretSummary[]) {
  return http.get('*/api/machines/m-1/secrets', () => HttpResponse.json(secrets()))
}

function createHandler(
  status = 201,
  body?: Record<string, unknown>,
  onCreated?: () => void,
) {
  return http.post('*/api/machines/m-1/secrets', async ({ request }) => {
    createRequests.count++
    createRequests.body = await request.json()
    if (status !== 201) return HttpResponse.json(body, { status })
    onCreated?.()
    return HttpResponse.json({ id: 'fresh' }, { status: 201 })
  })
}

function renderSecretList(permission: 'read' | 'write' | 'admin' = 'admin') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={queryClient}>
      <SecretList machineId="m-1" permission={permission} />
    </QueryClientProvider>,
  )
}

async function openCreateModal() {
  renderSecretList()
  await screen.findByText('existing')
  const user = userEvent.setup()
  await user.click(screen.getByTestId('create-secret-button'))
  expect(await screen.findByTestId('modal')).toBeInTheDocument()
  return user
}

describe('SecretList create', () => {
  beforeEach(resetCounters)

  it('opens a modal form with a name field and a masked value input', async () => {
    server.use(secretsHandler(() => [existing]), createHandler())
    await openCreateModal()

    expect(screen.getByLabelText(/^name/i)).toBeRequired()
    const value = screen.getByLabelText(/^value/i)
    expect(value).toBeRequired()
    expect(value).toHaveAttribute('type', 'password')
  })

  it('reveals and re-hides the value through the eye toggle', async () => {
    server.use(secretsHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    const value = screen.getByLabelText(/^value/i)
    expect(value).toHaveAttribute('type', 'password')

    await user.click(screen.getByTestId('toggle-value-visibility'))
    expect(value).toHaveAttribute('type', 'text')

    await user.click(screen.getByTestId('toggle-value-visibility'))
    expect(value).toHaveAttribute('type', 'password')
  })

  it('shows inline errors for empty fields and keeps the request unsent', async () => {
    server.use(secretsHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    // The submit button is disabled while invalid, so the empty-field
    // errors surface through onChange validation: touch both fields, then
    // clear them.
    const submit = screen.getByTestId('secret-form-submit')
    expect(submit).toBeDisabled()

    await user.type(screen.getByLabelText(/^name/i), 'x')
    await user.type(screen.getByLabelText(/^value/i), 'x')
    await user.clear(screen.getByLabelText(/^name/i))
    await user.clear(screen.getByLabelText(/^value/i))

    expect(await screen.findByText('Name is required')).toBeInTheDocument()
    expect(screen.getByText('Value is required')).toBeInTheDocument()
    expect(submit).toBeDisabled()
    expect(createRequests.count).toBe(0)
  })

  it('blocks submit with a whitespace-only value and sends no request', async () => {
    server.use(secretsHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/^name/i), 'db-password')
    await user.type(screen.getByLabelText(/^value/i), '   ')
    await user.click(screen.getByTestId('secret-form-submit'))

    expect(await screen.findByText('Value is required')).toBeInTheDocument()
    expect(createRequests.count).toBe(0)
  })

  it.each([
    ['-leading-dash', 'must start with a letter or digit'],
    ['has space', 'letters, digits'],
    ['träger-umlaut', 'letters, digits'],
  ])('blocks submit with the invalid name %s', async (name, expectedFragment) => {
    server.use(secretsHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/^name/i), name)
    await user.type(screen.getByLabelText(/^value/i), 'hunter2')
    await user.click(screen.getByTestId('secret-form-submit'))

    expect(await screen.findByText(expectedFragment, { exact: false })).toBeInTheDocument()
    expect(createRequests.count).toBe(0)
  })

  it('disables the submit button while the form is invalid and enables it once valid', async () => {
    server.use(secretsHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    const submit = screen.getByTestId('secret-form-submit')
    expect(submit).toBeDisabled()

    await user.type(screen.getByLabelText(/^name/i), 'db-password')
    await user.type(screen.getByLabelText(/^value/i), 'hunter2')
    await waitFor(() => expect(submit).toBeEnabled())

    // The eye toggle must never submit the form, whatever its default type.
    await user.click(screen.getByTestId('toggle-value-visibility'))
    expect(createRequests.count).toBe(0)
    expect(screen.getByTestId('modal')).toBeInTheDocument()
  })

  it('sends the trimmed name and the byte-identical untrimmed value', async () => {
    server.use(secretsHandler(() => [existing]), createHandler())
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/^name/i), '  db-password  ')
    await user.type(screen.getByLabelText(/^value/i), '  hunter2  ')
    await user.click(screen.getByTestId('secret-form-submit'))

    await waitFor(() => expect(createRequests.count).toBe(1))
    expect(createRequests.body).toEqual({
      name: 'db-password',
      value: '  hunter2  ',
    })
  })

  it('closes the modal and refetches the list so the new secret appears', async () => {
    let secrets = [existing]
    server.use(
      secretsHandler(() => secrets),
      createHandler(201, undefined, () => {
        secrets = [
          existing,
          secretFixture({
            id: 'fresh',
            name: 'fresh',
            createdAt: '2026-08-28T09:00:00Z',
            updatedAt: '2026-08-28T09:00:00Z',
          }),
        ]
      }),
    )
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/^name/i), 'fresh')
    await user.type(screen.getByLabelText(/^value/i), 'hunter2')
    await user.click(screen.getByTestId('secret-form-submit'))

    await waitFor(() => expect(screen.queryByTestId('modal')).not.toBeInTheDocument())
    expect(await screen.findByTestId('secrets-table')).toBeInTheDocument()
    expect(await screen.findByText('fresh')).toBeInTheDocument()
    expect(screen.getAllByTestId('secret-row')).toHaveLength(2)
    expect(createRequests.count).toBe(1)
  })

  it('keeps the modal open and shows the error envelope on a 409 conflict', async () => {
    server.use(
      secretsHandler(() => [existing]),
      createHandler(409, {
        requestId: 'req-51',
        error: {
          message: 'Secret with this name already exists',
          code: 'SECRET_ALREADY_EXISTS',
        },
      }),
    )
    const user = await openCreateModal()

    await user.type(screen.getByLabelText(/^name/i), 'existing')
    await user.type(screen.getByLabelText(/^value/i), 'hunter2')
    await user.click(screen.getByTestId('secret-form-submit'))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('SECRET_ALREADY_EXISTS')).toBeInTheDocument()
    expect(screen.getByText('Secret with this name already exists')).toBeInTheDocument()
    expect(screen.getByTestId('modal')).toBeInTheDocument()
    expect(screen.getByDisplayValue('existing')).toBeInTheDocument()
  })
})
