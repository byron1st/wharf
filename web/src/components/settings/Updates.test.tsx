import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ClockContext } from '@/hooks/useNow'
import type { UpdateStatus } from '@/lib/api'
import { atLocal } from '@/lib/dates'
import { Updates } from './Updates'

const today = '2026-11-09'
const now = atLocal(today, '16:00')

const status = (over: Partial<UpdateStatus> = {}): UpdateStatus => ({
  supported: true,
  current: '0.1.0',
  latest: '0.1.0',
  available: false,
  checked_at: atLocal(today, '14:07').toISOString(),
  release_url: null,
  notes: '',
  check_failed: false,
  published_at: null,
  ...over,
})

const available = (over: Partial<UpdateStatus> = {}) =>
  status({
    latest: '0.2.0',
    available: true,
    release_url: 'https://github.com/byron1st/wharf/releases/tag/v0.2.0',
    published_at: atLocal('2026-11-08', '10:00').toISOString(),
    ...over,
  })

/** Other answers a request other than GET /api/update. */
type Other = (url: string, init?: RequestInit) => Response | Promise<Response>

/** serve answers GET /api/update with update and routes every other request to other. */
function serve(update: UpdateStatus, other: Other = () => new Response(null, { status: 404 })) {
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) =>
    url === '/api/update' ? new Response(JSON.stringify(update)) : other(url, init),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderUpdates(update: UpdateStatus, other?: Other) {
  const fetchMock = serve(update, other)
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <ClockContext.Provider value={{ now, today }}>
        <Updates />
      </ClockContext.Provider>
    </QueryClientProvider>,
  )
  return fetchMock
}

afterEach(() => vi.unstubAllGlobals())

describe('Updates', () => {
  it('shows the version, a Latest badge and when it was checked', async () => {
    renderUpdates(status())

    expect(await screen.findByText('0.1.0')).toBeInTheDocument()
    expect(screen.getByText('Latest')).toBeInTheDocument()
    expect(screen.getByText('Last checked today at 14:07')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Check for updates' })).toBeEnabled()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('offers an available release with its date, notes and an update button', async () => {
    renderUpdates(available({ checked_at: atLocal('2026-11-08', '09:30').toISOString() }))

    expect(await screen.findByText('0.2.0 available')).toBeInTheDocument()
    expect(screen.queryByText('Latest')).toBeNull()
    expect(screen.getByText('Last checked Nov 8 at 09:30')).toBeInTheDocument()
    expect(screen.getByText('Wharf 0.2.0 is available')).toBeInTheDocument()
    expect(screen.getByText('· released Nov 8')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Release notes/ })).toHaveAttribute(
      'href',
      'https://github.com/byron1st/wharf/releases/tag/v0.2.0',
    )
    expect(screen.getByRole('button', { name: 'Update and restart' })).toBeEnabled()
  })

  it('says when the build never checks and offers no check', async () => {
    renderUpdates(status({ supported: false, current: '(devel)', latest: null, checked_at: null }))

    expect(await screen.findByText('(devel)')).toBeInTheDocument()
    expect(screen.getByText(/Update checks are off: .* update_check = false\./)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Check for updates' })).toBeNull()
    expect(screen.queryByText('Latest')).toBeNull()
  })

  it('says when the first check has not run yet', async () => {
    renderUpdates(status({ latest: null, checked_at: null }))

    expect(await screen.findByText('Not checked yet.')).toBeInTheDocument()
    expect(screen.queryByText('Latest')).toBeNull()
  })

  it('shows a failed check with a retry', async () => {
    renderUpdates(status({ check_failed: true }))

    expect(await screen.findByText('Last check failed')).toBeInTheDocument()
    expect(screen.getByRole('alert')).toHaveTextContent(/Couldn’t check for updates/)
    expect(screen.getByRole('button', { name: 'Retry' })).toBeEnabled()
  })

  it('checks on demand and shows what the check found', async () => {
    let answer: (r: Response) => void = () => {}
    renderUpdates(status(), (url, init) =>
      url === '/api/update/check' && init?.method === 'POST'
        ? new Promise<Response>((r) => (answer = r))
        : new Response(null, { status: 404 }),
    )

    await userEvent.click(await screen.findByRole('button', { name: 'Check for updates' }))

    expect(screen.getByRole('button', { name: 'Checking…' })).toBeDisabled()
    expect(screen.getByText('Checking GitHub releases…')).toBeInTheDocument()

    answer(new Response(JSON.stringify(available())))

    expect(await screen.findByText('Wharf 0.2.0 is available')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Check for updates' })).toBeEnabled()
  })

  it('retries a failed check and clears the error when it succeeds', async () => {
    const fetchMock = renderUpdates(status({ check_failed: true }), (url, init) =>
      url === '/api/update/check' && init?.method === 'POST'
        ? new Response(JSON.stringify(status()))
        : new Response(null, { status: 404 }),
    )

    await userEvent.click(await screen.findByRole('button', { name: 'Retry' }))

    expect(fetchMock).toHaveBeenCalledWith('/api/update/check', expect.objectContaining({ method: 'POST' }))
    expect(await screen.findByText('Last checked today at 14:07')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it('applies the update and reloads once the server is back', async () => {
    const reload = vi.fn()
    vi.stubGlobal('location', { ...window.location, reload })
    const fetchMock = renderUpdates(available(), (url, init) => {
      if (url === '/api/update/apply' && init?.method === 'POST') return new Response(null, { status: 202 })
      if (url === '/api/healthz') return new Response(JSON.stringify({ status: 'ok' }))
      return new Response(null, { status: 404 })
    })

    await userEvent.click(await screen.findByRole('button', { name: 'Update and restart' }))

    expect(fetchMock).toHaveBeenCalledWith('/api/update/apply', expect.objectContaining({ method: 'POST' }))
    expect(screen.getByRole('status')).toHaveTextContent('Updating to 0.2.0…')
    expect(screen.getByRole('button', { name: 'Check for updates' })).toBeDisabled()
    await waitFor(() => expect(reload).toHaveBeenCalled(), { timeout: 3000 })
  })

  it('offers the update again when the release fails verification', async () => {
    const fetchMock = renderUpdates(available(), (url) =>
      url === '/api/update/apply'
        ? new Response(JSON.stringify({ error: 'service: release checksum mismatch', code: 'invalid_release' }), {
            status: 502,
          })
        : new Response(null, { status: 404 }),
    )

    await userEvent.click(await screen.findByRole('button', { name: 'Update and restart' }))

    expect(await screen.findByRole('button', { name: 'Update and restart' })).toBeEnabled()
    expect(screen.queryByRole('status')).toBeNull()
    expect(fetchMock).not.toHaveBeenCalledWith('/api/healthz', expect.anything())
  })
})
