import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ClockContext } from '@/hooks/useNow'
import type { TaskListItem } from '@/lib/api'
import { atLocal } from '@/lib/dates'
import { ClosedTasksDialog, NO_CLOSED_FILTERS } from './ClosedTasksDialog'

// The design's clock: Monday, October 26, 2026.
const today = '2026-10-26'
const now = atLocal(today, '14:07')
const at = (day: string) => atLocal(day, '12:00').toISOString()

const task = (id: string, over: Partial<TaskListItem>): TaskListItem => ({
  id,
  originated: id.startsWith('jira-') ? 'jira' : 'local',
  title: `Title of ${id}`,
  description: '',
  status: 'done',
  start_date: null,
  due_date: null,
  todo_count: 0,
  done_todo_count: 0,
  position: null,
  closed_at: null,
  closed_since_last_sync: false,
  ...over,
})

const fixtures = (): TaskListItem[] => [
  task('jira-SEC-139', {
    title: '감사 로그 보존 기간 정책 정리',
    closed_at: at('2026-10-26'),
    closed_since_last_sync: true,
    todo_count: 3,
    done_todo_count: 3,
  }),
  task('local-WHF-002', { title: '결제 모듈 설계 문서 리뷰 반영', closed_at: at('2026-10-23'), todo_count: 4, done_todo_count: 4 }),
  task('local-WHF-005', { title: '사내 위키 결제 섹션 이전', status: 'canceled', closed_at: at('2026-10-21'), todo_count: 3, done_todo_count: 1 }),
  task('jira-SEC-127', { title: '레거시 서명 키 폐기', status: 'canceled', closed_at: at('2026-10-15') }),
  task('local-WHF-001', { title: '결제 실패 알림 대시보드', closed_at: null }),
  task('local-WHF-007', { title: 'Still open', status: 'in-progress' }),
]

/** serve stubs the server: GET /api/tasks returns tasks, and status PUTs change them as the server would. */
function serve(tasks: TaskListItem[], failPuts = false) {
  const puts: { url: string; body: unknown }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (init?.method === 'PUT') {
        const body = JSON.parse(String(init.body)) as { status: TaskListItem['status']; closed_at?: string }
        puts.push({ url, body })
        if (failPuts) return new Response(JSON.stringify({ error: 'disk failed' }), { status: 500 })
        const id = decodeURIComponent(url.split('/')[3])
        const t = tasks.find((x) => x.id === id)!
        const closed = body.status === 'done' || body.status === 'canceled'
        t.closed_at = closed ? (body.closed_at ?? new Date().toISOString()) : null
        t.status = body.status
        return new Response(null, { status: 204 })
      }
      return new Response(JSON.stringify({ tasks }))
    }),
  )
  return puts
}

function renderDialog(tasks = fixtures(), failPuts = false) {
  const puts = serve(tasks, failPuts)
  const props = { onClose: vi.fn(), onOpen: vi.fn() }
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  function Harness() {
    const [filters, setFilters] = useState(NO_CLOSED_FILTERS)
    return <ClosedTasksDialog filters={filters} onFilters={setFilters} {...props} />
  }
  render(
    <QueryClientProvider client={qc}>
      <ClockContext.Provider value={{ now, today }}>
        <Harness />
      </ClockContext.Provider>
    </QueryClientProvider>,
  )
  return { puts, ...props }
}

/** sections lists each visible group with the keys of its rows, in order. */
function sections() {
  return screen.queryAllByRole('region').map((s) => ({
    label: within(s).getByRole('heading').textContent,
    keys: within(s)
      .queryAllByRole('listitem')
      .map((li) => li.querySelector('.font-mono')?.textContent),
  }))
}

const tab = (name: RegExp) => screen.getByRole('tab', { name })

afterEach(() => vi.unstubAllGlobals())

describe('ClosedTasksDialog', () => {
  it('groups closed tasks by the week they closed, latest first, unknown times last', async () => {
    renderDialog()
    expect(await screen.findByRole('dialog', { name: 'Closed tasks' })).toBeInTheDocument()
    await waitFor(() => expect(sections()).toHaveLength(3))
    expect(sections()).toEqual([
      { label: 'This week1', keys: ['SEC-139'] },
      { label: 'Last week2', keys: ['WHF-002', 'WHF-005'] },
      { label: 'Earlier2', keys: ['SEC-127', 'WHF-001'] },
    ])
  })

  it('shows status, closing day, todo progress and the sync tag on each row', async () => {
    renderDialog()
    const jira = (await screen.findByText('SEC-139')).closest('li')!
    expect(within(jira).getByText('Closed today in Jira')).toBeInTheDocument()
    expect(within(jira).getByText('3/3 todos')).toBeInTheDocument()
    expect(within(jira).getByText('Closed since last sync')).toBeInTheDocument()
    expect(within(jira).getByRole('img', { name: 'Done' })).toBeInTheDocument()

    const canceled = screen.getByText('WHF-005').closest('li')!
    expect(within(canceled).getByText('Canceled')).toBeInTheDocument()
    expect(within(canceled).getByText('Closed Oct 21')).toBeInTheDocument()
    expect(within(canceled).getByText('1/3 todos')).toBeInTheDocument()
    expect(within(canceled).getByText('사내 위키 결제 섹션 이전')).toHaveClass('line-through')
    expect(within(canceled).queryByText('Closed since last sync')).not.toBeInTheDocument()

    expect(within(screen.getByText('WHF-001').closest('li')!).getByText('Closed')).toBeInTheDocument()
    expect(screen.queryByText('Still open')).not.toBeInTheDocument()
  })

  it('locks Jira rows and offers Reopen only on local rows', async () => {
    renderDialog()
    const jira = (await screen.findByText('SEC-127')).closest('li')!
    expect(within(jira).getByText('Closed in Jira')).toBeInTheDocument()
    expect(within(jira).queryByRole('button', { name: /reopen/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Reopen WHF-002' })).toBeInTheDocument()
  })

  it('searches by key or title and counts each status within the source', async () => {
    const user = userEvent.setup()
    renderDialog()
    await screen.findByText('SEC-139')
    expect(tab(/^All/)).toHaveTextContent('All5')
    expect(tab(/^Done/)).toHaveTextContent('Done3')
    expect(tab(/^Canceled/)).toHaveTextContent('Canceled2')

    await user.type(screen.getByRole('textbox', { name: 'Search closed tasks' }), 'sec-1')
    expect(sections().flatMap((s) => s.keys)).toEqual(['SEC-139', 'SEC-127'])
    await user.clear(screen.getByRole('textbox', { name: 'Search closed tasks' }))
    await user.type(screen.getByRole('textbox', { name: 'Search closed tasks' }), '결제')
    expect(sections().flatMap((s) => s.keys)).toEqual(['WHF-002', 'WHF-005', 'WHF-001'])
    await user.clear(screen.getByRole('textbox', { name: 'Search closed tasks' }))

    await user.click(screen.getByRole('button', { name: 'Jira' }))
    expect(screen.getByRole('button', { name: 'Jira' })).toHaveAttribute('aria-pressed', 'true')
    expect(tab(/^All/)).toHaveTextContent('All2')
    expect(tab(/^Done/)).toHaveTextContent('Done1')

    await user.click(tab(/^Canceled/))
    expect(tab(/^Canceled/)).toHaveAttribute('aria-selected', 'true')
    expect(sections().flatMap((s) => s.keys)).toEqual(['SEC-127'])

    await user.click(screen.getByRole('button', { name: 'Local' }))
    expect(sections().flatMap((s) => s.keys)).toEqual(['WHF-005'])
  })

  it('says so when nothing matches', async () => {
    const user = userEvent.setup()
    renderDialog()
    await screen.findByText('SEC-139')
    await user.type(screen.getByRole('textbox', { name: 'Search closed tasks' }), 'nothing like this')
    expect(screen.getByText('No closed tasks here')).toBeInTheDocument()
    expect(screen.getByText('Try another status or source.')).toBeInTheDocument()
    expect(sections()).toEqual([])
  })

  it('reopens a local task as Todo and undoes it with its closing time', async () => {
    const user = userEvent.setup()
    const { puts } = renderDialog()
    await user.click(await screen.findByRole('button', { name: 'Reopen WHF-005' }))

    expect(screen.getByRole('status')).toHaveTextContent('WHF-005 reopened as Todo. It’s back on Today.')
    expect(screen.queryByText('WHF-005')).not.toBeInTheDocument()
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(puts[0]).toEqual({ url: '/api/tasks/local-WHF-005/status', body: { status: 'todo' } })

    await user.click(screen.getByRole('button', { name: 'Undo' }))
    await waitFor(() => expect(puts).toHaveLength(2))
    expect(puts[1]).toEqual({
      url: '/api/tasks/local-WHF-005/status',
      body: { status: 'canceled', closed_at: at('2026-10-21') },
    })
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    await waitFor(() => expect(sections()[1]).toEqual({ label: 'Last week2', keys: ['WHF-002', 'WHF-005'] }))
  })

  it('keeps the row and shows no notice when the reopen fails', async () => {
    const user = userEvent.setup()
    const { puts } = renderDialog(fixtures(), true)
    await user.click(await screen.findByRole('button', { name: 'Reopen WHF-005' }))
    await waitFor(() => expect(puts).toHaveLength(1))
    expect(await screen.findByText('WHF-005')).toBeInTheDocument()
    expect(screen.queryByText(/reopened as Todo/)).not.toBeInTheDocument()
  })

  it('opens a row’s task', async () => {
    const user = userEvent.setup()
    const { onOpen } = renderDialog()
    await user.click(await screen.findByText('레거시 서명 키 폐기'))
    expect(onOpen).toHaveBeenCalledWith('jira-SEC-127')
  })

  it('closes on Escape and from the close button', async () => {
    const user = userEvent.setup()
    const { onClose } = renderDialog()
    await screen.findByText('SEC-139')
    await user.keyboard('{Escape}')
    expect(onClose).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('button', { name: 'Close' }))
    expect(onClose).toHaveBeenCalledTimes(2)
  })
})
