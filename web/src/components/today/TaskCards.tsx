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
import { Bell, ChevronDown, ChevronRight, FileText, Plus, Ticket, X } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { OriginDot, StatusIcon } from '@/components/common/badges'
import { Checkbox } from '@/components/ui/checkbox'
import { useTodoActions, useTodos } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { taskKey, type TaskListItem, type TaskStatus, type Todo } from '@/lib/api'
import { doneLabel, dueTag, fromTaskDate, nextTodo, reminderSummary, toneClass, whenPill, type Tone } from '@/lib/dates'
import { cn } from '@/lib/utils'
import { StatusChip } from './StatusChip'
import { END, GripIcon, type Selection, type TodoGroup } from './TodosCard'

/** TodoMeta is the when, reminder, note and requests line under a card todo's text. */
function TodoMeta({ todo }: { todo: Todo }) {
  const { now, today } = useNow()
  const done = todo.is_done
  const when = done ? null : whenPill(todo, today)
  const finished = done ? doneLabel(todo.completed_at, today) : null
  const rem = done ? null : reminderSummary(todo.reminders, now)
  const reqs = todo.service_desk_requests
  return (
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
  )
}

/** CardTodo is a todo on an unfolded card: its text wraps in full, with when, reminder, note and requests below. */
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
  const done = todo.is_done
  return (
    <div
      className={cn(
        'grid grid-cols-[18px_minmax(0,1fr)] items-start gap-2.5 px-[22px] py-2',
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
        <TodoMeta todo={todo} />
      </button>
    </div>
  )
}

/** NEXT_BG tints a card's Next up block by how urgent its todo is. */
const NEXT_BG: Record<Tone, string> = { over: 'bg-danger-tint', today: 'bg-today-tint', plain: 'bg-bg-muted' }

/** NextUp is a card's most urgent open todo, tinted by urgency. */
function NextUp({
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
  const { today } = useNow()
  return (
    <div
      aria-label="Next up"
      role="group"
      className={cn(
        'mx-2.5 flex flex-col gap-1.5 rounded-[10px] px-3 py-2.5',
        NEXT_BG[whenPill(todo, today).tone],
        selected && 'shadow-[inset_0_0_0_1px_var(--color-accent-line)]',
      )}
    >
      <span className="text-[11px] font-semibold tracking-[0.05em] text-ink-muted uppercase">Next up</span>
      <div className="grid grid-cols-[18px_minmax(0,1fr)] items-start gap-2.5">
        <Checkbox checked={false} label={todo.description} onChange={(v) => onToggleDone(todo, v)} className="mt-px" />
        <button type="button" onClick={onSelect} aria-pressed={selected} className="flex min-w-0 flex-col gap-[5px] text-left">
          <span
            className={cn(
              'text-[14px] leading-[1.45] break-keep [overflow-wrap:anywhere]',
              selected ? 'font-semibold' : 'font-medium',
            )}
          >
            {todo.description}
          </span>
          <TodoMeta todo={todo} />
        </button>
      </div>
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
      <span aria-hidden className="absolute top-[11px] left-[5px] z-[1] flex size-3 cursor-grab text-border-strong">
        <GripIcon />
      </span>
      {children}
      {line && <span aria-hidden className={cn('absolute inset-x-0 z-[1] h-0.5 bg-accent', line === 'top' ? 'top-0' : 'bottom-0')} />}
    </div>
  )
}

/** CardTodos lists a card's folded todos, and its done ones when shown; open ones reorder by drag within the card. */
function CardTodos({
  task,
  todos,
  skip,
  doneOpen,
  selection,
  onSelect,
  onToggleDone,
}: {
  task: TaskListItem
  todos: Todo[]
  /** skip are todos shown elsewhere on the card, left out of the done list. */
  skip: Todo[]
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
  const shown = new Set(skip.map((t) => t.id))
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
    <div className="flex flex-col">
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
    <div className="flex items-center gap-2.5 border-t border-tag-teal-line bg-accent-tint py-2 pr-3 pl-4">
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
        className="h-8 min-w-0 flex-1 bg-transparent px-0.5 text-[13px] outline-none placeholder:text-ink-muted"
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
  /** unfolded shows the todos after Next up. */
  unfolded: boolean
  onUnfold: (on: boolean) => void
  doneOpen: boolean
  onToggleDoneOpen: () => void
  onToggleDone: (todo: Todo, done: boolean) => void
  onSetStatus: (task: TaskListItem, status: TaskStatus) => void
  dragging: boolean
  dropHere: boolean
}

/** TaskCard is one task: its header drags the card and the whole card is a drop target. Only its Next up todo shows until it is unfolded. */
function TaskCard({
  group: { task, todos },
  selection,
  onSelect,
  adding,
  setAdding,
  unfolded,
  onUnfold,
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
  const next = nextTodo(todos, today)
  const rest = todos.filter((t) => t !== next)
  const late = todos.filter((t) => !t.is_done && whenPill(t, today).tone === 'over').length
  const expanded = unfolded || adding
  const isTodoSel = (t: Todo) => selection?.kind === 'todo' && selection.id === t.id
  return (
    <article
      ref={dropRef}
      data-card={task.id}
      aria-label={`${key} ${task.title}`}
      className={cn(
        'flex min-w-0 flex-col rounded-[14px] border bg-white',
        dropHere
          ? 'border-accent shadow-ring-drop'
          : isSel
            ? 'border-accent-line-strong shadow-ring'
            : 'border-border shadow-xs',
        dragging && 'opacity-45',
      )}
    >
      <header ref={dragRef} {...listeners} className="flex flex-col gap-2.5 px-4 pt-3.5 pb-3">
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
            'text-left text-[17px] leading-[1.38] font-semibold tracking-[-0.015em] break-keep [overflow-wrap:anywhere]',
            closed && 'text-ink-muted',
          )}
        >
          {task.title}
        </button>
        <div className="flex items-center gap-2.5 text-[12px] text-ink-muted tabular-nums">
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
              '-mx-1.5 flex h-6 items-center rounded-md border px-1.5',
              doneOpen
                ? 'border-[#ddd6fe] bg-tag-violet-bg text-tag-violet-ink'
                : ndone
                  ? 'border-transparent hover:border-border hover:bg-bg-subtle'
                  : 'border-transparent',
            )}
          >
            {task.done_todo_count}/{task.todo_count} done
          </button>
          {late > 0 && (
            <span className="inline-flex items-center gap-1 font-medium text-danger-ink">
              <span aria-hidden className="size-1.5 rounded-full bg-danger" />
              {late} late
            </span>
          )}
        </div>
      </header>
      {next ? (
        <NextUp todo={next} selected={isTodoSel(next)} onSelect={() => onSelect({ kind: 'todo', id: next.id })} onToggleDone={onToggleDone} />
      ) : (
        <p className="mx-4 py-2 text-[12.5px] text-ink-muted">Nothing due today.</p>
      )}
      {(expanded || doneOpen) && (
        <div className="pt-1.5">
          <CardTodos
            task={task}
            todos={expanded ? rest : []}
            skip={expanded ? todos : next ? [next] : []}
            doneOpen={doneOpen}
            selection={selection}
            onSelect={onSelect}
            onToggleDone={onToggleDone}
          />
          {adding && <NewCardTodo task={task} onDone={() => setAdding(false)} />}
        </div>
      )}
      <div className="mt-2.5 flex items-center gap-1 border-t border-border-subtle px-2 py-1.5">
        <button
          type="button"
          aria-expanded={expanded && rest.length > 0}
          disabled={rest.length === 0}
          onClick={() => {
            onUnfold(!expanded)
            if (expanded && adding) setAdding(false)
          }}
          className="flex h-[30px] items-center gap-1.5 rounded-md px-2 text-[12px] font-medium text-ink-secondary hover:bg-black/5 disabled:text-ink-faint disabled:hover:bg-transparent"
        >
          {rest.length > 0 && (expanded ? <ChevronDown size={13} aria-hidden /> : <ChevronRight size={13} aria-hidden />)}
          {rest.length === 0
            ? 'No other todos'
            : expanded
              ? `Hide ${rest.length} more`
              : `${rest.length} more todo${rest.length > 1 ? 's' : ''}`}
        </button>
        <button
          type="button"
          aria-label={`Add todo to ${key}`}
          onClick={() => {
            onUnfold(true)
            setAdding(true)
          }}
          className="ml-auto flex h-[30px] items-center gap-1.5 rounded-md px-2 text-[12px] font-medium text-ink-muted hover:bg-black/5"
        >
          <Plus size={13} strokeWidth={2.2} />
          Add todo
        </button>
      </div>
    </article>
  )
}

/** useUnfolded is the set of unfolded cards, remembered per browser. */
function useUnfolded() {
  const [unfolded, setState] = useState<Record<string, boolean>>(() => {
    try {
      return JSON.parse(localStorage.getItem('wharf.cardOpen') ?? '{}') as Record<string, boolean>
    } catch {
      return {}
    }
  })
  const set = (id: string, on: boolean) => {
    setState((prev) => {
      const next = { ...prev }
      if (on) next[id] = true
      else delete next[id]
      localStorage.setItem('wharf.cardOpen', JSON.stringify(next))
      return next
    })
  }
  return [unfolded, set] as const
}

function EndSlot() {
  const { setNodeRef, isOver } = useDroppable({ id: END })
  return (
    <div
      ref={setNodeRef}
      data-card={END}
      className={cn(
        'flex min-h-[120px] items-center justify-center rounded-[14px] border-[1.5px] border-dashed text-[12px] text-ink-muted',
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
  const [unfolded, setUnfolded] = useUnfolded()
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
    <section aria-label="Tasks" className="grid min-w-0 grid-cols-[repeat(auto-fill,minmax(310px,1fr))] items-start gap-4">
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
              unfolded={!!unfolded[id]}
              onUnfold={(on) => setUnfolded(id, on)}
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
