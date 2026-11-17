import { Check } from 'lucide-react'
import { useState } from 'react'
import { STATUS_LABEL, STATUSES, StatusIcon } from '@/components/common/badges'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import type { TaskListItem, TaskStatus } from '@/lib/api'
import { cn } from '@/lib/utils'

const CHIP: Record<TaskStatus, string> = {
  todo: 'border-border bg-bg-muted text-ink-secondary',
  'in-progress': 'border-warning-line bg-warning-tint text-warning-ink',
  done: 'border-[#ddd6fe] bg-tag-violet-bg text-tag-violet-ink',
  canceled: 'border-border bg-bg-muted text-ink-muted',
}

const chip = 'flex h-6 items-center gap-[5px] rounded-full border pr-2 pl-[5px] text-[12px] font-medium whitespace-nowrap'

/** StatusChip is a task's labeled status pill; it opens the status menu of a local task and is read-only for a Jira one. */
export function StatusChip({ task, onSetStatus }: { task: TaskListItem; onSetStatus: (status: TaskStatus) => void }) {
  const [open, setOpen] = useState(false)
  const label = STATUS_LABEL[task.status]
  if (task.originated !== 'local') {
    return (
      <span role="img" aria-label={`Status: ${label}`} title={`${label} · from Jira`} className={cn(chip, CHIP[task.status])}>
        <StatusIcon status={task.status} />
        {label}
      </span>
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
