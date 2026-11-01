import { CornerDownRight, CornerLeftUp, Link2, Pencil, Plus, X } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { OriginDot, OriginIcon, StatusIcon } from '@/components/common/badges'
import { SearchPicker } from '@/components/pickers/SearchPicker'
import { useTasks } from '@/hooks/data'
import { taskKey, taskOrigin, type TaskDetail, type TaskSummary } from '@/lib/api'
import { cn } from '@/lib/utils'

type Kind = 'parent' | 'children' | 'linked'

/** RelationChip is solid for a task in Wharf, which opens it, and dashed for one only known by ID. */
export function RelationChip({
  id,
  task,
  rich,
  onOpen,
  onRemove,
}: {
  id: string
  task?: TaskSummary
  rich?: boolean
  onOpen?: (id: string) => void
  onRemove?: () => void
}) {
  const remove = onRemove && (
    <button
      type="button"
      aria-label={`Remove ${id}`}
      onClick={onRemove}
      className="flex size-6 shrink-0 items-center justify-center rounded-md text-[#71717a] opacity-0 group-focus-within:opacity-100 group-hover:opacity-100 hover:bg-black/5"
    >
      <X size={12} strokeWidth={2.2} />
    </button>
  )
  if (!task) {
    return (
      <div className="group flex h-9 min-w-0 items-center gap-2 rounded-md border border-dashed border-[#d4d4d8] bg-[#fafafa] pr-1.5 pl-2.5">
        {rich && <span className="size-3 shrink-0 rounded-full border border-dashed border-[#d4d4d8]" />}
        <span className="min-w-0 truncate font-mono text-[12px] text-[#3f3f46]">{id}</span>
        <span className="ml-auto shrink-0 text-[11px] text-[#71717a]">Not in Wharf</span>
        {remove}
      </div>
    )
  }
  return (
    <div className="group flex h-9 min-w-0 items-center gap-1 rounded-md border border-[#e4e4e7] bg-white pr-1 shadow-card hover:bg-[#fafafa]">
      <button
        type="button"
        onClick={() => onOpen?.(task.id)}
        title={`Open ${taskKey(task.id)}`}
        className="flex h-full min-w-0 flex-1 items-center gap-2 pl-2.5 text-left text-[13px]"
      >
        {rich ? (
          <>
            <StatusIcon status={task.status} />
            <OriginIcon origin={task.originated} />
          </>
        ) : (
          <OriginDot origin={task.originated} className="size-2" />
        )}
        <span className="shrink-0 font-mono text-[11px] text-[#52525b]">{taskKey(task.id)}</span>
        <span className="min-w-0 truncate">{task.title}</span>
      </button>
      {remove}
    </div>
  )
}

/** TaskPicker searches the stored tasks, or takes a typed ID such as jira-SEC-110 for a task not in Wharf. */
function TaskPicker({ exclude, onPick }: { exclude: Set<string>; onPick: (id: string) => void }) {
  const { data: tasks = [] } = useTasks()
  const candidates = tasks.filter((t) => !exclude.has(t.id))
  return (
    <SearchPicker
      placeholder="Search tasks or type an ID like jira-SEC-110"
      empty="No matching tasks."
      items={candidates.map((t) => ({
        id: t.id,
        value: `${taskKey(t.id)} ${t.title}`,
        content: (
          <>
            <OriginDot origin={t.originated} />
            <span className="shrink-0 font-mono text-[11px] text-[#52525b]">{taskKey(t.id)}</span>
            <span className="min-w-0 truncate">{t.title}</span>
          </>
        ),
      }))}
      onPick={onPick}
      footer={(search) => {
        const id = search.trim()
        if (!taskOrigin(id) || id.length < 7 || exclude.has(id) || tasks.some((t) => t.id === id)) return null
        return (
          <button
            type="button"
            onClick={() => onPick(id)}
            className="flex h-9 w-full items-center gap-2 border-t border-[#f4f4f5] px-3 text-left text-[12px] text-[#3f3f46] hover:bg-[#fafafa]"
          >
            <Plus size={12} />
            Add <span className="font-mono">{id}</span> by ID
          </button>
        )
      }}
    />
  )
}

const ICON: Record<Kind, ReactNode> = {
  parent: <CornerLeftUp size={13} />,
  children: <CornerDownRight size={13} />,
  linked: <Link2 size={13} />,
}
const TITLE: Record<Kind, string> = { parent: 'Parent', children: 'Children', linked: 'Linked' }

/**
 * Relations shows a task's parent, children and links. With onChange, each
 * section gets a + (or a pencil for the parent) and chips can be removed.
 */
export function Relations({
  task,
  onChange,
  onOpen,
  rich,
  emptyChildren = 'None',
}: {
  task: TaskDetail
  onChange?: (patch: { parent_id?: string; children_ids?: string[]; linked_ids?: string[] }) => void
  onOpen: (id: string) => void
  /** rich is the task modal's style: icons, counts and status glyphs. */
  rich?: boolean
  emptyChildren?: string
}) {
  const [adding, setAdding] = useState<Kind | null>(null)
  const { data: tasks = [] } = useTasks()
  const stored = new Map<string, TaskSummary>(tasks.map((t) => [t.id, t]))
  for (const t of [task.parent, ...task.children, ...task.linked]) if (t) stored.set(t.id, t)
  const ids: Record<Kind, string[]> = {
    parent: task.parent_id ? [task.parent_id] : [],
    children: task.children_ids,
    linked: task.linked_ids,
  }
  const related = new Set([task.id, ...ids.parent, ...ids.children, ...ids.linked])

  const set = (kind: Kind, next: string[]) => {
    if (!onChange) return
    if (kind === 'parent') onChange({ parent_id: next[0] ?? '' })
    else onChange({ [kind === 'children' ? 'children_ids' : 'linked_ids']: next })
  }

  return (
    <div className={cn('flex flex-col', rich ? 'gap-4' : 'gap-3')}>
      {(['parent', 'children', 'linked'] as const).map((kind) => (
        <div key={kind} className="flex flex-col gap-2">
          <div className={cn('flex h-[26px] items-center gap-1.5 text-[12px]', rich ? 'text-[#52525b]' : 'text-[#71717a]')}>
            {rich && ICON[kind]}
            {TITLE[kind]}
            {rich && kind !== 'parent' && ids[kind].length > 0 && <span>· {ids[kind].length}</span>}
            {onChange && (
              <button
                type="button"
                aria-label={kind === 'parent' ? 'Change parent' : kind === 'children' ? 'Add child task' : 'Link a task'}
                onClick={() => setAdding(adding === kind ? null : kind)}
                className="ml-auto flex size-[26px] items-center justify-center rounded-md text-[#71717a] hover:bg-[#f4f4f5]"
              >
                {kind === 'parent' && ids.parent.length ? <Pencil size={13} /> : <Plus size={14} />}
              </button>
            )}
          </div>
          {ids[kind].map((id) => (
            <RelationChip
              key={id}
              id={id}
              task={stored.get(id)}
              rich={rich}
              onOpen={onOpen}
              onRemove={onChange ? () => set(kind, ids[kind].filter((x) => x !== id)) : undefined}
            />
          ))}
          {ids[kind].length === 0 && adding !== kind && (
            <span className="text-[12px] text-[#a1a1aa]">{kind === 'children' ? emptyChildren : 'None'}</span>
          )}
          {adding === kind && (
            <TaskPicker
              exclude={related}
              onPick={(id) => {
                setAdding(null)
                set(kind, kind === 'parent' ? [id] : [...ids[kind], id])
              }}
            />
          )}
        </div>
      ))}
    </div>
  )
}

