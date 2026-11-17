import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import type { TaskDetail, TaskListItem } from '@/lib/api'
import { App } from './App'

const closed: TaskListItem = {
  id: 'local-WHF-002',
  originated: 'local',
  title: 'Review the payment design',
  description: '',
  status: 'done',
  start_date: null,
  due_date: null,
  todo_count: 0,
  done_todo_count: 0,
  position: null,
  closed_at: new Date().toISOString(),
  closed_since_last_sync: false,
}

const detail: TaskDetail = {
  ...closed,
  parent_id: null,
  children_ids: [],
  linked_ids: [],
  parent: null,
  children: [],
  linked: [],
}

/** renderApp serves one closed task and empty lists for everything else. */
function renderApp() {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string) => {
      if (url === `/api/tasks/${closed.id}`) return new Response(JSON.stringify(detail))
      return new Response(JSON.stringify({ tasks: [closed], todos: [], requests: [], last_sync: null }))
    }),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={['/']}>
        <App />
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

beforeAll(() => {
  // Radix Select uses pointer capture and scrolling, which jsdom lacks.
  Element.prototype.hasPointerCapture ??= () => false
  Element.prototype.releasePointerCapture ??= () => {}
  Element.prototype.scrollIntoView ??= () => {}
})

afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.clear()
})

describe('App', () => {
  it('opens Closed tasks from the header and returns to it from a task opened there', async () => {
    const user = userEvent.setup()
    renderApp()

    const header = screen.getByRole('banner')
    const buttons = within(header).getAllByRole('button')
    const archive = within(header).getByRole('button', { name: 'Closed tasks' })
    expect(buttons.indexOf(archive)).toBe(buttons.indexOf(within(header).getByRole('button', { name: 'Settings' })) - 1)

    await user.click(archive)
    const dialog = await screen.findByRole('dialog', { name: 'Closed tasks' })
    await user.click(await within(dialog).findByText('Review the payment design'))

    expect(screen.queryByRole('dialog', { name: 'Closed tasks' })).not.toBeInTheDocument()
    expect(await screen.findByRole('dialog', { name: /Review the payment design/ })).toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(await screen.findByRole('dialog', { name: 'Closed tasks' })).toBeInTheDocument()

    await user.keyboard('{Escape}')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('keeps a task closed in its detail on Today until the next visit', async () => {
    const user = userEvent.setup()
    let open: TaskListItem = { ...closed, id: 'local-WHF-003', title: 'Write the plan', status: 'todo', closed_at: null }
    const puts: string[] = []
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        if (init?.method === 'PUT') {
          const body = JSON.parse(String(init.body)) as { status: TaskListItem['status'] }
          puts.push(body.status)
          open = { ...open, status: body.status }
          return new Response(null, { status: 204 })
        }
        if (url === `/api/tasks/${open.id}`) return new Response(JSON.stringify({ ...detail, ...open }))
        return new Response(JSON.stringify({ tasks: [open], todos: [], requests: [], last_sync: null }))
      }),
    )
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={qc}>
        <MemoryRouter initialEntries={[`/task/${open.id}`]}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    const dialog = await screen.findByRole('dialog', { name: /Write the plan/ })
    await user.click(within(dialog).getByRole('button', { name: 'Edit' }))
    await user.click(within(dialog).getByRole('combobox', { name: 'Status' }))
    await user.click(await screen.findByRole('option', { name: 'Done' }))
    await user.click(within(dialog).getByRole('button', { name: /Save changes/ }))
    await waitFor(() => expect(puts).toEqual(['done']))

    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(within(screen.getByRole('region', { name: 'Todos' })).getByText('Write the plan')).toBeInTheDocument()
  })

  it('opens a task from the Todo view over the board and returns to the board', async () => {
    const user = userEvent.setup()
    localStorage.setItem('wharf.layout', 'todo')
    const open: TaskListItem = { ...closed, id: 'local-WHF-003', title: 'Write the plan', status: 'todo', closed_at: null }
    const plan = {
      id: 1, task_id: open.id, description: 'Outline the plan', note: '', is_done: false, completed_at: null,
      start_date: null, end_date: null, reminders: [], position: 0, service_desk_requests: [],
    }
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string) => {
        if (url === `/api/tasks/${open.id}`) return new Response(JSON.stringify({ ...detail, ...open }))
        const todos = url.startsWith('/api/todos?is_done=false') ? [plan] : []
        return new Response(JSON.stringify({ tasks: [open], todos, requests: [], last_sync: null }))
      }),
    )
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={qc}>
        <MemoryRouter initialEntries={['/']}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>,
    )

    const board = await screen.findByRole('region', { name: 'Todo board' })
    await user.click(await within(board).findByTitle('WHF-003 · Write the plan — show task'))
    await user.click(within(await screen.findByRole('complementary', { name: 'Inspector' })).getByRole('button', { name: /^Open/ }))

    expect(await screen.findByRole('dialog', { name: /Write the plan/ })).toBeInTheDocument()
    expect(screen.getByRole('region', { name: 'Todo board', hidden: true })).toBeInTheDocument()

    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(within(screen.getByRole('region', { name: 'Todo board' })).getByText('Outline the plan')).toBeInTheDocument()
  })

})
