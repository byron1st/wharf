import { ChevronDown, ChevronRight, FileText, Plus, Ticket, X } from 'lucide-react'
import { useState } from 'react'
import { Kbd, OriginDot } from '@/components/common/badges'
import { Checkbox } from '@/components/ui/checkbox'
import { useTodoActions } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { taskKey, type TaskListItem, type Todo } from '@/lib/api'
import { dueTag, fromTaskDate, reminderSummary, toneClass, whenPill } from '@/lib/dates'
import { cn } from '@/lib/utils'

export type Selection = { kind: 'todo'; id: number } | { kind: 'task'; id: string } | { kind: 'req'; id: string } | null

export interface TodoGroup {
  task: TaskListItem
  todos: Todo[]
}

const ROW = 'grid grid-cols-[28px_minmax(0,1fr)_104px_104px] items-center gap-3 px-4'

function useCollapsed() {
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(() => {
    try {
      return JSON.parse(localStorage.getItem('wharf.collapsed') ?? '{}') as Record<string, boolean>
    } catch {
      return {}
    }
  })
  const set = (id: string, v: boolean) => {
    const next = { ...collapsed, [id]: v }
    if (!v) delete next[id]
    setCollapsed(next)
    localStorage.setItem('wharf.collapsed', JSON.stringify(next))
  }
  return [collapsed, set] as const
}

function NewTodoRow({ task, onDone }: { task: TaskListItem; onDone: () => void }) {
  const [text, setText] = useState('')
  const { create } = useTodoActions()
  const key = taskKey(task.id)
  const add = () => {
    const d = text.trim()
    if (!d) return
    setText('')
    void create({
      task_id: task.id,
      description: d,
      note: '',
      is_done: false,
      start_date: null,
      end_date: null,
      reminders: [],
    }).catch(() => setText(d))
  }
  return (
    <div className="grid min-h-[46px] grid-cols-[28px_minmax(0,1fr)_auto] items-center gap-3 border-b border-[#ccfbf1] bg-primary-subtle pr-3 pl-4">
      <span className="flex justify-center">
        <span className="size-[18px] rounded-[5px] border-[1.5px] border-dashed border-[#2dd4bf]" />
      </span>
      <input
        autoFocus
        aria-label={`New todo for ${key}`}
        placeholder={`New todo for ${key}`}
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.nativeEvent.isComposing) add()
          if (e.key === 'Escape') onDone()
        }}
        className="h-8 min-w-0 bg-transparent text-[14px] outline-none placeholder:text-[#71717a]"
      />
      <span className="flex items-center gap-2 text-[12px] text-[#71717a]">
        <span className="flex items-center gap-1">
          <Kbd accent>Enter</Kbd>add
        </span>
        <span className="flex items-center gap-1">
          <Kbd>Esc</Kbd>cancel
        </span>
        <button
          type="button"
          aria-label="Cancel"
          onClick={onDone}
          className="flex size-[26px] items-center justify-center rounded-md text-[#71717a] hover:bg-black/5"
        >
          <X size={13} strokeWidth={2.2} />
        </button>
      </span>
    </div>
  )
}

function TodoRow({ todo, selected, onSelect }: { todo: Todo; selected: boolean; onSelect: () => void }) {
  const { now, today } = useNow()
  const { update } = useTodoActions()
  const when = whenPill(todo, today)
  const rem = reminderSummary(todo.reminders, now)
  const reqs = todo.service_desk_requests
  return (
    <div
      className={cn(ROW, 'min-h-11 border-b border-[#f4f4f5]', selected ? 'bg-primary-subtle shadow-[inset_0_0_0_1px_#99f6e4]' : 'bg-white')}
    >
      <span className="flex justify-center">
        <Checkbox checked={false} label={todo.description} onChange={() => update(todo, { is_done: true })} />
      </span>
      <button
        type="button"
        onClick={onSelect}
        aria-pressed={selected}
        className={cn('flex h-[42px] min-w-0 items-center gap-2 text-left text-[14px]', selected && 'font-semibold')}
      >
        <span className="truncate">{todo.description}</span>
        {todo.note.trim() && <FileText size={14} className="shrink-0 text-[#71717a]" aria-label="Has a note" />}
        {reqs.length > 0 && (
          <span
            title={reqs.map((r) => `${r.key} ${r.title}`).join('\n')}
            className="inline-flex h-5 shrink-0 items-center gap-[3px] rounded-full bg-[#eef2ff] px-1.5 text-[11px] font-medium text-[#4338ca]"
          >
            <Ticket size={11} strokeWidth={2.4} />
            {reqs.length}
          </span>
        )}
      </button>
      <span
        title={rem && rem.sent ? `${rem.sent} sent` : undefined}
        className={cn('text-[12px] tabular-nums', rem?.upcoming ? 'text-[#6d28d9]' : 'text-[#a1a1aa]')}
      >
        {rem?.label}
      </span>
      <span className="flex justify-end">
        <span className={cn('rounded-md px-2 py-0.5 text-[12px] font-medium whitespace-nowrap tabular-nums', toneClass[when.tone])}>
          {when.label}
        </span>
      </span>
    </div>
  )
}

export function TodosCard({
  groups,
  completed,
  selection,
  onSelect,
  adding,
  setAdding,
}: {
  groups: TodoGroup[]
  completed: Todo[]
  selection: Selection
  onSelect: (s: Selection) => void
  adding: string | null
  setAdding: (taskID: string | null) => void
}) {
  const { today } = useNow()
  const { update } = useTodoActions()
  const [collapsed, setCollapsed] = useCollapsed()
  const [showCompleted, setShowCompleted] = useState(true)

  return (
    <section aria-label="Todos" className="min-w-0 overflow-x-auto rounded-xl border border-[#e4e4e7] bg-white shadow-card">
      <div className="min-w-[520px]">
        <div className={cn(ROW, 'h-9 border-b border-[#e4e4e7] text-[12px] font-medium text-[#71717a]')}>
          <span />
          <span>Todo</span>
          <span>Reminder</span>
          <span className="text-right">When</span>
        </div>

        {groups.map(({ task, todos }) => {
          const open = !collapsed[task.id] || adding === task.id
          const isSel = selection?.kind === 'task' && selection.id === task.id
          const due = dueTag(fromTaskDate(task.due_date), today, task.status === 'done' || task.status === 'canceled')
          const pct = task.todo_count ? Math.round((task.done_todo_count / task.todo_count) * 100) : 0
          const key = taskKey(task.id)
          return (
            <div key={task.id}>
              <div
                className={cn(
                  'grid min-h-11 grid-cols-[28px_minmax(0,1fr)_auto] items-center gap-3 border-b border-[#f4f4f5] px-4',
                  isSel ? 'bg-primary-subtle shadow-[inset_0_0_0_1px_#99f6e4]' : 'bg-[#fafafa]',
                )}
              >
                <button
                  type="button"
                  aria-expanded={open}
                  aria-label={`Collapse or expand ${key}`}
                  onClick={() => setCollapsed(task.id, open)}
                  className="flex size-7 items-center justify-center rounded-md text-[#71717a] hover:bg-black/5"
                >
                  {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                </button>
                <button
                  type="button"
                  aria-pressed={isSel}
                  title="Show task details"
                  onClick={() => onSelect({ kind: 'task', id: task.id })}
                  className="flex h-[42px] min-w-0 items-center gap-2 text-left"
                >
                  <OriginDot origin={task.originated} className="size-2" />
                  <span className="shrink-0 font-mono text-[12px] text-[#52525b]">{key}</span>
                  <span className={cn('min-w-0 truncate text-[14px] font-semibold underline-offset-[3px]', isSel && 'underline')}>
                    {task.title}
                  </span>
                  <span className={cn('shrink-0 rounded-md px-[7px] py-px text-[12px] font-medium', toneClass[due.tone])}>
                    {due.label}
                  </span>
                </button>
                <span className="flex items-center gap-2 text-[12px] text-[#71717a] tabular-nums">
                  <span className="flex h-[5px] w-10 overflow-hidden rounded-full bg-[#e4e4e7]">
                    <span className="bg-[#7c3aed]" style={{ width: `${pct}%` }} />
                  </span>
                  <span>
                    {task.done_todo_count}/{task.todo_count}
                  </span>
                  <button
                    type="button"
                    aria-label={`Add todo to ${key}`}
                    title={`Add todo to ${key}`}
                    onClick={() => setAdding(adding === task.id ? null : task.id)}
                    className={cn(
                      'flex size-7 items-center justify-center rounded-md border',
                      adding === task.id
                        ? 'border-primary bg-primary text-white'
                        : 'border-[#e4e4e7] bg-white text-[#52525b] hover:bg-[#fafafa]',
                    )}
                  >
                    <Plus size={14} strokeWidth={2.2} />
                  </button>
                </span>
              </div>
              {open && (
                <>
                  {adding === task.id && <NewTodoRow task={task} onDone={() => setAdding(null)} />}
                  {todos.map((t) => (
                    <TodoRow
                      key={t.id}
                      todo={t}
                      selected={selection?.kind === 'todo' && selection.id === t.id}
                      onSelect={() => onSelect({ kind: 'todo', id: t.id })}
                    />
                  ))}
                </>
              )}
            </div>
          )
        })}

        {groups.length === 0 && (
          <p className="m-0 px-4 py-10 text-center text-[14px] text-[#71717a]">
            Nothing to do today. Add a task with <span className="font-medium text-[#3f3f46]">New task</span>, or sync
            Atlassian Cloud.
          </p>
        )}

        <button
          type="button"
          aria-expanded={showCompleted}
          onClick={() => setShowCompleted(!showCompleted)}
          className="grid min-h-11 w-full grid-cols-[28px_minmax(0,1fr)_auto] items-center gap-3 bg-white px-4 text-left text-[#52525b]"
        >
          <span className="flex justify-center text-[#71717a]">
            {showCompleted ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </span>
          <span className="flex items-center gap-2 text-[13px] font-semibold">
            <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden>
              <circle cx="8" cy="8" r="7" fill="#7c3aed" />
              <path d="m5 8.2 2 2 4-4.2" stroke="#fff" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
            Completed today
          </span>
          <span className="text-[12px] text-[#71717a] tabular-nums">{completed.length}</span>
        </button>
        {showCompleted &&
          completed.map((t) => {
            const isSel = selection?.kind === 'todo' && selection.id === t.id
            return (
              <div
                key={t.id}
                className={cn(
                  'grid min-h-10 grid-cols-[28px_minmax(0,1fr)_auto] items-center gap-3 border-t border-[#f4f4f5] px-4',
                  isSel ? 'bg-primary-subtle' : 'bg-white',
                )}
              >
                <span className="flex justify-center">
                  <Checkbox checked label={t.description} onChange={() => update(t, { is_done: false })} />
                </span>
                <button
                  type="button"
                  onClick={() => onSelect({ kind: 'todo', id: t.id })}
                  className="min-w-0 truncate text-left text-[14px] text-[#71717a] line-through"
                >
                  {t.description}
                </button>
                <span className="font-mono text-[11px] text-[#a1a1aa]">{taskKey(t.task_id)}</span>
              </div>
            )
          })}
      </div>
    </section>
  )
}
