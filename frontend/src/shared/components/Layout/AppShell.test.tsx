import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'
import { setBearerToken } from '@shared/utils/api'
import { renderApp } from '@/test/auth'

function renderShell(at = '/', username = 'alice') {
  return renderApp(at, {
    resolveInitialState: async () => ({ username }),
    usernameProbe: false,
  })
}

afterEach(() => {
  setBearerToken(null)
})

const prefersDark = (query: string) => query.includes('dark')

function mockSystemPreference(dark: boolean) {
  window.matchMedia = (query: string) => ({
    matches: dark && prefersDark(query),
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  })
}

describe('app shell', () => {
  it('renders the header with username and sidebar navigation around page content', async () => {
    renderShell('/')

    expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
    expect(screen.getByText('alice')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Dashboard' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Machines' })).toBeInTheDocument()

    await userEvent.click(screen.getByRole('link', { name: 'Machines' }))

    expect(await screen.findByRole('heading', { name: 'Machines' })).toBeInTheDocument()
  })
})

describe('color mode', () => {
  const originalMatchMedia = window.matchMedia

  afterEach(() => {
    window.matchMedia = originalMatchMedia
    localStorage.clear()
    document.body.classList.remove('chakra-ui-light', 'chakra-ui-dark')
  })

  it('flips light/dark on toggle and persists the choice in localStorage', async () => {
    localStorage.clear()
    mockSystemPreference(false)
    renderShell('/')

    expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
    expect(document.body.classList.contains('chakra-ui-light')).toBe(true)
    expect(localStorage.getItem('chakra-ui-color-mode')).toBe('light')

    await userEvent.click(screen.getByRole('button', { name: 'Switch to dark mode' }))

    expect(document.body.classList.contains('chakra-ui-dark')).toBe(true)
    expect(document.documentElement.dataset.theme).toBe('dark')
    expect(localStorage.getItem('chakra-ui-color-mode')).toBe('dark')
    expect(screen.getByRole('button', { name: 'Switch to light mode' })).toBeInTheDocument()
  })

  it('defaults to the system color preference when no choice is stored', async () => {
    localStorage.clear()
    mockSystemPreference(true)
    renderShell('/')

    expect(await screen.findByRole('heading', { name: 'Dashboard' })).toBeInTheDocument()
    expect(document.body.classList.contains('chakra-ui-dark')).toBe(true)
    expect(document.documentElement.dataset.theme).toBe('dark')
  })
})
