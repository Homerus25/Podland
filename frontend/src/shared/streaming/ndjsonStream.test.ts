import { describe, expect, it, vi } from 'vitest'
import { consumeNdjsonStream } from '@shared/streaming/ndjsonStream'
import { chunkedStream, streamFixtures } from '@/test/stream'

const { progressEvent, completeEvent, errorEvent } = streamFixtures

async function collect(chunks: string[]) {
  const events: unknown[] = []
  await consumeNdjsonStream(chunkedStream(chunks), (event) => events.push(event))
  return events
}

describe('consumeNdjsonStream', () => {
  it('parses each NDJSON line into an event', async () => {
    const events = await collect([
      `${JSON.stringify(progressEvent({ state: { current: 10, total: 100, status: 'pulling' } }))}\n`,
      `${JSON.stringify(completeEvent())}\n`,
    ])

    expect(events).toHaveLength(2)
    expect(events[0]).toMatchObject({ type: 'progress', state: { current: 10, total: 100 } })
    expect(events[1]).toMatchObject({ type: 'complete' })
  })

  it('reassembles events split across reads', async () => {
    const line1 = JSON.stringify(progressEvent())
    const line2 = JSON.stringify(errorEvent('OPERATION_CANCELLED', 'Cancelled'))
    const events = await collect([line1.slice(0, 20), `${line1.slice(20)}\n${line2.slice(0, 5)}`, `${line2.slice(5)}\n`])

    expect(events).toHaveLength(2)
    expect(events[0]).toMatchObject({ type: 'progress' })
    expect(events[1]).toMatchObject({ type: 'error', error: { code: 'OPERATION_CANCELLED' } })
  })

  it('logs and skips malformed lines without failing the stream', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const good = JSON.stringify(completeEvent())

    const events = await collect(['not json\n', `${good}\n`, '{broken\n'])

    expect(events).toEqual([expect.objectContaining({ type: 'complete' })])
    expect(warn).toHaveBeenCalledTimes(2)
    warn.mockRestore()
  })

  it('cancels the reader and rethrows when the stream errors (abort path)', async () => {
    const encoder = new TextEncoder()
    let errorStream: ((reason: Error) => void) | undefined
    let streamCancelled = false
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode(`${JSON.stringify(progressEvent())}\n`))
        errorStream = (reason) => controller.error(reason)
      },
      cancel() {
        streamCancelled = true
      },
    })

    const events: unknown[] = []
    const consumption = consumeNdjsonStream(stream, (event) => events.push(event))
    await new Promise((resolve) => setTimeout(resolve, 0))
    errorStream?.(new Error('aborted'))

    await expect(consumption).rejects.toThrow('aborted')
    expect(events).toHaveLength(1)
    expect(streamCancelled).toBe(false)
    // After the rejection the reader is released and the stream is cancelled,
    // so it is no longer readable or locked.
    expect(stream.locked).toBe(false)
  })
})
