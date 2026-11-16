import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ClockContext } from '@/hooks/useNow'
import type { Todo } from '@/lib/api'
import { atLocal } from '@/lib/dates'
import { useReminderNotifications } from './useReminderNotifications'

const today = '2026-10-26'

const todo: Todo = {
  id: 7,
  task_id: 'local-WHF-003',
  description: 'Call the vendor',
  note: '',
  is_done: false,
  completed_at: null,
  start_date: null,
  end_date: null,
  reminders: [atLocal(today, '14:08').toISOString(), atLocal(today, '15:00').toISOString()],
  position: null,
  service_desk_requests: [],
}

function Probe() {
  useReminderNotifications()
  return null
}

/** stubNotification replaces the browser API with a recorder granted the given permission. */
function stubNotification(permission: NotificationPermission) {
  const shown: { title: string; options?: NotificationOptions }[] = []
  class FakeNotification {
    static permission = permission
    static requestPermission = vi.fn()
    onclick: (() => void) | null = null
    constructor(title: string, options?: NotificationOptions) {
      shown.push({ title, options })
    }
    close() {}
  }
  vi.stubGlobal('Notification', FakeNotification)
  return shown
}

function setup() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ todos: [todo] }))),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const wrap = (time: string): ReactNode => (
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <ClockContext.Provider value={{ now: atLocal(today, time), today }}>
          <Probe />
        </ClockContext.Provider>
      </MemoryRouter>
    </QueryClientProvider>
  )
  const view = render(wrap('14:07'))
  return { tick: (time: string) => view.rerender(wrap(time)), qc }
}

afterEach(() => vi.unstubAllGlobals())

describe('useReminderNotifications', () => {
  it('notifies once when a reminder goes off', async () => {
    const shown = stubNotification('granted')
    const { tick, qc } = setup()
    await waitFor(() => expect(qc.getQueryData(['todos', { is_done: false }])).toBeDefined())
    expect(shown).toEqual([])

    tick('14:08')
    expect(shown).toEqual([
      {
        title: 'Call the vendor',
        options: { body: 'WHF-003 · Mon, Oct 26 · 14:08', tag: `wharf-reminder-7-${todo.reminders[0]}` },
      },
    ])

    tick('14:09')
    expect(shown).toHaveLength(1)
  })

  it('stays silent without permission', async () => {
    const shown = stubNotification('denied')
    const { tick, qc } = setup()
    await waitFor(() => expect(qc.getQueryData(['todos', { is_done: false }])).toBeDefined())
    tick('15:00')
    expect(shown).toEqual([])
  })
})
