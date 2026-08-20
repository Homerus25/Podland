import { act, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { http, HttpResponse } from 'msw'
import { afterEach, describe, expect, it } from 'vitest'
import { setBearerToken } from '@shared/utils/api'
import { loginViaForm, renderApp } from '@/test/auth'
import { server } from '@/test/server'
import type { User } from '@shared/types/api'

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

afterEach(() => {
  setBearerToken(null)
})

describe('app routing guard', () => {
  it('redirects an unauthenticated deep link to /login without rendering protected content', async () => {
    renderApp('/machines')

    expect(await screen.findByLabelText(/username/i)).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Machines' })).not.toBeInTheDocument()
    expect(screen.getByTestId('location')).toHaveTextContent('/login')
    expect(screen.getByTestId('auth-user')).toHaveTextContent('anonymous')
  })

  it('returns the user to the originally requested page after login', async () => {
    renderApp('/machines?tab=running')

    await screen.findByLabelText(/username/i)
    await loginViaForm()

    expect(await screen.findByRole('heading', { name: 'Machines' })).toBeInTheDocument()
    expect(screen.getByTestId('location')).toHaveTextContent('/machines?tab=running')
    expect(screen.getByTestId('auth-user')).toHaveTextContent('alice')
  })

  it('shows a full-page spinner while the initial auth state resolves, never the login form', async () => {
    const resolution = deferred<User | null>()
    renderApp('/machines', { resolveInitialState: () => resolution.promise })

    expect(screen.getByRole('status')).toBeInTheDocument()
    expect(screen.queryByLabelText(/username/i)).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Machines' })).not.toBeInTheDocument()

    await act(async () => {
      resolution.resolve(null)
    })

    expect(await screen.findByLabelText(/username/i)).toBeInTheDocument()
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
  })

  it('logs out: calls POST /api/auth/logout, clears auth state, and lands on /login', async () => {
    let logoutCalls = 0
    server.use(
      http.post('*/api/auth/logout', () => {
        logoutCalls++
        return HttpResponse.json({ message: 'Logged out successfully' })
      }),
    )
    renderApp('/', { resolveInitialState: async () => ({ username: 'alice' }) })

    expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
    expect(screen.getByTestId('auth-user')).toHaveTextContent('alice')

    await userEvent.click(screen.getByRole('button', { name: /log out/i }))

    expect(await screen.findByLabelText(/username/i)).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Dashboard' })).not.toBeInTheDocument()
    expect(screen.getByTestId('auth-user')).toHaveTextContent('anonymous')
    expect(logoutCalls).toBe(1)
  })

  it('still clears state and lands on /login when the revocation call fails', async () => {
    server.use(
      http.post('*/api/auth/logout', () => {
        return HttpResponse.json(
          { requestId: 'req-1', error: { message: 'session store down', code: 'INTERNAL_ERROR' } },
          { status: 500 },
        )
      }),
    )
    renderApp('/', { resolveInitialState: async () => ({ username: 'alice' }) })

    expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('button', { name: /log out/i }))

    expect(await screen.findByLabelText(/username/i)).toBeInTheDocument()
    expect(screen.getByTestId('auth-user')).toHaveTextContent('anonymous')
  })
})
