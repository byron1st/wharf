import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LAST_SYNC_POLL_MS, keys, useTasks } from './data'
import { SyncProvider } from './useSync'

const record = (synced_at: string) => ({
  synced_at,
  issues: 1,
  issue_failures: 0,
  open_requests: 0,
  closed_requests: 0,
  request_failures: 0,
})

function Tasks() {
  useTasks()
  return null
}

/** renderSync serves the last sync from lastSyncs, one per read, and counts the task list reads. */
function renderSync(lastSyncs: string[]) {
  let reads = 0
  const taskReads = { count: 0 }
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url === '/api/sync/atlassian') {
        const at = lastSyncs[Math.min(reads++, lastSyncs.length - 1)]
        return new Response(JSON.stringify({ last_sync: record(at) }))
      }
      if (url === '/api/tasks') {
        taskReads.count++
        return new Response(JSON.stringify({ tasks: [] }))
      }
      return new Response(JSON.stringify({ site_url: '', email: '', api_token: '' }))
    }),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <SyncProvider>
          <Tasks />
        </SyncProvider>
      </MemoryRouter>
    </QueryClientProvider>,
  )
  return { qc, taskReads }
}

afterEach(() => vi.unstubAllGlobals())

describe('SyncProvider', () => {
  it('re-reads the last sync every minute', async () => {
    const { qc } = renderSync(['2026-10-26T05:02:00Z'])

    await waitFor(() => expect(qc.getQueryData(keys.lastSync)).toBeDefined())
    const query = qc.getQueryCache().find({ queryKey: keys.lastSync })!
    expect(LAST_SYNC_POLL_MS).toBe(60_000)
    expect(query.observers[0].options).toMatchObject({ refetchInterval: LAST_SYNC_POLL_MS })
  })

  it('refetches the other data when a sync it did not run is recorded', async () => {
    const { qc, taskReads } = renderSync(['2026-10-26T05:02:00Z', '2026-10-26T06:02:00Z'])
    await waitFor(() => expect(qc.getQueryData(keys.lastSync)).toBeDefined())
    await waitFor(() => expect(taskReads.count).toBe(1))

    await qc.refetchQueries({ queryKey: keys.lastSync })

    await waitFor(() => expect(taskReads.count).toBe(2))
  })

  it('leaves the other data alone while the last sync stays the same', async () => {
    const { qc, taskReads } = renderSync(['2026-10-26T05:02:00Z'])
    await waitFor(() => expect(qc.getQueryData(keys.lastSync)).toBeDefined())
    await waitFor(() => expect(taskReads.count).toBe(1))

    await qc.refetchQueries({ queryKey: keys.lastSync })
    await new Promise((r) => setTimeout(r, 20))

    expect(taskReads.count).toBe(1)
  })
})
