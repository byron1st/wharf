import { Diamond, Laptop, Ticket } from 'lucide-react'
import type { Origin, ServiceDeskRequest, TaskStatus } from '@/lib/api'
import { cn } from '@/lib/utils'

export const ORIGIN_DOT: Record<string, string> = { jira: 'bg-[#3b82f6]', local: 'bg-tag-teal-solid' }

/** OriginDot is the 7px square-ish origin marker before task keys. */
export function OriginDot({ origin, className }: { origin?: Origin; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn('inline-block size-[7px] shrink-0 rounded-[2px]', ORIGIN_DOT[origin ?? ''] ?? 'bg-tag-gray-solid', className)}
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
        jira ? 'bg-tag-blue-bg text-tag-blue-ink' : 'bg-accent-tint text-accent',
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
      ? { label: 'Jira', cls: 'bg-tag-blue-bg border-tag-blue-line text-tag-blue-ink', Icon: Diamond }
      : origin === 'sd'
        ? { label: 'Service Desk', cls: 'bg-tag-indigo-bg border-tag-indigo-line text-tag-indigo-ink', Icon: Ticket }
        : { label: 'Local', cls: 'bg-accent-tint border-tag-teal-line text-accent', Icon: Laptop }
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
  if (status === 'done')
    return (
      <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden className="shrink-0">
        <circle cx="8" cy="8" r="7" className="fill-tag-violet-solid" />
        <path d="m5 8.2 2 2 4-4.2" fill="none" stroke="#fff" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" />
      </svg>
    )
  if (status === 'canceled')
    return (
      <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden className="shrink-0">
        <circle cx="8" cy="8" r="7" className="fill-tag-gray-solid" />
        <path d="m5.6 5.6 4.8 4.8M10.4 5.6l-4.8 4.8" stroke="#fff" strokeWidth="1.6" strokeLinecap="round" />
      </svg>
    )
  if (status === 'in-progress')
    return (
      <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden className="shrink-0">
        <circle cx="8" cy="8" r="6.25" fill="none" className="stroke-warning" strokeWidth="1.5" />
        <path d="M8 3.5a4.5 4.5 0 0 1 0 9z" className="fill-warning" />
      </svg>
    )
  return (
    <svg width={size} height={size} viewBox="0 0 16 16" aria-hidden className="shrink-0">
      <circle cx="8" cy="8" r="6.25" fill="none" className="stroke-ink-muted" strokeWidth="1.5" />
    </svg>
  )
}

const STATUS_BADGE: Record<TaskStatus, string> = {
  todo: 'bg-bg-muted border-border text-ink-secondary',
  'in-progress': 'bg-warning-tint border-warning-line text-warning-ink',
  done: 'bg-tag-violet-bg border-[#ddd6fe] text-tag-violet-ink',
  canceled: 'bg-bg-muted border-border text-ink-muted',
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
  pending: { bg: 'var(--color-tag-gray-bg)', color: 'var(--color-tag-gray-ink)', dot: 'var(--color-tag-gray-solid)' },
  'in progress': { bg: 'var(--color-warning-tint)', color: 'var(--color-warning-ink)', dot: 'var(--color-warning)' },
  'security review': { bg: '#fdf2f8', color: '#9d174d', dot: '#db2777' },
}

/** reqStatusStyle colors a Service Desk status by name, falling back to its Jira category. */
export function reqStatusStyle(r: Pick<ServiceDeskRequest, 'status' | 'status_category'>) {
  const byName = REQ_STATUS[r.status.toLowerCase()]
  if (byName) return byName
  if (r.status_category === 'DONE') return { bg: 'var(--color-tag-gray-bg)', color: 'var(--color-ink-muted)', dot: 'var(--color-tag-gray-solid)' }
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
        accent ? 'border-accent-line text-accent-ink' : 'border-border text-ink-secondary',
      )}
    >
      {children}
    </kbd>
  )
}
