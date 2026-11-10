import { Check, FileText, Link2, Ticket, Trash2, X } from 'lucide-react'
import { useState } from 'react'
import { OriginDot, reqStatusStyle } from '@/components/common/badges'
import { DateField } from '@/components/pickers/DateField'
import { Reminders } from '@/components/pickers/Reminders'
import { SearchPicker } from '@/components/pickers/SearchPicker'
import { useSavingTodo, useTodoActions } from '@/hooks/data'
import { useDraft } from '@/hooks/useDraft'
import { useNow } from '@/hooks/useNow'
import { taskKey, type ServiceDeskRequest, type TaskListItem, type Todo } from '@/lib/api'
import { dayLabel, diffDays } from '@/lib/dates'
import { cn } from '@/lib/utils'
import { InspectorHeader, Label } from './InspectorShell'

/** SavedMark is the autosave state in the Inspector header. */
export function SavedMark({ saving }: { saving: boolean }) {
  return saving ? (
    <span className="ml-auto text-[12px] text-ink-muted">Saving…</span>
  ) : (
    <span className="ml-auto inline-flex items-center gap-1 text-[12px] text-success-ink">
      <Check size={12} strokeWidth={2.6} />
      Saved
    </span>
  )
}

/** RequestLinks lists a todo's Service Desk requests with unlink, and a picker to link more. */
export function RequestLinks({
  todo,
  requests,
  onSelect,
}: {
  todo: Todo
  requests: ServiceDeskRequest[]
  onSelect?: (key: string) => void
}) {
  const { link, unlink } = useTodoActions()
  const [picking, setPicking] = useState(false)
  const linked = new Set(todo.service_desk_requests.map((r) => r.key))
  const candidates = requests.filter((r) => !linked.has(r.key))

  return (
    <div className="flex flex-col gap-1.5">
      {todo.service_desk_requests.map((r) => {
        const s = reqStatusStyle(r)
        return (
          <div
            key={r.key}
            className="flex min-h-9 min-w-0 items-center gap-1 rounded-lg border border-tag-indigo-line bg-[#fafaff] pr-1 pl-2.5"
          >
            <button
              type="button"
              disabled={!onSelect}
              onClick={() => onSelect?.(r.key)}
              className="flex h-[34px] min-w-0 flex-1 items-center gap-2 text-left"
            >
              <span className="shrink-0 font-mono text-[11px] text-tag-indigo-ink">{r.key}</span>
              <span className="min-w-0 truncate text-[13px]">{r.title}</span>
              <span
                className="ml-auto shrink-0 rounded-[5px] px-1.5 py-px text-[11px] font-medium"
                style={{ background: s.bg, color: s.color }}
              >
                {r.status}
              </span>
            </button>
            <button
              type="button"
              aria-label={`Unlink ${r.key}`}
              onClick={() => unlink(todo, r.key)}
              className="flex size-[26px] shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-black/5"
            >
              <X size={12} strokeWidth={2.2} />
            </button>
          </div>
        )
      })}
      {picking && (
        <SearchPicker
          placeholder="Search by key or title"
          empty={candidates.length ? 'No matching requests.' : 'All open requests are already linked.'}
          items={candidates.map((r) => ({
            id: r.key,
            value: `${r.key} ${r.title}`,
            content: (
              <>
                <span className="size-[7px] shrink-0 rounded-full" style={{ background: reqStatusStyle(r).dot }} />
                <span className="shrink-0 font-mono text-[11px] text-tag-indigo-ink">{r.key}</span>
                <span className="min-w-0 truncate">{r.title}</span>
              </>
            ),
          }))}
          onPick={(key) => {
            const r = candidates.find((c) => c.key === key)
            if (r) link(todo, r)
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
        {picking ? 'Cancel' : 'Link a request'}
      </button>
    </div>
  )
}

export function TodoInspector({
  todo,
  task,
  requests,
  onClose,
  onSelectTask,
  onSelectRequest,
  onToggleDone,
}: {
  todo: Todo
  task: TaskListItem | undefined
  requests: ServiceDeskRequest[]
  onClose: () => void
  onSelectTask: (id: string) => void
  onSelectRequest: (key: string) => void
  onToggleDone: (todo: Todo, done: boolean) => void
}) {
  const { today } = useNow()
  const { update, remove } = useTodoActions()
  const saving = useSavingTodo(todo.id)
  const description = useDraft(todo.description, (v) => v.trim() && update(todo, { description: v.trim() }))
  const note = useDraft(todo.note, (v) => update(todo, { note: v }))
  const over = !todo.is_done && !!todo.end_date && todo.end_date < today

  return (
    <>
      <InspectorHeader onClose={onClose}>
        <span className="text-[13px] font-semibold">Todo</span>
        <span className="font-mono text-[11px] text-ink-muted">#{todo.id}</span>
        <SavedMark saving={saving || description.dirty || note.dirty} />
      </InspectorHeader>
      <div className="flex flex-col gap-[18px] p-4">
        <label className="flex flex-col gap-1.5 text-[12px] font-medium text-ink-secondary">
          Description
          <textarea
            rows={2}
            value={description.value}
            onChange={(e) => description.onChange(e.target.value)}
            onBlur={description.onBlur}
            className="resize-none rounded-md border border-border px-2.5 py-2 text-[14px] leading-[1.45] font-medium text-ink shadow-xs outline-none focus:border-accent focus:shadow-ring"
          />
        </label>

        <label className="flex flex-col gap-1.5 text-[12px] font-medium text-ink-secondary">
          <span className="flex items-center gap-1.5">
            <FileText size={13} />
            Note
            <span className="ml-auto font-normal text-ink-faint">Plain text · only in Wharf</span>
          </span>
          <textarea
            rows={5}
            value={note.value}
            placeholder="Context, links, commands, decisions…"
            onChange={(e) => note.onChange(e.target.value)}
            onBlur={note.onBlur}
            className="resize-y rounded-md border border-border bg-bg-subtle px-2.5 py-2 text-[13px] leading-[1.6] font-normal text-ink shadow-xs outline-none focus:border-accent focus:shadow-ring"
          />
        </label>

        <div className="flex flex-col gap-1.5">
          <Label>Task</Label>
          <button
            type="button"
            onClick={() => onSelectTask(todo.task_id)}
            title="Show task details"
            className="flex h-8 min-w-0 items-center gap-2 rounded-md border border-border bg-white px-2.5 text-[13px] shadow-xs hover:bg-bg-subtle"
          >
            <OriginDot origin={task?.originated} className="size-2" />
            <span className="shrink-0 font-mono text-[11px] text-ink-secondary">{taskKey(todo.task_id)}</span>
            <span className="min-w-0 truncate">{task?.title}</span>
          </button>
        </div>

        <div className="flex flex-col gap-2">
          <div className="grid grid-cols-2 gap-2">
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label>Start</Label>
              <DateField
                title="Start date"
                value={todo.start_date}
                other={todo.end_date}
                pairText={todo.end_date ? `Ends ${dayLabel(todo.end_date)}` : undefined}
                hi={todo.end_date}
                onChange={(v) => update(todo, { start_date: v })}
                className="h-8"
              />
            </div>
            <div className="flex min-w-0 flex-col gap-1.5">
              <Label>End</Label>
              <DateField
                title="End date"
                value={todo.end_date}
                other={todo.start_date}
                pairText={todo.start_date ? `Starts ${dayLabel(todo.start_date)}` : undefined}
                lo={todo.start_date}
                tone={over ? 'over' : undefined}
                align="end"
                onChange={(v) => update(todo, { end_date: v })}
                className="h-8"
              />
            </div>
          </div>
          {over && (
            <p className="m-0 text-[12px] text-danger-ink">
              Ended {diffDays(todo.end_date!, today)} days ago — pick a new end date.
            </p>
          )}
        </div>

        <div className="flex flex-col gap-1.5">
          <Label>Reminders</Label>
          <Reminders
            layout="stack"
            reminders={todo.reminders}
            end={todo.end_date}
            onChange={(reminders) => update(todo, { reminders })}
          />
        </div>

        <div className="flex flex-col gap-1.5">
          <Label className="flex items-center gap-1.5">
            <Ticket size={13} className="text-tag-indigo-solid" />
            Service Desk requests
          </Label>
          <RequestLinks todo={todo} requests={requests} onSelect={onSelectRequest} />
        </div>

        <div className="flex items-center gap-2 border-t border-border-subtle pt-3.5">
          <button
            type="button"
            onClick={() => onToggleDone(todo, !todo.is_done)}
            className={cn(
              'flex h-[34px] flex-1 items-center justify-center gap-1.5 rounded-md border text-[13px] font-medium',
              todo.is_done
                ? 'border-[#ddd6fe] bg-tag-violet-bg text-[#6d28d9]'
                : 'border-border bg-white text-ink hover:bg-bg-subtle',
            )}
          >
            <Check size={14} strokeWidth={2.4} />
            {todo.is_done ? 'Completed — undo' : 'Mark as done'}
          </button>
          <button
            type="button"
            aria-label="Delete todo"
            title="Delete todo"
            onClick={() => {
              remove(todo)
              onClose()
            }}
            className="flex size-[34px] items-center justify-center rounded-md border border-border bg-white text-danger hover:bg-danger-tint"
          >
            <Trash2 size={14} />
          </button>
        </div>
      </div>
    </>
  )
}
