import { CircleCheck, CircleX, Diamond, Laptop, Ticket } from 'lucide-react'
import type { Origin, ServiceDeskRequest, TaskStatus } from '@/lib/api'
import { cn } from '@/lib/utils'

export const ORIGIN_DOT: Record<string, string> = { jira: 'bg-[#3b82f6]', local: 'bg-[#14b8a6]' }

/** OriginDot is the 7px square-ish origin marker before task keys. */
export function OriginDot({ origin, className }: { origin?: Origin; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn('inline-block size-[7px] shrink-0 rounded-[2px]', ORIGIN_DOT[origin ?? ''] ?? 'bg-[#a1a1aa]', className)}
    />
  )
}

/** OriginIcon is the small boxed Jira diamond or Local laptop of rich relation chips. */
export function OriginIcon({ origin }: { origin: Origin }) {
  const jira = origin === 'jira'
  const Icon = jira ? Diamond : Laptop
  return (
    <span
      aria-hidden
      className={cn(
        'flex size-4 shrink-0 items-center justify-center rounded',
        jira ? 'bg-[#eff6ff] text-[#1d4ed8]' : 'bg-[#f0fdfa] text-[#0f766e]',
      )}
    >
      <Icon size={10} strokeWidth={2.4} />
    </span>
  )
}

/** OriginBadge is the Jira / Local / Service Desk label. */
export function OriginBadge({ origin, icon = false }: { origin: Origin | 'sd'; icon?: boolean }) {
  const s =
    origin === 'jira'
      ? { label: 'Jira', cls: 'bg-[#eff6ff] border-[#dbeafe] text-[#1d4ed8]', Icon: Diamond }
      : origin === 'sd'
        ? { label: 'Service Desk', cls: 'bg-[#eef2ff] border-[#e0e7ff] text-[#4338ca]', Icon: Ticket }
        : { label: 'Local', cls: 'bg-[#f0fdfa] border-[#ccfbf1] text-[#0f766e]', Icon: Laptop }
  return (
    <span
      className={cn(
        'inline-flex h-5 shrink-0 items-center gap-1 rounded-[5px] border px-1.5 text-[11px] font-semibold',
        s.cls,
      )}
    >
      {icon && <s.Icon size={11} strokeWidth={2.2} />}
      {s.label}
    </span>
  )
}

export const STATUS_LABEL: Record<TaskStatus, string> = {
  todo: 'Todo',
  'in-progress': 'In progress',
  done: 'Done',
  canceled: 'Canceled',
}

/** StatusIcon is the task status glyph: open circle, half-filled amber, violet check, gray cross. */
export function StatusIcon({ status, size = 14 }: { status: TaskStatus; size?: number }) {
  if (status === 'done') return <CircleCheck size={size} className="shrink-0 text-[#7c3aed]" aria-hidden />
  if (status === 'canceled') return <CircleX size={size} className="shrink-0 text-[#a1a1aa]" aria-hidden />
  if (status === 'in-progress')
    return (
      <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden className="shrink-0">
        <circle cx="8" cy="8" r="6.25" fill="none" stroke="#d97706" strokeWidth="1.5" />
        <path d="M8 3.5a4.5 4.5 0 0 1 0 9z" fill="#d97706" />
      </svg>
    )
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden className="shrink-0">
      <circle cx="8" cy="8" r="6.25" fill="none" stroke="#71717a" strokeWidth="1.5" />
    </svg>
  )
}

const STATUS_BADGE: Record<TaskStatus, string> = {
  todo: 'bg-[#f4f4f5] border-[#e4e4e7] text-[#3f3f46]',
  'in-progress': 'bg-[#fffbeb] border-[#fde68a] text-[#92400e]',
  done: 'bg-[#f5f3ff] border-[#ddd6fe] text-[#5b21b6]',
  canceled: 'bg-[#f4f4f5] border-[#e4e4e7] text-[#71717a]',
}

export function StatusBadge({ status, className }: { status: TaskStatus; className?: string }) {
  return (
    <span
      className={cn(
        'inline-flex h-9 items-center gap-2 rounded-md border px-3 text-[14px] font-medium',
        STATUS_BADGE[status],
        className,
      )}
    >
      <StatusIcon status={status} size={15} />
      {STATUS_LABEL[status]}
    </span>
  )
}

const REQ_STATUS: Record<string, { bg: string; color: string; dot: string }> = {
  open: { bg: '#f0f9ff', color: '#0369a1', dot: '#0ea5e9' },
  pending: { bg: '#f4f4f5', color: '#52525b', dot: '#a1a1aa' },
  'in progress': { bg: '#fffbeb', color: '#92400e', dot: '#d97706' },
  'security review': { bg: '#fdf2f8', color: '#9d174d', dot: '#db2777' },
}

/** reqStatusStyle colors a Service Desk status by name, falling back to its Jira category. */
export function reqStatusStyle(r: Pick<ServiceDeskRequest, 'status' | 'status_category'>) {
  const byName = REQ_STATUS[r.status.toLowerCase()]
  if (byName) return byName
  if (r.status_category === 'DONE') return { bg: '#f4f4f5', color: '#71717a', dot: '#a1a1aa' }
  if (r.status_category === 'INDETERMINATE') return REQ_STATUS['in progress']
  return REQ_STATUS.open
}

export function ReqStatusPill({ request, className }: { request: ServiceDeskRequest; className?: string }) {
  const s = reqStatusStyle(request)
  return (
    <span
      className={cn('inline-flex h-5 shrink-0 items-center rounded-[5px] px-1.5 text-[11px] font-medium', className)}
      style={{ background: s.bg, color: s.color }}
    >
      {request.status}
    </span>
  )
}

export function Kbd({ children, accent }: { children: string; accent?: boolean }) {
  return (
    <kbd
      className={cn(
        'rounded border bg-white px-[5px] font-mono text-[11px]',
        accent ? 'border-[#99f6e4] text-[#115e59]' : 'border-[#e4e4e7] text-[#52525b]',
      )}
    >
      {children}
    </kbd>
  )
}
