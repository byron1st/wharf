import { useQueryClient } from '@tanstack/react-query'
import { Check, CircleAlert, CircleArrowUp, ExternalLink, LoaderCircle, RefreshCw } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { keys, useUpdateStatus } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { ApiError, NetworkError, api } from '@/lib/api'
import { hhmm, monthDay, ymd } from '@/lib/dates'
import { serverDown, waitForServer } from '@/lib/errors'
import { notify } from '@/lib/toast'

/** Phase is what this panel is doing: checking now, applying the update, or waiting for the restarted server. */
type Phase = 'idle' | 'checking' | 'applying' | 'restarting'

/**
 * Updates is the Version section of the General settings: the running version,
 * what the last update check found, a check on demand, and Update and restart,
 * which replaces the server binary and reloads the page once the server is back.
 */
export function Updates() {
  const qc = useQueryClient()
  const { data: update } = useUpdateStatus()
  const { today } = useNow()
  const [phase, setPhase] = useState<Phase>('idle')
  if (!update) return null

  const checked = update.checked_at ? new Date(update.checked_at) : null
  const busy = phase !== 'idle'
  const updating = phase === 'applying' || phase === 'restarting'
  const latest = update.latest ?? ''

  const check = async () => {
    setPhase('checking')
    try {
      qc.setQueryData(keys.update, await api.checkUpdate())
    } catch (err) {
      if (err instanceof NetworkError) serverDown()
      else notify.error({ title: "Couldn't check for updates", description: err instanceof Error ? err.message : String(err) })
    } finally {
      setPhase('idle')
    }
  }

  const apply = async () => {
    setPhase('applying')
    try {
      await api.applyUpdate()
    } catch (err) {
      setPhase('idle')
      if (err instanceof NetworkError) return serverDown()
      notify.error({
        title: "Couldn't update Wharf",
        description:
          err instanceof ApiError && err.code === 'invalid_release'
            ? 'The downloaded release did not pass verification. Nothing was changed.'
            : err instanceof Error
              ? err.message
              : String(err),
      })
      return
    }
    // The server answers 202 and then restarts into the new binary.
    setPhase('restarting')
    if (await waitForServer()) {
      window.location.reload()
    } else {
      setPhase('idle')
      serverDown()
    }
  }

  const when = checked && `${ymd(checked) === today ? 'today' : monthDay(ymd(checked))} at ${hhmm(checked)}`
  const note =
    phase === 'checking'
      ? 'Checking GitHub releases…'
      : !update.supported
        ? 'Update checks are off: a development build, not macOS on Apple silicon, or update_check = false.'
        : update.check_failed
          ? `Last check failed${when ? ` · last successful check ${when}` : ''}`
          : when
            ? `Last checked ${when}`
            : 'Not checked yet.'

  return (
    <section aria-labelledby="settings-version" className="flex flex-col gap-3.5">
      <div className="flex items-center gap-2">
        <h3 id="settings-version" className="m-0 text-[15px] font-semibold">
          Version
        </h3>
        <span className="text-[12px] text-ink-muted">Updates come from GitHub releases</span>
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex min-w-0 flex-[1_1_220px] flex-col gap-[3px]">
          <span className="flex flex-wrap items-baseline gap-2">
            <span className="text-[15px] font-semibold">Wharf</span>
            <span className="font-mono text-[13px]">{update.current}</span>
            {update.supported && checked && !update.available && !update.check_failed && (
              <span className="inline-flex h-5 items-center gap-1 self-center rounded-full bg-success-tint px-[7px] text-[11px] font-medium text-success-ink">
                <Check size={10} strokeWidth={3} />
                Latest
              </span>
            )}
            {update.available && (
              <span className="inline-flex h-5 items-center self-center rounded-full border border-accent-line bg-accent-tint px-[7px] text-[11px] font-medium text-accent">
                {latest} available
              </span>
            )}
          </span>
          <span className="text-[12px] text-ink-muted">{note}</span>
        </div>
        {update.supported && (
          <Button variant="outline" disabled={busy} onClick={() => void check()}>
            {phase === 'checking' ? <LoaderCircle size={14} className="animate-spin text-accent" /> : <RefreshCw size={14} />}
            {phase === 'checking' ? 'Checking…' : 'Check for updates'}
          </Button>
        )}
      </div>
      {update.available && !updating && (
        <div className="flex flex-col gap-2.5 rounded-lg border border-accent-line bg-accent-tint px-4 py-3.5">
          <span className="flex flex-wrap items-center gap-2 text-[13px]">
            <CircleArrowUp size={16} className="shrink-0 text-accent" />
            <span className="font-semibold text-accent-ink">Wharf {latest} is available</span>
            {update.published_at && (
              <span className="text-ink-muted">· released {monthDay(ymd(new Date(update.published_at)))}</span>
            )}
          </span>
          <div className="flex flex-wrap items-center gap-2.5 pt-0.5">
            {update.release_url && (
              <a
                href={update.release_url}
                target="_blank"
                rel="noreferrer"
                className="inline-flex items-center gap-1 text-[13px] font-medium text-accent no-underline hover:text-accent-hover"
              >
                Release notes
                <ExternalLink size={12} />
              </a>
            )}
            <Button className="ml-auto px-3.5" disabled={busy} onClick={() => void apply()}>
              Update and restart
            </Button>
          </div>
          <span className="text-[12px] leading-normal text-ink-muted">
            Downloads the new build, replaces the running one and restarts the server. This page reconnects by itself.
          </span>
        </div>
      )}
      {updating && (
        <div role="status" className="flex flex-col gap-2 rounded-lg border border-border bg-bg-surface px-4 py-3.5">
          <span className="flex items-center gap-2 text-[13px] font-semibold">
            <LoaderCircle size={14} className="shrink-0 animate-spin text-accent" />
            Updating to {latest}…
          </span>
          <span className="text-[12px] text-ink-muted">
            {phase === 'applying'
              ? 'Downloading the new build and replacing the binary…'
              : 'Restarting the server — this page reconnects in a moment.'}
          </span>
        </div>
      )}
      {update.supported && update.check_failed && !busy && (
        <div
          role="alert"
          className="flex flex-wrap items-center gap-2.5 rounded-lg border border-danger-line bg-danger-tint px-3 py-2.5 text-[13px] text-danger-ink"
        >
          <CircleAlert size={15} className="shrink-0" />
          <span className="flex-[1_1_220px]">
            Couldn’t check for updates. The server log (~/Library/Logs/wharf/wharf.log) has the details.
          </span>
          <Button size="xs" className="bg-danger px-3 hover:bg-danger-hover" disabled={busy} onClick={() => void check()}>
            Retry
          </Button>
        </div>
      )}
    </section>
  )
}
