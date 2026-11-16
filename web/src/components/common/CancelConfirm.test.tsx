import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { TaskDialog } from '@/components/task/TaskDialog'
import { TaskInspector } from '@/components/today/TaskInspector'
import { keys } from '@/hooks/data'
import type { TaskDetail, TaskListItem } from '@/lib/api'

const detail: TaskDetail = {
  id: 'local-WHF-001',
  originated: 'local',
  title: 'Draft the release notes',
  description: '',
  status: 'todo',
  start_date: null,
  due_date: null,
  parent_id: null,
  children_ids: [],
  linked_ids: [],
  parent: null,
  children: [],
  linked: [],
}

const listed = (todoCount: number): TaskListItem => ({
  ...detail,
  todo_count: todoCount,
  done_todo_count: 0,
  position: null,
  closed_at: null,
  closed_since_last_sync: false,
})

beforeAll(() => {
  // Radix Select uses pointer capture and scrolling, which jsdom lacks.
  Element.prototype.hasPointerCapture ??= () => false
  Element.prototype.releasePointerCapture ??= () => {}
  Element.prototype.scrollIntoView ??= () => {}
})

afterEach(() => vi.unstubAllGlobals())

/** renderWith shows ui for a task with todoCount todos and records each PUT. */
function renderWith(ui: React.ReactNode, todoCount: number) {
  const puts: { url: string; body: { status?: string } }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === 'PUT') {
        puts.push({ url, body: JSON.parse(String(init.body)) as { status?: string } })
        return new Response(null, { status: 204 })
      }
      if (url === '/api/tasks') return new Response(JSON.stringify({ tasks: [listed(todoCount)] }))
      if (url.startsWith('/api/tasks/')) return new Response(JSON.stringify(detail))
      if (url.startsWith('/api/todos')) return new Response(JSON.stringify({ todos: [] }))
      return new Response(JSON.stringify({}))
    }),
  )
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  qc.setQueryData(keys.tasks, [listed(todoCount)])
  render(
    <QueryClientProvider client={qc}>
      <MemoryRouter>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )
  return puts
}

async function pickStatus(user: ReturnType<typeof userEvent.setup>, label: string) {
  await user.click(await screen.findByRole('combobox', { name: 'Status' }))
  await user.click(await screen.findByRole('option', { name: label }))
}

const inspector = <TaskInspector id={detail.id} onClose={() => {}} onOpen={() => {}} onSelectTask={() => {}} />
const taskDialog = <TaskDialog id={detail.id} onClose={() => {}} onOpen={() => {}} />

describe('canceling from the Task inspector', () => {
  it('asks first when the task has todos, and sends nothing when kept', async () => {
    const user = userEvent.setup()
    const puts = renderWith(inspector, 3)

    await pickStatus(user, 'Canceled')
    const dialog = await screen.findByRole('dialog', { name: 'Cancel WHF-001?' })
    expect(dialog).toHaveTextContent('Canceling deletes its 3 todos.')
    await user.click(within(dialog).getByRole('button', { name: 'Keep task' }))
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(puts).toHaveLength(0)

    await pickStatus(user, 'Canceled')
    await user.click(await screen.findByRole('button', { name: 'Cancel and delete todos' }))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(puts[0].url).toBe('/api/tasks/local-WHF-001')
    expect(puts[0].body.status).toBe('canceled')
  })

  it('cancels a task without todos at once', async () => {
    const user = userEvent.setup()
    const puts = renderWith(inspector, 0)

    await pickStatus(user, 'Canceled')
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(screen.queryByRole('dialog')).toBeNull()
    expect(puts[0].body.status).toBe('canceled')
  })
})

describe('canceling from the task detail', () => {
  it('asks on save when the edit cancels a task with todos', async () => {
    const user = userEvent.setup()
    const puts = renderWith(taskDialog, 1)

    await user.click(await screen.findByRole('button', { name: 'Edit' }))
    await pickStatus(user, 'Canceled')
    await user.click(screen.getByRole('button', { name: /Save changes/ }))
    const confirm = await screen.findByRole('dialog', { name: 'Cancel WHF-001?' })
    expect(confirm).toHaveTextContent('Canceling deletes its 1 todo.')

    await user.click(within(confirm).getByRole('button', { name: 'Keep task' }))
    expect(screen.queryByRole('dialog', { name: 'Cancel WHF-001?' })).toBeNull()
    expect(puts).toHaveLength(0)
    // The edit stays open to change or save again.
    expect(screen.getByRole('button', { name: /Save changes/ })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /Save changes/ }))
    await user.click(await screen.findByRole('button', { name: 'Cancel and delete todos' }))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(puts[0].body.status).toBe('canceled')
  })

  it('saves other edits without asking', async () => {
    const user = userEvent.setup()
    const puts = renderWith(taskDialog, 1)

    await user.click(await screen.findByRole('button', { name: 'Edit' }))
    await pickStatus(user, 'Done')
    await user.click(screen.getByRole('button', { name: /Save changes/ }))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(screen.queryByRole('dialog', { name: 'Cancel WHF-001?' })).toBeNull()
    expect(puts[0].body.status).toBe('done')
  })
})
