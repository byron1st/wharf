import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
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
  ...over,
})

function renderUpdates(update: UpdateStatus) {
  vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify(update))))
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <ClockContext.Provider value={{ now, today }}>
        <Updates />
      </ClockContext.Provider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('Updates', () => {
  it('shows the version and when it was checked', async () => {
    renderUpdates(status())

    expect(await screen.findByText('0.1.0')).toBeInTheDocument()
    expect(screen.getByText(/Up to date\. Checked 14:07\./)).toBeInTheDocument()
  })

  it('names an available release and the day of an older check', async () => {
    renderUpdates(status({ latest: '0.2.0', available: true, checked_at: atLocal('2026-11-08', '09:30').toISOString() }))

    expect(await screen.findByText(/Wharf 0\.2\.0 is available\. Checked 2026-11-08 09:30\./)).toBeInTheDocument()
  })

  it('says when the build never checks', async () => {
    renderUpdates(status({ supported: false, current: '(devel)', latest: null, checked_at: null }))

    expect(await screen.findByText('(devel)')).toBeInTheDocument()
    expect(screen.getByText(/Update checks are off: .* update_check = false\./)).toBeInTheDocument()
  })

  it('says when the first check has not run yet', async () => {
    renderUpdates(status({ latest: null, checked_at: null }))

    expect(await screen.findByText(/Not checked yet\./)).toBeInTheDocument()
  })
})
