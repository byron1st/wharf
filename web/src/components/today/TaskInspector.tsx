import { ArrowUpRight, Lock, Trash2 } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { OriginBadge, STATUS_LABEL, StatusIcon } from '@/components/common/badges'
import { Relations } from '@/components/common/Relations'
import { MarkdownEditor, MdIcon, ModeTabs } from '@/components/markdown/MarkdownEditor'
import { Markdown } from '@/components/markdown/Markdown'
import { DateField } from '@/components/pickers/DateField'
import { Select, SelectContent, SelectItem, SelectTrigger } from '@/components/ui/select'
import { useSavingTask, useTask, useTaskActions } from '@/hooks/data'
import { useDraft } from '@/hooks/useDraft'
import { useNow } from '@/hooks/useNow'
import { taskKey, type TaskDetail, type TaskStatus } from '@/lib/api'
import { dayLabel, fromTaskDate, monthDay, toTaskDate } from '@/lib/dates'
import { cn } from '@/lib/utils'
import { InspectorHeader, Label } from './InspectorShell'
import { SavedMark } from './TodoInspector'

export const STATUSES: TaskStatus[] = ['todo', 'in-progress', 'done', 'canceled']

/** StatusSelect picks a local task's status. */
export function StatusSelect({
  value,
  onChange,
  className,
  hideIcon,
}: {
  value: TaskStatus
  onChange: (s: TaskStatus) => void
  className?: string
  hideIcon?: boolean
}) {
  return (
    <Select value={value} onValueChange={(v) => onChange(v as TaskStatus)}>
      <SelectTrigger aria-label="Status" className={className} hideIcon={hideIcon}>
        <span className="flex min-w-0 items-center gap-1.5">
          <StatusIcon status={value} />
          <span className="truncate">{STATUS_LABEL[value]}</span>
        </span>
      </SelectTrigger>
      <SelectContent>
        {STATUSES.map((s) => (
          <SelectItem key={s} value={s}>
            <span className="flex items-center gap-2">
              <StatusIcon status={s} />
              {STATUS_LABEL[s]}
            </span>
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

/** dueTone colors a due field red when overdue and orange when due today. */
export function dueTone(due: string | null, today: string, status: TaskStatus) {
  if (!due || status === 'done' || status === 'canceled') return undefined
  if (due < today) return 'over' as const
  return due === today ? ('today' as const) : undefined
}

function ReadField({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        'flex h-[34px] min-w-0 items-center gap-1.5 truncate rounded-md border border-[#f4f4f5] bg-[#fafafa] px-2 text-[13px]',
        className,
      )}
    >
      {children}
    </span>
  )
}

function LocalFields({ task }: { task: TaskDetail }) {
  const { today } = useNow()
  const { update } = useTaskActions()
  const title = useDraft(task.title, (v) => v.trim() && update(task, { title: v.trim() }))
  const start = fromTaskDate(task.start_date)
  const due = fromTaskDate(task.due_date)
  return (
    <>
      <label className="flex flex-col gap-1.5 text-[12px] font-medium text-[#3f3f46]">
        Title
        <input
          value={title.value}
          onChange={(e) => title.onChange(e.target.value)}
          onBlur={title.onBlur}
          onKeyDown={(e) => e.key === 'Enter' && (e.target as HTMLInputElement).blur()}
          className="h-9 rounded-md border border-[#e4e4e7] px-2.5 text-[14px] font-semibold text-[#09090b] shadow-card outline-none focus:border-primary focus:shadow-ring"
        />
      </label>
      <div className="grid grid-cols-[minmax(0,1.25fr)_minmax(0,1fr)_minmax(0,1fr)] gap-2">
        <div className="flex min-w-0 flex-col gap-1.5">
          <Label>Status</Label>
          <StatusSelect
            value={task.status}
            onChange={(status) => update(task, { status })}
            className="h-[34px] px-2"
            hideIcon
          />
        </div>
        <div className="flex min-w-0 flex-col gap-1.5">
          <Label>Start</Label>
          <DateField
            title="Start date"
            value={start}
            other={due}
            pairText={due ? `Due ${dayLabel(due)}` : undefined}
            hi={due}
            showIcon={false}
            format={monthDay}
            align="end"
            onChange={(v) => update(task, { start_date: toTaskDate(v) })}
            className="h-[34px] px-2"
          />
        </div>
        <div className="flex min-w-0 flex-col gap-1.5">
          <Label>Due</Label>
          <DateField
            title="Due date"
            value={due}
            other={start}
            pairText={start ? `Starts ${dayLabel(start)}` : undefined}
            lo={start}
            tone={dueTone(due, today, task.status)}
            showIcon={false}
            format={monthDay}
            align="end"
            onChange={(v) => update(task, { due_date: toTaskDate(v) })}
            className="h-[34px] px-2"
          />
        </div>
      </div>
    </>
  )
}

function JiraFields({ task }: { task: TaskDetail }) {
  const { today } = useNow()
  const start = fromTaskDate(task.start_date)
  const due = fromTaskDate(task.due_date)
  const tone = dueTone(due, today, task.status)
  return (
    <>
      <div role="note" className="flex gap-2 rounded-lg border border-[#bfdbfe] bg-[#eff6ff] px-2.5 py-2 text-[12px] leading-normal text-[#1e40af]">
        <Lock size={14} className="mt-0.5 shrink-0" />
        Managed in Jira. Fields show the last synced snapshot and can’t be edited here.
      </div>
      <div className="flex flex-col gap-1.5">
        <Label>Title</Label>
        <span className="text-[15px] leading-[1.4] font-semibold">{task.title}</span>
      </div>
      <div className="grid grid-cols-[minmax(0,1.25fr)_minmax(0,1fr)_minmax(0,1fr)] gap-2">
        <div className="flex min-w-0 flex-col gap-1.5">
          <Label>Status</Label>
          <ReadField>
            <StatusIcon status={task.status} />
            {STATUS_LABEL[task.status]}
          </ReadField>
        </div>
        <div className="flex min-w-0 flex-col gap-1.5">
          <Label>Start</Label>
          <ReadField className={start ? '' : 'text-[#a1a1aa]'}>{start ? monthDay(start) : 'Not set'}</ReadField>
        </div>
        <div className="flex min-w-0 flex-col gap-1.5">
          <Label>Due</Label>
          <ReadField
            className={cn(
              !due && 'text-[#a1a1aa]',
              tone === 'over' && 'border-[#fecaca] bg-[#fef2f2] text-[#b91c1c]',
              tone === 'today' && 'border-[#fed7aa] bg-[#fff7ed] text-[#9a3412]',
            )}
          >
            {due ? monthDay(due) : 'Not set'}
          </ReadField>
        </div>
      </div>
    </>
  )
}

export function TaskInspector({
  id,
  onClose,
  onOpen,
  onSelectTask,
}: {
  id: string
  onClose: () => void
  onOpen: (id: string) => void
  onSelectTask: (id: string) => void
}) {
  const { data: task, isError } = useTask(id)
  const { update, remove } = useTaskActions()
  const saving = useSavingTask(id)
  const [mode, setMode] = useState<'preview' | 'write'>('preview')

  if (!task) {
    return (
      <>
        <InspectorHeader onClose={onClose}>
          <span className="mr-auto text-[13px] font-semibold">Task</span>
        </InspectorHeader>
        <p className="m-0 p-4 text-[13px] text-[#71717a]">{isError ? 'This task no longer exists.' : 'Loading…'}</p>
      </>
    )
  }

  const local = task.originated === 'local'
  return (
    <>
      <InspectorHeader onClose={onClose}>
        <span className="text-[13px] font-semibold">Task</span>
        <OriginBadge origin={local ? 'local' : 'jira'} />
        <span className="font-mono text-[12px] text-[#52525b]">{taskKey(task.id)}</span>
        {local && saving && <SavedMark saving />}
        <button
          type="button"
          onClick={() => onOpen(task.id)}
          className={cn(
            'inline-flex h-7 items-center gap-1 rounded-md px-2 text-[12px] font-medium text-[#3f3f46] hover:bg-[#f4f4f5]',
            !(local && saving) && 'ml-auto',
          )}
        >
          Open
          <ArrowUpRight size={12} />
        </button>
      </InspectorHeader>
      <div className="flex flex-col gap-[18px] p-4">
        {local ? <LocalFields key={task.id} task={task} /> : <JiraFields task={task} />}

        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-2">
            <Label>Description</Label>
            <span className="flex items-center gap-1 text-[11px] text-[#a1a1aa]">
              <MdIcon />
              {local ? 'Markdown' : 'Markdown from Jira'}
            </span>
            {local && (
              <span className="ml-auto">
                <ModeTabs mode={mode} onMode={setMode} />
              </span>
            )}
          </div>
          {local ? (
            <MarkdownEditor
              key={task.id}
              value={task.description}
              onSave={(description) => update(task, { description })}
              saving={saving}
              mode={mode}
              onMode={setMode}
              rows={8}
              emptyText="Add a description…"
            />
          ) : task.description.trim() ? (
            <div className="rounded-lg border border-[#f4f4f5] bg-[#fafafa] p-3">
              <Markdown source={task.description} />
            </div>
          ) : (
            <span className="text-[12px] text-[#a1a1aa]">No description</span>
          )}
        </div>

        <div className="flex flex-col gap-3 border-t border-[#f4f4f5] pt-4">
          <span className="text-[13px] font-semibold">Relations</span>
          <Relations
            task={task}
            onOpen={onSelectTask}
            onChange={local ? (patch) => update(task, patch) : undefined}
          />
        </div>

        {local && (
          <div className="flex items-center gap-2 border-t border-[#f4f4f5] pt-3.5">
            <span className="font-mono text-[11px] text-[#a1a1aa]">{task.id}</span>
            <button
              type="button"
              onClick={() => {
                onClose()
                void remove(task.id)
              }}
              className="ml-auto flex h-8 items-center gap-1.5 rounded-md border border-[#e4e4e7] bg-white px-2.5 text-[13px] font-medium text-[#dc2626] hover:bg-[#fef2f2]"
            >
              <Trash2 size={13} />
              Delete task
            </button>
          </div>
        )}
      </div>
    </>
  )
}
