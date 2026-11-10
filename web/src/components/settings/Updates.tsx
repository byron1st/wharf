import { useUpdateStatus } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { hhmm, ymd } from '@/lib/dates'

/** Updates shows the running version and what the last update check found. */
export function Updates() {
  const { data: update } = useUpdateStatus()
  const { today } = useNow()
  if (!update) return null

  const checked = update.checked_at ? new Date(update.checked_at) : null
  const state = !update.supported
    ? 'Update checks are off: a development build, not macOS on Apple silicon, or update_check = false.'
    : !checked
      ? 'Not checked yet.'
      : update.available
        ? `Wharf ${update.latest} is available.`
        : 'Up to date.'
  return (
    <div className="overflow-hidden rounded-lg border border-border">
      <div className="grid min-h-10 grid-cols-[108px_minmax(0,1fr)] items-center px-3.5 text-[13px]">
        <span className="text-ink-muted">Version</span>
        <span>
          <span className="font-mono">{update.current}</span>
          <span className="text-[12px] text-ink-muted">
            {' · '}
            {state}
            {checked && ` Checked ${ymd(checked) === today ? '' : `${ymd(checked)} `}${hhmm(checked)}.`}
          </span>
        </span>
      </div>
    </div>
  )
}
