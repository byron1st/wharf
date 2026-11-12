import {
  DndContext,
  DragOverlay,
  PointerSensor,
  pointerWithin,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import { Check, ChevronDown, ChevronRight, ChevronsDownUp, ChevronsUpDown, FileText, Plus, Ticket, X } from 'lucide-react'
import { useState } from 'react'
import { Kbd, OriginDot, STATUS_LABEL, StatusIcon } from '@/components/common/badges'
import { Checkbox } from '@/components/ui/checkbox'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useTodoActions, useTodos } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { taskKey, type TaskListItem, type TaskStatus, type Todo } from '@/lib/api'
import { doneLabel, dueTag, fromTaskDate, reminderSummary, toneClass, whenPill } from '@/lib/dates'
import { cn } from '@/lib/utils'

export type Selection = { kind: 'todo'; id: number } | { kind: 'task'; id: string } | { kind: 'req'; id: string } | null

export interface TodoGroup {
  task: TaskListItem
  /** todos are the group's todos for today, with the ones checked this session kept in place. */
  todos: Todo[]
}

const ROW = 'grid grid-cols-[28px_minmax(0,1fr)_104px_104px] items-center gap-3 px-4'
const STATUSES: TaskStatus[] = ['todo', 'in-progress', 'done', 'canceled']
/** END is the droppable ID of the "move to the end" area. */
const END = '__end'

function useCollapsed() {
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>(() => {
    try {
      return JSON.parse(localStorage.getItem('wharf.collapsed') ?? '{}') as Record<string, boolean>
    } catch {
      return {}
    }
  })
  const set = (ids: string[], v: boolean) => {
    const next = { ...collapsed }
    for (const id of ids) {
      if (v) next[id] = true
      else delete next[id]
    }
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
    <div className="grid min-h-[46px] grid-cols-[28px_minmax(0,1fr)_auto] items-center gap-3 border-b border-tag-teal-line bg-accent-tint pr-3 pl-4">
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
        className="h-8 min-w-0 bg-transparent text-[14px] outline-none placeholder:text-ink-muted"
      />
      <span className="flex items-center gap-2 text-[12px] text-ink-muted">
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
          className="flex size-[26px] items-center justify-center rounded-md text-ink-muted hover:bg-black/5"
        >
          <X size={13} strokeWidth={2.2} />
        </button>
      </span>
    </div>
  )
}

/** TodoRow is an open todo, or a done one struck through with when it was done. */
function TodoRow({
  todo,
  selected,
  onSelect,
  onToggleDone,
}: {
  todo: Todo
  selected: boolean
  onSelect: () => void
  onToggleDone: (todo: Todo, done: boolean) => void
}) {
  const { now, today } = useNow()
  const done = todo.is_done
  const when = done ? null : whenPill(todo, today)
  const finished = done ? doneLabel(todo.completed_at, today) : null
  const rem = done ? null : reminderSummary(todo.reminders, now)
  const reqs = todo.service_desk_requests
  return (
    <div
      className={cn(ROW, 'min-h-11 border-b border-border-subtle', selected ? 'bg-accent-tint shadow-[inset_0_0_0_1px_var(--color-accent-line)]' : 'bg-white')}
    >
      <span className="flex justify-center">
        <Checkbox checked={done} label={todo.description} onChange={(v) => onToggleDone(todo, v)} />
      </span>
      <button
        type="button"
        onClick={onSelect}
        aria-pressed={selected}
        className={cn(
          'flex h-[42px] min-w-0 items-center gap-2 text-left text-[14px]',
          selected && 'font-semibold',
          done && 'text-ink-muted',
        )}
      >
        <span className={cn('truncate', done && 'line-through')}>{todo.description}</span>
        {todo.note.trim() && <FileText size={14} className="shrink-0 text-ink-muted" aria-label="Has a note" />}
        {reqs.length > 0 && (
          <span
            title={reqs.map((r) => `${r.key} ${r.title}`).join('\n')}
            className="inline-flex h-5 shrink-0 items-center gap-[3px] rounded-full bg-tag-indigo-bg px-1.5 text-[11px] font-medium text-tag-indigo-ink"
          >
            <Ticket size={11} strokeWidth={2.4} />
            {reqs.length}
          </span>
        )}
      </button>
      <span
        title={rem && rem.sent ? `${rem.sent} sent` : undefined}
        className={cn('text-[12px] tabular-nums', rem?.upcoming ? 'text-[#6d28d9]' : 'text-ink-muted')}
      >
        {rem?.label}
      </span>
      <span className="flex justify-end">
        {when && (
          <span className={cn('rounded-md px-2 py-0.5 text-[12px] font-medium whitespace-nowrap tabular-nums', toneClass[when.tone])}>
            {when.label}
          </span>
        )}
        {finished && (
          <span
            className={cn(
              'rounded-md px-2 py-0.5 text-[12px] font-medium whitespace-nowrap tabular-nums',
              finished.today ? 'bg-tag-violet-bg text-[#6d28d9]' : 'bg-bg-muted text-ink-muted',
            )}
          >
            {finished.label}
          </span>
        )}
      </span>
    </div>
  )
}

/** DoneTodos are a task's done todos, oldest first, except those already shown in place. */
function DoneTodos({
  task,
  shown,
  selection,
  onSelect,
  onToggleDone,
}: {
  task: TaskListItem
  shown: Todo[]
  selection: Selection
  onSelect: (s: Selection) => void
  onToggleDone: (todo: Todo, done: boolean) => void
}) {
  const { data = [] } = useTodos({ task_id: task.id, is_done: true })
  const skip = new Set(shown.map((t) => t.id))
  const done = data
    .filter((t) => t.is_done && !skip.has(t.id))
    .sort((a, b) => (a.completed_at ?? '').localeCompare(b.completed_at ?? ''))
  return done.map((t) => (
    <TodoRow
      key={t.id}
      todo={t}
      selected={selection?.kind === 'todo' && selection.id === t.id}
      onSelect={() => onSelect({ kind: 'todo', id: t.id })}
      onToggleDone={onToggleDone}
    />
  ))
}

/** TaskStatusButton opens the status menu of a local task; a Jira task's status is read-only. */
function TaskStatusButton({ task, onSetStatus }: { task: TaskListItem; onSetStatus: (status: TaskStatus) => void }) {
  const [open, setOpen] = useState(false)
  const label = STATUS_LABEL[task.status]
  if (task.originated !== 'local') {
    return (
      <span role="img" aria-label={`Status: ${label}`} title={`${label} · from Jira`} className="flex size-[26px] shrink-0 items-center justify-center">
        <StatusIcon status={task.status} size={16} />
      </span>
    )
  }
  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={`Status: ${label}`}
          title={`${label} · change status`}
          // Radix opens the menu on pointerdown, so keep it from arming the header's drag too.
          onPointerDown={(e) => e.stopPropagation()}
          className={cn(
            'flex size-[26px] shrink-0 items-center justify-center rounded-md border',
            open ? 'border-accent bg-accent-tint' : 'border-transparent hover:bg-black/5',
          )}
        >
          <StatusIcon status={task.status} size={16} />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" sideOffset={4} className="w-[176px] min-w-0">
        {STATUSES.map((s) => (
          <DropdownMenuItem
            key={s}
            onSelect={() => s !== task.status && onSetStatus(s)}
            className={cn('items-center gap-2 px-2 py-0 h-8', s === task.status && 'bg-bg-muted')}
          >
            <StatusIcon status={s} />
            <span className="flex-1">{STATUS_LABEL[s]}</span>
            {s === task.status && <Check size={12} strokeWidth={3} className="text-accent" aria-label="Current" />}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function GripIcon() {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
      {[6, 12, 18].map((y) => (
        <g key={y}>
          <circle cx="9" cy={y} r="1.6" />
          <circle cx="15" cy={y} r="1.6" />
        </g>
      ))}
    </svg>
  )
}

interface GroupProps {
  group: TodoGroup
  selection: Selection
  onSelect: (s: Selection) => void
  adding: string | null
  setAdding: (taskID: string | null) => void
  open: boolean
  onCollapse: () => void
  doneOpen: boolean
  onToggleDoneOpen: () => void
  onToggleDone: (todo: Todo, done: boolean) => void
  onSetStatus: (task: TaskListItem, status: TaskStatus) => void
  dragging: boolean
  dropHere: boolean
  onMoveBy: (delta: -1 | 1) => void
}

/** TaskGroup is a task header and its todos; the header drags the group and the whole group is a drop target. */
function TaskGroup({
  group: { task, todos },
  selection,
  onSelect,
  adding,
  setAdding,
  open,
  onCollapse,
  doneOpen,
  onToggleDoneOpen,
  onToggleDone,
  onSetStatus,
  dragging,
  dropHere,
  onMoveBy,
}: GroupProps) {
  const { today } = useNow()
  const { setNodeRef: dragRef, listeners } = useDraggable({ id: task.id })
  const { setNodeRef: dropRef } = useDroppable({ id: task.id })
  const isSel = selection?.kind === 'task' && selection.id === task.id
  const closed = task.status === 'done' || task.status === 'canceled'
  const due = dueTag(fromTaskDate(task.due_date), today, closed)
  const pct = task.todo_count ? Math.round((task.done_todo_count / task.todo_count) * 100) : 0
  const key = taskKey(task.id)
  const ndone = task.done_todo_count
  const rings = [dropHere && 'inset 0 2px 0 var(--color-accent)', isSel && 'inset 0 0 0 1px var(--color-accent-line)'].filter(Boolean)
  return (
    <div ref={dropRef} className={cn(dragging && 'opacity-45')}>
      <div
        ref={dragRef}
        {...listeners}
        style={{ boxShadow: rings.join(', ') || undefined }}
        className={cn(
          'relative grid min-h-11 grid-cols-[28px_minmax(0,1fr)_auto] items-center gap-3 border-b border-border-subtle px-4',
          isSel ? 'bg-accent-tint' : 'bg-bg-subtle',
        )}
      >
        <button
          type="button"
          data-grip={task.id}
          aria-label={`Reorder ${key}`}
          title="Drag to reorder · ⌥↑ / ⌥↓ to move"
          onKeyDown={(e) => {
            if (!e.altKey || (e.key !== 'ArrowUp' && e.key !== 'ArrowDown')) return
            e.preventDefault()
            onMoveBy(e.key === 'ArrowUp' ? -1 : 1)
          }}
          className="absolute top-1/2 left-[3px] flex size-3 -translate-y-1/2 cursor-grab text-ink-faint focus-visible:text-accent"
        >
          <GripIcon />
        </button>
        <button
          type="button"
          aria-expanded={open}
          aria-label={`Collapse or expand ${key}`}
          onClick={onCollapse}
          className="flex size-7 items-center justify-center rounded-md text-ink-muted hover:bg-black/5"
        >
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </button>
        <div className="flex min-w-0 items-center gap-1.5">
          <TaskStatusButton task={task} onSetStatus={(s) => onSetStatus(task, s)} />
          <button
            type="button"
            aria-pressed={isSel}
            title="Show task details"
            onClick={() => onSelect({ kind: 'task', id: task.id })}
            className="flex h-[42px] min-w-0 flex-1 items-center gap-2 text-left"
          >
            <OriginDot origin={task.originated} className="size-2" />
            <span className="shrink-0 font-mono text-[12px] text-ink-secondary">{key}</span>
            <span
              className={cn(
                'min-w-0 truncate text-[14px] font-semibold underline-offset-[3px]',
                closed && 'text-ink-muted',
                isSel && 'underline',
              )}
            >
              {task.title}
            </span>
            <span className={cn('shrink-0 rounded-md px-[7px] py-px text-[12px] font-medium', toneClass[due.tone])}>
              {due.label}
            </span>
          </button>
        </div>
        <span className="flex items-center gap-2 text-[12px] text-ink-muted tabular-nums">
          <span className="flex h-[5px] w-10 overflow-hidden rounded-full bg-bg-pressed">
            <span className="bg-tag-violet-solid" style={{ width: `${pct}%` }} />
          </span>
          <button
            type="button"
            aria-pressed={doneOpen}
            disabled={!ndone && !doneOpen}
            title={doneOpen ? 'Hide done todos' : ndone ? `Show ${ndone} done todo${ndone > 1 ? 's' : ''}` : 'No done todos yet'}
            onClick={onToggleDoneOpen}
            className={cn(
              'flex h-6 items-center rounded-md border px-1.5',
              doneOpen
                ? 'border-[#ddd6fe] bg-tag-violet-bg text-tag-violet-ink'
                : ndone
                  ? 'border-border bg-white hover:bg-bg-subtle'
                  : 'border-transparent',
            )}
          >
            {task.done_todo_count}/{task.todo_count}
          </button>
          <button
            type="button"
            aria-label={`Add todo to ${key}`}
            title={`Add todo to ${key}`}
            onClick={() => setAdding(adding === task.id ? null : task.id)}
            className={cn(
              'flex size-7 items-center justify-center rounded-md border',
              adding === task.id
                ? 'border-accent bg-accent text-white'
                : 'border-border bg-white text-ink-secondary hover:bg-bg-subtle',
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
              onToggleDone={onToggleDone}
            />
          ))}
          {doneOpen && (
            <DoneTodos task={task} shown={todos} selection={selection} onSelect={onSelect} onToggleDone={onToggleDone} />
          )}
        </>
      )}
    </div>
  )
}

function EndZone() {
  const { setNodeRef, isOver } = useDroppable({ id: END })
  return (
    <div
      ref={setNodeRef}
      className={cn(
        'mx-4 my-1.5 flex h-9 items-center justify-center rounded-lg border border-dashed text-[12px] text-ink-muted',
        isOver ? 'border-accent bg-accent-tint' : 'border-border-strong bg-bg-subtle',
      )}
    >
      Drop here to move to the end
    </div>
  )
}

export function TodosCard({
  groups,
  selection,
  onSelect,
  adding,
  setAdding,
  onToggleDone,
  onSetStatus,
  onMove,
}: {
  groups: TodoGroup[]
  selection: Selection
  onSelect: (s: Selection) => void
  adding: string | null
  setAdding: (taskID: string | null) => void
  onToggleDone: (todo: Todo, done: boolean) => void
  onSetStatus: (task: TaskListItem, status: TaskStatus) => void
  /** onMove puts the task id right before the task before, or last when before is null. */
  onMove: (id: string, before: string | null) => void
}) {
  const [collapsed, setCollapsed] = useCollapsed()
  const [doneOpen, setDoneOpen] = useState<Record<string, boolean>>({})
  const [active, setActive] = useState<string | null>(null)
  const [over, setOver] = useState<string | null>(null)
  // A small distance keeps clicks on the header's buttons from starting a drag.
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))
  const ids = groups.map((g) => g.task.id)
  const dragged = groups.find((g) => g.task.id === active)?.task
  const isOpen = (id: string) => !collapsed[id] || adding === id
  const allCollapsed = groups.length > 0 && !ids.some(isOpen)
  const toggleAllLabel = allCollapsed ? 'Expand all tasks' : 'Collapse all tasks'
  const toggleAll = () => {
    setCollapsed(ids, !allCollapsed)
    if (!allCollapsed) setAdding(null)
  }

  const moveBy = (id: string, delta: -1 | 1) => {
    const i = ids.indexOf(id)
    if (i + delta < 0 || i + delta >= ids.length) return
    onMove(id, delta < 0 ? ids[i - 1] : (ids[i + 2] ?? null))
    // Moving the group in the DOM drops focus, so put it back on its grip.
    requestAnimationFrame(() => document.querySelector<HTMLElement>(`[data-grip="${CSS.escape(id)}"]`)?.focus())
  }

  const reset = () => {
    setActive(null)
    setOver(null)
  }

  return (
    <section aria-label="Todos" className="min-w-0 overflow-x-auto rounded-xl border border-border bg-white shadow-xs">
      <div className="min-w-[520px]">
        <div className={cn(ROW, 'h-9 border-b border-border text-[12px] font-medium text-ink-muted')}>
          <span className="flex justify-center">
            <button
              type="button"
              aria-label={toggleAllLabel}
              title={toggleAllLabel}
              onClick={toggleAll}
              className="flex size-7 items-center justify-center rounded-md text-ink-muted hover:bg-black/5"
            >
              {allCollapsed ? <ChevronsUpDown size={14} /> : <ChevronsDownUp size={14} />}
            </button>
          </span>
          <span>Todo</span>
          <span>Reminder</span>
          <span className="text-right">When</span>
        </div>

        <DndContext
          sensors={sensors}
          collisionDetection={pointerWithin}
          onDragStart={(e) => setActive(String(e.active.id))}
          onDragOver={(e) => setOver(e.over ? String(e.over.id) : null)}
          onDragCancel={reset}
          onDragEnd={(e) => {
            reset()
            const id = String(e.active.id)
            const to = e.over ? String(e.over.id) : null
            if (!to || to === id) return
            onMove(id, to === END ? null : to)
          }}
        >
          {groups.map((g) => {
            const id = g.task.id
            const open = isOpen(id)
            return (
              <TaskGroup
                key={id}
                group={g}
                selection={selection}
                onSelect={onSelect}
                adding={adding}
                setAdding={setAdding}
                open={open}
                onCollapse={() => setCollapsed([id], open)}
                doneOpen={!!doneOpen[id]}
                onToggleDoneOpen={() => setDoneOpen((m) => ({ ...m, [id]: !m[id] }))}
                onToggleDone={onToggleDone}
                onSetStatus={onSetStatus}
                dragging={active === id}
                dropHere={!!active && active !== id && over === id}
                onMoveBy={(d) => moveBy(id, d)}
              />
            )
          })}
          {active && <EndZone />}
          <DragOverlay dropAnimation={null}>
            {dragged && (
              <div className="flex h-11 max-w-[420px] cursor-grabbing items-center gap-2 rounded-lg border border-border bg-white px-3 shadow-lg">
                <StatusIcon status={dragged.status} size={16} />
                <OriginDot origin={dragged.originated} className="size-2" />
                <span className="shrink-0 font-mono text-[12px] text-ink-secondary">{taskKey(dragged.id)}</span>
                <span className="min-w-0 truncate text-[14px] font-semibold">{dragged.title}</span>
              </div>
            )}
          </DragOverlay>
        </DndContext>

        {groups.length === 0 && (
          <p className="m-0 px-4 py-10 text-center text-[14px] text-ink-muted">
            Nothing to do today. Add a task with <span className="font-medium text-ink-secondary">New task</span>, or sync
            Atlassian Cloud.
          </p>
        )}
      </div>
    </section>
  )
}
