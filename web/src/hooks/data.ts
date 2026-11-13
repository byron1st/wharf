import { useIsMutating, useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import {
  api,
  todoInput,
  taskInput,
  type ServiceDeskRequest,
  type TaskDetail,
  type TaskInput,
  type TaskListItem,
  type TaskStatus,
  type Todo,
  type TodoFilter,
  type TodoInput,
} from '@/lib/api'
import { isClosedStatus } from '@/lib/dates'
import { reportSaveError } from '@/lib/errors'

export const keys = {
  tasks: ['tasks'] as const,
  task: (id: string) => ['task', id] as const,
  todos: (f: TodoFilter) => ['todos', f] as const,
  allTodos: ['todos'] as const,
  requests: ['requests'] as const,
  lastSync: ['lastSync'] as const,
  atlassian: ['settings', 'atlassian'] as const,
  app: ['settings', 'app'] as const,
  update: ['update'] as const,
}

export const useTasks = () => useQuery({ queryKey: keys.tasks, queryFn: api.listTasks })

export const useTask = (id: string | null | undefined) =>
  useQuery({ queryKey: keys.task(id ?? ''), queryFn: () => api.getTask(id!), enabled: !!id })

export const useTodos = (f: TodoFilter, enabled = true) =>
  useQuery({ queryKey: keys.todos(f), queryFn: () => api.listTodos(f), enabled })

export const useRequests = () => useQuery({ queryKey: keys.requests, queryFn: api.listRequests })

export const useLastSync = () => useQuery({ queryKey: keys.lastSync, queryFn: api.lastSync })

export const useAtlassianSettings = () => useQuery({ queryKey: keys.atlassian, queryFn: api.atlassianSettings })

export const useAppSettings = () => useQuery({ queryKey: keys.app, queryFn: api.appSettings })

/** useUpdateStatus re-reads the server's update check hourly; the server itself checks far less often. */
export const useUpdateStatus = () =>
  useQuery({ queryKey: keys.update, queryFn: api.updateStatus, refetchInterval: 60 * 60 * 1000 })

/**
 * queued runs saves of one item one after another, so a full-object PUT can
 * never land before an earlier one and undo it.
 */
const queues = new Map<string, Promise<unknown>>()
function queued<T>(key: string, fn: () => Promise<T>): Promise<T> {
  const run = (queues.get(key) ?? Promise.resolve()).catch(() => {}).then(fn)
  queues.set(key, run)
  void run.finally(() => queues.get(key) === run && queues.delete(key)).catch(() => {})
  return run
}

/** cachedTodo is the todo id as cached now, with every optimistic patch applied. */
function cachedTodo(qc: QueryClient, id: number): Todo | undefined {
  for (const [, list] of qc.getQueriesData<Todo[]>({ queryKey: keys.allTodos })) {
    const t = list?.find((x) => x.id === id)
    if (t) return t
  }
}

type Snapshot = [readonly unknown[], unknown][]

function snapshot(qc: QueryClient): Snapshot {
  return [
    ...qc.getQueriesData({ queryKey: keys.allTodos }),
    ...qc.getQueriesData({ queryKey: keys.tasks }),
    ...qc.getQueriesData({ queryKey: ['task'] }),
  ]
}

function restore(qc: QueryClient, s: Snapshot) {
  for (const [key, data] of s) qc.setQueryData(key, data)
}

/** patchTodo applies fn to the todo id in every cached todo list; null removes it. */
function patchTodo(qc: QueryClient, id: number, fn: (t: Todo) => Todo | null) {
  qc.setQueriesData<Todo[]>({ queryKey: keys.allTodos }, (old) =>
    old?.flatMap((t) => {
      if (t.id !== id) return [t]
      const next = fn(t)
      return next ? [next] : []
    }),
  )
}

/** nextClosedAt is the closing time the server keeps for t after a status change, as the server decides it. */
function nextClosedAt(t: TaskListItem, status: TaskStatus, closedAt?: string): string | null {
  if (!isClosedStatus(status)) return null
  if (isClosedStatus(t.status)) return t.closed_at
  return closedAt ?? new Date().toISOString()
}

/** patchCounts moves a task's todo counts, so group progress follows a change before the refetch. */
function patchCounts(qc: QueryClient, taskID: string, todos: number, done: number) {
  qc.setQueryData<TaskListItem[]>(keys.tasks, (old) =>
    old?.map((t) =>
      t.id === taskID
        ? { ...t, todo_count: t.todo_count + todos, done_todo_count: t.done_todo_count + done }
        : t,
    ),
  )
}

/**
 * patchPositions moves todo id to position to among its task's cached todos,
 * shifting the ones between as the server does, so lists sorted by position
 * show the new order before the refetch.
 */
function patchPositions(qc: QueryClient, id: number, taskID: string, to: number) {
  const from = cachedTodo(qc, id)?.position
  if (from == null || from === to) return
  qc.setQueriesData<Todo[]>({ queryKey: keys.allTodos }, (old) =>
    old?.map((t) => {
      if (t.task_id !== taskID || t.position == null) return t
      if (t.id === id) return { ...t, position: to }
      if (from < to && t.position > from && t.position <= to) return { ...t, position: t.position - 1 }
      if (to < from && t.position >= to && t.position < from) return { ...t, position: t.position + 1 }
      return t
    }),
  )
}

function settle(qc: QueryClient) {
  void qc.invalidateQueries({ queryKey: keys.allTodos })
  void qc.invalidateQueries({ queryKey: keys.tasks })
}

interface TodoUpdate {
  todo: Todo
  patch: Partial<TodoInput>
}

/** useSavingTodo reports whether a save of the todo id is in flight. */
export const useSavingTodo = (id: number | undefined) =>
  useIsMutating({
    mutationKey: ['todo-save'],
    predicate: (m) => (m.state.variables as TodoUpdate | undefined)?.todo.id === id,
  }) > 0

/** useTodoActions writes todos optimistically; a failed write is undone and reported with Retry. */
export function useTodoActions() {
  const qc = useQueryClient()

  const update = useMutation({
    mutationKey: ['todo-save'],
    // The body is built when the save is sent, from the cache that already
    // holds this patch and every earlier one, so concurrent edits of other
    // fields are kept.
    // Moves within a task also go one at a time, since each position assumes
    // the earlier moves have landed.
    mutationFn: ({ todo, patch }: TodoUpdate) =>
      queued(`todo-${todo.id}`, () => {
        const save = () => api.updateTodo(todo.id, todoInput(cachedTodo(qc, todo.id) ?? todo, patch))
        return patch.position === undefined ? save() : queued(`todo-order-${todo.task_id}`, save)
      }),
    onMutate: async ({ todo, patch }) => {
      await qc.cancelQueries({ queryKey: keys.allTodos })
      const snap = snapshot(qc)
      // Shift siblings first, while the cache still holds the todo's old position.
      if (patch.position !== undefined) patchPositions(qc, todo.id, todo.task_id, patch.position)
      patchTodo(qc, todo.id, (t) => {
        const next = { ...t, ...patch } as Todo
        if (patch.is_done !== undefined && patch.is_done !== t.is_done) {
          next.completed_at = patch.is_done ? new Date().toISOString() : null
        }
        return next
      })
      if (patch.is_done !== undefined && patch.is_done !== todo.is_done) {
        patchCounts(qc, todo.task_id, 0, patch.is_done ? 1 : -1)
      }
      return snap
    },
    onError: (err, vars, snap) => {
      if (snap) restore(qc, snap)
      reportSaveError(err, 'todo', () => update.mutate(vars))
    },
    onSettled: () => settle(qc),
  })

  const create = useMutation({
    mutationFn: (input: TodoInput) => api.createTodo(input),
    onMutate: (input) => {
      patchCounts(qc, input.task_id, 1, 0)
    },
    onError: (err, input) => {
      patchCounts(qc, input.task_id, -1, 0)
      reportSaveError(err, 'todo', () => create.mutate(input))
    },
    onSettled: () => settle(qc),
  })

  const remove = useMutation({
    mutationFn: (todo: Todo) => api.deleteTodo(todo.id),
    onMutate: async (todo) => {
      await qc.cancelQueries({ queryKey: keys.allTodos })
      const snap = snapshot(qc)
      patchTodo(qc, todo.id, () => null)
      patchCounts(qc, todo.task_id, -1, todo.is_done ? -1 : 0)
      return snap
    },
    onError: (err, todo, snap) => {
      if (snap) restore(qc, snap)
      reportSaveError(err, 'todo', () => remove.mutate(todo))
    },
    onSettled: () => settle(qc),
  })

  const link = useMutation({
    mutationFn: ({ todo, request }: { todo: Todo; request: ServiceDeskRequest }) =>
      api.linkRequest(todo.id, request.key),
    onMutate: async ({ todo, request }) => {
      await qc.cancelQueries({ queryKey: keys.allTodos })
      const snap = snapshot(qc)
      patchTodo(qc, todo.id, (t) => ({
        ...t,
        service_desk_requests: [...t.service_desk_requests.filter((r) => r.key !== request.key), request].sort(
          (a, b) => a.key.localeCompare(b.key),
        ),
      }))
      return snap
    },
    onError: (err, vars, snap) => {
      if (snap) restore(qc, snap)
      reportSaveError(err, 'todo', () => link.mutate(vars))
    },
    onSettled: () => settle(qc),
  })

  const unlink = useMutation({
    mutationFn: ({ todo, key }: { todo: Todo; key: string }) => api.unlinkRequest(todo.id, key),
    onMutate: async ({ todo, key }) => {
      await qc.cancelQueries({ queryKey: keys.allTodos })
      const snap = snapshot(qc)
      patchTodo(qc, todo.id, (t) => ({
        ...t,
        service_desk_requests: t.service_desk_requests.filter((r) => r.key !== key),
      }))
      return snap
    },
    onError: (err, vars, snap) => {
      if (snap) restore(qc, snap)
      reportSaveError(err, 'todo', () => unlink.mutate(vars))
    },
    onSettled: () => settle(qc),
  })

  return {
    update: (todo: Todo, patch: Partial<TodoInput>) => update.mutate({ todo, patch }),
    create: (input: TodoInput) => create.mutateAsync(input),
    remove: (todo: Todo) => remove.mutate(todo),
    link: (todo: Todo, request: ServiceDeskRequest) => link.mutate({ todo, request }),
    unlink: (todo: Todo, key: string) => unlink.mutate({ todo, key }),
  }
}

/** useSavingTask reports whether a save of the task id is in flight. */
export const useSavingTask = (id: string | undefined) =>
  useIsMutating({
    mutationKey: ['task-save'],
    predicate: (m) => (m.state.variables as { task: TaskDetail } | undefined)?.task.id === id,
  }) > 0

/** useTaskActions writes local tasks; updates are optimistic like todo updates. */
export function useTaskActions() {
  const qc = useQueryClient()

  const update = useMutation({
    mutationKey: ['task-save'],
    mutationFn: ({ task, patch }: { task: TaskDetail; patch: Partial<TaskInput> }) =>
      queued(`task-${task.id}`, () =>
        api.updateTask(task.id, taskInput(qc.getQueryData<TaskDetail>(keys.task(task.id)) ?? task, patch)),
      ),
    onMutate: async ({ task, patch }) => {
      await qc.cancelQueries({ queryKey: keys.task(task.id) })
      const snap = snapshot(qc)
      const { parent_id: parentID, ...fields } = patch
      qc.setQueryData<TaskDetail>(keys.task(task.id), (old) =>
        old ? { ...old, ...fields, ...(parentID !== undefined ? { parent_id: parentID || null } : {}) } : old,
      )
      qc.setQueryData<TaskListItem[]>(keys.tasks, (old) =>
        old?.map((t) => (t.id === task.id ? { ...t, ...fields } : t)),
      )
      return snap
    },
    onError: (err, vars, snap) => {
      if (snap) restore(qc, snap)
      reportSaveError(err, 'task', () => update.mutate(vars))
    },
    onSettled: (_d, _e, { task }) => {
      void qc.invalidateQueries({ queryKey: keys.task(task.id) })
      void qc.invalidateQueries({ queryKey: keys.tasks })
    },
  })

  const create = useMutation({
    mutationFn: (title: string) =>
      api.createTask({
        title,
        description: '',
        status: 'todo',
        start_date: null,
        due_date: null,
        parent_id: '',
        children_ids: [],
        linked_ids: [],
      }),
    onSettled: () => qc.invalidateQueries({ queryKey: keys.tasks }),
  })

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteTask(id),
    onError: (err, id) => reportSaveError(err, 'task', () => remove.mutate(id)),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: keys.tasks })
      void qc.invalidateQueries({ queryKey: keys.allTodos })
      void qc.invalidateQueries({ queryKey: ['task'] })
    },
  })

  const setStatus = useMutation({
    mutationKey: ['task-save'],
    mutationFn: ({ task, status, closedAt }: { task: { id: string }; status: TaskStatus; closedAt?: string }) =>
      queued(`task-${task.id}`, () => api.setTaskStatus(task.id, status, closedAt)),
    onMutate: async ({ task, status, closedAt }) => {
      await qc.cancelQueries({ queryKey: keys.tasks })
      await qc.cancelQueries({ queryKey: keys.task(task.id) })
      const snap = snapshot(qc)
      qc.setQueryData<TaskDetail>(keys.task(task.id), (old) => (old ? { ...old, status } : old))
      qc.setQueryData<TaskListItem[]>(keys.tasks, (old) =>
        old?.map((t) => (t.id === task.id ? { ...t, status, closed_at: nextClosedAt(t, status, closedAt) } : t)),
      )
      return snap
    },
    onError: (err, vars, snap) => {
      if (snap) restore(qc, snap)
      reportSaveError(err, 'task', () => setStatus.mutate(vars))
    },
    onSettled: (_d, _e, { task }) => {
      void qc.invalidateQueries({ queryKey: keys.task(task.id) })
      void qc.invalidateQueries({ queryKey: keys.tasks })
    },
  })

  const reorder = useMutation({
    mutationFn: (ids: string[]) => queued('task-order', () => api.reorderTasks(ids)),
    onMutate: async (ids) => {
      await qc.cancelQueries({ queryKey: keys.tasks })
      const snap = snapshot(qc)
      const at = new Map(ids.map((id, i) => [id, i]))
      qc.setQueryData<TaskListItem[]>(keys.tasks, (old) => old?.map((t) => ({ ...t, position: at.get(t.id) ?? t.position })))
      return snap
    },
    onError: (err, ids, snap) => {
      if (snap) restore(qc, snap)
      reportSaveError(err, 'task', () => reorder.mutate(ids))
    },
    onSettled: () => qc.invalidateQueries({ queryKey: keys.tasks }),
  })

  return {
    update: (task: TaskDetail, patch: Partial<TaskInput>) => update.mutate({ task, patch }),
    /** setStatus changes only the status of a local task. */
    /** setStatus sets a local task's status; closedAt brings back an earlier closing time, for undoing a reopen. */
    setStatus: (task: { id: string }, status: TaskStatus, closedAt?: string) =>
      setStatus.mutate({ task, status, closedAt }),
    /** setStatusAsync is setStatus resolving once the server stored it; a failure is already reported. */
    setStatusAsync: (task: { id: string }, status: TaskStatus, closedAt?: string) =>
      setStatus.mutateAsync({ task, status, closedAt }),
    /** reorder stores ids, every task in Today's order, as the task positions. */
    reorder: (ids: string[]) => reorder.mutate(ids),
    updateAsync: (task: TaskDetail, patch: Partial<TaskInput>) => update.mutateAsync({ task, patch }),
    create: (title: string) => create.mutateAsync(title),
    remove: (id: string) => remove.mutateAsync(id),
  }
}
