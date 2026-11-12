import { Dialog as D } from 'radix-ui'
import { Copy, ExternalLink, Lock, MoreHorizontal, Pencil, RefreshCw, Trash2, X } from 'lucide-react'
import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { OriginBadge, STATUS_LABEL, StatusBadge, StatusIcon } from '@/components/common/badges'
import { Relations } from '@/components/common/Relations'
import { Markdown } from '@/components/markdown/Markdown'
import { MarkdownEditor, MdIcon, ModeTabs } from '@/components/markdown/MarkdownEditor'
import { DateField } from '@/components/pickers/DateField'
import { dueTone, StatusSelect } from '@/components/today/TaskInspector'
import { Button } from '@/components/ui/button'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useAtlassianSettings, useTask, useTaskActions } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { useSyncTask } from '@/hooks/useSync'
import { taskKey, type TaskDetail, type TaskInput } from '@/lib/api'
import { jiraURL } from '@/lib/jira'
import { dayLabel, fromTaskDate, longDate, relDays, toTaskDate } from '@/lib/dates'
import { notify } from '@/lib/toast'
import { cn } from '@/lib/utils'
import { TaskTodos } from './TaskTodos'

function Card({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="flex flex-col gap-4 rounded-xl border border-border bg-white p-4 shadow-xs">
      <h2 className="m-0 text-[14px] font-semibold">{title}</h2>
      {children}
    </section>
  )
}

function Detail({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid min-h-8 grid-cols-[100px_minmax(0,1fr)] items-center text-[13px]">
      <span className="text-ink-muted">{label}</span>
      <div className="min-w-0">{children}</div>
    </div>
  )
}

type Draft = Pick<TaskInput, 'title' | 'description' | 'status' | 'start_date' | 'due_date' | 'parent_id' | 'children_ids' | 'linked_ids'>

function draftOf(t: TaskDetail): Draft {
  return {
    title: t.title,
    description: t.description,
    status: t.status,
    start_date: t.start_date,
    due_date: t.due_date,
    parent_id: t.parent_id ?? '',
    children_ids: t.children_ids,
    linked_ids: t.linked_ids,
  }
}

function Body({
  task,
  onClose,
  onOpen,
  onEditing,
}: {
  task: TaskDetail
  onClose: () => void
  onOpen: (id: string) => void
  /** onEditing hands over a cancel callback while editing, so Esc cancels the edit instead of closing. */
  onEditing: (cancel: (() => void) | null) => void
}) {
  const { today } = useNow()
  const { updateAsync, remove } = useTaskActions()
  const { data: atlassian } = useAtlassianSettings()
  const syncTask = useSyncTask()
  const local = task.originated === 'local'
  const [draft, setDraft] = useState<Draft | null>(null)
  const [mode, setMode] = useState<'preview' | 'write'>('write')
  const [saving, setSaving] = useState(false)
  const editing = local && draft !== null
  const view = draft ?? draftOf(task)
  const start = fromTaskDate(view.start_date)
  const due = fromTaskDate(view.due_date)
  const link = jiraURL(atlassian?.site_url, taskKey(task.id))
  const set = (patch: Partial<Draft>) => setDraft((d) => ({ ...(d ?? draftOf(task)), ...patch }))
  useEffect(() => onEditing(editing ? () => setDraft(null) : null), [editing, onEditing])

  const save = async () => {
    if (!draft || saving) return
    setSaving(true)
    try {
      await updateAsync(task, { ...draft, title: draft.title.trim() || task.title })
      setDraft(null)
    } catch {
      // updateAsync already undid the change and showed the toast.
    } finally {
      setSaving(false)
    }
  }

  useEffect(() => {
    if (!editing) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault()
        void save()
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  })

  const shown: TaskDetail = { ...task, ...view, parent_id: view.parent_id || null }

  return (
    <>
      <header className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex min-w-0 flex-col gap-2.5">
          <div className="flex items-center gap-2">
            <span className="[&>span]:h-[22px] [&>span]:px-2 [&>span]:text-[12px]">
              <OriginBadge origin={local ? 'local' : 'jira'} icon />
            </span>
            <span className="font-mono text-[13px] text-ink-secondary">{taskKey(task.id)}</span>
            {editing && (
              <span className="inline-flex h-[22px] items-center gap-1 rounded-full border border-[#fed7aa] bg-today-tint px-2 text-[12px] font-medium text-[#9a3412]">
                <Pencil size={11} />
                Editing
              </span>
            )}
            {!local && (
              <span className="inline-flex h-[22px] items-center gap-1 rounded-full bg-bg-muted px-2 text-[12px] font-medium text-ink-secondary">
                <Lock size={11} />
                Read-only
              </span>
            )}
            {local && (
              <button
                type="button"
                aria-label="Copy task ID"
                title="Copy task ID"
                onClick={() => {
                  void navigator.clipboard?.writeText(task.id)
                  notify.success({ title: 'Copied', description: task.id })
                }}
                className="flex size-[26px] items-center justify-center rounded-md text-ink-muted hover:bg-bg-muted"
              >
                <Copy size={13} />
              </button>
            )}
          </div>
          {editing ? (
            <input
              aria-label="Title"
              autoFocus
              value={view.title}
              onChange={(e) => set({ title: e.target.value })}
              className="h-11 min-w-[min(520px,100%)] rounded-lg border border-accent px-3 text-[22px] font-semibold tracking-[-0.01em] shadow-[0_0_0_3px_rgba(15,118,110,0.12)] outline-none"
            />
          ) : (
            <D.Title className="m-0 text-[28px] leading-[34px] font-semibold tracking-[-0.02em]">{task.title}</D.Title>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {editing ? (
            <>
              <Button variant="outline" onClick={() => setDraft(null)}>
                Cancel
              </Button>
              <Button onClick={() => void save()} disabled={saving} className="px-4">
                Save changes
                <kbd className="rounded bg-white/20 px-[5px] font-mono text-[11px]">⌘↵</kbd>
              </Button>
            </>
          ) : (
            <>
              <StatusBadge status={task.status} />
              {local && (
                <Button
                  variant="outline"
                  className="px-3.5"
                  onClick={() => {
                    setDraft(draftOf(task))
                    setMode('write')
                  }}
                >
                  <Pencil size={15} />
                  Edit
                </Button>
              )}
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="outline" size="icon" aria-label="More actions">
                    <MoreHorizontal size={16} />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent className="w-[290px]">
                  {local ? (
                    <DropdownMenuItem
                      className="text-danger"
                      onSelect={() => {
                        onClose()
                        void remove(task.id)
                      }}
                    >
                      <Trash2 size={15} className="mt-0.5" />
                      Delete task
                    </DropdownMenuItem>
                  ) : (
                    <>
                      <DropdownMenuItem disabled={!link} onSelect={() => link && window.open(link, '_blank', 'noopener')}>
                        <ExternalLink size={15} className="mt-0.5" />
                        <span className="flex min-w-0 flex-1 flex-col">
                          <span className="flex items-center">
                            Open in Jira<span className="ml-auto text-[11px] text-ink-muted">Browser</span>
                          </span>
                          <span className="font-mono text-[11px] text-ink-muted">
                            {link ? `…/browse/${taskKey(task.id)}` : 'Set the site URL in Settings'}
                          </span>
                        </span>
                      </DropdownMenuItem>
                      <DropdownMenuItem onSelect={() => syncTask(task.id, link)}>
                        <RefreshCw size={15} className="mt-0.5" />
                        <span className="flex flex-col">
                          Sync from Jira
                          <span className="text-[12px] text-ink-muted">
                            Pull the latest title, status, dates and description for this issue.
                          </span>
                        </span>
                      </DropdownMenuItem>
                    </>
                  )}
                </DropdownMenuContent>
              </DropdownMenu>
            </>
          )}
          <span aria-hidden className="mx-0.5 h-6 w-px bg-bg-pressed" />
          <button
            type="button"
            aria-label="Close task detail"
            title="Close (Esc)"
            onClick={onClose}
            className="flex size-8 items-center justify-center rounded-md text-ink-secondary hover:bg-bg-muted"
          >
            <X size={18} />
          </button>
        </div>
      </header>

      <div className="flex flex-wrap items-start gap-6">
        <div className="flex min-w-0 flex-[999_1_520px] flex-col gap-5">
          {!local && (
            <div role="note" className="flex gap-3 rounded-lg border border-[#bfdbfe] bg-tag-blue-bg px-4 py-3 text-[13px] text-[#1e40af]">
              <Lock size={16} className="mt-0.5 shrink-0" />
              <div>
                <p className="m-0 font-semibold">Managed in Jira</p>
                <p className="m-0 leading-normal">
                  Title, status, dates and relations are a snapshot from the last Jira sync and can’t be edited here. Until
                  you sync this issue from Jira, this copy stays as it is. Your todos below are kept only in Wharf.
                </p>
              </div>
            </div>
          )}
          <section aria-label="Description" className="flex flex-col gap-2.5">
            <div className="flex items-center gap-2">
              <h2 className="m-0 text-[13px] font-semibold text-ink-secondary">Description</h2>
              <MdIcon />
              {!local && <span className="text-[12px] text-ink-muted">Markdown from Jira</span>}
              {editing && (
                <span className="ml-auto">
                  <ModeTabs mode={mode} onMode={setMode} />
                </span>
              )}
            </div>
            {editing ? (
              <MarkdownEditor
                value={view.description}
                onSave={(description) => set({ description })}
                mode={mode}
                onMode={setMode}
                rows={12}
                emptyText="Add a description…"
              />
            ) : task.description.trim() ? (
              <Markdown source={task.description} />
            ) : (
              <span className="text-[13px] text-ink-faint">No description</span>
            )}
          </section>
          <TaskTodos taskID={task.id} jira={!local} />
        </div>

        <div className="flex min-w-0 flex-[1_1_280px] flex-col gap-4">
          <Card title="Details">
            <div className="flex flex-col gap-1">
              <Detail label="Status">
                {editing ? (
                  <StatusSelect value={view.status} onChange={(status) => set({ status })} />
                ) : (
                  <span className="flex items-center gap-1.5">
                    <StatusIcon status={task.status} />
                    {STATUS_LABEL[task.status]}
                  </span>
                )}
              </Detail>
              <Detail label="Start">
                {editing ? (
                  <DateField
                    title="Start date"
                    value={start}
                    other={due}
                    pairText={due ? `Due ${dayLabel(due)}` : undefined}
                    hi={due}
                    format={longDate}
                    align="end"
                    onChange={(v) => set({ start_date: toTaskDate(v) })}
                  />
                ) : start ? (
                  <span className="tabular-nums">{longDate(start)}</span>
                ) : (
                  <span className="text-ink-faint">Not set</span>
                )}
              </Detail>
              <Detail label="Due">
                {editing ? (
                  <DateField
                    title="Due date"
                    value={due}
                    other={start}
                    pairText={start ? `Starts ${dayLabel(start)}` : undefined}
                    lo={start}
                    tone={dueTone(due, today, view.status)}
                    format={longDate}
                    align="end"
                    onChange={(v) => set({ due_date: toTaskDate(v) })}
                  />
                ) : due ? (
                  <span
                    className={cn(
                      'tabular-nums',
                      dueTone(due, today, task.status) === 'over' && 'text-danger-ink',
                      dueTone(due, today, task.status) === 'today' && 'text-[#9a3412]',
                    )}
                  >
                    {longDate(due)}
                    <span className="text-ink-muted"> · {relDays(due, today)}</span>
                  </span>
                ) : (
                  <span className="text-ink-faint">Not set</span>
                )}
              </Detail>
              <Detail label="ID">
                <span className="font-mono text-[12px]">{task.id}</span>
              </Detail>
            </div>
          </Card>
          <Card title="Relations">
            <Relations
              task={shown}
              rich
              onOpen={onOpen}
              onChange={editing ? (patch) => set(patch) : undefined}
              emptyChildren={local ? 'None' : 'No child tasks'}
            />
            {!local && (
              <p className="m-0 text-[12px] leading-normal text-ink-muted">
                To group this issue with your own work, add it to a local task’s children or links.
              </p>
            )}
          </Card>
        </div>
      </div>
    </>
  )
}

/** TaskDialog is the task detail modal at /task/:id. */
export function TaskDialog({ id, onClose, onOpen }: { id: string; onClose: () => void; onOpen: (id: string) => void }) {
  const { data: task, isError } = useTask(id)
  const cancelEdit = useRef<(() => void) | null>(null)
  const onEditing = useCallback((cancel: (() => void) | null) => {
    cancelEdit.current = cancel
  }, [])
  return (
    <D.Root open onOpenChange={(o) => !o && onClose()}>
      <D.Portal>
        <D.Overlay className="fixed inset-0 z-40 flex items-start justify-center overflow-y-auto bg-[rgba(9,9,11,0.4)] px-6 py-10">
          <D.Content
            aria-describedby={undefined}
            onOpenAutoFocus={(e) => e.preventDefault()}
            onEscapeKeyDown={(e) => {
              if (!cancelEdit.current) return
              e.preventDefault()
              cancelEdit.current()
            }}
            className="flex max-h-[calc(100vh-80px)] w-full max-w-[960px] flex-col gap-6 overflow-y-auto rounded-[14px] border border-border bg-white px-7 pt-6 pb-7 shadow-xl outline-none"
          >
            {task ? (
              <Body key={task.id} task={task} onClose={onClose} onOpen={onOpen} onEditing={onEditing} />
            ) : (
              <div className="flex items-center justify-between">
                <D.Title className="m-0 text-[15px] font-medium text-ink-muted">
                  {isError ? 'That task no longer exists.' : 'Loading…'}
                </D.Title>
                <button
                  type="button"
                  aria-label="Close task detail"
                  onClick={onClose}
                  className="flex size-8 items-center justify-center rounded-md text-ink-secondary hover:bg-bg-muted"
                >
                  <X size={18} />
                </button>
              </div>
            )}
          </D.Content>
        </D.Overlay>
      </D.Portal>
    </D.Root>
  )
}
