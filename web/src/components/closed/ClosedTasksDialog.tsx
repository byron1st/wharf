import { Dialog as D } from 'radix-ui'
import { Lock, RotateCcw, Search, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { OriginDot, StatusIcon } from '@/components/common/badges'
import { Button } from '@/components/ui/button'
import { useTaskActions, useTasks } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { taskKey, type TaskListItem, type TaskStatus } from '@/lib/api'
import { closedGroup, closedLabel, type ClosedGroup } from '@/lib/dates'
import { cn } from '@/lib/utils'

export type ClosedStatus = 'all' | 'done' | 'canceled'
export type ClosedSource = 'all' | 'local' | 'jira'

/** ClosedFilters are the dialog's search and filters; App keeps them while a task opened from the list is shown. */
export interface ClosedFilters {
  query: string
  status: ClosedStatus
  source: ClosedSource
}

export const NO_CLOSED_FILTERS: ClosedFilters = { query: '', status: 'all', source: 'all' }

const STATUS_TABS: { id: ClosedStatus; label: string }[] = [
  { id: 'all', label: 'All' },
  { id: 'done', label: 'Done' },
  { id: 'canceled', label: 'Canceled' },
]

const SOURCES: { id: ClosedSource; label: string }[] = [
  { id: 'all', label: 'All sources' },
  { id: 'local', label: 'Local' },
  { id: 'jira', label: 'Jira' },
]

const GROUPS: { id: ClosedGroup; label: string }[] = [
  { id: 'this-week', label: 'This week' },
  { id: 'last-week', label: 'Last week' },
  { id: 'earlier', label: 'Earlier' },
]

/** Reopened is the local task last reopened from the list, with what Undo puts back. */
interface Reopened {
  id: string
  status: TaskStatus
  closedAt: string | null
}

const isClosed = (t: TaskListItem) => t.status === 'done' || t.status === 'canceled'

/** byClosedAt orders the latest closing first, unknown times last, then by ID. */
function byClosedAt(a: TaskListItem, b: TaskListItem): number {
  if (a.closed_at !== b.closed_at) {
    if (!a.closed_at) return 1
    if (!b.closed_at) return -1
    return new Date(b.closed_at).getTime() - new Date(a.closed_at).getTime()
  }
  return a.id.localeCompare(b.id)
}

function ClosedRow({ task, today, onOpen, onReopen }: { task: TaskListItem; today: string; onOpen: () => void; onReopen: () => void }) {
  const jira = task.originated === 'jira'
  const done = task.status === 'done'
  const key = taskKey(task.id)
  return (
    <li className="grid grid-cols-[20px_minmax(0,1fr)_auto] items-start gap-3 border-t border-border-subtle px-6 py-3">
      <span className="mt-px" role="img" aria-label={done ? 'Done' : 'Canceled'}>
        <StatusIcon status={task.status} size={16} />
      </span>
      <button type="button" onClick={onOpen} className="flex min-w-0 flex-col gap-1 text-left">
        <span className="flex flex-wrap items-center gap-2">
          <span className="inline-flex items-center gap-1.5">
            <OriginDot origin={task.originated} className="size-2" />
            <span className="font-mono text-[12px] text-ink-secondary">{key}</span>
          </span>
          <span
            className={cn(
              'text-[14px] leading-[1.45] font-medium break-keep [overflow-wrap:anywhere]',
              done ? 'text-ink-secondary' : 'text-ink-muted line-through',
            )}
          >
            {task.title}
          </span>
        </span>
        <span className="flex flex-wrap items-center gap-1.5 text-[12px] text-ink-muted">
          <span className={cn('font-medium', done && 'text-tag-violet-ink')}>{done ? 'Done' : 'Canceled'}</span>
          <span aria-hidden>·</span>
          <span className="tabular-nums">{closedLabel(task.closed_at, today, jira)}</span>
          <span aria-hidden>·</span>
          <span className="tabular-nums">
            {task.done_todo_count}/{task.todo_count} todos
          </span>
          {task.closed_since_last_sync && (
            <span className="inline-flex h-[18px] items-center rounded-full bg-tag-blue-bg px-1.5 text-[11px] font-medium text-tag-blue-ink">
              Closed since last sync
            </span>
          )}
        </span>
      </button>
      <span className="flex items-center gap-1.5">
        {jira ? (
          <span title="Status comes from Jira" className="inline-flex h-7 items-center gap-1.5 px-2 text-[12px] text-ink-muted">
            <Lock size={12} />
            Closed in Jira
          </span>
        ) : (
          <Button
            variant="outline"
            size="xs"
            title="Move back to Todo and show it on Today"
            aria-label={`Reopen ${key}`}
            onClick={onReopen}
          >
            <RotateCcw size={13} strokeWidth={2.2} />
            Reopen
          </Button>
        )}
      </span>
    </li>
  )
}

/**
 * ClosedTasksDialog is the archive at /closed: done and canceled tasks grouped
 * by when they closed, with search, status and source filters, and Reopen
 * (with Undo) for local tasks. Jira tasks follow Jira and are read-only.
 */
export function ClosedTasksDialog({
  filters,
  onFilters,
  onClose,
  onOpen,
}: {
  filters: ClosedFilters
  onFilters: (f: ClosedFilters) => void
  onClose: () => void
  onOpen: (id: string) => void
}) {
  const { today } = useNow()
  const { data: tasks = [] } = useTasks()
  const { setStatus } = useTaskActions()
  const [reopened, setReopened] = useState<Reopened | null>(null)
  const set = (patch: Partial<ClosedFilters>) => onFilters({ ...filters, ...patch })

  const closed = useMemo(() => tasks.filter(isClosed).sort(byClosedAt), [tasks])
  const q = filters.query.trim().toLowerCase()
  const inSource = closed.filter(
    (t) =>
      (filters.source === 'all' || t.originated === filters.source) &&
      (!q || taskKey(t.id).toLowerCase().includes(q) || t.title.toLowerCase().includes(q)),
  )
  const count = (s: ClosedStatus) => inSource.filter((t) => s === 'all' || t.status === s).length
  const shown = inSource.filter((t) => filters.status === 'all' || t.status === filters.status)
  const sections = GROUPS.map((g) => ({ ...g, rows: shown.filter((t) => closedGroup(t.closed_at, today) === g.id) })).filter(
    (g) => g.rows.length > 0,
  )

  const reopen = (t: TaskListItem) => {
    setReopened({ id: t.id, status: t.status, closedAt: t.closed_at })
    setStatus(t, 'todo')
  }

  const undo = () => {
    if (!reopened) return
    setStatus({ id: reopened.id }, reopened.status, reopened.closedAt ?? undefined)
    setReopened(null)
  }

  return (
    <D.Root open onOpenChange={(o) => !o && onClose()}>
      <D.Portal>
        <D.Overlay className="fixed inset-0 z-40 flex items-start justify-center bg-[rgba(9,9,11,0.4)] px-6 pt-14">
          <D.Content
            onOpenAutoFocus={(e) => e.preventDefault()}
            className="flex h-[min(800px,calc(100vh-112px))] w-full max-w-[860px] flex-col overflow-hidden rounded-[14px] border border-border bg-white shadow-xl outline-none"
          >
            <header className="flex items-start gap-3 border-b border-border-subtle pt-[18px] pr-5 pb-3.5 pl-6">
              <div className="flex min-w-0 flex-col gap-1">
                <D.Title className="m-0 text-[17px] font-semibold tracking-[-0.01em]">Closed tasks</D.Title>
                <D.Description className="m-0 text-[13px] leading-normal text-ink-muted">
                  Done and canceled tasks. A Jira issue shows up here once it closes in Jira after Wharf imported it.
                </D.Description>
              </div>
              <D.Close
                aria-label="Close"
                title="Close (Esc)"
                className="ml-auto flex size-8 shrink-0 items-center justify-center rounded-md text-ink-secondary hover:bg-bg-muted"
              >
                <X size={18} />
              </D.Close>
            </header>

            <div className="flex flex-wrap items-center gap-2.5 border-b border-border-subtle px-6 py-3">
              <label className="flex h-8 min-w-0 flex-[1_1_220px] items-center gap-2 rounded-md border border-border bg-white px-2.5 shadow-xs focus-within:border-accent">
                <Search size={14} className="shrink-0 text-ink-muted" aria-hidden />
                <span className="sr-only">Search closed tasks</span>
                <input
                  type="text"
                  value={filters.query}
                  onChange={(e) => set({ query: e.target.value })}
                  placeholder="Search by key or title"
                  className="min-w-0 flex-1 border-0 bg-transparent p-0 text-[13px] text-ink outline-none placeholder:text-ink-faint"
                />
              </label>
              <div role="tablist" aria-label="Status" className="inline-flex items-center gap-0.5 rounded-lg border border-border bg-bg-muted p-0.5">
                {STATUS_TABS.map((s) => {
                  const on = filters.status === s.id
                  return (
                    <button
                      key={s.id}
                      type="button"
                      role="tab"
                      aria-selected={on}
                      onClick={() => set({ status: s.id })}
                      className={cn(
                        'flex h-[26px] items-center gap-1.5 rounded-md px-2.5 text-[12px] font-medium',
                        on ? 'bg-white text-ink shadow-xs' : 'text-ink-muted',
                      )}
                    >
                      {s.label}
                      <span className={cn('tabular-nums', on ? 'text-ink-muted' : 'text-ink-faint')}>{count(s.id)}</span>
                    </button>
                  )
                })}
              </div>
              <div role="group" aria-label="Source" className="inline-flex items-center gap-1.5">
                {SOURCES.map((s) => {
                  const on = filters.source === s.id
                  return (
                    <button
                      key={s.id}
                      type="button"
                      aria-pressed={on}
                      onClick={() => set({ source: s.id })}
                      className={cn(
                        'flex h-7 items-center gap-1.5 rounded-full border px-2.5 text-[12px] font-medium',
                        on ? 'border-accent bg-accent-tint text-accent-ink' : 'border-border bg-white text-ink-secondary',
                      )}
                    >
                      {s.id !== 'all' && <OriginDot origin={s.id} className="size-2" />}
                      {s.label}
                    </button>
                  )
                })}
              </div>
            </div>

            <div className="min-h-0 flex-1 overflow-y-auto pt-2 pb-5">
              {reopened && (
                <div
                  role="status"
                  className="mx-6 mt-2 mb-1 flex flex-wrap items-center gap-2.5 rounded-lg border border-accent-line bg-accent-tint px-3 py-2.5 text-[13px] text-accent-ink"
                >
                  <RotateCcw size={15} strokeWidth={2.2} className="shrink-0" aria-hidden />
                  <span className="flex-[1_1_200px]">{taskKey(reopened.id)} reopened as Todo. It’s back on Today.</span>
                  <button
                    type="button"
                    onClick={undo}
                    className="h-7 rounded-md border border-accent-line bg-white px-2.5 text-[12px] font-medium text-accent-ink"
                  >
                    Undo
                  </button>
                </div>
              )}
              {sections.map((sec) => (
                <section key={sec.id} aria-label={sec.label} className="flex flex-col">
                  <h3 className="m-0 px-6 pt-3.5 pb-1.5 text-[12px] font-medium text-ink-muted">
                    {sec.label}
                    <span className="ml-1.5 text-ink-faint tabular-nums">{sec.rows.length}</span>
                  </h3>
                  <ul className="m-0 list-none p-0">
                    {sec.rows.map((t) => (
                      <ClosedRow key={t.id} task={t} today={today} onOpen={() => onOpen(t.id)} onReopen={() => reopen(t)} />
                    ))}
                  </ul>
                </section>
              ))}
              {sections.length === 0 && (
                <div className="flex flex-col items-center gap-1.5 px-6 py-14 text-center">
                  <span className="text-[14px] font-medium">No closed tasks here</span>
                  <span className="text-[13px] text-ink-muted">Try another status or source.</span>
                </div>
              )}
            </div>
          </D.Content>
        </D.Overlay>
      </D.Portal>
    </D.Root>
  )
}
