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
import { Bell, Check, FileText, Plus, Ticket, X } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { OriginDot, STATUS_LABEL, StatusIcon } from '@/components/common/badges'
import { Checkbox } from '@/components/ui/checkbox'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import { useTodoActions, useTodos } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { taskKey, type TaskListItem, type TaskStatus, type Todo } from '@/lib/api'
import { doneLabel, dueTag, fromTaskDate, reminderSummary, toneClass, whenPill } from '@/lib/dates'
import { cn } from '@/lib/utils'
import { END, GripIcon, STATUSES, type Selection, type TodoGroup } from './TodosCard'

const CHIP: Record<TaskStatus, string> = {
  todo: 'border-border bg-bg-muted text-ink-secondary',
  'in-progress': 'border-warning-line bg-warning-tint text-warning-ink',
  done: 'border-[#ddd6fe] bg-tag-violet-bg text-tag-violet-ink',
  canceled: 'border-border bg-bg-muted text-ink-muted',
}

const chip = 'flex h-6 items-center gap-[5px] rounded-full border pr-2 pl-[5px] text-[12px] font-medium whitespace-nowrap'

/** StatusChip is a card's labeled status pill; it opens the status menu of a local task and is disabled for a Jira one. */
function StatusChip({ task, onSetStatus }: { task: TaskListItem; onSetStatus: (status: TaskStatus) => void }) {
  const [open, setOpen] = useState(false)
  const label = STATUS_LABEL[task.status]
  if (task.originated !== 'local') {
    return (
      <button
        type="button"
        aria-disabled
        aria-label={`Status: ${label}`}
        title={`${label} · from Jira`}
        className={cn(chip, CHIP[task.status], 'cursor-default')}
      >
        <StatusIcon status={task.status} />
        {label}
      </button>
    )
  }
  return (
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={`Status: ${label}`}
          title={`${label} · change status`}
          // Radix opens the menu on pointerdown, so keep it from arming the card's drag too.
          onPointerDown={(e) => e.stopPropagation()}
          className={cn(chip, CHIP[task.status], open && 'border-accent')}
        >
          <StatusIcon status={task.status} />
          {label}
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" sideOffset={4} className="w-[176px] min-w-0">
        {STATUSES.map((s) => (
          <DropdownMenuItem
            key={s}
            onSelect={() => s !== task.status && onSetStatus(s)}
            className={cn('h-8 items-center gap-2 px-2 py-0', s === task.status && 'bg-bg-muted')}
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

/** CardTodo is a todo on a card: its text wraps in full, with when, reminder, note and requests below. */
function CardTodo({
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
      className={cn(
        'grid grid-cols-[18px_minmax(0,1fr)] items-start gap-2.5 px-3.5 py-2',
        selected ? 'bg-accent-tint shadow-[inset_0_0_0_1px_var(--color-accent-line)]' : 'bg-white',
      )}
    >
      <Checkbox checked={done} label={todo.description} onChange={(v) => onToggleDone(todo, v)} className="mt-px" />
      <button type="button" onClick={onSelect} aria-pressed={selected} className="flex min-w-0 flex-col gap-1 text-left">
        <span
          className={cn(
            'text-[13.5px] leading-[1.45] break-keep [overflow-wrap:anywhere]',
            selected && 'font-semibold',
            done && 'text-ink-muted line-through',
          )}
        >
          {todo.description}
        </span>
        <span className="flex flex-wrap items-center gap-1.5 leading-[normal]">
          {when && (
            <span className={cn('rounded-[5px] px-1.5 py-px text-[11px] font-medium whitespace-nowrap', toneClass[when.tone])}>
              {when.label}
            </span>
          )}
          {finished && (
            <span
              className={cn(
                'rounded-[5px] px-1.5 py-px text-[11px] font-medium whitespace-nowrap',
                finished.today ? 'bg-tag-violet-bg text-[#6d28d9]' : 'bg-bg-muted text-ink-muted',
              )}
            >
              {finished.label}
            </span>
          )}
          {rem && (
            <span
              title={rem.sent ? `${rem.sent} sent` : undefined}
              className={cn('inline-flex items-center gap-1 text-[11px] tabular-nums', rem.upcoming ? 'text-[#6d28d9]' : 'text-ink-faint')}
            >
              <Bell size={12} />
              {rem.label}
            </span>
          )}
          {todo.note.trim() && <FileText size={13} className="shrink-0 text-ink-muted" aria-label="Has a note" />}
          {reqs.length > 0 && (
            <span
              title={reqs.map((r) => `${r.key} ${r.title}`).join('\n')}
              className="inline-flex h-[18px] items-center gap-[3px] rounded-full bg-tag-indigo-bg px-[5px] text-[11px] font-medium text-tag-indigo-ink"
            >
              <Ticket size={11} strokeWidth={2.4} />
              {reqs.length}
            </span>
          )}
        </span>
      </button>
    </div>
  )
}

/** DraggableCardTodo is an open todo that drags onto, and takes drops from, the open todos of its card. */
function DraggableCardTodo({ todo, dragging, line, children }: { todo: Todo; dragging: boolean; line: 'top' | 'bottom' | null; children: ReactNode }) {
  const { setNodeRef: dragRef, listeners } = useDraggable({ id: todo.id })
  const { setNodeRef: dropRef } = useDroppable({ id: todo.id })
  return (
    <div
      ref={(node) => {
        dragRef(node)
        dropRef(node)
      }}
      {...listeners}
      data-todo={todo.id}
      className={cn('relative', dragging && 'opacity-45')}
    >
      <span aria-hidden className="absolute top-[11px] left-0.5 z-[1] flex size-3 cursor-grab text-border-strong">
        <GripIcon />
      </span>
      {children}
      {line && <span aria-hidden className={cn('absolute inset-x-0 z-[1] h-0.5 bg-accent', line === 'top' ? 'top-0' : 'bottom-0')} />}
    </div>
  )
}

/** CardTodos lists a card's todos, and its done ones when shown; open ones reorder by drag within the card. */
function CardTodos({
  task,
  todos,
  doneOpen,
  selection,
  onSelect,
  onToggleDone,
}: {
  task: TaskListItem
  todos: Todo[]
  doneOpen: boolean
  selection: Selection
  onSelect: (s: Selection) => void
  onToggleDone: (todo: Todo, done: boolean) => void
}) {
  const { update } = useTodoActions()
  const { data: doneData = [] } = useTodos({ task_id: task.id, is_done: true }, doneOpen)
  const [active, setActive] = useState<number | null>(null)
  const [over, setOver] = useState<number | null>(null)
  // A small distance keeps clicks on the row's checkbox and text from starting a drag.
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))
  const ids = todos.map((t) => t.id)
  const dragged = todos.find((t) => t.id === active)
  const shown = new Set(ids)
  const done = doneOpen
    ? doneData
        .filter((t) => t.is_done && !shown.has(t.id))
        .sort((a, b) => (a.completed_at ?? '').localeCompare(b.completed_at ?? ''))
    : []

  const reset = () => {
    setActive(null)
    setOver(null)
  }

  const row = (t: Todo) => (
    <CardTodo
      key={t.id}
      todo={t}
      selected={selection?.kind === 'todo' && selection.id === t.id}
      onSelect={() => onSelect({ kind: 'todo', id: t.id })}
      onToggleDone={onToggleDone}
    />
  )

  return (
    <div className="flex flex-col py-1">
      <DndContext
        sensors={sensors}
        collisionDetection={pointerWithin}
        onDragStart={(e) => setActive(Number(e.active.id))}
        onDragOver={(e) => setOver(e.over ? Number(e.over.id) : null)}
        onDragCancel={reset}
        onDragEnd={(e) => {
          reset()
          const from = todos.find((t) => t.id === Number(e.active.id))
          const to = e.over && todos.find((t) => t.id === Number(e.over?.id))
          if (!from || !to || from.id === to.id || to.position == null) return
          update(from, { position: to.position })
        }}
      >
        {todos.map((t) =>
          t.is_done ? (
            row(t)
          ) : (
            <DraggableCardTodo
              key={t.id}
              todo={t}
              dragging={active === t.id}
              line={
                active !== null && active !== t.id && over === t.id
                  ? ids.indexOf(active) < ids.indexOf(t.id)
                    ? 'bottom'
                    : 'top'
                  : null
              }
            >
              {row(t)}
            </DraggableCardTodo>
          ),
        )}
        <DragOverlay dropAnimation={null}>
          {dragged && (
            <div className="flex h-11 max-w-[360px] cursor-grabbing items-center gap-2 rounded-lg border border-border bg-white px-3 text-[13.5px] shadow-lg">
              <span className="size-[18px] shrink-0 rounded-[5px] border-[1.5px] border-border-control" />
              <span className="min-w-0 truncate">{dragged.description}</span>
            </div>
          )}
        </DragOverlay>
      </DndContext>
      {done.map(row)}
      {todos.length === 0 && done.length === 0 && (
        <p className="m-0 px-3.5 py-2.5 text-[12px] text-ink-muted">No todos for today yet.</p>
      )}
    </div>
  )
}

function NewCardTodo({ task, onDone }: { task: TaskListItem; onDone: () => void }) {
  const [text, setText] = useState('')
  const { create } = useTodoActions()
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
    <div className="flex items-center gap-2 rounded-b-xl border-t border-tag-teal-line bg-accent-tint py-2 pr-2.5 pl-3.5">
      <span className="size-[18px] shrink-0 rounded-[5px] border-[1.5px] border-dashed border-[#2dd4bf]" />
      <input
        autoFocus
        aria-label={`New todo for ${taskKey(task.id)}`}
        placeholder="New todo — Enter to add"
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && !e.nativeEvent.isComposing) add()
          if (e.key === 'Escape') onDone()
        }}
        className="h-8 min-w-0 flex-1 bg-transparent text-[13px] outline-none placeholder:text-ink-muted"
      />
      <button
        type="button"
        aria-label="Cancel"
        onClick={onDone}
        className="flex size-[26px] shrink-0 items-center justify-center rounded-md text-ink-muted hover:bg-black/5"
      >
        <X size={13} strokeWidth={2.2} />
      </button>
    </div>
  )
}

interface CardProps {
  group: TodoGroup
  selection: Selection
  onSelect: (s: Selection) => void
  adding: boolean
  setAdding: (on: boolean) => void
  doneOpen: boolean
  onToggleDoneOpen: () => void
  onToggleDone: (todo: Todo, done: boolean) => void
  onSetStatus: (task: TaskListItem, status: TaskStatus) => void
  dragging: boolean
  dropHere: boolean
}

/** TaskCard is one task: its header drags the card and the whole card is a drop target. */
function TaskCard({
  group: { task, todos },
  selection,
  onSelect,
  adding,
  setAdding,
  doneOpen,
  onToggleDoneOpen,
  onToggleDone,
  onSetStatus,
  dragging,
  dropHere,
}: CardProps) {
  const { today } = useNow()
  const { setNodeRef: dragRef, listeners } = useDraggable({ id: task.id })
  const { setNodeRef: dropRef } = useDroppable({ id: task.id })
  const isSel = selection?.kind === 'task' && selection.id === task.id
  const closed = task.status === 'done' || task.status === 'canceled'
  const due = dueTag(fromTaskDate(task.due_date), today, closed)
  const pct = task.todo_count ? Math.round((task.done_todo_count / task.todo_count) * 100) : 0
  const key = taskKey(task.id)
  const ndone = task.done_todo_count
  return (
    <article
      ref={dropRef}
      data-card={task.id}
      aria-label={`${key} ${task.title}`}
      className={cn(
        'flex min-w-0 flex-col rounded-xl border bg-white',
        dropHere
          ? 'border-accent shadow-[0_0_0_2px_rgba(15,118,110,0.35)]'
          : isSel
            ? 'border-[#5eead4] shadow-ring'
            : 'border-border shadow-xs',
        dragging && 'opacity-45',
      )}
    >
      <header
        ref={dragRef}
        {...listeners}
        className={cn(
          'flex flex-col gap-2 rounded-t-xl border-b border-border-subtle px-3.5 py-3',
          isSel ? 'bg-accent-tint' : 'bg-bg-subtle',
        )}
      >
        <div className="flex min-w-0 items-center gap-2">
          <StatusChip task={task} onSetStatus={(s) => onSetStatus(task, s)} />
          <span className="inline-flex min-w-0 items-center gap-1.5">
            <OriginDot origin={task.originated} className="size-2" />
            <span title={key} className="truncate font-mono text-[12px] text-ink-secondary">{key}</span>
          </span>
          <span className={cn('ml-auto shrink-0 rounded-md px-[7px] py-px text-[12px] font-medium whitespace-nowrap', toneClass[due.tone])}>
            {due.label}
          </span>
          <span aria-hidden title="Drag to reorder" className="flex size-3 shrink-0 cursor-grab text-ink-faint">
            <GripIcon />
          </span>
        </div>
        <button
          type="button"
          aria-pressed={isSel}
          title="Show task details"
          onClick={() => onSelect({ kind: 'task', id: task.id })}
          className={cn(
            'text-left text-[15px] leading-[1.4] font-semibold tracking-[-0.01em] break-keep [overflow-wrap:anywhere]',
            closed && 'text-ink-muted',
          )}
        >
          {task.title}
        </button>
        <div className="flex items-center gap-2">
          <span className="flex h-1.5 flex-1 overflow-hidden rounded-full bg-bg-pressed">
            <span className="rounded-full bg-tag-violet-solid" style={{ width: `${pct}%` }} />
          </span>
          <button
            type="button"
            aria-pressed={doneOpen}
            disabled={!ndone && !doneOpen}
            title={doneOpen ? 'Hide done todos' : ndone ? `Show ${ndone} done todo${ndone > 1 ? 's' : ''}` : 'No done todos yet'}
            onClick={onToggleDoneOpen}
            className={cn(
              'flex h-6 items-center rounded-md border px-1.5 text-[12px] tabular-nums',
              doneOpen
                ? 'border-[#ddd6fe] bg-tag-violet-bg text-tag-violet-ink'
                : ndone
                  ? 'border-border bg-white text-ink-muted hover:bg-bg-subtle'
                  : 'border-transparent text-ink-muted',
            )}
          >
            {task.done_todo_count}/{task.todo_count}
          </button>
        </div>
      </header>
      <CardTodos
        task={task}
        todos={todos}
        doneOpen={doneOpen}
        selection={selection}
        onSelect={onSelect}
        onToggleDone={onToggleDone}
      />
      {adding ? (
        <NewCardTodo task={task} onDone={() => setAdding(false)} />
      ) : (
        <button
          type="button"
          aria-label={`Add todo to ${key}`}
          onClick={() => setAdding(true)}
          className="flex h-9 items-center gap-1.5 rounded-b-xl border-t border-border-subtle bg-white px-3.5 text-left text-[12px] font-medium text-ink-muted hover:bg-bg-subtle"
        >
          <Plus size={13} strokeWidth={2.2} />
          Add todo
        </button>
      )}
    </article>
  )
}

function EndSlot() {
  const { setNodeRef, isOver } = useDroppable({ id: END })
  return (
    <div
      ref={setNodeRef}
      data-card={END}
      className={cn(
        'flex min-h-[120px] items-center justify-center rounded-xl border-[1.5px] border-dashed text-[12px] text-ink-muted',
        isOver ? 'border-accent bg-accent-tint' : 'border-border-strong bg-bg-subtle',
      )}
    >
      Drop here to move to the end
    </div>
  )
}

/** TaskCards is the Card view of Today: one card per task, reordered by drag. */
export function TaskCards({
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
  const [doneOpen, setDoneOpen] = useState<Record<string, boolean>>({})
  const [active, setActive] = useState<string | null>(null)
  const [over, setOver] = useState<string | null>(null)
  // A small distance keeps clicks on the header's buttons from starting a drag.
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))
  const dragged = groups.find((g) => g.task.id === active)?.task

  const reset = () => {
    setActive(null)
    setOver(null)
  }

  if (groups.length === 0) {
    return (
      <section aria-label="Tasks" className="rounded-xl border border-border bg-white shadow-xs">
        <p className="m-0 px-4 py-10 text-center text-[14px] text-ink-muted">
          Nothing to do today. Add a task with <span className="font-medium text-ink-secondary">New task</span>, or sync
          Atlassian Cloud.
        </p>
      </section>
    )
  }

  return (
    <section aria-label="Tasks" className="grid min-w-0 grid-cols-[repeat(auto-fill,minmax(300px,1fr))] items-start gap-4">
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
          return (
            <TaskCard
              key={id}
              group={g}
              selection={selection}
              onSelect={onSelect}
              adding={adding === id}
              setAdding={(on) => setAdding(on ? id : null)}
              doneOpen={!!doneOpen[id]}
              onToggleDoneOpen={() => setDoneOpen((m) => ({ ...m, [id]: !m[id] }))}
              onToggleDone={onToggleDone}
              onSetStatus={onSetStatus}
              dragging={active === id}
              dropHere={!!active && active !== id && over === id}
            />
          )
        })}
        {active && <EndSlot />}
        <DragOverlay dropAnimation={null}>
          {dragged && (
            <div className="flex h-11 max-w-[360px] cursor-grabbing items-center gap-2 rounded-lg border border-border bg-white px-3 shadow-lg">
              <StatusIcon status={dragged.status} size={16} />
              <OriginDot origin={dragged.originated} className="size-2" />
              <span className="shrink-0 font-mono text-[12px] text-ink-secondary">{taskKey(dragged.id)}</span>
              <span className="min-w-0 truncate text-[14px] font-semibold">{dragged.title}</span>
            </div>
          )}
        </DragOverlay>
      </DndContext>
    </section>
  )
}
