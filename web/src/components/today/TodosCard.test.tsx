import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ClockContext } from '@/hooks/useNow'
import type { TaskListItem, Todo } from '@/lib/api'
import { atLocal } from '@/lib/dates'
import { TodosCard, type TodoGroup } from './TodosCard'

const today = '2026-10-26'
const now = atLocal(today, '14:07')

const task = (id: string, over: Partial<TaskListItem> = {}): TaskListItem => ({
  id,
  originated: id.startsWith('jira-') ? 'jira' : 'local',
  title: `Title of ${id}`,
  description: '',
  status: 'in-progress',
  start_date: null,
  due_date: null,
  todo_count: 0,
  done_todo_count: 0,
  position: null,
  closed_at: null,
  closed_since_last_sync: false,
  ...over,
})

const todo = (id: number, over: Partial<Todo> = {}): Todo => ({
  id,
  task_id: 'local-WHF-003',
  description: `Todo ${id}`,
  note: '',
  is_done: false,
  completed_at: null,
  start_date: null,
  end_date: null,
  reminders: [],
  position: id,
  service_desk_requests: [],
  ...over,
})

function renderCard(groups: TodoGroup[], adding: string | null = null) {
  const props = { onToggleDone: vi.fn(), onSetStatus: vi.fn(), onMove: vi.fn(), setAdding: vi.fn() }
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <ClockContext.Provider value={{ now, today }}>
        <TodosCard groups={groups} selection={null} onSelect={vi.fn()} adding={adding} {...props} />
      </ClockContext.Provider>
    </QueryClientProvider>,
  )
  return props
}

/** stubRects lays the todo rows out 44px apart in the given order, so dnd-kit can find the row under the pointer. */
function stubRects(order: number[]) {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    const i = this.dataset.todo ? order.indexOf(Number(this.dataset.todo)) : -1
    const top = i < 0 ? -1000 : i * 44
    return { x: 0, y: top, top, left: 0, right: 500, bottom: top + 44, width: 500, height: 44, toJSON: () => ({}) }
  })
}

/** drag presses on the text, moves to the middle of the row at index to, and lets go. */
async function drag(text: string, to: number) {
  // The drag overlay of an earlier drag repeats the text until it is gone.
  const el = await waitFor(() => screen.getByText(text))
  const at = (y: number) => ({ clientX: 100, clientY: y, isPrimary: true, button: 0 })
  const y = Number(el.closest<HTMLElement>('[data-todo]')?.getBoundingClientRect().top ?? -1000) + 22
  fireEvent.pointerDown(el, at(y))
  fireEvent.pointerMove(el, at(y + 6))
  fireEvent.pointerMove(el, at(to * 44 + 22))
  fireEvent.pointerUp(el, at(to * 44 + 22))
}

/** stubSaves records the todo saves the card sends. */
function stubSaves() {
  const saves: { url: string; body: Record<string, unknown> }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === 'PUT') {
        saves.push({ url, body: JSON.parse(String(init.body)) as Record<string, unknown> })
        return new Response(null, { status: 204 })
      }
      return new Response(JSON.stringify({ todos: [], tasks: [] }))
    }),
  )
  return saves
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
  localStorage.clear()
})

describe('TodosCard', () => {
  it('changes a local task status from its menu', async () => {
    const local = task('local-WHF-003')
    const { onSetStatus } = renderCard([{ task: local, todos: [] }])

    await userEvent.click(screen.getByRole('button', { name: 'Status: In progress' }))
    const menu = await screen.findByRole('menu')
    await userEvent.click(within(menu).getByRole('menuitem', { name: 'Done' }))

    expect(onSetStatus).toHaveBeenCalledWith(local, 'done')
  })

  it('shows a Jira task status read-only', () => {
    renderCard([{ task: task('jira-SEC-142'), todos: [] }])

    expect(screen.queryByRole('button', { name: /^Status:/ })).toBeNull()
    expect(screen.getByRole('img', { name: 'Status: In progress' })).toHaveAttribute('title', 'In progress · from Jira')
  })

  it('keeps a checked todo in place, struck through, and unchecks it', () => {
    const done = todo(1, { is_done: true, completed_at: now.toISOString() })
    const { onToggleDone } = renderCard([{ task: task('local-WHF-003'), todos: [done, todo(2)] }])

    expect(screen.getByText('Done today')).toBeInTheDocument()
    expect(screen.getByText('Todo 1')).toHaveClass('line-through')

    fireEvent.click(screen.getByRole('checkbox', { name: 'Todo 1' }))
    expect(onToggleDone).toHaveBeenCalledWith(done, false)
  })

  it('shows and hides a task’s done todos from its progress count', async () => {
    const kept = todo(1, { is_done: true, completed_at: now.toISOString() })
    const older = todo(5, { is_done: true, completed_at: atLocal('2026-10-22', '10:00').toISOString() })
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ todos: [older, kept] })))
    vi.stubGlobal('fetch', fetchMock)
    renderCard([
      { task: task('local-WHF-003', { todo_count: 3, done_todo_count: 2 }), todos: [kept] },
      { task: task('local-WHF-004', { todo_count: 1 }), todos: [] },
    ])

    expect(screen.getByRole('button', { name: '0/1' })).toBeDisabled()

    fireEvent.click(screen.getByRole('button', { name: '2/3' }))
    expect(await screen.findByText('Done Oct 22')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith('/api/todos?task_id=local-WHF-003&is_done=true', expect.anything())
    // The kept todo is not listed twice.
    expect(screen.getAllByText('Todo 1')).toHaveLength(1)

    fireEvent.click(screen.getByRole('button', { name: '2/3' }))
    await waitFor(() => expect(screen.queryByText('Done Oct 22')).toBeNull())
  })

  it('moves a task up or down with Option and the arrow keys on its grip', () => {
    const { onMove } = renderCard([
      { task: task('local-WHF-001'), todos: [] },
      { task: task('local-WHF-002'), todos: [] },
      { task: task('jira-SEC-3'), todos: [] },
    ])

    fireEvent.keyDown(screen.getByRole('button', { name: 'Reorder WHF-002' }), { key: 'ArrowDown', altKey: true })
    expect(onMove).toHaveBeenLastCalledWith('local-WHF-002', null)

    fireEvent.keyDown(screen.getByRole('button', { name: 'Reorder WHF-001' }), { key: 'ArrowDown', altKey: true })
    expect(onMove).toHaveBeenLastCalledWith('local-WHF-001', 'jira-SEC-3')

    fireEvent.keyDown(screen.getByRole('button', { name: 'Reorder SEC-3' }), { key: 'ArrowUp', altKey: true })
    expect(onMove).toHaveBeenLastCalledWith('jira-SEC-3', 'local-WHF-002')

    fireEvent.keyDown(screen.getByRole('button', { name: 'Reorder WHF-001' }), { key: 'ArrowUp', altKey: true })
    fireEvent.keyDown(screen.getByRole('button', { name: 'Reorder WHF-001' }), { key: 'ArrowDown' })
    expect(onMove).toHaveBeenCalledTimes(3)
  })

  it('moves an open todo onto another of its task by drag', async () => {
    const saves = stubSaves()
    stubRects([1, 2, 3])
    renderCard([{ task: task('local-WHF-003'), todos: [todo(1), todo(2), todo(3)] }])

    await drag('Todo 1', 2)

    await waitFor(() => expect(saves).toHaveLength(1))
    expect(saves[0].url).toBe('/api/todos/1')
    expect(saves[0].body).toMatchObject({ description: 'Todo 1', position: 3 })
  })

  it('moves a todo up to the position of the one it lands on', async () => {
    const saves = stubSaves()
    stubRects([4, 7, 9])
    renderCard([{ task: task('local-WHF-003'), todos: [todo(4), todo(7), todo(9)] }])

    await drag('Todo 9', 0)

    await waitFor(() => expect(saves).toHaveLength(1))
    expect(saves[0].url).toBe('/api/todos/9')
    expect(saves[0].body).toMatchObject({ position: 4 })
  })

  it('does not drag a done todo or drop onto one', async () => {
    const saves = stubSaves()
    stubRects([1, 2, 3])
    const done = todo(2, { is_done: true, completed_at: now.toISOString() })
    renderCard([{ task: task('local-WHF-003'), todos: [todo(1), done, todo(3)] }])

    expect(screen.getByText('Todo 2').closest('[data-todo]')).toBeNull()
    await drag('Todo 2', 0)
    await drag('Todo 1', 1)

    await new Promise((r) => setTimeout(r, 20))
    expect(saves).toHaveLength(0)
  })

  it('does not move a todo onto another task or back onto itself', async () => {
    const saves = stubSaves()
    stubRects([1, 2, 5])
    renderCard([
      { task: task('local-WHF-003'), todos: [todo(1), todo(2)] },
      { task: task('local-WHF-004'), todos: [todo(5, { task_id: 'local-WHF-004' })] },
    ])

    await drag('Todo 1', 2)
    await drag('Todo 1', 0)

    await new Promise((r) => setTimeout(r, 20))
    expect(saves).toHaveLength(0)
  })
  it('does not click the checkbox a drag began and ended on', async () => {
    stubSaves()
    stubRects([1, 2])
    const first = todo(1)
    const { onToggleDone } = renderCard([{ task: task('local-WHF-003'), todos: [first, todo(2)] }])
    const box = screen.getByRole('checkbox', { name: 'Todo 1' })

    await drag('Todo 1', 0)
    fireEvent.click(box)
    expect(onToggleDone).not.toHaveBeenCalled()

    // dnd-kit swallows clicks for 50ms after a drag.
    await new Promise((r) => setTimeout(r, 60))
    fireEvent.click(box)
    expect(onToggleDone).toHaveBeenCalledWith(first, true)
  })

  it('collapses every task from the column header, then expands them all again', () => {
    const { setAdding } = renderCard([
      { task: task('local-WHF-001'), todos: [todo(1)] },
      { task: task('jira-SEC-3'), todos: [todo(2, { task_id: 'jira-SEC-3' })] },
    ])
    const groupToggles = () => screen.getAllByRole('button', { name: /^Collapse or expand / })

    fireEvent.click(screen.getByRole('button', { name: 'Collapse all tasks' }))
    for (const b of groupToggles()) expect(b).toHaveAttribute('aria-expanded', 'false')
    expect(screen.queryByText('Todo 1')).toBeNull()
    expect(screen.queryByText('Todo 2')).toBeNull()
    expect(JSON.parse(localStorage.getItem('wharf.collapsed') ?? '{}')).toEqual({
      'local-WHF-001': true,
      'jira-SEC-3': true,
    })

    setAdding.mockClear()
    fireEvent.click(screen.getByRole('button', { name: 'Expand all tasks' }))
    expect(setAdding).not.toHaveBeenCalled()
    for (const b of groupToggles()) expect(b).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText('Todo 1')).toBeInTheDocument()
    expect(screen.getByText('Todo 2')).toBeInTheDocument()
    expect(JSON.parse(localStorage.getItem('wharf.collapsed') ?? '{}')).toEqual({})
    expect(screen.getByRole('button', { name: 'Collapse all tasks' })).toBeInTheDocument()
  })

  it('collapses every task when only some are collapsed', () => {
    renderCard([
      { task: task('local-WHF-001'), todos: [] },
      { task: task('local-WHF-002'), todos: [] },
    ])

    fireEvent.click(screen.getByRole('button', { name: 'Collapse or expand WHF-001' }))
    fireEvent.click(screen.getByRole('button', { name: 'Collapse all tasks' }))

    for (const b of screen.getAllByRole('button', { name: /^Collapse or expand / })) {
      expect(b).toHaveAttribute('aria-expanded', 'false')
    }
    expect(screen.getByRole('button', { name: 'Expand all tasks' })).toBeInTheDocument()
  })

  it('closes the new todo row when collapsing every task', () => {
    const { setAdding } = renderCard([{ task: task('local-WHF-001'), todos: [] }], 'local-WHF-001')

    fireEvent.click(screen.getByRole('button', { name: 'Collapse all tasks' }))
    expect(setAdding).toHaveBeenCalledWith(null)
  })

  it('closes the new todo row when collapsing its task', () => {
    const { setAdding } = renderCard([{ task: task('local-WHF-001'), todos: [] }], 'local-WHF-001')

    fireEvent.click(screen.getByRole('button', { name: 'Collapse or expand WHF-001' }))
    expect(setAdding).toHaveBeenCalledWith(null)
  })

  it('disables the collapse-all button when there are no tasks', () => {
    renderCard([])

    expect(screen.getByRole('button', { name: 'Collapse all tasks' })).toBeDisabled()
  })
})
