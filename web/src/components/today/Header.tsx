import { CircleAlert, Plus, RefreshCw, Settings } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { Kbd, OriginBadge } from '@/components/common/badges'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useLastSync } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { useSync } from '@/hooks/useSync'
import { headerDate, hhmm, ymd } from '@/lib/dates'
import { cn } from '@/lib/utils'

const pill = 'flex h-8 items-center gap-1.5 rounded-full border px-2.5 text-[12px] font-medium whitespace-nowrap tabular-nums'

function SyncButton() {
  const { status, run } = useSync()
  const { data: last } = useLastSync()
  const { today } = useNow()
  const at = last ? new Date(last.synced_at) : null
  const lastText = at ? (ymd(at) === today ? hhmm(at) : `${at.getMonth() + 1}/${at.getDate()} ${hhmm(at)}`) : null
  const busy = status === 'busy'
  const failed = status === 'failed'
  return (
    <button
      type="button"
      onClick={run}
      disabled={busy}
      aria-label="Sync Atlassian Cloud"
      title={busy ? 'Syncing Atlassian Cloud…' : failed ? 'Last sync failed — click to retry' : `Sync Atlassian Cloud${lastText ? ` · last synced ${lastText}` : ''}`}
      className={cn(
        pill,
        busy
          ? 'border-accent-line bg-accent-tint text-accent'
          : failed
            ? 'border-danger-line bg-danger-tint text-danger-ink'
            : 'border-border bg-white text-ink-secondary hover:bg-bg-subtle',
      )}
    >
      {busy ? (
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden className="animate-spin">
          <circle cx="12" cy="12" r="9" className="stroke-tag-teal-line" strokeWidth="3" />
          <path d="M21 12a9 9 0 0 0-9-9" className="stroke-accent" strokeWidth="3" strokeLinecap="round" />
        </svg>
      ) : failed ? (
        <CircleAlert size={14} />
      ) : (
        <RefreshCw size={14} />
      )}
      {busy ? 'Syncing…' : failed ? 'Sync failed' : lastText ? `Synced ${lastText}` : 'Sync now'}
    </button>
  )
}

/** NewTaskButton opens the "New task" popover; N opens it from anywhere outside a text field. */
function NewTaskButton({ onCreate }: { onCreate: (title: string) => Promise<void> }) {
  const [open, setOpen] = useState(false)
  const [title, setTitle] = useState('')
  const [busy, setBusy] = useState(false)
  const input = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if (e.key.toLowerCase() !== 'n' || e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"], [role="dialog"]')) return
      e.preventDefault()
      setOpen(true)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  const create = async () => {
    const t = title.trim()
    if (!t || busy) return
    setBusy(true)
    try {
      await onCreate(t)
      setOpen(false)
      setTitle('')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        setOpen(o)
        if (!o) setTitle('')
      }}
    >
      <PopoverTrigger asChild>
        <button
          type="button"
          title="New local task (N)"
          aria-haspopup="dialog"
          className={cn(
            pill,
            'pr-3',
            open
              ? 'border-accent bg-accent-tint text-accent-ink shadow-ring'
              : 'border-border bg-white text-ink-secondary hover:bg-bg-subtle',
          )}
        >
          <Plus size={14} strokeWidth={2.2} />
          New task
        </button>
      </PopoverTrigger>
      <PopoverContent
        align="end"
        sideOffset={8}
        aria-label="New local task"
        className="flex w-[340px] flex-col gap-2.5 p-3.5"
        onOpenAutoFocus={(e) => {
          e.preventDefault()
          input.current?.focus()
        }}
      >
        <span className="flex items-center gap-2 text-[13px] font-semibold">
          New task
          <OriginBadge origin="local" />
        </span>
        <input
          ref={input}
          aria-label="Task title"
          placeholder="Task title"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && !e.nativeEvent.isComposing) {
              e.preventDefault()
              void create()
            }
          }}
          className="h-8 rounded-md border border-border px-3 text-[13px] outline-none focus:border-accent focus:shadow-ring"
        />
        <span className="text-[12px] leading-normal text-ink-muted">
          Opens in the inspector so you can add a due date, description and todos next.
        </span>
        <div className="flex items-center gap-2 border-t border-border-subtle pt-2.5 text-[12px] text-ink-muted">
          <span className="flex items-center gap-1">
            <Kbd accent>Enter</Kbd>create
          </span>
          <span className="flex items-center gap-1">
            <Kbd>Esc</Kbd>cancel
          </span>
          <Button variant="outline" size="xs" className="ml-auto" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button size="xs" disabled={!title.trim() || busy} onClick={() => void create()}>
            Create
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  )
}

export function Header({ onCreateTask, onSettings }: { onCreateTask: (title: string) => Promise<void>; onSettings: () => void }) {
  const { now } = useNow()
  return (
    <header className="flex flex-wrap items-center gap-x-4 gap-y-3">
      <h1 className="m-0 text-[22px] leading-[28px] font-semibold tracking-[-0.02em]">Wharf</h1>
      <div className="ml-auto flex items-center gap-3">
        <SyncButton />
        <NewTaskButton onCreate={onCreateTask} />
        <span className="h-4 w-px bg-bg-pressed" />
        <span className="flex items-baseline gap-2.5 tabular-nums">
          <time className="text-[13px] font-medium text-today-ink">{headerDate(now)}</time>
          <span aria-hidden className="h-[14px] w-px self-center bg-bg-pressed" />
          <time aria-label="Current time" className="text-[15px] font-semibold tracking-[-0.01em]">
            {hhmm(now)}
          </time>
        </span>
        <button
          type="button"
          aria-label="Settings"
          title="Settings (⌘,)"
          onClick={onSettings}
          className="flex size-8 items-center justify-center rounded-md border border-border bg-white text-ink-secondary shadow-xs hover:bg-bg-subtle"
        >
          <Settings size={17} />
        </button>
      </div>
    </header>
  )
}
