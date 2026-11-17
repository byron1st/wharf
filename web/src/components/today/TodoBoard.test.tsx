import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ClockContext } from '@/hooks/useNow'
import type { TaskListItem, Todo } from '@/lib/api'
import { atLocal } from '@/lib/dates'
import { TodoBoard } from './TodoBoard'
import type { Selection, TodoGroup } from './TodosCard'

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
  task_id: 'local-WHF-001',
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

/** sends records the requests the board sends, answering GETs with no todos. */
function stubFetch() {
  const sends: { method: string; url: string; body: Record<string, unknown> }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET'
      if (method !== 'GET') {
        sends.push({ method, url, body: JSON.parse(String(init?.body ?? '{}')) as Record<string, unknown> })
        return method === 'POST' ? new Response(JSON.stringify({ id: 99 })) : new Response(null, { status: 204 })
      }
      return new Response(JSON.stringify({ todos: [], tasks: [] }))
    }),
  )
  return sends
}

function renderBoard(groups: TodoGroup[], { doneToday = [], selection = null }: { doneToday?: Todo[]; selection?: Selection } = {}) {
  const props = { onSelect: vi.fn(), onToggleDone: vi.fn() }
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <ClockContext.Provider value={{ now, today }}>
        <TodoBoard groups={groups} doneToday={doneToday} selection={selection} {...props} />
      </ClockContext.Provider>
    </QueryClientProvider>,
  )
  return props
}

const column = (name: string) => screen.getByRole('region', { name })
const cardsIn = (name: string) =>
  within(column(name))
    .queryAllByRole('checkbox')
    .map((c) => c.getAttribute('aria-label'))

const COLS = ['late', 'today', 'later', 'nodate']

/** stubRects lays the columns out side by side, 300px wide, and their cards 60px apart from y=100. */
function stubRects() {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    let left = -1000
    let top = -1000
    let height = 50
    if (this.dataset.col) {
      left = COLS.indexOf(this.dataset.col) * 300
      top = 0
      height = 800
    } else if (this.dataset.todo) {
      const col = this.closest<HTMLElement>('[data-col]')
      left = COLS.indexOf(col?.dataset.col ?? '') * 300
      top = 100 + [...(col?.querySelectorAll('[data-todo]') ?? [])].indexOf(this) * 60
    }
    return { x: left, y: top, top, left, right: left + 290, bottom: top + height, width: 290, height, toJSON: () => ({}) }
  })
}

/** drag presses on text, moves to (x, y), and lets go. */
async function drag(text: string, x: number, y: number) {
  const el = screen.getByText(text)
  const rect = el.closest<HTMLElement>('[data-todo]')!.getBoundingClientRect()
  const at = (cx: number, cy: number) => ({ clientX: cx, clientY: cy, isPrimary: true, button: 0 })
  fireEvent.pointerDown(el, at(rect.left + 20, rect.top + 20))
  fireEvent.pointerMove(el, at(rect.left + 20, rect.top + 26))
  fireEvent.pointerMove(el, at(x, y))
  await new Promise((r) => setTimeout(r, 0))
  fireEvent.pointerUp(el, at(x, y))
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('TodoBoard', () => {
  it('sorts Today’s open todos into Late, Due today, Upcoming and No date', () => {
    stubFetch()
    renderBoard([
      {
        task: task('local-WHF-001'),
        todos: [
          todo(1, { end_date: '2026-10-24' }),
          todo(2, { end_date: today }),
          todo(3, { start_date: '2026-10-20', end_date: '2026-10-30' }),
          todo(4),
        ],
      },
    ])

    expect(cardsIn('Late')).toEqual(['Todo 1'])
    expect(cardsIn('Due today')).toEqual(['Todo 2'])
    expect(cardsIn('Upcoming')).toEqual(['Todo 3'])
    expect(cardsIn('No date')).toEqual(['Todo 4'])
    expect(within(column('Late')).getByText('2 days late')).toBeInTheDocument()
    expect(within(column('Upcoming')).getByText('By Oct 30')).toBeInTheDocument()
    // Due today and No date say the when in their header instead.
    expect(within(column('Due today')).queryByText('Due today', { selector: 'span.rounded-\\[5px\\]' })).toBeNull()
  })

  it('counts each column and says when one is empty', () => {
    stubFetch()
    renderBoard([{ task: task('local-WHF-001'), todos: [todo(1), todo(2)] }])

    expect(within(column('No date')).getByText('2')).toBeInTheDocument()
    expect(within(column('Late')).getByText('Nothing late. Nice.')).toBeInTheDocument()
    expect(within(column('Due today')).getByText('Drop a todo here to finish it today')).toBeInTheDocument()
    expect(within(column('Upcoming')).getByText('Nothing scheduled')).toBeInTheDocument()
  })

  it('keeps todos done today at the bottom of their column', () => {
    stubFetch()
    const done = todo(5, { is_done: true, completed_at: now.toISOString(), end_date: today })
    const other = todo(6, { task_id: 'local-WHF-009', is_done: true, completed_at: now.toISOString() })
    const { onToggleDone } = renderBoard([{ task: task('local-WHF-001'), todos: [todo(1, { end_date: today }), done] }], {
      doneToday: [done, other],
    })

    const col = column('Due today')
    expect(cardsIn('Due today')).toEqual(['Todo 1', 'Todo 5'])
    expect(within(col).getByText('Done today · 1')).toBeInTheDocument()
    expect(within(col).getByText('Todo 5')).toHaveClass('line-through')
    // A todo of a task not on Today stays off the board.
    expect(screen.queryByText('Todo 6')).toBeNull()

    fireEvent.click(within(col).getByRole('checkbox', { name: 'Todo 5' }))
    expect(onToggleDone).toHaveBeenCalledWith(done, false)
  })

  it('selects a todo from its text and its task from the task line, and marks them', () => {
    stubFetch()
    const groups = [{ task: task('local-WHF-001'), todos: [todo(1)] }]
    const { onSelect } = renderBoard(groups, { selection: { kind: 'todo', id: 1 } })

    expect(screen.getByRole('button', { name: 'Todo 1' })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('button', { name: 'Todo 1' }).closest('[data-todo]')).toHaveClass('shadow-ring')

    fireEvent.click(screen.getByRole('button', { name: 'Todo 1' }))
    expect(onSelect).toHaveBeenLastCalledWith({ kind: 'todo', id: 1 })
    fireEvent.click(screen.getByTitle('WHF-001 · Title of local-WHF-001 — show task'))
    expect(onSelect).toHaveBeenLastCalledWith({ kind: 'task', id: 'local-WHF-001' })
  })

  it('shows a todo’s reminder, note and requests on its card', () => {
    stubFetch()
    renderBoard([
      {
        task: task('local-WHF-001'),
        todos: [
          todo(1, {
            note: 'see thread',
            reminders: [atLocal(today, '16:00').toISOString()],
            service_desk_requests: [{ key: 'CIS-1', title: 'VPN', status: 'Open', status_category: 'new', created_at: '', status_at: '' }],
          }),
        ],
      },
    ])

    expect(screen.getByText('Today 16:00')).toBeInTheDocument()
    expect(screen.getByLabelText('Has a note')).toBeInTheDocument()
    expect(screen.getByTitle('CIS-1 VPN')).toHaveTextContent('1')
  })

  it('filters the board by task from the chips', () => {
    stubFetch()
    renderBoard([
      { task: task('local-WHF-001'), todos: [todo(1), todo(2)] },
      { task: task('local-WHF-002'), todos: [todo(3, { task_id: 'local-WHF-002' })] },
      { task: task('local-WHF-003'), todos: [] },
    ])
    const chips = within(screen.getByRole('group', { name: 'Filter by task' }))

    expect(chips.getByRole('button', { name: /^All tasks/ })).toHaveTextContent('All tasks3')
    expect(chips.queryByTitle(/^WHF-003/)).toBeNull()

    fireEvent.click(chips.getByTitle('WHF-002 · Title of local-WHF-002'))
    expect(cardsIn('No date')).toEqual(['Todo 3'])
    expect(chips.getByTitle('WHF-002 · Title of local-WHF-002')).toHaveAttribute('aria-pressed', 'true')

    fireEvent.click(chips.getByRole('button', { name: /^All tasks/ }))
    expect(cardsIn('No date')).toEqual(['Todo 1', 'Todo 2', 'Todo 3'])
  })

  it('orders a column by board position, then by end date and task', () => {
    stubFetch()
    renderBoard([
      { task: task('local-WHF-001'), todos: [todo(1, { end_date: '2026-10-20' }), todo(2, { end_date: '2026-10-24' })] },
      { task: task('local-WHF-002'), todos: [todo(3, { task_id: 'local-WHF-002', end_date: '2026-10-20', board_column: 'late', board_position: 0 })] },
    ])

    expect(cardsIn('Late')).toEqual(['Todo 3', 'Todo 1', 'Todo 2'])
  })

  it.each([
    { to: 'Due today', x: 300 + 100, want: { start_date: '2026-10-20', end_date: today } },
    { to: 'Upcoming', x: 600 + 100, want: { start_date: '2026-10-20', end_date: '2026-10-27' } },
  ])('changes the dates of a todo dragged to $to and stores its place there', async ({ x, want }) => {
    const sends = stubFetch()
    stubRects()
    renderBoard([{ task: task('local-WHF-001'), todos: [todo(1, { start_date: '2026-10-20' }), todo(2, { end_date: '2026-10-30' })] }])

    await drag('Todo 1', x, 600)

    await waitFor(() => expect(sends).toHaveLength(2))
    expect(sends[0]).toMatchObject({ method: 'PUT', url: '/api/todos/1', body: { description: 'Todo 1', ...want } })
    expect(sends[1]).toEqual({
      method: 'PUT',
      url: '/api/todos/board-order',
      body: { column: x > 600 ? 'later' : 'today', ids: x > 600 ? [2, 1] : [1] },
    })
  })

  it('clears the end date of a todo dragged to No date, before the card it lands on', async () => {
    const sends = stubFetch()
    stubRects()
    renderBoard([{ task: task('local-WHF-001'), todos: [todo(1, { end_date: today }), todo(2)] }])

    // No date's first card, Todo 2, sits at y 100–150.
    await drag('Todo 1', 900 + 100, 120)

    await waitFor(() => expect(sends).toHaveLength(2))
    expect(sends[0]).toMatchObject({ url: '/api/todos/1', body: { end_date: null } })
    expect(sends[1]).toMatchObject({ url: '/api/todos/board-order', body: { column: 'nodate', ids: [1, 2] } })
  })

  it('reorders a column without changing dates', async () => {
    const sends = stubFetch()
    stubRects()
    renderBoard([{ task: task('local-WHF-001'), todos: [todo(1), todo(2), todo(3)] }])

    // Todo 3 lands on Todo 1, the first card of No date.
    await drag('Todo 3', 900 + 100, 120)

    await waitFor(() => expect(sends).toHaveLength(1))
    expect(sends[0]).toEqual({ method: 'PUT', url: '/api/todos/board-order', body: { column: 'nodate', ids: [3, 1, 2] } })
  })

  it('keeps the todos a filter hides in the column order it stores', async () => {
    const sends = stubFetch()
    stubRects()
    renderBoard([
      { task: task('local-WHF-001'), todos: [todo(1), todo(2)] },
      { task: task('local-WHF-002'), todos: [todo(3, { task_id: 'local-WHF-002' })] },
    ])

    fireEvent.click(screen.getByTitle('WHF-001 · Title of local-WHF-001'))
    // Todo 2 lands on Todo 1, the first card of the filtered No date column.
    await drag('Todo 2', 900 + 100, 120)

    await waitFor(() => expect(sends).toHaveLength(1))
    expect(sends[0].body).toEqual({ column: 'nodate', ids: [2, 1, 3] })
  })

  it('drops the task filter once the task has no open todo left', () => {
    stubFetch()
    const groups = [
      { task: task('local-WHF-001'), todos: [todo(1)] },
      { task: task('local-WHF-002'), todos: [todo(2, { task_id: 'local-WHF-002' })] },
    ]
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const board = (g: TodoGroup[]) => (
      <QueryClientProvider client={qc}>
        <ClockContext.Provider value={{ now, today }}>
          <TodoBoard groups={g} doneToday={[]} selection={null} onSelect={vi.fn()} onToggleDone={vi.fn()} />
        </ClockContext.Provider>
      </QueryClientProvider>
    )
    const { rerender } = render(board(groups))

    fireEvent.click(screen.getByTitle('WHF-002 · Title of local-WHF-002'))
    expect(cardsIn('No date')).toEqual(['Todo 2'])

    rerender(board([groups[0], { ...groups[1], todos: [todo(2, { task_id: 'local-WHF-002', is_done: true })] }]))
    expect(screen.getByRole('button', { name: /^All tasks/ })).toHaveAttribute('aria-pressed', 'true')
    expect(cardsIn('No date')).toEqual(['Todo 1'])
  })

  it('takes no drops in Late from another column, and does not drag a done todo', async () => {
    const sends = stubFetch()
    stubRects()
    const done = todo(5, { is_done: true, completed_at: now.toISOString() })
    renderBoard([{ task: task('local-WHF-001'), todos: [todo(1, { end_date: '2026-10-20' }), todo(2), done] }], {
      doneToday: [done],
    })

    await drag('Todo 2', 100, 600)
    await drag('Todo 5', 300 + 100, 600)

    await new Promise((r) => setTimeout(r, 20))
    expect(sends).toHaveLength(0)
    expect(cardsIn('Late')).toEqual(['Todo 1'])
  })

  it('adds a todo from the New todo popover, opened with T', async () => {
    const sends = stubFetch()
    const user = userEvent.setup()
    renderBoard([
      { task: task('local-WHF-001'), todos: [todo(1)] },
      { task: task('local-WHF-002'), todos: [todo(2, { task_id: 'local-WHF-002' })] },
    ])

    await user.keyboard('t')
    const dialog = await screen.findByRole('dialog', { name: 'New todo' })
    expect(within(dialog).getByText('Lands in No date')).toBeInTheDocument()
    expect(within(dialog).getByRole('button', { name: 'Add todo' })).toBeDisabled()

    await user.type(within(dialog).getByRole('textbox', { name: 'Todo' }), 'Write the runbook')
    await user.selectOptions(within(dialog).getByRole('combobox', { name: 'Task' }), 'local-WHF-002')
    await user.click(within(dialog).getByRole('radio', { name: 'Tomorrow' }))
    expect(within(dialog).getByText('Lands in Upcoming')).toBeInTheDocument()
    await user.click(within(dialog).getByRole('button', { name: 'Add todo' }))

    await waitFor(() => expect(sends).toHaveLength(1))
    expect(sends[0]).toMatchObject({
      method: 'POST',
      url: '/api/todos',
      body: { task_id: 'local-WHF-002', description: 'Write the runbook', end_date: '2026-10-27', start_date: null },
    })
    expect(screen.queryByRole('dialog', { name: 'New todo' })).toBeNull()
  })

  it('defaults the New todo task to the one the board is filtered by, and adds with Enter', async () => {
    const sends = stubFetch()
    const user = userEvent.setup()
    renderBoard([
      { task: task('local-WHF-001'), todos: [todo(1)] },
      { task: task('local-WHF-002'), todos: [todo(2, { task_id: 'local-WHF-002' })] },
    ])

    await user.click(screen.getByTitle('WHF-002 · Title of local-WHF-002'))
    await user.click(screen.getByRole('button', { name: /New todo/ }))
    const dialog = await screen.findByRole('dialog', { name: 'New todo' })
    expect(within(dialog).getByRole('combobox', { name: 'Task' })).toHaveValue('local-WHF-002')

    await user.click(within(dialog).getByRole('radio', { name: 'Today' }))
    await user.type(within(dialog).getByRole('textbox', { name: 'Todo' }), 'Call back{Enter}')

    await waitFor(() => expect(sends).toHaveLength(1))
    expect(sends[0].body).toMatchObject({ task_id: 'local-WHF-002', description: 'Call back', end_date: today })
  })
})
