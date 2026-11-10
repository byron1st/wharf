import { useQueryClient } from '@tanstack/react-query'
import { useMemo, useState } from 'react'
import { useLocation, useNavigate, useSearchParams } from 'react-router'
import { UpdateBanner } from '@/components/common/UpdateBanner'
import { keys, useRequests, useTaskActions, useTasks, useTodoActions, useTodos } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import type { TaskListItem, TaskStatus, Todo } from '@/lib/api'
import { compareTodayTasks, isTodayTask, isTodayTodo, reorderedIDs, startOfToday } from '@/lib/dates'
import { Header } from './Header'
import { RequestInspector } from './RequestInspector'
import { RequestsCard } from './RequestsCard'
import { TaskInspector } from './TaskInspector'
import { TodoInspector } from './TodoInspector'
import { TodosCard, type Selection, type TodoGroup } from './TodosCard'

/** selectionOf reads the Inspector selection from ?todo=, ?task= or ?req=. */
function selectionOf(params: URLSearchParams): Selection {
  const todo = Number(params.get('todo'))
  if (todo) return { kind: 'todo', id: todo }
  const task = params.get('task')
  if (task) return { kind: 'task', id: task }
  const req = params.get('req')
  if (req) return { kind: 'req', id: req }
  return null
}

/** toggled returns a copy of s with v added, or removed when on is false. */
function toggled<T>(s: Set<T>, v: T, on: boolean): Set<T> {
  const next = new Set(s)
  if (on) next.add(v)
  else next.delete(v)
  return next
}

export function TodayPage() {
  const { now, today } = useNow()
  const navigate = useNavigate()
  const location = useLocation()
  const qc = useQueryClient()
  const [params, setParams] = useSearchParams()
  const selection = selectionOf(params)
  const [adding, setAdding] = useState<string | null>(null)
  const { create, setStatus, reorder } = useTaskActions()
  const { update } = useTodoActions()
  // Todos checked and tasks whose status changed in this visit stay where
  // they are instead of leaving Today; the next visit tidies them away.
  const [keptTodos, setKeptTodos] = useState<Set<number>>(() => new Set())
  const [keptTasks, setKeptTasks] = useState<Set<string>>(() => new Set())

  const { data: tasks = [] } = useTasks()
  const { data: open = [] } = useTodos({ is_done: false })
  const since = startOfToday(now)
  const { data: doneToday = [] } = useTodos({ completed_since: since })
  const { data: requests = [] } = useRequests()

  // A todo toggled in one list is patched in place, so merge both lists and
  // pick each todo by its current state. Todos done today include the kept ones.
  const { todayTodos, all } = useMemo(() => {
    const byID = new Map<number, Todo>()
    for (const t of [...open, ...doneToday]) byID.set(t.id, t)
    const all = [...byID.values()]
    return { all, todayTodos: all.filter((t) => keptTodos.has(t.id) || isTodayTodo(t, today)) }
  }, [open, doneToday, today, keptTodos])

  const taskMap = useMemo(() => new Map(tasks.map((t) => [t.id, t])), [tasks])

  // Open tasks are listed even without todos for today, so todos can be added to them.
  const groups: TodoGroup[] = useMemo(() => {
    const byTask = new Map<string, Todo[]>()
    for (const t of todayTodos) byTask.set(t.task_id, [...(byTask.get(t.task_id) ?? []), t])
    return tasks
      .filter((t) => isTodayTask(t, byTask.has(t.id)) || adding === t.id || keptTasks.has(t.id))
      .sort(compareTodayTasks)
      .map((task) => ({
        task,
        todos: (byTask.get(task.id) ?? []).sort((a, b) => (a.position ?? 0) - (b.position ?? 0)),
      }))
  }, [tasks, todayTodos, adding, keptTasks])

  const toggleDone = (todo: Todo, done: boolean) => {
    update(todo, { is_done: done })
    setKeptTodos((s) => toggled(s, todo.id, done))
  }

  const changeStatus = (task: TaskListItem, status: TaskStatus) => {
    setStatus(task, status)
    setKeptTasks((s) => toggled(s, task.id, true))
  }

  const move = (id: string, before: string | null) => {
    const ids = reorderedIDs(tasks, id, before)
    if (ids) reorder(ids)
  }

  const select = (s: Selection) => {
    const next = new URLSearchParams()
    if (s) next.set(s.kind, String(s.id))
    setParams(next)
  }

  const openTask = (id: string) => navigate(`/task/${encodeURIComponent(id)}`, { state: { back: location.search } })

  const createTask = async (title: string) => {
    const id = await create(title)
    setAdding(id)
    select({ kind: 'task', id })
  }

  let selectedTodo: Todo | undefined
  if (selection?.kind === 'todo') {
    selectedTodo = all.find((t) => t.id === selection.id)
    // A todo picked from a request may be in neither Today list.
    for (const [, list] of qc.getQueriesData<Todo[]>({ queryKey: keys.allTodos })) {
      selectedTodo ??= list?.find((t) => t.id === selection.id)
    }
  }
  const selectedReq = selection?.kind === 'req' ? requests.find((r) => r.key === selection.id) : undefined
  const close = () => select(null)

  return (
    <main className="box-border flex min-h-screen flex-col gap-4 px-8 pt-7 pb-10">
      <Header onCreateTask={createTask} onSettings={() => navigate('/settings/general', { state: { back: location.search } })} />
      <UpdateBanner />
      <div className="flex flex-wrap items-start gap-4">
        <div className="flex min-w-0 flex-[999_1_520px] flex-col gap-4">
          <TodosCard
            groups={groups}
            selection={selection}
            onSelect={select}
            adding={adding}
            setAdding={setAdding}
            onToggleDone={toggleDone}
            onSetStatus={changeStatus}
            onMove={move}
          />
        </div>
        <div className="flex min-w-0 flex-[1_1_340px] flex-col gap-3">
          <RequestsCard requests={requests} selection={selection} onSelect={select} />
          {selection && (
            <aside aria-label="Inspector" className="flex min-w-0 flex-col rounded-xl border border-border bg-white shadow-xs">
              {selection.kind === 'todo' && selectedTodo && (
                <TodoInspector
                  key={selectedTodo.id}
                  todo={selectedTodo}
                  task={taskMap.get(selectedTodo.task_id)}
                  requests={requests}
                  onClose={close}
                  onSelectTask={(id) => select({ kind: 'task', id })}
                  onSelectRequest={(id) => select({ kind: 'req', id })}
                  onToggleDone={toggleDone}
                />
              )}
              {selection.kind === 'task' && (
                <TaskInspector
                  key={selection.id}
                  id={selection.id}
                  onClose={close}
                  onOpen={openTask}
                  onSelectTask={(id) => select({ kind: 'task', id })}
                />
              )}
              {selection.kind === 'req' && selectedReq && (
                <RequestInspector
                  key={selectedReq.key}
                  request={selectedReq}
                  tasks={taskMap}
                  openTodos={open.filter((t) => !t.is_done)}
                  onClose={close}
                  onSelectTodo={(id) => select({ kind: 'todo', id })}
                />
              )}
              {((selection.kind === 'todo' && !selectedTodo) || (selection.kind === 'req' && !selectedReq)) && (
                <div className="flex items-center justify-between px-4 py-3 text-[13px] text-ink-muted">
                  Not found. It may have been removed.
                  <button type="button" onClick={close} className="text-[12px] font-medium text-ink-secondary hover:underline">
                    Close
                  </button>
                </div>
              )}
            </aside>
          )}
        </div>
      </div>
    </main>
  )
}
