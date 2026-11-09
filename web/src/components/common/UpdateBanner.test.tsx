import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { UpdateStatus } from '@/lib/api'
import { UpdateBanner } from './UpdateBanner'

const status = (over: Partial<UpdateStatus> = {}): UpdateStatus => ({
  supported: true,
  current: '0.1.0',
  latest: '0.2.0',
  available: true,
  checked_at: '2026-11-09T07:00:00Z',
  release_url: 'https://github.com/byron1st/wharf/releases/tag/v0.2.0',
  notes: '',
  ...over,
})

/** serve answers GET /api/update with update and routes every other request to other. */
function serve(update: UpdateStatus, other: (url: string, init?: RequestInit) => Response = () => new Response(null, { status: 404 })) {
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) =>
    url === '/api/update' ? new Response(JSON.stringify(update)) : other(url, init),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

function renderBanner() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <UpdateBanner />
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('UpdateBanner', () => {
  it('offers a newer release with its notes', async () => {
    serve(status())
    renderBanner()

    expect(await screen.findByText(/is available\. You have 0\.1\.0\./)).toBeInTheDocument()
    expect(screen.getByText('0.2.0')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Release notes/ })).toHaveAttribute(
      'href',
      'https://github.com/byron1st/wharf/releases/tag/v0.2.0',
    )
    expect(screen.getByRole('button', { name: 'Upgrade' })).toBeEnabled()
  })

  it.each([
    ['up to date', status({ available: false, latest: '0.1.0' })],
    ['unsupported', status({ supported: false, available: false, latest: null, checked_at: null, release_url: null })],
  ])('stays hidden when %s', async (_, update) => {
    const fetchMock = serve(update)
    renderBanner()

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith('/api/update', expect.anything()))
    expect(screen.queryByRole('status')).toBeNull()
  })

  it('applies the update and reloads once the server is back', async () => {
    const reload = vi.fn()
    vi.stubGlobal('location', { ...window.location, reload })
    const fetchMock = serve(status(), (url, init) => {
      if (url === '/api/update/apply' && init?.method === 'POST') return new Response(null, { status: 202 })
      if (url === '/api/healthz') return new Response(JSON.stringify({ status: 'ok' }))
      return new Response(null, { status: 404 })
    })
    renderBanner()

    await userEvent.click(await screen.findByRole('button', { name: 'Upgrade' }))

    expect(fetchMock).toHaveBeenCalledWith('/api/update/apply', expect.objectContaining({ method: 'POST' }))
    expect(screen.getByRole('button', { name: 'Upgrading…' })).toBeDisabled()
    await waitFor(() => expect(reload).toHaveBeenCalled(), { timeout: 3000 })
  })

  it('re-enables the button when the release fails verification', async () => {
    const fetchMock = serve(status(), (url) =>
      url === '/api/update/apply'
        ? new Response(JSON.stringify({ error: 'service: release checksum mismatch', code: 'invalid_release' }), {
            status: 502,
          })
        : new Response(null, { status: 404 }),
    )
    renderBanner()

    await userEvent.click(await screen.findByRole('button', { name: 'Upgrade' }))

    expect(await screen.findByRole('button', { name: 'Upgrade' })).toBeEnabled()
    expect(fetchMock).not.toHaveBeenCalledWith('/api/healthz', expect.anything())
  })
})
