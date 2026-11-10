import { ChevronDown, ChevronUp, Ticket } from 'lucide-react'
import { useState } from 'react'
import { reqStatusStyle } from '@/components/common/badges'
import { useNow } from '@/hooks/useNow'
import type { ServiceDeskRequest } from '@/lib/api'
import { diffDays, ymd } from '@/lib/dates'
import { cn } from '@/lib/utils'
import type { Selection } from './TodosCard'

/** RequestsCard lists the open Service Desk requests; the header expands it in place. */
export function RequestsCard({
  requests,
  selection,
  onSelect,
}: {
  requests: ServiceDeskRequest[]
  selection: Selection
  onSelect: (s: Selection) => void
}) {
  const { today } = useNow()
  const [open, setOpen] = useState(() => localStorage.getItem('wharf.requestsOpen') !== 'false')
  const toggle = () => {
    localStorage.setItem('wharf.requestsOpen', String(!open))
    setOpen(!open)
  }
  const changed = requests.filter((r) => ymd(new Date(r.status_at)) === today).length

  return (
    <section
      aria-label="Service Desk requests"
      className={cn('overflow-hidden rounded-xl border bg-white shadow-xs', open ? 'border-[#c7d2fe]' : 'border-border')}
    >
      <button
        type="button"
        aria-expanded={open}
        onClick={toggle}
        className={cn('flex min-h-11 w-full items-center gap-2 pr-3 pl-3.5 text-left text-ink', open ? 'bg-[#f5f7ff]' : 'bg-white')}
      >
        <Ticket size={15} className="text-tag-indigo-solid" />
        <span className="text-[13px] font-semibold">Requests</span>
        <span className="flex h-5 min-w-5 items-center justify-center rounded-full bg-tag-indigo-bg px-1.5 text-[11px] font-semibold text-tag-indigo-ink tabular-nums">
          {requests.length}
        </span>
        {changed > 0 && (
          <span className="inline-flex items-center gap-[5px] text-[12px] font-medium text-tag-indigo-ink">
            <span className="size-1.5 rounded-full bg-tag-indigo-solid" />
            {changed} changed today
          </span>
        )}
        <span className="ml-auto flex text-ink-muted">{open ? <ChevronUp size={14} /> : <ChevronDown size={14} />}</span>
      </button>
      {open && (
        <div className="flex max-h-80 flex-col gap-0.5 overflow-y-auto border-t border-border-subtle p-1.5">
          {requests.map((r) => {
            const s = reqStatusStyle(r)
            const isSel = selection?.kind === 'req' && selection.id === r.key
            const days = diffDays(ymd(new Date(r.status_at)), today)
            return (
              <div
                key={r.key}
                className={cn(
                  'grid min-h-11 grid-cols-[12px_minmax(0,1fr)_auto] items-center gap-2 rounded-lg px-2',
                  isSel && 'bg-tag-indigo-bg shadow-[inset_0_0_0_1px_#c7d2fe]',
                )}
              >
                <span className="flex justify-center">
                  <span className="size-[7px] rounded-full" style={{ background: s.dot }} />
                </span>
                <button
                  type="button"
                  aria-pressed={isSel}
                  onClick={() => onSelect({ kind: 'req', id: r.key })}
                  className="flex h-[42px] min-w-0 flex-col justify-center gap-px text-left text-ink"
                >
                  <span className={cn('max-w-full truncate text-[13px]', isSel && 'font-semibold')}>{r.title}</span>
                  <span className="flex items-center gap-1.5 text-[11px] text-ink-muted tabular-nums">
                    <span className="font-mono text-tag-indigo-ink">{r.key}</span>
                    {days <= 0 ? (
                      <span className="font-semibold text-tag-indigo-ink">changed today</span>
                    ) : (
                      <span>{days}d in status</span>
                    )}
                  </span>
                </button>
                <span
                  className="rounded-[5px] px-1.5 py-px text-[11px] font-medium whitespace-nowrap"
                  style={{ background: s.bg, color: s.color }}
                >
                  {r.status}
                </span>
              </div>
            )
          })}
          {requests.length === 0 && <p className="m-0 p-2.5 text-[12px] text-ink-muted">No open requests.</p>}
        </div>
      )}
    </section>
  )
}
