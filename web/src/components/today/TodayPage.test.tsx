import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { KeptTasksProvider } from '@/hooks/useKeptTasks'
import type { TaskListItem, Todo } from '@/lib/api'
import { TodayPage } from './TodayPage'

const task: TaskListItem = {
  id: 'local-WHF-003',
  originated: 'local',
  title: 'Draft the release notes',
  description: '',
  status: 'in-progress',
  start_date: null,
  due_date: null,
  todo_count: 0,
  done_todo_count: 0,
  position: null,
  closed_at: null,
  closed_since_last_sync: false,
}

function renderPage() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ tasks: [task], todos: [], requests: [], last_sync: null }))),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>
        <TodayPage />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const tabs = () => within(screen.getByRole('tablist', { name: 'Today layout' }))

afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.clear()
})

describe('TodayPage layout', () => {
  it('starts in the List view, with the layout tabs right after the title', async () => {
    renderPage()

    const header = screen.getByRole('banner')
    expect(screen.getByRole('heading', { name: 'Wharf' }).nextElementSibling).toBe(within(header).getByRole('tablist'))
    expect(tabs().getByRole('tab', { name: 'List' })).toHaveAttribute('aria-selected', 'true')
    expect(tabs().getByRole('tab', { name: 'Card' })).toHaveAttribute('aria-selected', 'false')
    expect(await screen.findByRole('region', { name: 'Todos' })).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Tasks' })).toBeNull()
  })

  it('switches to the Card view and back, and remembers the choice', async () => {
    const user = userEvent.setup()
    const { unmount } = renderPage()

    await user.click(tabs().getByRole('tab', { name: 'Card' }))
    expect(tabs().getByRole('tab', { name: 'Card' })).toHaveAttribute('aria-selected', 'true')
    expect(await screen.findByRole('article', { name: 'WHF-003 Draft the release notes' })).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Todos' })).toBeNull()
    expect(localStorage.getItem('wharf.layout')).toBe('card')

    unmount()
    renderPage()
    expect(tabs().getByRole('tab', { name: 'Card' })).toHaveAttribute('aria-selected', 'true')
    expect(await screen.findByRole('region', { name: 'Tasks' })).toBeInTheDocument()

    await user.click(tabs().getByRole('tab', { name: 'List' }))
    expect(await screen.findByRole('region', { name: 'Todos' })).toBeInTheDocument()
    expect(localStorage.getItem('wharf.layout')).toBe('list')
  })

  it('moves between the layout tabs with the arrow keys', async () => {
    const user = userEvent.setup()
    renderPage()

    const list = tabs().getByRole('tab', { name: 'List' })
    const card = tabs().getByRole('tab', { name: 'Card' })
    expect(list).toHaveAttribute('tabindex', '0')
    expect(card).toHaveAttribute('tabindex', '-1')

    list.focus()
    await user.keyboard('{ArrowRight}')
    expect(card).toHaveAttribute('aria-selected', 'true')
    expect(card).toHaveFocus()
    expect(localStorage.getItem('wharf.layout')).toBe('card')

    await user.keyboard('{ArrowLeft}')
    expect(list).toHaveAttribute('aria-selected', 'true')
    expect(list).toHaveFocus()
  })

  it('switches to the Todo view, remembers it, and moves to it with the arrow keys', async () => {
    const user = userEvent.setup()
    const { unmount } = renderPage()

    await user.click(tabs().getByRole('tab', { name: 'Todo' }))
    expect(tabs().getByRole('tab', { name: 'Todo' })).toHaveAttribute('aria-selected', 'true')
    expect(await screen.findByRole('region', { name: 'Todo board' })).toBeInTheDocument()
    expect(screen.queryByRole('region', { name: 'Todos' })).toBeNull()
    expect(localStorage.getItem('wharf.layout')).toBe('todo')

    unmount()
    renderPage()
    expect(await screen.findByRole('region', { name: 'Todo board' })).toBeInTheDocument()

    const todo = tabs().getByRole('tab', { name: 'Todo' })
    todo.focus()
    await user.keyboard('{ArrowRight}')
    expect(tabs().getByRole('tab', { name: 'List' })).toHaveAttribute('aria-selected', 'true')
    await user.keyboard('{ArrowLeft}')
    expect(todo).toHaveAttribute('aria-selected', 'true')
    expect(todo).toHaveFocus()
  })

  it('falls back to the List view for an unknown saved layout', () => {
    localStorage.setItem('wharf.layout', 'grid')
    renderPage()

    expect(tabs().getByRole('tab', { name: 'List' })).toHaveAttribute('aria-selected', 'true')
  })
})

describe('TodayPage closed tasks', () => {
  const at = (id: string, status: TaskListItem['status'], title: string, todoCount = 0): TaskListItem => ({
    ...task,
    id,
    status,
    title,
    todo_count: todoCount,
  })
  const openTodo = (id: number, taskID: string): Todo => ({
    id,
    task_id: taskID,
    description: `todo ${id}`,
    note: '',
    is_done: false,
    completed_at: null,
    start_date: null,
    end_date: null,
    reminders: [],
    position: 0,
    service_desk_requests: [],
  })

  /** renderWith serves tasks and todos at url, and records each status change. */
  function renderWith(tasks: TaskListItem[], todos: Todo[], url = '/') {
    const puts: { url: string; body: unknown }[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          const body = JSON.parse(String(init.body)) as { status: TaskListItem['status'] }
          puts.push({ url, body })
          // Serve the stored status from now on, as the server would.
          tasks = tasks.map((t) => (url.startsWith(`/api/tasks/${t.id}`) ? { ...t, status: body.status } : t))
          return new Response(null, { status: 204 })
        }
        const detail = tasks.find((t) => url === `/api/tasks/${t.id}`)
        if (detail) return new Response(JSON.stringify({ ...detail, parent_id: null, children_ids: [], linked_ids: [], parent: null, children: [], linked: [] }))
        if (url.startsWith('/api/tasks')) return new Response(JSON.stringify({ tasks }))
        if (url.startsWith('/api/todos')) return new Response(JSON.stringify({ todos: url.includes('is_done=false') ? todos : [] }))
        return new Response(JSON.stringify({ requests: [], last_sync: null }))
      }),
    )
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={qc}>
        <MemoryRouter initialEntries={[url]}>
          <KeptTasksProvider>
            <TodayPage />
          </KeptTasksProvider>
        </MemoryRouter>
      </QueryClientProvider>,
    )
    return puts
  }

  it('hides done and canceled tasks even with open todos for today', async () => {
    renderWith(
      [at('local-WHF-001', 'todo', 'Open task'), at('local-WHF-002', 'done', 'Done task', 1), at('local-WHF-004', 'canceled', 'Canceled task', 1)],
      [openTodo(1, 'local-WHF-002'), openTodo(2, 'local-WHF-004')],
    )

    expect(await screen.findByText('Open task')).toBeInTheDocument()
    expect(screen.queryByText('Done task')).toBeNull()
    expect(screen.queryByText('Canceled task')).toBeNull()
    expect(screen.queryByText('todo 1')).toBeNull()
    expect(screen.queryByText('todo 2')).toBeNull()
  })

  it('asks before canceling a task with todos, and sends nothing when kept', async () => {
    const user = userEvent.setup()
    const puts = renderWith([at('local-WHF-001', 'todo', 'Open task', 2)], [openTodo(1, 'local-WHF-001')])

    await user.click(await screen.findByRole('button', { name: 'Status: Todo' }))
    await user.click(screen.getByRole('menuitem', { name: /Canceled/ }))
    const dialog = await screen.findByRole('dialog', { name: 'Cancel WHF-001?' })
    expect(dialog).toHaveTextContent('Canceling deletes its 2 todos.')

    await user.click(within(dialog).getByRole('button', { name: 'Keep task' }))
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(puts).toHaveLength(0)

    await user.click(screen.getByRole('button', { name: 'Status: Todo' }))
    await user.click(screen.getByRole('menuitem', { name: /Canceled/ }))
    await user.click(await screen.findByRole('button', { name: 'Cancel and delete todos' }))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(puts[0]).toEqual({ url: '/api/tasks/local-WHF-001/status', body: { status: 'canceled' } })
    // A task closed in this visit stays until the next one.
    expect(screen.getByText('Open task')).toBeInTheDocument()
  })

  it('cancels a task without todos without asking', async () => {
    const user = userEvent.setup()
    const puts = renderWith([at('local-WHF-001', 'todo', 'Empty task')], [])

    await user.click(await screen.findByRole('button', { name: 'Status: Todo' }))
    await user.click(screen.getByRole('menuitem', { name: /Canceled/ }))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(puts[0].body).toEqual({ status: 'canceled' })
  })

  it('closes a task with todos as done without asking', async () => {
    const user = userEvent.setup()
    const puts = renderWith([at('local-WHF-001', 'todo', 'Busy task', 1)], [openTodo(1, 'local-WHF-001')])

    await user.click(await screen.findByRole('button', { name: 'Status: Todo' }))
    await user.click(screen.getByRole('menuitem', { name: /Done/ }))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(puts[0].body).toEqual({ status: 'done' })
  })

  it('keeps a task closed from the inspector on Today for this visit', async () => {
    const user = userEvent.setup()
    Element.prototype.hasPointerCapture ??= () => false
    Element.prototype.scrollIntoView ??= () => {}
    const puts = renderWith([at('local-WHF-001', 'todo', 'Open task')], [], '/?task=local-WHF-001')

    const inspector = await screen.findByRole('complementary', { name: 'Inspector' })
    await user.click(await within(inspector).findByRole('combobox', { name: 'Status' }))
    await user.click(await screen.findByRole('option', { name: 'Done' }))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(within(screen.getByRole('region', { name: 'Todos' })).getByText('Open task')).toBeInTheDocument()
  })

})
