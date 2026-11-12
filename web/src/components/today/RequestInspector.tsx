import { ExternalLink, Lock, Link2, ListChecks, X } from 'lucide-react'
import { useState } from 'react'
import { OriginBadge, OriginDot, reqStatusStyle } from '@/components/common/badges'
import { SearchPicker } from '@/components/pickers/SearchPicker'
import { jiraURL } from '@/components/task/TaskDialog'
import { Checkbox } from '@/components/ui/checkbox'
import { useAtlassianSettings, useTodoActions, useTodos } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { taskKey, type ServiceDeskRequest, type TaskListItem, type Todo } from '@/lib/api'
import { ageText, hhmm, isTodayTodo, monthDay, ymd } from '@/lib/dates'
import { cn } from '@/lib/utils'
import { InspectorHeader, Label } from './InspectorShell'

const stamp = (iso: string) => {
  const d = new Date(iso)
  return `${monthDay(ymd(d))} · ${hhmm(d)}`
}

export function RequestInspector({
  request,
  tasks,
  openTodos,
  onClose,
  onSelectTodo,
}: {
  request: ServiceDeskRequest
  tasks: Map<string, TaskListItem>
  /** openTodos are the candidates for linking: every todo not done yet. */
  openTodos: Todo[]
  onClose: () => void
  onSelectTodo: (id: number) => void
}) {
  const { today } = useNow()
  const { update, link, unlink } = useTodoActions()
  const { data: linked = [] } = useTodos({ service_desk_request: request.key })
  const { data: atlassian } = useAtlassianSettings()
  const browse = jiraURL(atlassian?.site_url, request.key)
  const [picking, setPicking] = useState(false)
  const s = reqStatusStyle(request)
  const statusAge = ageText(ymd(new Date(request.status_at)), today)
  const openAge = ageText(ymd(new Date(request.created_at)), today)
  const linkedIDs = new Set(linked.map((t) => t.id))
  const candidates = openTodos
    .filter((t) => !linkedIDs.has(t.id))
    .map((t) => ({ t, today: isTodayTodo(t, today) }))
    .sort((a, b) => Number(b.today) - Number(a.today))

  return (
    <>
      <InspectorHeader onClose={onClose}>
        <span className="text-[13px] font-semibold">Request</span>
        <OriginBadge origin="sd" />
        {browse ? (
          <a
            href={browse}
            target="_blank"
            rel="noopener noreferrer"
            title={`Open ${request.key} in Jira (new tab)`}
            aria-label={`Open ${request.key} in Jira (new tab)`}
            className="-ml-0.5 mr-auto inline-flex h-[22px] items-center gap-1 rounded-[5px] px-1.5 font-mono text-[12px] text-tag-indigo-ink underline decoration-tag-indigo-ink/35 underline-offset-[3px] hover:bg-tag-indigo-bg"
          >
            {request.key}
            <ExternalLink size={11} strokeWidth={2.2} className="shrink-0" />
          </a>
        ) : (
          <span className="mr-auto font-mono text-[12px] text-ink-secondary">{request.key}</span>
        )}
      </InspectorHeader>
      <div className="flex flex-col gap-[18px] p-4">
        <div role="note" className="flex gap-2 rounded-lg border border-[#c7d2fe] bg-tag-indigo-bg px-3 py-2 text-[12px] leading-normal text-tag-indigo-ink">
          <Lock size={14} className="mt-0.5 shrink-0" />
          Synced from Service Desk with Jira sync. It leaves Today once it’s Done or Canceled.
        </div>
        <div className="flex flex-col gap-1.5">
          <Label>Title</Label>
          <span className="text-[15px] leading-[1.4] font-semibold">{request.title}</span>
        </div>
        <div className="flex flex-col gap-1.5">
          <Label>Status</Label>
          <span
            className="inline-flex h-8 w-fit items-center gap-1.5 rounded-md px-2.5 text-[13px] font-medium"
            style={{ background: s.bg, color: s.color }}
          >
            <span className="size-1.5 rounded-full" style={{ background: s.dot }} />
            {request.status}
          </span>
        </div>
        <div className="flex flex-col gap-2">
          <div className="grid grid-cols-2 gap-2">
            {[
              ['In status since', stamp(request.status_at)],
              ['Created', stamp(request.created_at)],
            ].map(([label, value]) => (
              <div key={label} className="flex min-w-0 flex-col gap-1.5">
                <Label>{label}</Label>
                <span className="flex h-[34px] items-center rounded-md bg-bg-subtle px-2 text-[13px] tabular-nums">{value}</span>
              </div>
            ))}
          </div>
          <div className="flex gap-4 text-[12px] text-ink-muted">
            <span>{statusAge === 'today' ? `Since today in ${request.status}` : `${statusAge} in ${request.status}`}</span>
            <span>Open for {openAge === 'today' ? 'less than a day' : openAge}</span>
          </div>
        </div>

        <div className="flex flex-col gap-1.5 border-t border-border-subtle pt-4">
          <div className="flex items-center gap-1.5 text-[12px] font-medium text-ink-secondary">
            <ListChecks size={13} />
            Linked todos
            <span className="font-normal text-ink-muted">{linked.length}</span>
            <span className="ml-auto font-normal text-ink-faint">Saved only in Wharf</span>
          </div>
          {linked.map((t) => (
            <div key={t.id} className="flex min-w-0 items-center gap-2 rounded-lg border border-border py-1.5 pr-1 pl-2.5">
              <Checkbox checked={t.is_done} label={t.description} onChange={(v) => update(t, { is_done: v })} />
              <button type="button" onClick={() => onSelectTodo(t.id)} className="flex min-w-0 flex-1 flex-col text-left">
                <span className={cn('truncate text-[13px]', t.is_done && 'text-ink-muted line-through')}>{t.description}</span>
                <span className="flex min-w-0 items-center gap-1 text-[11px] text-ink-muted">
                  <OriginDot origin={tasks.get(t.task_id)?.originated} className="size-[6px]" />
                  <span className="shrink-0 font-mono">{taskKey(t.task_id)}</span>
                  <span className="truncate">{tasks.get(t.task_id)?.title}</span>
                </span>
              </button>
              <button
                type="button"
                aria-label={`Unlink ${t.description}`}
                onClick={() => unlink(t, request.key)}
                className="flex size-[26px] shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-black/5"
              >
                <X size={12} strokeWidth={2.2} />
              </button>
            </div>
          ))}
          {picking && (
            <SearchPicker
              placeholder="Search todos or task keys"
              empty={candidates.length ? 'No matching todos.' : 'Every open todo is already linked.'}
              items={candidates.map(({ t, today: isToday }) => ({
                id: String(t.id),
                value: `${taskKey(t.task_id)} ${t.description}`,
                content: (
                  <>
                    <OriginDot origin={tasks.get(t.task_id)?.originated} />
                    <span className="shrink-0 font-mono text-[11px] text-ink-secondary">{taskKey(t.task_id)}</span>
                    <span className="min-w-0 truncate">{t.description}</span>
                    {isToday && <span className="ml-auto shrink-0 text-[10px] font-semibold text-today-ink">TODAY</span>}
                  </>
                ),
              }))}
              onPick={(id) => {
                const t = openTodos.find((x) => String(x.id) === id)
                if (t) link(t, request)
                setPicking(false)
              }}
            />
          )}
          <button
            type="button"
            aria-expanded={picking}
            onClick={() => setPicking(!picking)}
            className="flex h-8 items-center justify-center gap-1.5 rounded-md border border-dashed border-[#c7d2fe] bg-white text-[12px] font-medium text-tag-indigo-ink hover:bg-[#fafaff]"
          >
            <Link2 size={12} strokeWidth={2.2} />
            {picking ? 'Cancel' : 'Link an existing todo'}
          </button>
        </div>
      </div>
    </>
  )
}
