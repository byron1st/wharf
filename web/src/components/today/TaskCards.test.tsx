import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ClockContext } from '@/hooks/useNow'
import type { TaskListItem, Todo } from '@/lib/api'
import { atLocal } from '@/lib/dates'
import { TaskCards } from './TaskCards'
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

function renderCards(groups: TodoGroup[], { adding = null, selection = null }: { adding?: string | null; selection?: Selection } = {}) {
  const props = { onSelect: vi.fn(), onToggleDone: vi.fn(), onSetStatus: vi.fn(), onMove: vi.fn(), setAdding: vi.fn() }
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <ClockContext.Provider value={{ now, today }}>
        <TaskCards groups={groups} selection={selection} adding={adding} {...props} />
      </ClockContext.Provider>
    </QueryClientProvider>,
  )
  return props
}

const card = (name: RegExp | string) => screen.getByRole('article', { name })

/** stubRects lays cards out 200px apart and the todo rows of each 44px apart, in the given orders. */
function stubRects(cards: string[], todos: number[] = []) {
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockImplementation(function (this: HTMLElement) {
    let top = -1000
    let height = 44
    if (this.dataset.card) {
      const i = cards.indexOf(this.dataset.card)
      top = i < 0 ? cards.length * 200 : i * 200
      height = 180
    } else if (this.dataset.todo) {
      top = todos.indexOf(Number(this.dataset.todo)) * 44
    }
    return { x: 0, y: top, top, left: 0, right: 300, bottom: top + height, width: 300, height, toJSON: () => ({}) }
  })
}

/** drag presses on el, moves to y, and lets go. */
async function drag(el: HTMLElement, from: number, to: number) {
  const at = (y: number) => ({ clientX: 100, clientY: y, isPrimary: true, button: 0 })
  fireEvent.pointerDown(el, at(from))
  fireEvent.pointerMove(el, at(from + 6))
  fireEvent.pointerMove(el, at(to))
  await new Promise((r) => setTimeout(r, 0))
  fireEvent.pointerUp(el, at(to))
}

/** stubSaves records the todo saves the cards send. */
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

/** unfold opens the folded todos of the card whose key matches name. */
const unfold = (name: RegExp) => fireEvent.click(within(card(name)).getByRole('button', { name: /more todos?$/ }))

describe('TaskCards', () => {
  it('shows one card per task in order, in an auto-fill grid', () => {
    renderCards([
      { task: task('local-WHF-001', { due_date: '2026-10-26T00:00:00Z', todo_count: 4, done_todo_count: 1 }), todos: [todo(1)] },
      { task: task('jira-SEC-3', { title: 'Rotate keys' }), todos: [] },
    ])

    const grid = screen.getByRole('region', { name: 'Tasks' })
    expect(grid).toHaveClass('grid-cols-[repeat(auto-fill,minmax(310px,1fr))]', 'items-start', 'gap-4')
    expect(within(grid).getAllByRole('article').map((a) => a.getAttribute('aria-label'))).toEqual([
      'WHF-001 Title of local-WHF-001',
      'SEC-3 Rotate keys',
    ])

    const first = card(/^WHF-001/)
    expect(within(first).getByRole('button', { name: 'Status: In progress' })).toHaveTextContent('In progress')
    expect(within(first).getByText('WHF-001')).toBeInTheDocument()
    expect(within(first).getByText('Due today')).toBeInTheDocument()
    expect(within(first).getByRole('button', { name: '1/4 done' })).toBeInTheDocument()
    expect(within(first).getByRole('checkbox', { name: 'Todo 1' })).toBeInTheDocument()
    expect(within(first).getByRole('button', { name: 'Add todo to WHF-001' })).toHaveTextContent('Add todo')

    expect(within(card(/^SEC-3/)).getByText('Nothing due today.')).toBeInTheDocument()
  })

  it('wraps long titles and todo text in full instead of truncating them', () => {
    const long = '결제 시스템 리팩터링 후속 작업과 정산 배치 마이그레이션 일정 조율 및 운영팀 공유'
    renderCards([{ task: task('local-WHF-001', { title: long }), todos: [todo(1, { description: long })] }])

    const [title, text] = screen.getAllByText(long)
    for (const el of [title, text]) {
      expect(el).toHaveClass('break-keep', '[overflow-wrap:anywhere]')
      expect(el).not.toHaveClass('truncate')
    }
  })

  it('shows a todo’s when, reminder, note and requests under its text', () => {
    renderCards([
      {
        task: task('local-WHF-003'),
        todos: [
          todo(1, {
            end_date: today,
            note: 'see thread',
            reminders: [atLocal(today, '16:00').toISOString()],
            service_desk_requests: [{ key: 'CIS-1', title: 'VPN', status: 'Open', status_category: 'new', created_at: '', status_at: '' }],
          }),
        ],
      },
    ])

    expect(screen.getByText('Due today', { selector: 'button span' })).toBeInTheDocument()
    expect(screen.getByText('Today 16:00')).toBeInTheDocument()
    expect(screen.getByLabelText('Has a note')).toBeInTheDocument()
    expect(screen.getByTitle('CIS-1 VPN')).toHaveTextContent('1')
  })

  it('selects a task from its title and a todo from its text, and marks the selected ones', () => {
    const groups = [{ task: task('local-WHF-003'), todos: [todo(1)] }]
    const { onSelect } = renderCards(groups)

    fireEvent.click(screen.getByRole('button', { name: 'Title of local-WHF-003' }))
    expect(onSelect).toHaveBeenLastCalledWith({ kind: 'task', id: 'local-WHF-003' })
    fireEvent.click(screen.getByText('Todo 1'))
    expect(onSelect).toHaveBeenLastCalledWith({ kind: 'todo', id: 1 })
  })

  it('rings the selected task’s card', () => {
    renderCards([{ task: task('local-WHF-003'), todos: [] }, { task: task('local-WHF-004'), todos: [] }], {
      selection: { kind: 'task', id: 'local-WHF-003' },
    })

    expect(card(/^WHF-003/)).toHaveClass('shadow-ring')
    expect(within(card(/^WHF-003/)).getByRole('button', { name: 'Title of local-WHF-003' })).toHaveAttribute('aria-pressed', 'true')
    expect(card(/^WHF-004/)).not.toHaveClass('shadow-ring')
  })

  it('highlights the selected todo', () => {
    renderCards([{ task: task('local-WHF-003'), todos: [todo(1), todo(2)] }], { selection: { kind: 'todo', id: 2 } })
    unfold(/^WHF-003/)

    expect(screen.getByText('Todo 2').closest('button')).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByText('Todo 2')).toHaveClass('font-semibold')
    expect(screen.getByText('Todo 1')).not.toHaveClass('font-semibold')
  })

  it('checks and unchecks a todo', () => {
    const done = todo(1, { is_done: true, completed_at: now.toISOString() })
    const open = todo(2)
    const { onToggleDone } = renderCards([{ task: task('local-WHF-003'), todos: [done, open] }])
    unfold(/^WHF-003/)

    expect(screen.getByText('Todo 1')).toHaveClass('line-through')
    expect(screen.getByText('Done today')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('checkbox', { name: 'Todo 1' }))
    expect(onToggleDone).toHaveBeenCalledWith(done, false)
    fireEvent.click(screen.getByRole('checkbox', { name: 'Todo 2' }))
    expect(onToggleDone).toHaveBeenCalledWith(open, true)
  })

  it('changes a local task status from its chip', async () => {
    const local = task('local-WHF-003')
    const { onSetStatus } = renderCards([{ task: local, todos: [] }])

    await userEvent.click(screen.getByRole('button', { name: 'Status: In progress' }))
    const menu = await screen.findByRole('menu')
    await userEvent.click(within(menu).getByRole('menuitem', { name: 'Done' }))

    expect(onSetStatus).toHaveBeenCalledWith(local, 'done')
  })

  it('shows a Jira task’s status chip read-only', () => {
    renderCards([{ task: task('jira-SEC-142'), todos: [] }])

    expect(screen.queryByRole('button', { name: /^Status:/ })).toBeNull()
    const chip = screen.getByRole('img', { name: 'Status: In progress' })
    expect(chip).toHaveTextContent('In progress')
    expect(chip).toHaveAttribute('title', 'In progress · from Jira')
  })

  it('shows and hides a task’s done todos from its progress count', async () => {
    const older = todo(5, { is_done: true, completed_at: atLocal('2026-10-22', '10:00').toISOString() })
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ todos: [older] })))
    vi.stubGlobal('fetch', fetchMock)
    renderCards([
      { task: task('local-WHF-003', { todo_count: 2, done_todo_count: 1 }), todos: [todo(1)] },
      { task: task('local-WHF-004', { todo_count: 1 }), todos: [] },
    ])

    expect(screen.getByRole('button', { name: '0/1 done' })).toBeDisabled()
    expect(fetchMock).not.toHaveBeenCalled()

    fireEvent.click(screen.getByRole('button', { name: '1/2 done' }))
    expect(await screen.findByText('Done Oct 22')).toBeInTheDocument()
    expect(fetchMock).toHaveBeenCalledWith('/api/todos?task_id=local-WHF-003&is_done=true', expect.anything())

    fireEvent.click(screen.getByRole('button', { name: '1/2 done' }))
    await waitFor(() => expect(screen.queryByText('Done Oct 22')).toBeNull())
  })

  it('opens the new todo row from Add todo', () => {
    const { setAdding } = renderCards([{ task: task('local-WHF-003'), todos: [] }])

    fireEvent.click(screen.getByRole('button', { name: 'Add todo to WHF-003' }))
    expect(setAdding).toHaveBeenLastCalledWith('local-WHF-003')
  })

  it('adds a todo from the open new todo row and closes it with Escape or Cancel', async () => {
    const posts: unknown[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (_url: string, init?: RequestInit) => {
        if (init?.method === 'POST') posts.push(JSON.parse(String(init.body)))
        return new Response(JSON.stringify({ id: 9 }))
      }),
    )
    const { setAdding } = renderCards([{ task: task('local-WHF-003'), todos: [] }], { adding: 'local-WHF-003' })

    // The footer keeps Add todo while the new todo row is open.
    expect(screen.getByRole('button', { name: 'Add todo to WHF-003' })).toBeInTheDocument()
    const input = screen.getByRole('textbox', { name: 'New todo for WHF-003' })
    expect(input).toHaveFocus()
    await userEvent.type(input, 'Write the runbook{Enter}')
    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0]).toMatchObject({ task_id: 'local-WHF-003', description: 'Write the runbook' })
    expect(input).toHaveValue('')

    await userEvent.keyboard('{Escape}')
    expect(setAdding).toHaveBeenLastCalledWith(null)
    setAdding.mockClear()
    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(setAdding).toHaveBeenLastCalledWith(null)
  })

  it('moves a card before the card it is dropped on', async () => {
    stubRects(['local-WHF-001', 'local-WHF-002', 'local-WHF-003'])
    const { onMove } = renderCards([
      { task: task('local-WHF-001'), todos: [] },
      { task: task('local-WHF-002'), todos: [] },
      { task: task('local-WHF-003'), todos: [] },
    ])

    await drag(within(card(/^WHF-003/)).getByText('WHF-003'), 420, 20)

    await waitFor(() => expect(onMove).toHaveBeenCalledWith('local-WHF-003', 'local-WHF-001'))
  })

  it('shows the end slot only while dragging, and moves a card dropped there to the end', async () => {
    stubRects(['local-WHF-001', 'local-WHF-002'])
    const { onMove } = renderCards([
      { task: task('local-WHF-001'), todos: [] },
      { task: task('local-WHF-002'), todos: [] },
    ])
    expect(screen.queryByText('Drop here to move to the end')).toBeNull()

    const key = within(card(/^WHF-001/)).getByText('WHF-001')
    const at = (y: number) => ({ clientX: 100, clientY: y, isPrimary: true, button: 0 })
    fireEvent.pointerDown(key, at(20))
    fireEvent.pointerMove(key, at(26))
    expect(await screen.findByText('Drop here to move to the end')).toBeInTheDocument()
    fireEvent.pointerMove(key, at(420))
    await new Promise((r) => setTimeout(r, 0))
    fireEvent.pointerUp(key, at(420))

    await waitFor(() => expect(onMove).toHaveBeenCalledWith('local-WHF-001', null))
    expect(screen.queryByText('Drop here to move to the end')).toBeNull()
  })

  it('reorders a todo within its card by drag', async () => {
    const saves = stubSaves()
    stubRects([], [2, 3, 4])
    renderCards([{ task: task('local-WHF-003'), todos: [todo(1), todo(2), todo(3), todo(4)] }])
    unfold(/^WHF-003/)

    await drag(screen.getByText('Todo 2'), 22, 2 * 44 + 22)

    await waitFor(() => expect(saves).toHaveLength(1))
    expect(saves[0].url).toBe('/api/todos/2')
    expect(saves[0].body).toMatchObject({ description: 'Todo 2', position: 4 })
  })

  it('does not move a todo onto another card', async () => {
    const saves = stubSaves()
    stubRects([], [2, 3, 6])
    renderCards([
      { task: task('local-WHF-003'), todos: [todo(1), todo(2), todo(3)] },
      { task: task('local-WHF-004'), todos: [todo(5, { task_id: 'local-WHF-004' }), todo(6, { task_id: 'local-WHF-004' })] },
    ])
    unfold(/^WHF-003/)
    unfold(/^WHF-004/)

    await drag(screen.getByText('Todo 2'), 22, 2 * 44 + 22)

    await new Promise((r) => setTimeout(r, 20))
    expect(saves).toHaveLength(0)
  })

  it('does not click the checkbox a drag began and ended on', async () => {
    stubSaves()
    stubRects([], [2, 3])
    const first = todo(2)
    const { onToggleDone } = renderCards([{ task: task('local-WHF-003'), todos: [todo(1), first, todo(3)] }])
    unfold(/^WHF-003/)
    const box = screen.getByRole('checkbox', { name: 'Todo 2' })

    const text = screen.getByText('Todo 2')
    const at = (y: number) => ({ clientX: 100, clientY: y, isPrimary: true, button: 0 })
    fireEvent.pointerDown(text, at(22))
    fireEvent.pointerMove(text, at(28))
    fireEvent.pointerMove(text, at(22))
    fireEvent.pointerUp(text, at(22))
    fireEvent.click(box)
    expect(onToggleDone).not.toHaveBeenCalled()

    // dnd-kit swallows clicks for 50ms after a drag.
    await new Promise((r) => setTimeout(r, 60))
    fireEvent.click(box)
    expect(onToggleDone).toHaveBeenCalledWith(first, true)
  })

  it('shows the empty message when there are no tasks', () => {
    renderCards([])

    expect(screen.getByText(/Nothing to do today/)).toBeInTheDocument()
  })

  it('puts the most urgent open todo in Next up, tinted by urgency', () => {
    renderCards([
      { task: task('local-WHF-001'), todos: [todo(1), todo(2, { end_date: today }), todo(3, { end_date: '2026-10-24' })] },
      { task: task('local-WHF-002'), todos: [todo(4, { task_id: 'local-WHF-002' }), todo(5, { task_id: 'local-WHF-002', end_date: today })] },
      { task: task('local-WHF-003'), todos: [todo(6, { task_id: 'local-WHF-003' })] },
    ])

    const next = (name: RegExp) => within(card(name)).getByRole('group', { name: 'Next up' })
    expect(within(next(/^WHF-001/)).getByText('Todo 3')).toBeInTheDocument()
    expect(next(/^WHF-001/)).toHaveClass('bg-danger-tint')
    expect(within(next(/^WHF-002/)).getByText('Todo 5')).toBeInTheDocument()
    expect(next(/^WHF-002/)).toHaveClass('bg-today-tint')
    expect(within(next(/^WHF-003/)).getByText('Todo 6')).toBeInTheDocument()
    expect(next(/^WHF-003/)).toHaveClass('bg-bg-muted')
  })

  it('says nothing is due when a card has no open todo', () => {
    renderCards([{ task: task('local-WHF-001'), todos: [todo(1, { is_done: true, completed_at: now.toISOString() })] }])

    expect(screen.queryByRole('group', { name: 'Next up' })).toBeNull()
    expect(screen.getByText('Nothing due today.')).toBeInTheDocument()
  })

  it('shows how many todos are done and how many open ones are late', () => {
    renderCards([
      {
        task: task('local-WHF-001', { todo_count: 5, done_todo_count: 2 }),
        todos: [todo(1, { end_date: '2026-10-24' }), todo(2, { end_date: '2026-10-25' }), todo(3, { end_date: '2026-10-20', is_done: true })],
      },
      { task: task('local-WHF-002', { todo_count: 1 }), todos: [todo(4, { task_id: 'local-WHF-002', end_date: today })] },
    ])

    expect(within(card(/^WHF-001/)).getByRole('button', { name: '2/5 done' })).toBeInTheDocument()
    expect(within(card(/^WHF-001/)).getByText('2 late')).toBeInTheDocument()
    expect(within(card(/^WHF-002/)).queryByText(/late$/)).toBeNull()
  })

  it('folds the todos after Next up, and remembers an unfolded card', () => {
    const groups = [
      { task: task('local-WHF-001'), todos: [todo(1), todo(2), todo(3)] },
      { task: task('local-WHF-002'), todos: [todo(4, { task_id: 'local-WHF-002' })] },
    ]
    renderCards(groups)

    expect(screen.queryByText('Todo 2')).toBeNull()
    expect(within(card(/^WHF-002/)).getByRole('button', { name: 'No other todos' })).toBeDisabled()

    fireEvent.click(within(card(/^WHF-001/)).getByRole('button', { name: '2 more todos' }))
    expect(screen.getByText('Todo 2')).toBeInTheDocument()
    expect(screen.getByText('Todo 3')).toBeInTheDocument()
    expect(within(card(/^WHF-001/)).getByRole('button', { name: 'Hide 2 more' })).toHaveAttribute('aria-expanded', 'true')
    expect(JSON.parse(localStorage.getItem('wharf.cardOpen') ?? '{}')).toEqual({ 'local-WHF-001': true })

    cleanup()
    renderCards(groups)
    expect(screen.getByText('Todo 2')).toBeInTheDocument()

    fireEvent.click(within(card(/^WHF-001/)).getByRole('button', { name: 'Hide 2 more' }))
    expect(screen.queryByText('Todo 2')).toBeNull()
    expect(JSON.parse(localStorage.getItem('wharf.cardOpen') ?? '{}')).toEqual({})
  })

  it('folds a card in one click while its new todo row is open', () => {
    const { setAdding } = renderCards([{ task: task('local-WHF-001'), todos: [todo(1), todo(2)] }], { adding: 'local-WHF-001' })

    fireEvent.click(screen.getByRole('button', { name: 'Hide 1 more' }))
    expect(setAdding).toHaveBeenLastCalledWith(null)
    expect(JSON.parse(localStorage.getItem('wharf.cardOpen') ?? '{}')).toEqual({})
  })

  it('lists a todo checked this visit with the done todos of a folded card', async () => {
    const kept = todo(1, { is_done: true, completed_at: now.toISOString() })
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({ todos: [kept] }))))
    renderCards([{ task: task('local-WHF-001', { todo_count: 2, done_todo_count: 1 }), todos: [kept, todo(2)] }])

    expect(screen.queryByText('Todo 1')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: '1/2 done' }))
    expect(await screen.findByText('Todo 1')).toHaveClass('line-through')
  })

  it('unfolds a card when adding a todo to it', () => {
    const { setAdding } = renderCards([{ task: task('local-WHF-001'), todos: [todo(1), todo(2)] }])

    fireEvent.click(screen.getByRole('button', { name: 'Add todo to WHF-001' }))
    expect(setAdding).toHaveBeenLastCalledWith('local-WHF-001')
    expect(screen.getByText('Todo 2')).toBeInTheDocument()
    expect(JSON.parse(localStorage.getItem('wharf.cardOpen') ?? '{}')).toEqual({ 'local-WHF-001': true })
  })
})
