import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ChakraProvider } from '@chakra-ui/react'
import { MemoryRouter } from 'react-router-dom'
import { http, HttpResponse } from 'msw'
import { afterEach, describe, expect, it } from 'vitest'
import { App } from '@/App'
import { theme } from '@/theme'
import { AuthProvider } from '@shared/context/AuthContext'
import { api, setBearerToken } from '@shared/utils/api'
import { useAuth } from '@features/auth/hooks/useAuth'
import { server } from '@/test/server'

function UsernameProbe() {
  const { user } = useAuth()
  return <div data-testid="auth-user">{user?.username ?? 'anonymous'}</div>
}

function renderApp(at = '/login') {
  return render(
    <ChakraProvider theme={theme}>
      <MemoryRouter initialEntries={[at]}>
        <AuthProvider>
          <UsernameProbe />
          <App />
        </AuthProvider>
      </MemoryRouter>
    </ChakraProvider>,
  )
}

const loginSuccessHandler = http.post('*/api/auth/login', () => {
  return HttpResponse.json({ token: 'test-access-token' })
})

// Mirrors the backend error envelope (backend/internal/errors/handlers.go)
function loginFailureHandler(status: number, code: string, message: string) {
  return http.post('*/api/auth/login', () => {
    return HttpResponse.json(
      { requestId: 'req-1', error: { message, code } },
      { status },
    )
  })
}

async function loginViaForm(username = 'alice', password = 'wonderland') {
  await userEvent.type(screen.getByLabelText(/username/i), username)
  await userEvent.type(screen.getByLabelText(/password/i), password)
  await userEvent.click(screen.getByRole('button', { name: /log in/i }))
}

afterEach(() => {
  setBearerToken(null)
})

describe('LoginPage', () => {
  it('logs the user in and lands on the placeholder dashboard', async () => {
    server.use(loginSuccessHandler)
    renderApp('/login')

    await loginViaForm()

    await waitFor(() => {
      expect(screen.getByText('Manage Podman on remote machines')).toBeInTheDocument()
    })
  })

  it('stores username and memory-only token, and sends the Bearer header on API requests', async () => {
    let authHeader: string | null = null
    server.use(
      loginSuccessHandler,
      http.get('*/api/machines', ({ request }) => {
        authHeader = request.headers.get('authorization')
        return HttpResponse.json([])
      }),
    )
    renderApp('/login')

    await loginViaForm()
    await waitFor(() => {
      expect(screen.getByTestId('auth-user')).toHaveTextContent('alice')
    })

    await api.get('/machines')

    expect(authHeader).toBe('Bearer test-access-token')

    // Color-mode preference may live in storage, but no auth material may.
    const storedValues = [...Object.values(localStorage), ...Object.values(sessionStorage)]
    expect(storedValues.some((value) => value.includes('test-access-token'))).toBe(false)
    expect(storedValues.some((value) => value.includes('alice'))).toBe(false)
  })

  it('shows the error envelope inline when credentials are rejected', async () => {
    server.use(loginFailureHandler(401, 'INVALID_CREDENTIALS', 'Invalid username or password'))
    renderApp('/login')

    await loginViaForm()

    expect(await screen.findByText('Invalid username or password')).toBeInTheDocument()
    expect(screen.getByText('INVALID_CREDENTIALS')).toBeInTheDocument()
    expect(screen.queryByText('Manage Podman on remote machines')).not.toBeInTheDocument()
  })

  it('substitutes generic text for internal errors, hiding leaked details', async () => {
    server.use(loginFailureHandler(500, 'PANIC', 'database is on fire'))
    renderApp('/login')

    await loginViaForm()

    expect(await screen.findByText('An unexpected error occurred')).toBeInTheDocument()
    expect(screen.getByText('INTERNAL_ERROR')).toBeInTheDocument()
    expect(screen.queryByText('database is on fire')).not.toBeInTheDocument()
  })

  it('shows inline validation errors and never submits when fields are empty', async () => {
    renderApp('/login')

    await userEvent.click(screen.getByRole('button', { name: /log in/i }))

    expect(await screen.findByText('Username is required')).toBeInTheDocument()
    expect(screen.getByText('Password is required')).toBeInTheDocument()
    expect(screen.queryByText('Manage Podman on remote machines')).not.toBeInTheDocument()
  })

  it('treats whitespace-only credentials as empty', async () => {
    renderApp('/login')

    await loginViaForm('   ', '   ')

    expect(await screen.findByText('Username is required')).toBeInTheDocument()
    expect(screen.getByText('Password is required')).toBeInTheDocument()
  })
})
