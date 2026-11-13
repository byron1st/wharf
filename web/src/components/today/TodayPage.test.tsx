import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { TaskListItem } from '@/lib/api'
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

  it('falls back to the List view for an unknown saved layout', () => {
    localStorage.setItem('wharf.layout', 'grid')
    renderPage()

    expect(tabs().getByRole('tab', { name: 'List' })).toHaveAttribute('aria-selected', 'true')
  })
})
