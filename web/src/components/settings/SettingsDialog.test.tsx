import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ClockContext } from '@/hooks/useNow'
import { atLocal } from '@/lib/dates'
import { SettingsDialog } from './SettingsDialog'

const today = '2026-10-26'
const now = atLocal(today, '14:30')

const lastSync = {
  synced_at: atLocal(today, '14:02').toISOString(),
  issues: 6,
  issue_failures: 0,
  open_requests: 4,
  closed_requests: 0,
  request_failures: 0,
}

/** serve answers the Atlassian tab's requests with the stored interval; save answers its PUT. */
function serve(interval: number, save: (body: unknown) => Response = () => new Response(null, { status: 204 })) {
  const saved: unknown[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url === '/api/settings/atlassian/sync') {
        if (init?.method === 'PUT') {
          const body = JSON.parse(String(init.body)) as unknown
          saved.push(body)
          const res = save(body)
          if (res.ok) interval = (body as { interval_minutes: number }).interval_minutes
          return res
        }
        return new Response(JSON.stringify({ interval_minutes: interval }))
      }
      if (url === '/api/settings/atlassian') {
        return new Response(JSON.stringify({ site_url: 'https://example.atlassian.net', email: 'me@example.com', api_token: 't' }))
      }
      if (url === '/api/sync/atlassian') return new Response(JSON.stringify({ last_sync: lastSync }))
      return new Response(null, { status: 404 })
    }),
  )
  return saved
}

function renderAtlassian() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <ClockContext.Provider value={{ now, today }}>
        <SettingsDialog tab="atlassian" onTab={() => {}} onClose={() => {}} />
      </ClockContext.Provider>
    </QueryClientProvider>,
  )
}

const radio = (name: string) => screen.getByRole('radio', { name })

afterEach(() => vi.unstubAllGlobals())

describe('Settings › Atlassian Cloud › Sync automatically', () => {
  it('offers the seven intervals with the stored one checked', async () => {
    serve(60)
    renderAtlassian()

    const group = await screen.findByRole('radiogroup', { name: 'Sync automatically' })
    expect([...group.querySelectorAll('[role="radio"]')].map((r) => r.textContent)).toEqual([
      'Off',
      '30 min',
      '1 h',
      '3 h',
      '6 h',
      '12 h',
      '24 h',
    ])
    expect(radio('1 h')).toHaveAttribute('aria-checked', 'true')
    expect(radio('Off')).toHaveAttribute('aria-checked', 'false')
    expect(screen.getByText('Runs every hour, and whenever you start it')).toBeInTheDocument()
    expect(
      screen.getByText('Next automatic sync today 15:02 · counted from the last sync, including manual ones.'),
    ).toBeInTheDocument()
  })

  it('saves a picked interval at once and counts the next sync from the last one', async () => {
    const saved = serve(60)
    renderAtlassian()

    await userEvent.click(await screen.findByRole('radio', { name: '12 h' }))

    expect(saved).toEqual([{ interval_minutes: 720 }])
    await waitFor(() => expect(radio('12 h')).toHaveAttribute('aria-checked', 'true'))
    expect(screen.getByText('Runs every 12 hours, and whenever you start it')).toBeInTheDocument()
    expect(
      screen.getByText('Next automatic sync tomorrow 02:02 · counted from the last sync, including manual ones.'),
    ).toBeInTheDocument()
  })

  it('says syncs only run when started while automatic syncs are off', async () => {
    const saved = serve(30)
    renderAtlassian()

    await userEvent.click(await screen.findByRole('radio', { name: 'Off' }))

    expect(saved).toEqual([{ interval_minutes: 0 }])
    await waitFor(() => expect(radio('Off')).toHaveAttribute('aria-checked', 'true'))
    expect(screen.getByText('Runs only when you start it')).toBeInTheDocument()
    expect(
      screen.getByText('Off — sync only runs from Sync now or the sync button in the Today header.'),
    ).toBeInTheDocument()
  })

  it('goes back to the stored interval when the save fails', async () => {
    serve(60, () => new Response(JSON.stringify({ error: 'service: invalid atlassian sync interval' }), { status: 400 }))
    renderAtlassian()

    await userEvent.click(await screen.findByRole('radio', { name: '3 h' }))

    await waitFor(() => expect(radio('1 h')).toHaveAttribute('aria-checked', 'true'))
    expect(radio('3 h')).toHaveAttribute('aria-checked', 'false')
  })
})
