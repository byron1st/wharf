import {
  DndContext,
  DragOverlay,
  PointerSensor,
  pointerWithin,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type CollisionDetection,
} from '@dnd-kit/core'
import { Bell, FileText, Plus, Ticket } from 'lucide-react'
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { OriginDot } from '@/components/common/badges'
import { Checkbox } from '@/components/ui/checkbox'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useTodoActions } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { taskKey, type TaskListItem, type Todo } from '@/lib/api'
import {
  addDays,
  BOARD_COLUMNS,
  boardColumn,
  boardMoveDates,
  compareBoardTodos,
  reminderSummary,
  toneClass,
  whenPill,
  type BoardColumn,
} from '@/lib/dates'
import { cn } from '@/lib/utils'
import { GripIcon, type Selection, type TodoGroup } from './TodosCard'

const COLUMN: Record<
  BoardColumn,
  { label: string; hint: string; empty: string; dot: string; title: string; count: string; bg: string; line: string }
> = {
  late: {
    label: 'Late',
    hint: 'End date passed',
    empty: 'Nothing late. Nice.',
    dot: 'bg-danger',
    title: 'text-danger-ink',
    count: 'bg-[#fee2e2] text-danger-ink',
    bg: 'bg-[#fef7f7]',
    line: 'border-[#fde2e2]',
  },
  today: {
    label: 'Due today',
    hint: 'Ends today',
    empty: 'Drop a todo here to finish it today',
    dot: 'bg-today',
    title: 'text-[#9a3412]',
    count: 'bg-[#ffedd5] text-[#9a3412]',
    bg: 'bg-[#fffaf5]',
    line: 'border-[#fdebd8]',
  },
  later: {
    label: 'Upcoming',
    hint: 'Due after today',
    empty: 'Nothing scheduled',
    dot: 'bg-accent',
    title: 'text-ink',
    count: 'bg-bg-pressed text-ink-secondary',
    bg: 'bg-[#f7f7f8]',
    line: 'border-bg-skeleton',
  },
  nodate: {
    label: 'No date',
    hint: 'No end date yet',
    empty: 'Drop here to clear the date',
    dot: 'bg-ink-faint',
    title: 'text-ink',
    count: 'bg-bg-pressed text-ink-secondary',
    bg: 'bg-[#f7f7f8]',
    line: 'border-bg-skeleton',
  },
}

/** COL_PREFIX marks a column's droppable ID; a card's droppable ID is its todo ID. */
const COL_PREFIX = 'col:'

/** cardFirst prefers the card under the pointer over the column around it. */
const cardFirst: CollisionDetection = (args) => {
  const hits = pointerWithin(args)
  return [...hits.filter((h) => typeof h.id === 'number'), ...hits.filter((h) => typeof h.id !== 'number')]
}

/** BoardCard is a todo on the board: its task, its text, and its when, reminder, note and requests. */
function BoardCard({
  todo,
  task,
  column,
  selected,
  taskSelected,
  onSelect,
  onToggleDone,
  onMoveKey,
  dragging,
  line,
}: {
  todo: Todo
  task: TaskListItem | undefined
  column: BoardColumn
  selected: boolean
  taskSelected: boolean
  onSelect: (s: Selection) => void
  onToggleDone: (todo: Todo, done: boolean) => void
  onMoveKey: (key: 'ArrowUp' | 'ArrowDown' | 'ArrowLeft' | 'ArrowRight') => void
  dragging: boolean
  /** line marks the side of the card the dragged todo will land on. */
  line: 'top' | 'bottom' | null
}) {
  const { now, today } = useNow()
  const done = todo.is_done
  const { setNodeRef: dragRef, listeners } = useDraggable({ id: todo.id, disabled: done })
  const { setNodeRef: dropRef } = useDroppable({ id: todo.id, disabled: done })
  const key = taskKey(todo.task_id)
  const when = whenPill(todo, today)
  const showWhen = !done && column !== 'today' && column !== 'nodate'
  const rem = done ? null : reminderSummary(todo.reminders, now)
  const reqs = done ? [] : todo.service_desk_requests
  const note = !done && todo.note.trim() !== ''
  return (
    <>
      {line === 'top' && <div aria-hidden className="-my-[3px] h-[3px] shrink-0 rounded-sm bg-accent" />}
      <div
        ref={(node) => {
          dragRef(node)
          dropRef(node)
        }}
        {...(done ? {} : listeners)}
        role="listitem"
        data-todo={todo.id}
        className={cn(
          'relative flex shrink-0 flex-col gap-2 rounded-[10px] border px-3 pt-2.5 pb-[11px]',
          selected
            ? 'border-accent-line-strong bg-accent-tint shadow-ring'
            : done
              ? 'border-bg-skeleton bg-[#fbfbfc]'
              : cn('bg-white shadow-xs', taskSelected ? 'border-accent-line' : column === 'late' ? 'border-danger-line' : 'border-border'),
          !done && 'cursor-grab',
          dragging && 'opacity-40',
        )}
      >
        <button
          type="button"
          title={`${key} · ${task?.title ?? ''} — show task`}
          onClick={() => onSelect({ kind: 'task', id: todo.task_id })}
          className="flex min-w-0 items-center gap-1.5 pr-3.5 text-left"
        >
          <OriginDot origin={task?.originated} />
          <span className={cn('shrink-0 font-mono text-[11px]', done ? 'text-ink-faint' : taskSelected ? 'text-accent' : 'text-ink-secondary')}>
            {key}
          </span>
          <span className={cn('min-w-0 truncate text-[11.5px]', done ? 'text-ink-faint' : taskSelected ? 'text-accent' : 'text-ink-muted')}>
            {task?.title}
          </span>
        </button>
        {!done && (
          <button
            type="button"
            data-grip={todo.id}
            aria-label={`Move ${todo.description}`}
            title="Drag to reorder or move · ⌥↑ / ⌥↓ to reorder · ⌥← / ⌥→ to move to another column"
            onKeyDown={(e) => {
              if (!e.altKey || !['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'].includes(e.key)) return
              e.preventDefault()
              onMoveKey(e.key as 'ArrowUp' | 'ArrowDown' | 'ArrowLeft' | 'ArrowRight')
            }}
            className="absolute top-[11px] right-[9px] flex cursor-grab text-border-strong focus-visible:text-accent"
          >
            <GripIcon />
          </button>
        )}
        <div className="grid grid-cols-[18px_minmax(0,1fr)] items-start gap-2.5">
          <Checkbox checked={done} label={todo.description} onChange={(v) => onToggleDone(todo, v)} className="mt-px" />
          <button
            type="button"
            aria-pressed={selected}
            onClick={() => onSelect({ kind: 'todo', id: todo.id })}
            className={cn(
              'min-w-0 text-left text-[13.5px] leading-[1.45] break-keep [overflow-wrap:anywhere]',
              selected && 'font-semibold',
              done && 'text-ink-muted line-through',
            )}
          >
            {todo.description}
          </button>
        </div>
        {(showWhen || rem || note || reqs.length > 0) && (
          <div className="flex flex-wrap items-center gap-1.5 pl-7">
            {showWhen && (
              <span className={cn('rounded-[5px] px-1.5 py-px text-[11px] font-medium whitespace-nowrap', toneClass[when.tone])}>
                {when.label}
              </span>
            )}
            {rem && (
              <span
                title={rem.sent ? `${rem.sent} sent` : undefined}
                className={cn('inline-flex items-center gap-1 text-[11px] tabular-nums', rem.upcoming ? 'text-[#6d28d9]' : 'text-ink-faint')}
              >
                <Bell size={12} aria-hidden />
                {rem.label}
              </span>
            )}
            {note && <FileText size={13} className="shrink-0 text-ink-muted" aria-label="Has a note" />}
            {reqs.length > 0 && (
              <span
                title={reqs.map((r) => `${r.key} ${r.title}`).join('\n')}
                className="inline-flex h-[18px] items-center gap-[3px] rounded-full bg-tag-indigo-bg px-[5px] text-[11px] font-medium text-tag-indigo-ink"
              >
                <Ticket size={11} strokeWidth={2.4} />
                {reqs.length}
              </span>
            )}
          </div>
        )}
      </div>
      {line === 'bottom' && <div aria-hidden className="-my-[3px] h-[3px] shrink-0 rounded-sm bg-accent" />}
    </>
  )
}

/** Column is one board column: its header, its open cards and the ones done today. */
function Column({
  column,
  open,
  done,
  accepts,
  hot,
  lineEnd,
  children,
}: {
  column: BoardColumn
  open: number
  done: number
  /** accepts says whether the todo being dragged may land here. */
  accepts: boolean
  /** hot is set while a todo that may land here is dragged over the column. */
  hot: boolean
  /** lineEnd marks the end of the column as where the dragged todo will land. */
  lineEnd: boolean
  children: { cards: ReactNode; doneCards: ReactNode }
}) {
  const c = COLUMN[column]
  const { setNodeRef } = useDroppable({ id: COL_PREFIX + column, disabled: !accepts })
  return (
    <section
      ref={setNodeRef}
      aria-label={c.label}
      data-col={column}
      className={cn(
        'flex min-h-0 min-w-0 flex-col overflow-hidden rounded-xl',
        hot ? 'bg-accent-tint shadow-[inset_0_0_0_2px_var(--color-accent)]' : c.bg,
      )}
    >
      <div className={cn('flex shrink-0 flex-col gap-0.5 border-b px-3 pt-3 pb-2.5', c.line)}>
        <span className="flex items-center gap-[7px]">
          <span aria-hidden className={cn('size-2 rounded-full', c.dot)} />
          <span className={cn('text-[13.5px] font-semibold', c.title)}>{c.label}</span>
          <span
            className={cn(
              'inline-flex h-[18px] min-w-5 items-center justify-center rounded-full px-1.5 text-[11px] font-semibold tabular-nums',
              c.count,
            )}
          >
            {open}
          </span>
        </span>
        <span className="pl-[15px] text-[11.5px] text-ink-muted">{c.hint}</span>
      </div>
      <div role="list" className="flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-2 pt-2.5 pb-3.5 [scrollbar-width:thin]">
        {children.cards}
        {lineEnd && <div aria-hidden className="-mt-0.5 h-[3px] shrink-0 rounded-sm bg-accent" />}
        {open === 0 && (
          <div
            className={cn(
              'shrink-0 rounded-[10px] border-[1.5px] border-dashed px-2.5 py-[18px] text-center text-[12px] text-ink-muted',
              hot ? 'border-accent' : 'border-border',
            )}
          >
            {c.empty}
          </div>
        )}
        {done > 0 && (
          <>
            <div className="mt-1 flex shrink-0 items-center gap-2 px-1 text-[11px] font-semibold tracking-[0.04em] text-ink-muted uppercase">
              <span>Done today · {done}</span>
              <span aria-hidden className="h-px flex-1 bg-bg-skeleton" />
            </div>
            {children.doneCards}
          </>
        )}
      </div>
    </section>
  )
}

const DUES = [
  { value: 'today', label: 'Today', lands: 'Lands in Due today' },
  { value: 'tomorrow', label: 'Tomorrow', lands: 'Lands in Upcoming' },
  { value: 'none', label: 'No date', lands: 'Lands in No date' },
] as const

type Due = (typeof DUES)[number]['value']

/** NewTodoButton opens the "New todo" popover; T opens it from anywhere outside a text field or dialog. */
function NewTodoButton({ tasks, defaultTask }: { tasks: TaskListItem[]; defaultTask: string | null }) {
  const { today } = useNow()
  const { create } = useTodoActions()
  const [open, setOpen] = useState(false)
  const [text, setText] = useState('')
  const [taskID, setTaskID] = useState('')
  const [due, setDue] = useState<Due>('none')
  const input = useRef<HTMLInputElement>(null)

  const show = (on: boolean) => {
    if (on && tasks.length === 0) return
    if (on) {
      setText('')
      setDue('none')
      setTaskID(defaultTask ?? tasks[0]?.id ?? '')
    }
    setOpen(on)
  }

  // The listener is added once and opens with the defaults of the latest render.
  const showRef = useRef(show)
  useEffect(() => {
    showRef.current = show
  })
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if (e.key.toLowerCase() !== 't' || e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"], [role="dialog"]')) return
      e.preventDefault()
      showRef.current(true)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const add = () => {
    const d = text.trim()
    if (!d || !taskID) return
    setOpen(false)
    void create({
      task_id: taskID,
      description: d,
      note: '',
      is_done: false,
      start_date: null,
      end_date: due === 'today' ? today : due === 'tomorrow' ? addDays(today, 1) : null,
      reminders: [],
    }).catch(() => {
      // The failure is already reported with a Retry.
    })
  }

  return (
    <Popover open={open} onOpenChange={show}>
      <PopoverTrigger asChild>
        <button
          type="button"
          title="New todo (T)"
          disabled={tasks.length === 0}
          className="flex h-8 shrink-0 items-center gap-[7px] rounded-lg bg-accent pr-2 pl-[11px] text-[13px] font-medium text-white hover:bg-accent-hover disabled:opacity-50"
        >
          <Plus size={14} strokeWidth={2.2} aria-hidden />
          New todo
          <kbd className="inline-flex h-[18px] min-w-[18px] items-center justify-center rounded bg-white/20 px-1 font-sans text-[11px] text-white/90">
            T
          </kbd>
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        sideOffset={8}
        aria-label="New todo"
        onOpenAutoFocus={(e) => {
          e.preventDefault()
          input.current?.focus()
        }}
        className="flex w-[360px] flex-col gap-3 p-3.5"
      >
        <label className="flex flex-col gap-1.5">
          <span className="text-[12px] font-medium text-ink-secondary">Todo</span>
          <input
            ref={input}
            value={text}
            placeholder="What needs doing?"
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' && !e.nativeEvent.isComposing) add()
            }}
            className="h-9 rounded-lg border border-border-strong px-2.5 text-[14px] outline-none placeholder:text-ink-muted focus:border-accent-line-strong focus:shadow-ring"
          />
        </label>
        <label className="flex flex-col gap-1.5">
          <span className="text-[12px] font-medium text-ink-secondary">Task</span>
          <select
            value={taskID}
            onChange={(e) => setTaskID(e.target.value)}
            className="h-8 rounded-lg border border-border bg-white px-2 text-[13px]"
          >
            {tasks.map((t) => (
              <option key={t.id} value={t.id}>
                {taskKey(t.id)} · {t.title}
              </option>
            ))}
          </select>
        </label>
        <div className="flex flex-col gap-1.5">
          <span className="text-[12px] font-medium text-ink-secondary">Due</span>
          <div role="radiogroup" aria-label="Due" className="flex gap-0.5 rounded-lg bg-bg-muted p-0.5">
            {DUES.map((o) => (
              <button
                key={o.value}
                type="button"
                role="radio"
                aria-checked={due === o.value}
                onClick={() => setDue(o.value)}
                className={cn(
                  'h-7 flex-1 rounded-md text-[12.5px] font-medium',
                  due === o.value ? 'bg-white text-ink shadow-xs' : 'text-ink-muted',
                )}
              >
                {o.label}
              </button>
            ))}
          </div>
        </div>
        <div className="flex items-center gap-2 pt-0.5">
          <span className="min-w-0 flex-1 text-[12px] text-ink-muted">{DUES.find((o) => o.value === due)?.lands}</span>
          <button
            type="button"
            onClick={() => setOpen(false)}
            className="h-8 rounded-lg border border-border bg-white px-3 text-[13px] font-medium text-ink-secondary hover:bg-bg-subtle"
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={!text.trim() || !taskID}
            onClick={add}
            className="h-8 rounded-lg bg-accent px-3.5 text-[13px] font-medium whitespace-nowrap text-white hover:bg-accent-hover disabled:bg-accent-line-strong"
          >
            Add todo
          </button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

/**
 * TodoBoard is the Todo view of Today: Today's open todos in Late, Due today,
 * Upcoming and No date columns, with the ones done today at the bottom of
 * theirs. Dragging a todo to another column changes its dates, and its place
 * in a column is stored as its board position.
 */
export function TodoBoard({
  groups,
  doneToday,
  selection,
  onSelect,
  onToggleDone,
}: {
  groups: TodoGroup[]
  /** doneToday are the todos completed today, whatever their task. */
  doneToday: Todo[]
  selection: Selection
  onSelect: (s: Selection) => void
  onToggleDone: (todo: Todo, done: boolean) => void
}) {
  const { today } = useNow()
  const { update, reorderBoard } = useTodoActions()
  const [picked, setPicked] = useState<string | null>(null)
  const [active, setActive] = useState<number | null>(null)
  const [over, setOver] = useState<string | number | null>(null)
  // A small distance keeps clicks on a card's checkbox, text and task from starting a drag.
  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 4 } }))

  // Derived once per data change, not on every drag-over render.
  const { tasks, taskMap, openTodos, counts, columns, done, colOf, indexOf } = useMemo(() => {
    const tasks = groups.map((g) => g.task)
    const taskMap = new Map(tasks.map((t) => [t.id, t]))
    const taskRank = new Map(tasks.map((t, i) => [t.id, i]))
    const openTodos = groups.flatMap((g) => g.todos.filter((t) => !t.is_done))
    const counts = new Map<string, number>()
    for (const t of openTodos) counts.set(t.task_id, (counts.get(t.task_id) ?? 0) + 1)

    /** columns are the open todos of each column, of every task, in board order. */
    const columns = new Map<BoardColumn, Todo[]>(BOARD_COLUMNS.map((c) => [c, []]))
    for (const t of openTodos) columns.get(boardColumn(t, today))?.push(t)
    for (const [c, list] of columns) list.sort((a, b) => compareBoardTodos(a, b, taskRank, c))
    const done = new Map<BoardColumn, Todo[]>(BOARD_COLUMNS.map((c) => [c, []]))
    // A todo checked here is done in the open list's cache before the done list is fetched again.
    const finished = new Map<number, Todo>()
    for (const t of [...doneToday, ...groups.flatMap((g) => g.todos)]) {
      if (t.is_done && taskMap.has(t.task_id)) finished.set(t.id, t)
    }
    for (const t of finished.values()) done.get(boardColumn(t, today))?.push(t)
    for (const [c, list] of done) list.sort((a, b) => compareBoardTodos(a, b, taskRank, c))
    const colOf = new Map<number, BoardColumn>()
    const indexOf = new Map<number, number>()
    for (const [c, list] of columns) {
      list.forEach((t, i) => {
        colOf.set(t.id, c)
        indexOf.set(t.id, i)
      })
    }
    return { tasks, taskMap, openTodos, counts, columns, done, colOf, indexOf }
  }, [groups, doneToday, today])
  // A task whose last open todo is done loses its chip, and with it the filter.
  const filter = picked && counts.has(picked) ? picked : null
  const shown = (list: Todo[]) => (filter ? list.filter((t) => t.task_id === filter) : list)
  const dragged = active === null ? undefined : openTodos.find((t) => t.id === active)
  const from = active === null ? undefined : colOf.get(active)
  /** accepts says whether the dragged todo may land in column c: Late only takes its own. */
  const accepts = (c: BoardColumn) => from !== undefined && (c !== 'late' || from === 'late')
  const overCol =
    typeof over === 'number' ? colOf.get(over) : typeof over === 'string' ? (over.slice(COL_PREFIX.length) as BoardColumn) : undefined

  const reset = () => {
    setActive(null)
    setOver(null)
  }

  /** below says whether card id sits below the dragged todo in its own column, so the todo lands after it. */
  const below = (id: number) => {
    if (active === null || from === undefined || colOf.get(id) !== from) return false
    return (indexOf.get(id) ?? 0) > (indexOf.get(active) ?? 0)
  }

  /**
   * place puts todo, now in column from, at index at among the other open
   * todos of column col (every task's, also the ones a filter hides), saving
   * its dates first when it changes column. Late takes no todo from elsewhere.
   */
  const place = (todo: Todo, from: BoardColumn, col: BoardColumn, at: number) => {
    if (col === 'late' && from !== 'late') return
    const current = (columns.get(col) ?? []).map((t) => t.id)
    const ids = current.filter((id) => id !== todo.id)
    ids.splice(Math.max(0, Math.min(at, ids.length)), 0, todo.id)
    if (col === from && ids.every((id, i) => id === current[i])) return
    // The order is the whole column's, so the todos done today there keep their places after the open ones.
    ids.push(...(done.get(col) ?? []).map((t) => t.id))
    let after: number | undefined
    if (col !== from) {
      const dates = boardMoveDates(todo, col, today)
      if (dates) {
        update(todo, dates)
        after = todo.id
      }
    }
    reorderBoard(col, ids, after)
  }

  const drop = (to: string | number | null) => {
    if (!dragged || from === undefined || to === null || to === dragged.id) return
    const col = typeof to === 'number' ? colOf.get(to) : (to.slice(COL_PREFIX.length) as BoardColumn)
    if (!col || !accepts(col)) return
    const others = (columns.get(col) ?? []).map((t) => t.id).filter((id) => id !== dragged.id)
    place(dragged, from, col, typeof to === 'number' ? others.indexOf(to) + (below(to) ? 1 : 0) : others.length)
  }

  /**
   * moveByKey moves todo with Option and an arrow key: up or down past the
   * next card shown in its column, or to the end of the column to the left
   * or right. The todo's grip keeps focus.
   */
  const moveByKey = (todo: Todo, key: 'ArrowUp' | 'ArrowDown' | 'ArrowLeft' | 'ArrowRight') => {
    const col = colOf.get(todo.id)
    if (!col) return
    if (key === 'ArrowLeft' || key === 'ArrowRight') {
      const to = BOARD_COLUMNS[BOARD_COLUMNS.indexOf(col) + (key === 'ArrowLeft' ? -1 : 1)]
      if (!to) return
      place(todo, col, to, Infinity)
    } else {
      const visible = shown(columns.get(col) ?? [])
      const neighbour = visible[visible.findIndex((t) => t.id === todo.id) + (key === 'ArrowUp' ? -1 : 1)]
      if (!neighbour) return
      const others = (columns.get(col) ?? []).map((t) => t.id).filter((id) => id !== todo.id)
      place(todo, col, col, others.indexOf(neighbour.id) + (key === 'ArrowDown' ? 1 : 0))
    }
    // Moving the card in the DOM drops focus, so put it back on its grip.
    requestAnimationFrame(() => document.querySelector<HTMLElement>(`[data-grip="${todo.id}"]`)?.focus())
  }

  const selectedTask = selection?.kind === 'task' ? selection.id : null
  const card = (t: Todo, column: BoardColumn) => (
    <BoardCard
      key={t.id}
      todo={t}
      task={taskMap.get(t.task_id)}
      column={column}
      selected={selection?.kind === 'todo' && selection.id === t.id}
      taskSelected={selectedTask === t.task_id}
      onSelect={onSelect}
      onToggleDone={onToggleDone}
      onMoveKey={(key) => moveByKey(t, key)}
      dragging={active === t.id}
      line={active !== null && active !== t.id && over === t.id && accepts(column) ? (below(t.id) ? 'bottom' : 'top') : null}
    />
  )

  return (
    <section aria-label="Todo board" className="flex h-[calc(100vh-116px)] min-h-[520px] min-w-0 flex-col gap-3">
      <div className="flex items-start gap-3">
        <div role="group" aria-label="Filter by task" className="flex min-w-0 flex-1 flex-wrap items-center gap-1.5 pt-0.5">
          <button
            type="button"
            aria-pressed={!filter}
            onClick={() => setPicked(null)}
            className={cn(
              'flex h-7 items-center gap-1.5 rounded-full border px-2.5 text-[12px] font-medium',
              !filter ? 'border-accent bg-accent text-white' : 'border-border bg-white text-ink hover:bg-bg-subtle',
            )}
          >
            All tasks
            <span className={cn('text-[11px] tabular-nums', !filter ? 'text-white/80' : 'text-ink-muted')}>{openTodos.length}</span>
          </button>
          {tasks
            .filter((t) => counts.has(t.id))
            .map((t) => {
              const on = filter === t.id
              return (
                <button
                  key={t.id}
                  type="button"
                  aria-pressed={on}
                  title={`${taskKey(t.id)} · ${t.title}`}
                  onClick={() => setPicked(on ? null : t.id)}
                  className={cn(
                    'flex h-7 max-w-[150px] items-center gap-1.5 rounded-full border pr-[5px] pl-[9px] text-[12px]',
                    on ? 'border-accent-line-strong bg-accent-tint text-accent-ink' : 'border-border bg-white text-ink hover:bg-bg-subtle',
                  )}
                >
                  <OriginDot origin={t.originated} />
                  <span className="shrink-0 font-mono text-[11.5px]">{taskKey(t.id)}</span>
                  <span className="min-w-0 truncate">{t.title}</span>
                  <span
                    className={cn(
                      'flex h-[18px] min-w-[18px] shrink-0 items-center justify-center rounded-full px-[5px] text-[11px] font-medium tabular-nums',
                      on ? 'bg-accent text-white' : 'bg-bg-muted text-ink-muted',
                    )}
                  >
                    {counts.get(t.id)}
                  </span>
                </button>
              )
            })}
        </div>
        <NewTodoButton tasks={tasks} defaultTask={filter} />
      </div>
      <DndContext
        sensors={sensors}
        collisionDetection={cardFirst}
        onDragStart={(e) => setActive(Number(e.active.id))}
        onDragOver={(e) => setOver(e.over ? e.over.id : null)}
        onDragCancel={reset}
        onDragEnd={(e) => {
          drop(e.over ? e.over.id : null)
          reset()
        }}
      >
        <div className="grid min-h-0 flex-1 grid-cols-4 gap-2.5">
          {BOARD_COLUMNS.map((c) => {
            const open = shown(columns.get(c) ?? [])
            const finished = shown(done.get(c) ?? [])
            const hot = active !== null && overCol === c && accepts(c)
            return (
              <Column
                key={c}
                column={c}
                open={open.length}
                done={finished.length}
                accepts={accepts(c)}
                hot={hot && from !== c}
                lineEnd={hot && typeof over === 'string' && open.length > 0}
              >
                {{ cards: open.map((t) => card(t, c)), doneCards: finished.map((t) => card(t, c)) }}
              </Column>
            )
          })}
        </div>
        <DragOverlay dropAnimation={null}>
          {dragged && (
            <div className="flex max-w-[280px] cursor-grabbing items-center gap-2 rounded-[10px] border border-border bg-white px-3 py-2.5 text-[13.5px] shadow-lg">
              <span className="size-[18px] shrink-0 rounded-[5px] border-[1.5px] border-border-control" />
              <span className="min-w-0 truncate">{dragged.description}</span>
            </div>
          )}
        </DragOverlay>
      </DndContext>
    </section>
  )
}
