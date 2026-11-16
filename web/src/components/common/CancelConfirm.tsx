import { Dialog as D } from 'radix-ui'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { useTasks } from '@/hooks/data'
import { taskKey, type TaskStatus } from '@/lib/api'

interface Pending {
  id: string
  /** count is how many todos the task has, or null when the task list has not loaded it. */
  count: number | null
  run: () => void
}

/**
 * useCancelConfirm guards a status change: canceling a task deletes its todos,
 * so a change to canceled waits for confirmation unless the task list shows
 * the task has none. guard runs every other change at once. Render dialog
 * where the hook is used.
 */
export function useCancelConfirm() {
  const { data: tasks = [] } = useTasks()
  const [pending, setPending] = useState<Pending | null>(null)

  const guard = (task: { id: string; status: TaskStatus }, next: TaskStatus, run: () => void) => {
    // An unknown count asks too, rather than delete todos unasked.
    const count = tasks.find((t) => t.id === task.id)?.todo_count ?? null
    if (next === 'canceled' && task.status !== 'canceled' && count !== 0) setPending({ id: task.id, count, run })
    else run()
  }

  const dialog = pending && (
    <D.Root open onOpenChange={(o) => !o && setPending(null)}>
      <D.Portal>
        <D.Overlay className="fixed inset-0 z-50 flex items-center justify-center bg-[rgba(9,9,11,0.4)] px-6">
          <D.Content className="flex w-full max-w-[420px] flex-col gap-4 rounded-[14px] border border-border bg-white p-6 shadow-xl outline-none">
            <div className="flex flex-col gap-1.5">
              <D.Title className="m-0 text-[17px] font-semibold tracking-[-0.01em]">Cancel {taskKey(pending.id)}?</D.Title>
              <D.Description className="m-0 text-[13px] leading-normal text-ink-muted">
                Canceling deletes its{' '}
                {pending.count === null ? 'todos' : `${pending.count} todo${pending.count > 1 ? 's' : ''}`}. Reopening the task won’t bring
                them back.
              </D.Description>
            </div>
            <div className="flex justify-end gap-2">
              <D.Close asChild>
                <Button variant="outline">Keep task</Button>
              </D.Close>
              <Button
                variant="destructive"
                onClick={() => {
                  pending.run()
                  setPending(null)
                }}
              >
                Cancel and delete todos
              </Button>
            </div>
          </D.Content>
        </D.Overlay>
      </D.Portal>
    </D.Root>
  )

  return { guard, dialog }
}
