import { DndContext, KeyboardSensor, PointerSensor, closestCenter, useSensor, useSensors, type DragEndEvent } from '@dnd-kit/core'
import {
  SortableContext,
  arrayMove,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Bell, CalendarDays, Check, FileText, GripVertical, MoreHorizontal, Pencil, Plus, Ticket } from 'lucide-react'
import { useState } from 'react'
import { RequestLinks } from '@/components/today/TodoInspector'
import { DateField } from '@/components/pickers/DateField'
import { Reminders } from '@/components/pickers/Reminders'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Switch } from '@/components/ui/switch'
import { useRequests, useTodoActions, useTodos } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import type { Todo } from '@/lib/api'
import { hhmm, isSent, longDate, monthDay, shortDay, sortReminders, ymd } from '@/lib/dates'
import { cn } from '@/lib/utils'

const chip = 'inline-flex h-[22px] items-center gap-1 rounded-md px-1.5 text-[12px] tabular-nums'

/** Meta is a todo row's chips: its dates, reminders, note and linked requests. */
function Meta({ todo }: { todo: Todo }) {
  const { now, today } = useNow()
  const { start_date: s, end_date: e } = todo
  const over = !todo.is_done && !!e && e < today
  const dueToday = !todo.is_done && e === today
  const period = s && e ? (s === e ? monthDay(e) : `${monthDay(s)} – ${monthDay(e)}`) : e ? monthDay(e) : s ? `From ${monthDay(s)}` : null
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {dueToday ? (
        <span className={cn(chip, 'bg-today-tint text-[#9a3412]')}>
          <CalendarDays size={12} />
          Due today
        </span>
      ) : (
        period && (
          <span className={cn(chip, over ? 'bg-danger-tint text-danger-ink' : 'bg-bg-muted text-ink-secondary')}>
            <CalendarDays size={12} />
            {period}
          </span>
        )
      )}
      {sortReminders(todo.reminders).map((r) => {
        const sent = isSent(r, now)
        const d = new Date(r)
        const day = shortDay(ymd(d), today)
        return (
          <span
            key={r}
            title={sent ? 'Sent' : 'Reminder'}
            className={cn(chip, sent ? 'border border-border text-ink-muted' : 'bg-[#faf8ff] text-[#6d28d9]')}
          >
            {sent ? <Check size={12} /> : <Bell size={12} />}
            {day === 'Today' ? 'Today' : monthDay(ymd(d))} · {hhmm(d)}
          </span>
        )
      })}
      {todo.note.trim() && (
        <span className={cn(chip, 'bg-bg-muted text-ink-secondary')}>
          <FileText size={12} />
          Note
        </span>
      )}
      {todo.service_desk_requests.length > 0 && (
        <span
          title={todo.service_desk_requests.map((r) => `${r.key} ${r.title}`).join('\n')}
          className={cn(chip, 'bg-tag-indigo-bg text-tag-indigo-ink')}
        >
          <Ticket size={12} />
          {todo.service_desk_requests.length}
        </span>
      )}
    </div>
  )
}

/** Editor is a todo's inline edit form; dates, reminders and note are saved together, links right away. */
function Editor({ todo, onClose }: { todo: Todo; onClose: () => void }) {
  const { update, remove } = useTodoActions()
  const { data: requests = [] } = useRequests()
  const [draft, setDraft] = useState({
    description: todo.description,
    start_date: todo.start_date,
    end_date: todo.end_date,
    reminders: todo.reminders,
    note: todo.note,
  })
  const set = (patch: Partial<typeof draft>) => setDraft((d) => ({ ...d, ...patch }))
  const save = () => {
    update(todo, { ...draft, description: draft.description.trim() || todo.description })
    onClose()
  }

  return (
    <div
      className="mt-2.5 flex flex-col gap-3.5 rounded-xl border border-border bg-white p-3.5 shadow-xs"
      onKeyDown={(e) => {
        if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) save()
      }}
    >
      <label className="flex flex-col gap-1.5 text-[12px] font-medium text-ink-secondary">
        Description
        <input
          value={draft.description}
          onChange={(e) => set({ description: e.target.value })}
          className="h-8 rounded-md border border-border px-2.5 text-[13px] font-normal text-ink outline-none focus:border-accent focus:shadow-ring"
        />
      </label>
      <div className="grid grid-cols-2 gap-3">
        <div className="flex min-w-0 flex-col gap-1.5">
          <span className="text-[12px] font-medium text-ink-secondary">Start date</span>
          <DateField
            title="Start date"
            value={draft.start_date}
            other={draft.end_date}
            pairText={draft.end_date ? `Ends ${monthDay(draft.end_date)}` : undefined}
            hi={draft.end_date}
            format={longDate}
            onChange={(v) => set({ start_date: v })}
            className="h-8 text-[13px]"
          />
        </div>
        <div className="flex min-w-0 flex-col gap-1.5">
          <span className="text-[12px] font-medium text-ink-secondary">End date</span>
          <DateField
            title="End date"
            value={draft.end_date}
            other={draft.start_date}
            pairText={draft.start_date ? `Starts ${monthDay(draft.start_date)}` : undefined}
            lo={draft.start_date}
            format={longDate}
            align="end"
            onChange={(v) => set({ end_date: v })}
            className="h-8 text-[13px]"
          />
        </div>
      </div>
      <div className="flex flex-col gap-1.5">
        <span className="text-[12px] font-medium text-ink-secondary">Reminders</span>
        <Reminders layout="inline" reminders={draft.reminders} end={draft.end_date} onChange={(reminders) => set({ reminders })} />
      </div>
      <label className="flex flex-col gap-1.5 text-[12px] font-medium text-ink-secondary">
        <span className="flex items-center gap-1.5">
          <FileText size={13} />
          Note
          <span className="ml-auto font-normal text-ink-faint">Plain text · only in Wharf</span>
        </span>
        <textarea
          rows={4}
          value={draft.note}
          placeholder="Context, links, commands, decisions…"
          onChange={(e) => set({ note: e.target.value })}
          className="resize-y rounded-md border border-border bg-bg-subtle px-2.5 py-2 text-[13px] leading-[1.6] font-normal text-ink outline-none focus:border-accent focus:shadow-ring"
        />
      </label>
      <div className="flex flex-col gap-1.5">
        <span className="flex items-center gap-1.5 text-[12px] font-medium text-ink-secondary">
          <Ticket size={13} className="text-tag-indigo-solid" />
          Service Desk requests
        </span>
        <RequestLinks todo={todo} requests={requests} />
      </div>
      <div className="flex items-center gap-2 border-t border-border-subtle pt-3">
        <button
          type="button"
          onClick={() => remove(todo)}
          className="h-8 rounded-md px-2.5 text-[13px] font-medium text-danger hover:bg-danger-tint"
        >
          Delete
        </button>
        <Button variant="outline" size="sm" className="ml-auto" onClick={onClose}>
          Cancel
        </Button>
        <Button size="sm" onClick={save}>
          Save
        </Button>
      </div>
    </div>
  )
}

function Row({ todo, editing, onEdit }: { todo: Todo; editing: boolean; onEdit: (v: boolean) => void }) {
  const { update } = useTodoActions()
  const { attributes, listeners, setNodeRef, setActivatorNodeRef, transform, transition, isDragging } = useSortable({
    id: todo.id,
  })
  return (
    <li
      ref={setNodeRef}
      style={{ transform: CSS.Translate.toString(transform), transition }}
      className={cn(
        'list-none border-b border-border-subtle px-4 py-3 last:border-b-0',
        editing ? 'bg-bg-subtle' : 'bg-white',
        isDragging && 'relative z-10 shadow-lg',
      )}
    >
      <div className="flex items-start gap-2.5">
        <button
          type="button"
          ref={setActivatorNodeRef}
          aria-label="Drag to reorder"
          className="mt-px flex h-[22px] w-5 shrink-0 cursor-grab items-center justify-center text-ink-faint active:cursor-grabbing"
          {...attributes}
          {...listeners}
        >
          <GripVertical size={14} />
        </button>
        <Checkbox
          checked={todo.is_done}
          label={todo.description}
          onChange={(v) => update(todo, { is_done: v })}
          className="mt-0.5"
        />
        <div className="flex min-w-0 flex-1 flex-col gap-1.5">
          <span className={cn('text-[14px] leading-[22px]', todo.is_done && 'text-ink-muted line-through')}>
            {todo.description}
          </span>
          <Meta todo={todo} />
        </div>
        <button
          type="button"
          aria-label={editing ? 'Close todo editor' : `Edit ${todo.description}`}
          aria-expanded={editing}
          onClick={() => onEdit(!editing)}
          className={cn(
            'flex size-[30px] shrink-0 items-center justify-center rounded-md text-ink-secondary hover:bg-bg-muted',
            editing && 'bg-bg-pressed',
          )}
        >
          <MoreHorizontal size={16} />
        </button>
      </div>
      {editing && (
        <div className="pl-[58px]">
          <Editor todo={todo} onClose={() => onEdit(false)} />
        </div>
      )}
    </li>
  )
}

/** TaskTodos is the Todos card of the task modal; todos stay editable whatever the task's mode or origin. */
export function TaskTodos({ taskID, jira }: { taskID: string; jira: boolean }) {
  const { data: todos = [] } = useTodos({ task_id: taskID })
  const { update, create } = useTodoActions()
  const [hideDone, setHideDone] = useState(false)
  const [editing, setEditing] = useState<number | null>(null)
  // A drop shows at once: the dropped order holds until the list refetches.
  const [dropped, setDropped] = useState<{ src: Todo[]; ids: number[] } | null>(null)
  const [text, setText] = useState('')
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const order = dropped?.src === todos ? dropped.ids : todos.map((t) => t.id)
  const byID = new Map(todos.map((t) => [t.id, t]))
  const ordered = order.map((id) => byID.get(id)).filter((t): t is Todo => !!t)
  const shown = hideDone ? ordered.filter((t) => !t.is_done) : ordered
  const done = todos.filter((t) => t.is_done).length
  const pct = todos.length ? Math.round((done / todos.length) * 100) : 0

  const onDragEnd = ({ active, over }: DragEndEvent) => {
    if (!over || active.id === over.id) return
    const from = order.indexOf(Number(active.id))
    const to = order.indexOf(Number(over.id))
    setDropped({ src: todos, ids: arrayMove(order, from, to) })
    const todo = byID.get(Number(active.id))
    if (todo) update(todo, { position: to })
  }

  const add = () => {
    const d = text.trim()
    if (!d) return
    setText('')
    void create({ task_id: taskID, description: d, note: '', is_done: false, start_date: null, end_date: null, reminders: [] }).catch(
      () => setText(d),
    )
  }

  return (
    <section aria-label="Todos" className="overflow-hidden rounded-xl border border-border bg-white shadow-xs">
      <div className="flex items-center gap-3 border-b border-border-subtle px-4 py-3">
        <h2 className="m-0 text-[15px] font-semibold">Todos</h2>
        <span className="text-[13px] text-ink-muted tabular-nums">
          {done} of {todos.length}
        </span>
        <span className="flex h-[5px] w-24 overflow-hidden rounded-full bg-bg-pressed">
          <span className="bg-accent" style={{ width: `${pct}%` }} />
        </span>
        {jira ? (
          <span className="ml-auto inline-flex h-6 items-center gap-1.5 rounded-full bg-accent-tint px-2.5 text-[12px] font-medium text-accent">
            <Pencil size={12} />
            Editable — stays in Wharf
          </span>
        ) : (
          <label className="ml-auto flex items-center gap-2 text-[13px] text-ink-secondary">
            <Switch checked={hideDone} onCheckedChange={setHideDone} />
            Hide completed
          </label>
        )}
      </div>
      <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
        <SortableContext items={shown.map((t) => t.id)} strategy={verticalListSortingStrategy}>
          <ul className="m-0 p-0">
            {shown.map((t) => (
              <Row key={t.id} todo={t} editing={editing === t.id} onEdit={(v) => setEditing(v ? t.id : null)} />
            ))}
          </ul>
        </SortableContext>
      </DndContext>
      <div className="flex items-center gap-2.5 border-t border-border-subtle px-4 py-2">
        <Plus size={16} className="ml-[30px] text-ink-faint" />
        <input
          aria-label="Add a todo"
          placeholder="Add a todo — press Enter"
          value={text}
          onChange={(e) => setText(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && !e.nativeEvent.isComposing && add()}
          className="h-8 min-w-0 flex-1 bg-transparent text-[14px] outline-none placeholder:text-ink-faint"
        />
      </div>
    </section>
  )
}
