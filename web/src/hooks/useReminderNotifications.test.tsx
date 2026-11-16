import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
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

function Probe({ now, onOpen }: { now: Date; onOpen: (taskID: string) => void }) {
  useReminderNotifications(now, onOpen)
  return null
}

/** instances are the notifications the latest stubNotification constructed. */
let instances: { onclick: (() => void) | null }[] = []

/** stubNotification replaces the browser API with a recorder granted the given permission. */
function stubNotification(permission: NotificationPermission) {
  const shown: { title: string; options?: NotificationOptions }[] = []
  instances = []
  class FakeNotification {
    static permission = permission
    static requestPermission = vi.fn()
    onclick: (() => void) | null = null
    constructor(title: string, options?: NotificationOptions) {
      shown.push({ title, options })
      instances.push(this)
    }
    close() {}
  }
  vi.stubGlobal('Notification', FakeNotification)
  return shown
}

function setup(fetchTodos: () => Promise<Response> = async () => new Response(JSON.stringify({ todos: [todo] }))) {
  vi.stubGlobal('fetch', vi.fn(fetchTodos))
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const onOpen = vi.fn()
  const wrap = (time: string): ReactNode => (
    <QueryClientProvider client={qc}>
      <Probe now={atLocal(today, time)} onOpen={onOpen} />
    </QueryClientProvider>
  )
  const view = render(wrap('14:07'))
  return { tick: (time: string) => view.rerender(wrap(time)), qc, onOpen }
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

  it('skips reminders missed for longer than the catch-up window', async () => {
    const shown = stubNotification('granted')
    const { tick, qc } = setup()
    await waitFor(() => expect(qc.getQueryData(['todos', { is_done: false }])).toBeDefined())
    // Waking at 15:10 after sleeping since 14:07: 14:08 is too late, 15:00 is too.
    tick('15:10')
    expect(shown).toEqual([])
  })

  it('still notifies a reminder a few minutes late', async () => {
    const shown = stubNotification('granted')
    const { tick, qc } = setup()
    await waitFor(() => expect(qc.getQueryData(['todos', { is_done: false }])).toBeDefined())
    tick('14:12')
    expect(shown.map((n) => n.options?.tag)).toEqual([`wharf-reminder-7-${todo.reminders[0]}`])
  })

  it('polls open todos so reminders changed on another device apply', async () => {
    stubNotification('granted')
    const { qc } = setup()
    await waitFor(() => expect(qc.getQueryData(['todos', { is_done: false }])).toBeDefined())
    const query = qc.getQueryCache().find({ queryKey: ['todos', { is_done: false }] })!
    expect(query.observers[0].options).toMatchObject({ refetchInterval: 60_000, refetchIntervalInBackground: true })
  })

  it('opens the task when the notification is clicked', async () => {
    stubNotification('granted')
    const { tick, qc, onOpen } = setup()
    await waitFor(() => expect(qc.getQueryData(['todos', { is_done: false }])).toBeDefined())
    tick('14:08')
    instances[0].onclick?.()
    expect(onOpen).toHaveBeenCalledWith('local-WHF-003')
  })

  it('notifies a reminder that went off while the todos were still loading', async () => {
    const shown = stubNotification('granted')
    let respond: (r: Response) => void = () => {}
    const { tick, qc } = setup(() => new Promise((resolve) => (respond = resolve)))
    tick('14:08')
    expect(shown).toEqual([])
    respond(new Response(JSON.stringify({ todos: [todo] })))
    await waitFor(() => expect(qc.getQueryData(['todos', { is_done: false }])).toBeDefined())
    await waitFor(() => expect(shown).toHaveLength(1))
  })

  it('keeps the app running when the browser refuses to construct a notification', async () => {
    vi.stubGlobal(
      'Notification',
      class {
        static permission = 'granted'
        constructor() {
          throw new TypeError('Illegal constructor')
        }
      },
    )
    const { tick, qc } = setup()
    await waitFor(() => expect(qc.getQueryData(['todos', { is_done: false }])).toBeDefined())
    expect(() => tick('14:08')).not.toThrow()
  })

  it('stays silent without permission', async () => {
    const shown = stubNotification('denied')
    const { tick, qc } = setup()
    await waitFor(() => expect(qc.getQueryData(['todos', { is_done: false }])).toBeDefined())
    tick('15:00')
    expect(shown).toEqual([])
  })
})
