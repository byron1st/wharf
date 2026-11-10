import { Bell, Check, Plus, X } from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useNow } from '@/hooks/useNow'
import {
  addDays,
  atLocal,
  dayLabel,
  diffDays,
  dow,
  hhmm,
  isSent,
  isTime,
  parseYmd,
  reminderLabel,
  shortDay,
  sortReminders,
  ymd,
} from '@/lib/dates'
import { cn } from '@/lib/utils'
import { Calendar } from './Calendar'

interface FormProps {
  mode: 'new' | 'edit'
  /** at is the reminder being edited. */
  at?: string
  /** end is the todo's end date, for the "Morning of end date" preset. */
  end: string | null
  onSubmit: (at: string) => void
  onDelete?: () => void
  onCancel: () => void
}

/** ReminderForm is the reminder popover body: presets when adding, then a day and a 24-hour time. */
function ReminderForm({ mode, at, end, onSubmit, onDelete, onCancel }: FormProps) {
  const { now, today } = useNow()
  const initial = at ? new Date(at) : null
  const [day, setDay] = useState(initial ? ymd(initial) : addDays(today, 1))
  const [time, setTime] = useState(initial ? hhmm(initial) : '09:00')

  const bad = !isTime(time)
  const past = !bad && atLocal(day, time).getTime() <= now.getTime()
  const invalid = bad || past

  const presets: { title: string; when: string; at: Date }[] = []
  const inHour = new Date(now.getTime() + 3600_000)
  inHour.setSeconds(0, 0)
  presets.push({ title: 'In 1 hour', when: `${shortDay(ymd(inHour), today)} ${hhmm(inHour)}`, at: inHour })
  const evening = atLocal(today, '18:00')
  if (evening > now) presets.push({ title: 'This evening', when: 'Today 18:00', at: evening })
  presets.push({
    title: 'Tomorrow morning',
    when: `${dow(addDays(today, 1))} 09:00`,
    at: atLocal(addDays(today, 1), '09:00'),
  })
  if (end && diffDays(today, end) > 0) {
    presets.push({
      title: 'Morning of end date',
      when: `${dow(end)} ${parseYmd(end).getDate()} · 09:00`,
      at: atLocal(end, '09:00'),
    })
  }

  const submit = () => {
    if (!invalid) onSubmit(atLocal(day, time).toISOString())
  }

  return (
    <div className="flex flex-col gap-3">
      <span className="text-[12px] font-medium text-ink-muted">{mode === 'new' ? 'New reminder' : 'Edit reminder'}</span>
      {mode === 'new' && (
        <>
          <div className="-mx-1 -mt-1 flex flex-col">
            {presets.map((p) => (
              <button
                key={p.title}
                type="button"
                onClick={() => onSubmit(p.at.toISOString())}
                className="flex h-[35px] items-center gap-2.5 rounded-md px-2 text-left text-[13px] hover:bg-bg-muted"
              >
                <Bell size={14} className="text-tag-violet-solid" />
                <span className="flex-1">{p.title}</span>
                <span className="text-[12px] text-ink-muted tabular-nums">{p.when}</span>
              </button>
            ))}
          </div>
          <div className="border-t border-border-subtle pt-3 text-[12px] font-medium text-ink-muted">Pick a time</div>
        </>
      )}
      <Calendar today={today} month={day} selected={day} lo={today} tone="violet" onPick={setDay} />
      <div className="flex flex-col gap-1.5">
        <div className="flex items-center gap-2">
          <label htmlFor="reminder-time" className="text-[13px] text-ink-secondary">
            Time
          </label>
          <input
            id="reminder-time"
            value={time}
            inputMode="numeric"
            maxLength={5}
            aria-invalid={invalid}
            onChange={(e) => setTime(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && submit()}
            className={cn(
              'h-8 w-[72px] rounded-md border px-2 font-mono text-[13px] outline-none',
              invalid ? 'border-danger shadow-ring-red' : 'border-border focus:border-tag-violet-solid',
            )}
          />
          <span className="ml-auto text-[12px] text-ink-muted tabular-nums">
            {dayLabel(day)}
            {isTime(time) ? ` · ${time}` : ''}
          </span>
        </div>
        {invalid && (
          <p role="alert" className="m-0 flex items-center gap-1.5 text-[12px] text-danger-ink">
            {bad ? 'Use 24-hour time, like 09:30.' : `That time has already passed. It's ${hhmm(now)} now.`}
          </p>
        )}
      </div>
      <div className="flex items-center gap-2 border-t border-border-subtle pt-3">
        {mode === 'edit' && (
          <>
            <button
              type="button"
              onClick={onDelete}
              className="h-8 rounded-md px-2.5 text-[13px] font-medium text-danger hover:bg-danger-tint"
            >
              Delete
            </button>
            <Button variant="outline" size="sm" className="ml-auto" onClick={onCancel}>
              Cancel
            </Button>
          </>
        )}
        <Button
          size="sm"
          disabled={invalid}
          onClick={submit}
          className={cn(mode === 'new' && 'ml-auto', invalid && 'bg-[#99c7c2] opacity-100')}
        >
          {mode === 'new' ? 'Add reminder' : 'Save'}
        </Button>
      </div>
    </div>
  )
}

interface RemindersProps {
  reminders: string[]
  end: string | null
  onChange: (reminders: string[]) => void
  /** layout "stack" is the Inspector's full-width rows; "inline" the task modal's chips. */
  layout: 'stack' | 'inline'
}

/** Reminders lists a todo's reminders in violet; sent ones are gray, can't be edited, only removed. */
export function Reminders({ reminders, end, onChange, layout }: RemindersProps) {
  const { now } = useNow()
  const [open, setOpen] = useState<string | null>(null)
  const sorted = sortReminders(reminders)
  const stack = layout === 'stack'

  const replace = (old: string | null, next: string | null) => {
    setOpen(null)
    const list = sorted.filter((r) => r !== old)
    onChange(next ? sortReminders([...list, next]) : list)
  }

  const pop = (id: string, trigger: ReactNode, form: ReactNode) => (
    <Popover open={open === id} onOpenChange={(o) => setOpen(o ? id : null)}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent align="start">{form}</PopoverContent>
    </Popover>
  )

  return (
    <div className={cn('flex gap-1.5', stack ? 'flex-col' : 'flex-wrap items-center')}>
      {sorted.map((r) => {
        const sent = isSent(r, now)
        const editing = open === r
        const chip = (
          <div
            className={cn(
              'flex h-8 items-center rounded-md border text-[13px]',
              stack && 'w-full',
              sent ? 'border-border bg-bg-subtle text-ink-muted' : 'border-tag-violet-line bg-[#faf8ff] text-tag-violet-ink',
              editing && 'border-tag-violet-solid shadow-ring-violet',
            )}
          >
            {sent ? (
              <span
                className="flex h-full flex-1 items-center gap-1.5 pr-1.5 pl-2.5 tabular-nums"
                title="Already sent"
                aria-label={`Reminder sent ${reminderLabel(r)}`}
              >
                <Check size={13} />
                <span className="flex-1">{reminderLabel(r)}</span>
                <span className="text-[11px]">Sent</span>
              </span>
            ) : (
              pop(
                r,
                <button
                  type="button"
                  aria-label={`Edit reminder ${reminderLabel(r)}`}
                  className="flex h-full flex-1 items-center gap-1.5 pr-1.5 pl-2.5 text-left tabular-nums"
                >
                  <Bell size={13} className="text-tag-violet-solid" />
                  <span className="flex-1">{reminderLabel(r)}</span>
                </button>,
                <ReminderForm
                  mode="edit"
                  at={r}
                  end={end}
                  onSubmit={(at) => replace(r, at)}
                  onDelete={() => replace(r, null)}
                  onCancel={() => setOpen(null)}
                />,
              )
            )}
            <button
              type="button"
              aria-label={`Remove reminder ${reminderLabel(r)}`}
              onClick={() => replace(r, null)}
              className={cn(
                'mr-1.5 flex size-5 items-center justify-center rounded hover:bg-black/5',
                sent ? 'text-ink-faint' : 'text-tag-violet-solid',
              )}
            >
              <X size={13} />
            </button>
          </div>
        )
        return <div key={r}>{chip}</div>
      })}
      {pop(
        'new',
        <button
          type="button"
          className={cn(
            'flex h-8 items-center justify-center gap-1.5 rounded-md border px-2.5 text-[13px] font-medium',
            stack ? 'w-full' : '',
            open === 'new'
              ? 'border-solid border-tag-violet-solid text-tag-violet-ink shadow-ring-violet'
              : 'border-dashed border-border-strong text-ink-secondary hover:bg-bg-subtle',
          )}
        >
          <Plus size={14} />
          Add reminder
        </button>,
        <ReminderForm mode="new" end={end} onSubmit={(at) => replace(null, at)} onCancel={() => setOpen(null)} />,
      )}
    </div>
  )
}
