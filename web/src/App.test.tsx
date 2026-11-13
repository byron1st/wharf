import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
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

afterEach(() => vi.unstubAllGlobals())

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
})
