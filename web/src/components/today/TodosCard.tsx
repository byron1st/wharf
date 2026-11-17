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
import { Bell, ChevronDown, ChevronRight, ChevronsDownUp, ChevronsUpDown, FileText, Plus, Ticket, X } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Kbd, OriginDot, StatusIcon } from '@/components/common/badges'
import { Checkbox } from '@/components/ui/checkbox'
import { useTodoActions, useTodos } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { useStoredFlags } from '@/hooks/useStoredFlags'
import { taskKey, type TaskListItem, type TaskStatus, type Todo } from '@/lib/api'
import { doneLabel, dueTag, fromTaskDate, groupSummary, nextTodo, reminderSummary, toneClass, whenPill } from '@/lib/dates'
import { cn } from '@/lib/utils'
import { StatusChip } from './StatusChip'

export type Selection = { kind: 'todo'; id: number } | { kind: 'task'; id: string } | { kind: 'req'; id: string } | null

export interface TodoGroup {
  task: TaskListItem
  /** todos are the group's todos for today, with the ones checked this session kept in place. */
  todos: Todo[]
}

/** ROW is a todo row, indented under its task's guide line. */
const ROW = 'relative ml-11 grid min-h-[42px] grid-cols-[18px_minmax(0,1fr)_auto] items-center gap-3 rounded-lg py-1.5 pr-3.5 pl-[18px]'
/** END is the droppable ID of the "move to the end" area. */
export const END = '__end'

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
    <div className="flex items-center gap-2.5 rounded-b-xl border-t border-tag-teal-line bg-accent-tint py-2 pr-3 pl-4">
      <span className="size-[18px] shrink-0 rounded-[5px] border-[1.5px] border-dashed border-[#2dd4bf]" />
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
        className="h-[30px] min-w-0 flex-1 bg-transparent text-[13px] outline-none placeholder:text-ink-muted"
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
    <div className={cn(ROW, selected ? 'bg-accent-tint shadow-[inset_0_0_0_1px_var(--color-accent-line)]' : 'bg-white')}>
      <span aria-hidden className="absolute top-1/2 -left-[13px] h-0.5 w-2.5 bg-bg-skeleton" />
      <Checkbox checked={done} label={todo.description} onChange={(v) => onToggleDone(todo, v)} />
      <button
        type="button"
        onClick={onSelect}
        aria-pressed={selected}
        className="flex min-w-0 flex-wrap items-center gap-2 text-left"
      >
        <span
          className={cn(
            'text-[14px] leading-[1.45] [overflow-wrap:anywhere] break-keep',
            selected && 'font-semibold',
            done && 'text-ink-muted line-through',
          )}
        >
          {todo.description}
        </span>
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
      <span className="flex items-center gap-2.5">
        {rem && (
          <span
            title={rem.sent ? `${rem.sent} sent` : undefined}
            className={cn(
              'inline-flex items-center gap-1 text-[12px] whitespace-nowrap tabular-nums',
              rem.upcoming ? 'text-[#6d28d9]' : 'text-ink-muted',
            )}
          >
            <Bell size={12} aria-hidden />
            {rem.label}
          </span>
        )}
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

/** DraggableTodo is an open todo row that drags onto, and takes drops from, the open todos of its task. */
function DraggableTodo({
  todo,
  dragging,
  line,
  children,
}: {
  todo: Todo
  dragging: boolean
  /** line marks where a dropped todo will land. */
  line: 'top' | 'bottom' | null
  children: ReactNode
}) {
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
      <span aria-hidden className="absolute top-1/2 left-[47px] z-[1] flex size-3 -translate-y-1/2 cursor-grab text-border-strong">
        <GripIcon />
      </span>
      {children}
      {line && <span aria-hidden className={cn('absolute right-0 left-11 z-[1] h-0.5 bg-accent', line === 'top' ? 'top-0' : 'bottom-0')} />}
    </div>
  )
}

/** TodoRows lists a task's todos for today; open ones reorder by drag within the task. */
function TodoRows({
  todos,
  selection,
  onSelect,
  onToggleDone,
}: {
  todos: Todo[]
  selection: Selection
  onSelect: (s: Selection) => void
  onToggleDone: (todo: Todo, done: boolean) => void
}) {
  const { update } = useTodoActions()
  const [active, setActive] = useState<number | null>(null)
  const [over, setOver] = useState<number | null>(null)
  // A small distance keeps clicks on the row's checkbox and text from starting a drag.
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))
  const ids = todos.map((t) => t.id)
  const dragged = todos.find((t) => t.id === active)

  const reset = () => {
    setActive(null)
    setOver(null)
  }

  const row = (t: Todo) => (
    <TodoRow
      key={t.id}
      todo={t}
      selected={selection?.kind === 'todo' && selection.id === t.id}
      onSelect={() => onSelect({ kind: 'todo', id: t.id })}
      onToggleDone={onToggleDone}
    />
  )

  return (
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
          <DraggableTodo
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
          </DraggableTodo>
        ),
      )}
      <DragOverlay dropAnimation={null}>
        {dragged && (
          <div className="flex h-11 max-w-[420px] cursor-grabbing items-center gap-2 rounded-lg border border-border bg-white px-3 text-[14px] shadow-lg">
            <span className="size-[18px] shrink-0 rounded-[5px] border-[1.5px] border-border-control" />
            <span className="min-w-0 truncate">{dragged.description}</span>
          </div>
        )}
      </DragOverlay>
    </DndContext>
  )
}

/** GripIcon is the six-dot drag handle. */
export function GripIcon() {
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
  const next = open || closed ? undefined : nextTodo(todos, today)
  return (
    <article
      ref={dropRef}
      aria-label={`${key} ${task.title}`}
      className={cn(
        'relative rounded-xl border bg-white',
        dropHere
          ? 'border-accent shadow-ring-drop'
          : isSel
            ? 'border-accent-line-strong shadow-ring'
            : 'border-border shadow-xs',
        dragging && 'opacity-45',
      )}
    >
      <div
        ref={dragRef}
        {...listeners}
        className={cn(
          'relative grid grid-cols-[28px_minmax(0,1fr)_auto] items-start gap-2.5 rounded-t-xl border-b py-3 pr-3.5 pl-4',
          open ? 'border-border-subtle' : 'rounded-b-xl border-transparent',
          isSel ? 'bg-accent-tint' : open ? 'bg-bg-subtle' : 'bg-white',
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
          className="absolute top-5 left-1 flex size-3 cursor-grab text-ink-faint focus-visible:text-accent"
        >
          <GripIcon />
        </button>
        <button
          type="button"
          aria-expanded={open}
          aria-label={`Collapse or expand ${key}`}
          onClick={onCollapse}
          className="mt-px flex size-7 items-center justify-center rounded-md text-ink-muted hover:bg-black/5"
        >
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </button>
        <div className="flex min-w-0 flex-col gap-1.5">
          <button
            type="button"
            aria-pressed={isSel}
            title="Show task details"
            onClick={() => onSelect({ kind: 'task', id: task.id })}
            className={cn(
              'text-left text-[16px] leading-[1.4] font-semibold tracking-[-0.01em] [overflow-wrap:anywhere] break-keep',
              closed && 'text-ink-muted',
            )}
          >
            {task.title}
          </button>
          <div className="flex flex-wrap items-center gap-2">
            <StatusChip task={task} onSetStatus={(s) => onSetStatus(task, s)} />
            <span className="inline-flex items-center gap-1.5">
              <OriginDot origin={task.originated} className="size-2" />
              <span className="font-mono text-[12px] text-ink-secondary">{key}</span>
            </span>
            <span className={cn('rounded-md px-[7px] py-px text-[12px] font-medium whitespace-nowrap', toneClass[due.tone])}>
              {due.label}
            </span>
            <span className="text-[12px] text-ink-muted">{groupSummary(todos, today)}</span>
          </div>
          {next && (
            <span className="flex min-w-0 items-center gap-1.5 text-[12.5px] text-ink-secondary">
              <span className="shrink-0 text-[11px] font-semibold tracking-[0.04em] text-ink-muted uppercase">Next</span>
              <span className="min-w-0 [overflow-wrap:anywhere] break-keep">{next.description}</span>
              <NextWhen todo={next} today={today} />
            </span>
          )}
        </div>
        <span className="flex items-center gap-2 pt-0.5">
          <button
            type="button"
            aria-pressed={doneOpen}
            disabled={!ndone && !doneOpen}
            title={doneOpen ? 'Hide done todos' : ndone ? `Show ${ndone} done todo${ndone > 1 ? 's' : ''}` : 'No done todos yet'}
            onClick={onToggleDoneOpen}
            className={cn(
              'flex h-7 items-center gap-2 rounded-md border px-2 text-[12px] tabular-nums',
              doneOpen
                ? 'border-[#ddd6fe] bg-tag-violet-bg text-tag-violet-ink'
                : ndone
                  ? 'border-border bg-white text-ink-muted hover:bg-bg-subtle'
                  : 'border-transparent text-ink-muted',
            )}
          >
            <span aria-hidden className="flex h-[5px] w-12 overflow-hidden rounded-full bg-bg-pressed">
              <span className="bg-tag-violet-solid" style={{ width: `${pct}%` }} />
            </span>
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
          <div className="relative py-0.5">
            <span aria-hidden className="absolute top-0 bottom-3.5 left-[30px] w-0.5 rounded-sm bg-bg-skeleton" />
            <TodoRows todos={todos} selection={selection} onSelect={onSelect} onToggleDone={onToggleDone} />
            {doneOpen && (
              <DoneTodos task={task} shown={todos} selection={selection} onSelect={onSelect} onToggleDone={onToggleDone} />
            )}
            {todos.length === 0 && adding !== task.id && !(doneOpen && ndone > 0) && <p className="m-0 ml-11 px-[18px] py-2.5 text-[12.5px] text-ink-muted">No todos for today yet.</p>}
          </div>
          {adding === task.id && <NewTodoRow task={task} onDone={() => setAdding(null)} />}
        </>
      )}
    </article>
  )
}

/** NextWhen is the when pill of a collapsed task's next todo. */
function NextWhen({ todo, today }: { todo: Todo; today: string }) {
  const w = whenPill(todo, today)
  return <span className={cn('shrink-0 rounded-[5px] px-1.5 py-px text-[11px] font-medium', toneClass[w.tone])}>{w.label}</span>
}

function EndZone() {
  const { setNodeRef, isOver } = useDroppable({ id: END })
  return (
    <div
      ref={setNodeRef}
      className={cn(
        'box-border flex h-10 items-center justify-center rounded-xl border-[1.5px] border-dashed text-[12px] text-ink-muted',
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
  const [collapsed, setCollapsed] = useStoredFlags('wharf.collapsed')
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
    <section aria-label="Todos" className="flex min-w-0 flex-col gap-3">
      <div className="-my-1 flex justify-end">
        <button
          type="button"
          disabled={groups.length === 0}
          onClick={toggleAll}
          className="flex h-7 items-center gap-1.5 rounded-md px-2 text-[12px] font-medium text-ink-muted hover:bg-black/5 disabled:opacity-50 disabled:hover:bg-transparent"
        >
          {allCollapsed ? <ChevronsUpDown size={14} aria-hidden /> : <ChevronsDownUp size={14} aria-hidden />}
          {toggleAllLabel}
        </button>
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
              onCollapse={() => {
                setCollapsed([id], open)
                if (open && adding === id) setAdding(null)
              }}
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
    </section>
  )
}
