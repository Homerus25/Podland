import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { LogViewer, type LogSource } from '@shared/components/LogViewer'

function fakeSource(overrides: Partial<LogSource> = {}): LogSource {
  return {
    fetchTail: vi.fn().mockResolvedValue(['line-1', 'line-2']),
    follow: vi.fn(),
    ...overrides,
  }
}

function renderViewer(source: LogSource) {
  return render(<LogViewer source={source} />)
}

describe('LogViewer', () => {
  it('fetches the last 100 lines by default and renders them', async () => {
    const source = fakeSource()
    renderViewer(source)

    expect(await screen.findAllByTestId('log-viewer-line')).toHaveLength(2)
    expect(screen.getByText('line-1')).toBeInTheDocument()
    expect(source.fetchTail).toHaveBeenCalledWith(100)
  })

  it('refetches with the selected tail when the selector changes', async () => {
    const source = fakeSource()
    const user = userEvent.setup()
    renderViewer(source)
    await screen.findAllByTestId('log-viewer-line')

    await user.selectOptions(screen.getByTestId('log-tail-select'), '50')

    await waitFor(() => expect(source.fetchTail).toHaveBeenLastCalledWith(50))
  })

  it('follow toggle defaults to off and enabling it opens the stream', async () => {
    const source = fakeSource()
    const user = userEvent.setup()
    renderViewer(source)
    await screen.findAllByTestId('log-viewer-line')

    const toggle = screen.getByTestId('log-follow-toggle')
    expect(toggle).not.toBeChecked()
    await user.click(toggle)

    await waitFor(() => expect(source.follow).toHaveBeenCalled())
    const [tail, , signal] = vi.mocked(source.follow).mock.calls[0]
    expect(tail).toBe(100)
    expect(signal).toBeInstanceOf(AbortSignal)
  })

  it('appends streamed lines live while following', async () => {
    const source = fakeSource({
      follow: vi.fn((_tail, onEvent: (event: { timestamp: string; level: string; message: string }) => void) => {
        onEvent({ timestamp: '2026-08-26T10:00:01Z', level: 'INFO', message: 'streamed-line' })
        return Promise.resolve()
      }),
    })
    const user = userEvent.setup()
    renderViewer(source)
    await screen.findAllByTestId('log-viewer-line')

    await user.click(screen.getByTestId('log-follow-toggle'))

    expect(await screen.findByText(/streamed-line/)).toBeInTheDocument()
  })

  it('auto-scrolls to the newest line while following', async () => {
    const source = fakeSource({
      follow: vi.fn((_tail, onEvent: (event: { timestamp: string; level: string; message: string }) => void) => {
        onEvent({ timestamp: '2026-08-26T10:00:01Z', level: 'INFO', message: 'newest' })
        return Promise.resolve()
      }),
    })
    const user = userEvent.setup()
    renderViewer(source)
    await screen.findAllByTestId('log-viewer-line')
    const scrollBox = screen.getByTestId('log-viewer-scroll')
    const scrollTopSpy = vi.spyOn(scrollBox, 'scrollTop', 'set')

    await user.click(screen.getByTestId('log-follow-toggle'))

    await waitFor(() => expect(scrollTopSpy).toHaveBeenCalled())
  })

  it('aborts the stream when follow is toggled off', async () => {
    let aborted = false
    const source = fakeSource({
      follow: vi.fn(
        (_tail: number, _onEvent: unknown, signal: AbortSignal) =>
          new Promise<void>((_resolve, reject) => {
            signal.addEventListener('abort', () => {
              aborted = true
              reject(new DOMException('Aborted', 'AbortError'))
            })
          }),
      ),
    })
    const user = userEvent.setup()
    renderViewer(source)
    await screen.findAllByTestId('log-viewer-line')
    await user.click(screen.getByTestId('log-follow-toggle'))
    await waitFor(() => expect(source.follow).toHaveBeenCalled())

    await user.click(screen.getByTestId('log-follow-toggle'))

    await waitFor(() => expect(aborted).toBe(true))
  })

  it('aborts the stream on unmount', async () => {
    let aborted = false
    const source = fakeSource({
      follow: vi.fn(
        (_tail: number, _onEvent: unknown, signal: AbortSignal) =>
          new Promise<void>((_resolve, reject) => {
            signal.addEventListener('abort', () => {
              aborted = true
              reject(new DOMException('Aborted', 'AbortError'))
            })
          }),
      ),
    })
    const user = userEvent.setup()
    const view = renderViewer(source)
    await screen.findAllByTestId('log-viewer-line')
    await user.click(screen.getByTestId('log-follow-toggle'))
    await waitFor(() => expect(source.follow).toHaveBeenCalled())

    view.unmount()

    await waitFor(() => expect(aborted).toBe(true))
  })

  it('surfaces an error event as code and message', async () => {
    const source = fakeSource({
      follow: vi.fn((_tail, onEvent: (event: { timestamp: string; level: string; message: string }) => void) => {
        onEvent({
          timestamp: '2026-08-26T10:00:00Z',
          level: 'ERROR',
          message: 'CONTAINER_NOT_FOUND: no such container',
        })
        return Promise.resolve()
      }),
    })
    const user = userEvent.setup()
    renderViewer(source)
    await screen.findAllByTestId('log-viewer-line')

    await user.click(screen.getByTestId('log-follow-toggle'))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('CONTAINER_NOT_FOUND')).toBeInTheDocument()
    expect(screen.getByText('no such container')).toBeInTheDocument()
  })

  it('does not duplicate lines when the follow stream replays the tail', async () => {
    const source = fakeSource({
      fetchTail: vi.fn().mockResolvedValue(['hist-line']),
      follow: vi.fn((_tail, onEvent: (event: { timestamp: string; level: string; message: string }) => void) => {
        onEvent({ timestamp: '2026-08-26T10:00:00Z', level: 'INFO', message: 'hist-line' })
        onEvent({ timestamp: '2026-08-26T10:00:01Z', level: 'INFO', message: 'new-line' })
        return Promise.resolve()
      }),
    })
    const user = userEvent.setup()
    renderViewer(source)
    expect(await screen.findByText('hist-line')).toBeInTheDocument()

    await user.click(screen.getByTestId('log-follow-toggle'))

    await waitFor(() => expect(screen.getByText(/new-line/)).toBeInTheDocument())
    expect(screen.getAllByText(/hist-line/)).toHaveLength(1)
  })

  it('restarts the stream with the new tail when the selector changes while following', async () => {
    const source = fakeSource({
      follow: vi.fn(
        (_tail: number, _onEvent: unknown, signal: AbortSignal) =>
          new Promise<void>((_resolve, reject) => {
            signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
          }),
      ),
    })
    const user = userEvent.setup()
    renderViewer(source)
    await screen.findAllByTestId('log-viewer-line')
    await user.click(screen.getByTestId('log-follow-toggle'))
    await waitFor(() => expect(source.follow).toHaveBeenCalledTimes(1))

    await user.selectOptions(screen.getByTestId('log-tail-select'), '500')

    await waitFor(() => expect(source.follow).toHaveBeenCalledTimes(2))
    expect(vi.mocked(source.follow).mock.calls[1][0]).toBe(500)
  })

  it('preserves the envelope when the follow request itself fails', async () => {
    const source = fakeSource({
      follow: vi.fn(() => Promise.reject({ code: 'MACHINE_UNREACHABLE', message: 'ssh failed' })),
    })
    const user = userEvent.setup()
    renderViewer(source)
    await screen.findAllByTestId('log-viewer-line')

    await user.click(screen.getByTestId('log-follow-toggle'))

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('MACHINE_UNREACHABLE')).toBeInTheDocument()
    expect(screen.getByText('ssh failed')).toBeInTheDocument()
  })

  it('shows the fetch error when the initial tail fetch fails', async () => {
    const source = fakeSource({
      fetchTail: vi.fn().mockRejectedValue({ code: 'MACHINE_UNREACHABLE', message: 'ssh failed' }),
    })
    renderViewer(source)

    expect(await screen.findByRole('alert')).toBeInTheDocument()
    expect(screen.getByText('MACHINE_UNREACHABLE')).toBeInTheDocument()
  })
})
