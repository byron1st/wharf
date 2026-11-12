import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { TaskDetail, TaskListItem, Todo } from '@/lib/api'
import { keys, useTaskActions, useTodoActions } from './data'

const todo: Todo = {
  id: 7,
  task_id: 'local-WHF-001',
  description: 'write the plan',
  note: '',
  is_done: false,
  completed_at: null,
  start_date: null,
  end_date: null,
  reminders: [],
  position: 0,
  service_desk_requests: [],
}

afterEach(() => vi.unstubAllGlobals())

describe('useTodoActions().update', () => {
  it('sends saves of one todo in order, each with the fields of the earlier ones', async () => {
    const bodies: Record<string, unknown>[] = []
    let release!: () => void
    const firstHeld = new Promise<void>((r) => (release = r))
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          bodies.push(JSON.parse(String(init.body)) as Record<string, unknown>)
          if (bodies.length === 1) await firstHeld
          return new Response(null, { status: 204 })
        }
        return new Response(JSON.stringify(url.startsWith('/api/todos') ? { todos: [todo] } : { tasks: [] }))
      }),
    )
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    qc.setQueryData(keys.todos({ is_done: false }), [todo])
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    const { result } = renderHook(() => useTodoActions(), { wrapper })

    // Both saves start from the same stale object, as an autosave and a click would.
    act(() => result.current.update(todo, { note: 'typed note' }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    act(() => result.current.update(todo, { end_date: '2026-10-30' }))
    await new Promise((r) => setTimeout(r, 20))
    expect(bodies).toHaveLength(1)

    release()
    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies[0]).toMatchObject({ note: 'typed note', end_date: null })
    expect(bodies[1]).toMatchObject({ note: 'typed note', end_date: '2026-10-30' })
  })
})

describe('useTodoActions().update with a position', () => {
  const at = (id: number, position: number, task_id = 'local-WHF-001'): Todo => ({ ...todo, id, task_id, position })

  it.each([
    { name: 'down', id: 1, to: 2, want: { 1: 2, 2: 0, 3: 1, 4: 3 } },
    { name: 'up', id: 4, to: 1, want: { 1: 0, 2: 2, 3: 3, 4: 1 } },
  ])('shifts the cached todos of the task at once when a todo moves $name', async ({ id, to, want }) => {
    let release!: () => void
    const held = new Promise<void>((r) => (release = r))
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_url: string, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          await held
          return new Response(null, { status: 204 })
        }
        return new Promise<Response>(() => {})
      }),
    )
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const other = at(9, 1, 'local-WHF-002')
    qc.setQueryData(keys.todos({ is_done: false }), [at(1, 0), at(2, 1), at(4, 3), other])
    qc.setQueryData(keys.todos({ completed_since: 'x' }), [{ ...at(3, 2), is_done: true }])
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    const { result } = renderHook(() => useTodoActions(), { wrapper })

    const moved = [at(1, 0), at(2, 1), at(3, 2), at(4, 3)].find((t) => t.id === id)!
    act(() => result.current.update(moved, { position: to }))

    await waitFor(() => {
      const cached = [
        ...(qc.getQueryData<Todo[]>(keys.todos({ is_done: false })) ?? []),
        ...(qc.getQueryData<Todo[]>(keys.todos({ completed_since: 'x' })) ?? []),
      ]
      expect(Object.fromEntries(cached.filter((t) => t.task_id === 'local-WHF-001').map((t) => [t.id, t.position]))).toEqual(want)
      expect(cached.find((t) => t.id === 9)?.position).toBe(1)
    })
    release()
  })
})

describe('useTodoActions().update moving todos of one task', () => {
  it('sends the second move only after the first lands', async () => {
    const urls: string[] = []
    let release!: () => void
    const firstHeld = new Promise<void>((r) => (release = r))
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          urls.push(url)
          if (urls.length === 1) await firstHeld
          return new Response(null, { status: 204 })
        }
        return new Promise<Response>(() => {})
      }),
    )
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const a = { ...todo, id: 1, position: 0 }
    const b = { ...todo, id: 2, position: 1 }
    qc.setQueryData(keys.todos({ is_done: false }), [a, b])
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    const { result } = renderHook(() => useTodoActions(), { wrapper })

    act(() => result.current.update(a, { position: 1 }))
    await waitFor(() => expect(urls).toEqual(['/api/todos/1']))
    act(() => result.current.update(b, { position: 1 }))
    await new Promise((r) => setTimeout(r, 20))
    expect(urls).toEqual(['/api/todos/1'])

    release()
    await waitFor(() => expect(urls).toEqual(['/api/todos/1', '/api/todos/2']))
  })
})

const task: TaskDetail = {
  id: 'local-WHF-001',
  originated: 'local',
  title: 'Plan the migration',
  description: '',
  status: 'todo',
  start_date: null,
  due_date: null,
  parent_id: 'jira-SEC-110',
  children_ids: [],
  linked_ids: ['jira-SEC-142'],
  parent: null,
  children: [],
  linked: [],
}

describe('useTaskActions().update', () => {
  it('sends saves of one task in order, each with the fields of the earlier ones', async () => {
    const bodies: Record<string, unknown>[] = []
    let release!: () => void
    const firstHeld = new Promise<void>((r) => (release = r))
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          bodies.push(JSON.parse(String(init.body)) as Record<string, unknown>)
          if (bodies.length === 1) await firstHeld
          return new Response(null, { status: 204 })
        }
        return new Response(JSON.stringify(url === '/api/tasks' ? { tasks: [] } : task))
      }),
    )
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    qc.setQueryData(keys.task(task.id), task)
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    const { result } = renderHook(() => useTaskActions(), { wrapper })

    // A title autosave and a status pick, both from the same stale detail.
    act(() => result.current.update(task, { title: 'Plan the Q4 migration' }))
    await waitFor(() => expect(bodies).toHaveLength(1))
    act(() => result.current.update(task, { status: 'in-progress' }))
    await new Promise((r) => setTimeout(r, 20))
    expect(bodies).toHaveLength(1)

    release()
    await waitFor(() => expect(bodies).toHaveLength(2))
    expect(bodies[0]).toMatchObject({ title: 'Plan the Q4 migration', status: 'todo' })
    expect(bodies[1]).toMatchObject({
      title: 'Plan the Q4 migration',
      status: 'in-progress',
      parent_id: 'jira-SEC-110',
      linked_ids: ['jira-SEC-142'],
    })
  })
})

describe('useTaskActions().setStatus and reorder', () => {
  const item = (id: string, position: number | null): TaskListItem => ({
    ...task,
    id,
    todo_count: 0,
    done_todo_count: 0,
    position,
  })

  function setup(fail: boolean) {
    const calls: { url: string; body: unknown }[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          calls.push({ url, body: JSON.parse(String(init.body)) })
          return fail
            ? new Response(JSON.stringify({ error: 'disk failed' }), { status: 500 })
            : new Response(null, { status: 204 })
        }
        return new Promise<Response>(() => {})
      }),
    )
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    qc.setQueryData(keys.tasks, [item('local-WHF-001', null), item('jira-SEC-1', null)])
    qc.setQueryData(keys.task(task.id), task)
    const wrapper = ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    const { result } = renderHook(() => useTaskActions(), { wrapper })
    return { qc, calls, actions: () => result.current }
  }

  it('sends only the status and shows it before the server answers', async () => {
    const { qc, calls, actions } = setup(false)
    act(() => actions().setStatus(task, 'done'))
    await waitFor(() => expect(calls).toHaveLength(1))
    expect(calls[0]).toEqual({ url: '/api/tasks/local-WHF-001/status', body: { status: 'done' } })
    expect(qc.getQueryData<TaskListItem[]>(keys.tasks)?.[0].status).toBe('done')
    expect(qc.getQueryData<TaskDetail>(keys.task(task.id))?.status).toBe('done')
  })

  it('stores every task position and undoes the order when the save fails', async () => {
    const ok = setup(false)
    act(() => ok.actions().reorder(['jira-SEC-1', 'local-WHF-001']))
    await waitFor(() => expect(ok.calls).toHaveLength(1))
    expect(ok.calls[0]).toEqual({ url: '/api/tasks/order', body: { ids: ['jira-SEC-1', 'local-WHF-001'] } })
    expect(ok.qc.getQueryData<TaskListItem[]>(keys.tasks)?.map((t) => [t.id, t.position])).toEqual([
      ['local-WHF-001', 1],
      ['jira-SEC-1', 0],
    ])

    const failed = setup(true)
    act(() => failed.actions().reorder(['jira-SEC-1', 'local-WHF-001']))
    await waitFor(() => expect(failed.calls).toHaveLength(1))
    await waitFor(() =>
      expect(failed.qc.getQueryData<TaskListItem[]>(keys.tasks)?.map((t) => t.position)).toEqual([null, null]),
    )
  })
})
