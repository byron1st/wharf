import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ClockContext } from '@/hooks/useNow'
import type { ServiceDeskRequest } from '@/lib/api'
import { atLocal } from '@/lib/dates'
import { RequestInspector } from './RequestInspector'

const today = '2026-11-12'
const now = atLocal(today, '14:07')

const request: ServiceDeskRequest = {
  key: 'CIS-12345',
  title: 'VPN access for a new contractor',
  status: 'In Progress',
  status_category: 'indeterminate',
  created_at: atLocal('2026-11-10', '09:00').toISOString(),
  status_at: atLocal('2026-11-11', '10:00').toISOString(),
}

/** renderInspector renders with the Atlassian settings already loaded, so the branch under test is the first render. */
function renderInspector(siteURL: string) {
  const settings = { site_url: siteURL, email: '', api_token: '' }
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) =>
      url === '/api/settings/atlassian' ? new Response(JSON.stringify(settings)) : new Response(JSON.stringify({ todos: [] })),
    ),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  qc.setQueryData(['settings', 'atlassian'], settings)
  render(
    <QueryClientProvider client={qc}>
      <ClockContext.Provider value={{ now, today }}>
        <RequestInspector request={request} tasks={new Map()} openTodos={[]} onClose={() => {}} onSelectTodo={() => {}} />
      </ClockContext.Provider>
    </QueryClientProvider>,
  )
}

afterEach(() => vi.unstubAllGlobals())

describe('RequestInspector', () => {
  it('links the request key to its Jira browse page in a new tab', () => {
    renderInspector('https://example.atlassian.net/')
    const link = screen.getByRole('link', { name: 'Open CIS-12345 in Jira (new tab)' })
    expect(link).toHaveAttribute('href', 'https://example.atlassian.net/browse/CIS-12345')
    expect(link).toHaveAttribute('target', '_blank')
    expect(link).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('shows the key as plain text when no site URL is set', () => {
    renderInspector('')
    expect(screen.getByText('CIS-12345')).toBeInTheDocument()
    expect(screen.queryByRole('link')).not.toBeInTheDocument()
  })
})
