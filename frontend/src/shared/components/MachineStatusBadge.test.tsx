import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { ChakraProvider } from '@chakra-ui/react'
import { theme } from '@/theme'
import {
  MachineStatusBadge,
  machineStatusColorScheme,
} from '@shared/components/MachineStatusBadge'

function renderBadge(props: Parameters<typeof MachineStatusBadge>[0]) {
  return render(
    <ChakraProvider theme={theme}>
      <MachineStatusBadge {...props} />
    </ChakraProvider>,
  )
}

describe('MachineStatusBadge', () => {
  it.each([
    ['online', 'green'],
    ['offline', 'red'],
    ['checking', 'gray'],
    ['unknown', 'gray'],
  ])('maps %s to %s', (status, scheme) => {
    expect(machineStatusColorScheme(status)).toBe(scheme)
  })

  it('renders online with its label', () => {
    renderBadge({ status: 'online' })

    const badge = screen.getByTestId('machine-status-badge')
    expect(badge).toHaveTextContent('online')
    expect(badge.className).toContain('chakra-badge')
  })

  it('renders offline with its label', () => {
    renderBadge({ status: 'offline' })

    expect(screen.getByTestId('machine-status-badge')).toHaveTextContent('offline')
  })

  it('renders checking with its label', () => {
    renderBadge({ status: 'checking' })

    expect(screen.getByTestId('machine-status-badge')).toHaveTextContent('checking')
  })

  it('renders unknown with its label', () => {
    renderBadge({ status: 'unknown' })

    expect(screen.getByTestId('machine-status-badge')).toHaveTextContent('unknown')
  })

  it('always shows the text label next to the colored dot', () => {
    renderBadge({ status: 'online' })

    expect(screen.getByTestId('machine-status-badge-dot')).toBeInTheDocument()
    expect(screen.getByTestId('machine-status-badge-label')).toHaveTextContent('online')
  })

  it('exposes checkedAt as a tooltip', async () => {
    renderBadge({ status: 'online', checkedAt: '2026-08-29T10:00:00Z' })

    fireEvent.focus(screen.getByTestId('machine-status-badge'))
    expect(await screen.findByRole('tooltip')).toHaveTextContent(
      'Checked at 2026-08-29T10:00:00Z',
    )
    expect(screen.getByTestId('machine-status-badge')).toHaveAttribute(
      'aria-label',
      'Checked at 2026-08-29T10:00:00Z',
    )
  })

  it('has no tooltip without checkedAt', () => {
    renderBadge({ status: 'checking' })

    expect(screen.getByTestId('machine-status-badge')).not.toHaveAttribute('aria-label')
  })
})
