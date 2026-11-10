// Typed client for the Wharf server API under /api. Shapes mirror
// internal/server/model.go.

export type TaskStatus = 'todo' | 'in-progress' | 'done' | 'canceled'
export type Origin = 'jira' | 'local' | 'notion' | 'gitlab'

export interface TaskSummary {
  id: string
  originated: Origin
  title: string
  description: string
  status: TaskStatus
  start_date: string | null
  due_date: string | null
}

export interface TaskListItem extends TaskSummary {
  todo_count: number
  done_todo_count: number
  /** position orders Today; null until the tasks are first reordered. */
  position: number | null
}

export interface TaskDetail extends TaskSummary {
  parent_id: string | null
  children_ids: string[]
  linked_ids: string[]
  parent: TaskSummary | null
  children: TaskSummary[]
  linked: TaskSummary[]
}

/** TaskInput is the body of POST /api/tasks and PUT /api/tasks/{id}. */
export interface TaskInput {
  title: string
  description: string
  status: TaskStatus
  start_date: string | null
  due_date: string | null
  parent_id: string
  children_ids: string[]
  linked_ids: string[]
}

export interface ServiceDeskRequest {
  key: string
  title: string
  status: string
  status_category: string
  created_at: string
  status_at: string
}

export interface Todo {
  id: number
  task_id: string
  description: string
  note: string
  is_done: boolean
  completed_at: string | null
  start_date: string | null
  end_date: string | null
  reminders: string[]
  position: number | null
  service_desk_requests: ServiceDeskRequest[]
}

/** TodoInput is the body of POST /api/todos and PUT /api/todos/{id}. */
export interface TodoInput {
  task_id: string
  description: string
  note: string
  is_done: boolean
  start_date: string | null
  end_date: string | null
  reminders: string[]
  position?: number
}

export interface TodoFilter {
  task_id?: string
  is_done?: boolean
  completed_since?: string
  service_desk_request?: string
}

export interface AtlassianSync {
  synced_at: string
  issues: number
  issue_failures: number
  open_requests: number
  closed_requests: number
  request_failures: number
  errors?: string[]
}

export interface TaskSync {
  task: TaskSummary
  changes: string[]
}

export interface AtlassianSettings {
  site_url: string
  email: string
  api_token: string
}

export interface AppSettings {
  config_path: string
  pending_config_path: string | null
  addr: string
  db_target: string
  version: string
}

/** UpdateStatus is what the server knows about a newer Wharf release. */
export interface UpdateStatus {
  /** supported is false for builds that never check: not a release, not macOS arm64, or update_check is off. */
  supported: boolean
  current: string
  latest: string | null
  available: boolean
  checked_at: string | null
  release_url: string | null
  notes: string
}

/** ApiError is a response with an error status; code tells apart errors that share one. */
export class ApiError extends Error {
  readonly status: number
  readonly code?: string

  constructor(status: number, message: string, code?: string) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
  }
}

/** NetworkError means the Wharf server did not answer at all. */
export class NetworkError extends Error {
  constructor(cause: unknown) {
    super("Can't reach the Wharf server", { cause })
    this.name = 'NetworkError'
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  let res: Response
  try {
    res = await fetch(path, {
      method,
      headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
  } catch (err) {
    throw new NetworkError(err)
  }

  if (!res.ok) {
    // A proxy in front of a stopped server answers 502/504 without our JSON body.
    const data = (await res.json().catch(() => null)) as { error?: string; code?: string } | null
    if (!data && (res.status === 502 || res.status === 504)) throw new NetworkError(res.statusText)
    throw new ApiError(res.status, data?.error ?? res.statusText, data?.code)
  }

  if (res.status === 204 || res.status === 202) return undefined as T
  return (await res.json()) as T
}

function query(params: Record<string, string | boolean | undefined>): string {
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '') q.set(k, String(v))
  }
  const s = q.toString()
  return s ? `?${s}` : ''
}

const enc = encodeURIComponent

export const api = {
  healthz: () => request<{ status: string }>('GET', '/api/healthz'),

  listTasks: () => request<{ tasks: TaskListItem[] }>('GET', '/api/tasks').then((r) => r.tasks),
  getTask: (id: string) => request<TaskDetail>('GET', `/api/tasks/${enc(id)}`),
  createTask: (t: TaskInput) => request<{ id: string }>('POST', '/api/tasks', t).then((r) => r.id),
  updateTask: (id: string, t: TaskInput) => request<void>('PUT', `/api/tasks/${enc(id)}`, t),
  deleteTask: (id: string) => request<void>('DELETE', `/api/tasks/${enc(id)}`),
  setTaskStatus: (id: string, status: TaskStatus) =>
    request<void>('PUT', `/api/tasks/${enc(id)}/status`, { status }),
  reorderTasks: (ids: string[]) => request<void>('PUT', '/api/tasks/order', { ids }),
  syncTask: (id: string) => request<TaskSync>('POST', `/api/tasks/${enc(id)}/sync`),

  listTodos: (f: TodoFilter) =>
    request<{ todos: Todo[] }>('GET', `/api/todos${query({ ...f })}`).then((r) => r.todos),
  createTodo: (t: TodoInput) => request<{ id: number }>('POST', '/api/todos', t).then((r) => r.id),
  updateTodo: (id: number, t: TodoInput) => request<void>('PUT', `/api/todos/${id}`, t),
  deleteTodo: (id: number) => request<void>('DELETE', `/api/todos/${id}`),
  linkRequest: (todoID: number, key: string) =>
    request<void>('PUT', `/api/todos/${todoID}/service-desk-requests/${enc(key)}`),
  unlinkRequest: (todoID: number, key: string) =>
    request<void>('DELETE', `/api/todos/${todoID}/service-desk-requests/${enc(key)}`),

  listRequests: () =>
    request<{ requests: ServiceDeskRequest[] }>('GET', '/api/service-desk/requests').then((r) => r.requests),

  syncAtlassian: () => request<AtlassianSync>('POST', '/api/sync/atlassian'),
  lastSync: () =>
    request<{ last_sync: AtlassianSync | null }>('GET', '/api/sync/atlassian').then((r) => r.last_sync),

  atlassianSettings: () => request<AtlassianSettings>('GET', '/api/settings/atlassian'),
  saveAtlassianSettings: (s: AtlassianSettings) => request<void>('PUT', '/api/settings/atlassian', s),
  testAtlassianSettings: (s: AtlassianSettings) =>
    request<{ checked_at: string }>('POST', '/api/settings/atlassian/test', s),

  appSettings: () => request<AppSettings>('GET', '/api/settings/app'),
  chooseConfig: () =>
    request<{ pending_config_path: string | null }>('POST', '/api/settings/app/config/choose'),
  clearPendingConfig: () => request<void>('DELETE', '/api/settings/app/config/pending'),
  openConfig: () => request<void>('POST', '/api/settings/app/config/open'),
  restart: () => request<void>('POST', '/api/settings/app/restart'),

  updateStatus: () => request<UpdateStatus>('GET', '/api/update'),
  applyUpdate: () => request<void>('POST', '/api/update/apply'),
}

/** todoInput turns a stored todo into the full body PUT /api/todos/{id} expects, keeping its place. */
export function todoInput(t: Todo, patch: Partial<TodoInput> = {}): TodoInput {
  return {
    task_id: t.task_id,
    description: t.description,
    note: t.note,
    is_done: t.is_done,
    start_date: t.start_date,
    end_date: t.end_date,
    reminders: t.reminders,
    ...patch,
  }
}

/** taskInput turns a task detail into the full body PUT /api/tasks/{id} expects. */
export function taskInput(t: TaskDetail, patch: Partial<TaskInput> = {}): TaskInput {
  return {
    title: t.title,
    description: t.description,
    status: t.status,
    start_date: t.start_date,
    due_date: t.due_date,
    parent_id: t.parent_id ?? '',
    children_ids: t.children_ids,
    linked_ids: t.linked_ids,
    ...patch,
  }
}

/** taskKey is the key shown for a task ID: "local-WHF-003" → "WHF-003". */
export function taskKey(id: string): string {
  return id.replace(/^(jira|local|notion|gitlab)-/, '')
}

/** taskOrigin is the origin a task ID starts with, or undefined for an unknown prefix. */
export function taskOrigin(id: string): Origin | undefined {
  const m = /^(jira|local|notion|gitlab)-/.exec(id)
  return m ? (m[1] as Origin) : undefined
}
