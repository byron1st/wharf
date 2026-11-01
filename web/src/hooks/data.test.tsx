import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { TaskDetail, Todo } from '@/lib/api'
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
