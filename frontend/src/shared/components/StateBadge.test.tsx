import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { StateBadge, stateColorScheme } from '@shared/components/StateBadge'

describe('stateColorScheme', () => {
  it('maps running to green', () => {
    expect(stateColorScheme('running')).toBe('green')
  })

  it('maps exited to gray', () => {
    expect(stateColorScheme('exited')).toBe('gray')
  })

  it('maps paused to yellow', () => {
    expect(stateColorScheme('paused')).toBe('yellow')
  })

  it('maps dead to red', () => {
    expect(stateColorScheme('dead')).toBe('red')
  })

  it('maps created to blue', () => {
    expect(stateColorScheme('created')).toBe('blue')
  })

  it('maps restarting to orange', () => {
    expect(stateColorScheme('restarting')).toBe('orange')
  })

  it('falls back to a neutral scheme for unrecognized states', () => {
    expect(stateColorScheme('configured')).toBe('gray')
    expect(stateColorScheme('whatever')).toBe('gray')
  })

  it('matches states case-insensitively', () => {
    expect(stateColorScheme('Running')).toBe('green')
  })
})

describe('StateBadge', () => {
  it('renders the state text as a badge', () => {
    render(<StateBadge state="running" />)

    const badge = screen.getByTestId('state-badge')
    expect(badge).toHaveTextContent('running')
    expect(badge.className).toContain('chakra-badge')
  })
})
