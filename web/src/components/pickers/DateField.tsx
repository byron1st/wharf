import { CalendarDays, ChevronDown } from 'lucide-react'
import { useState } from 'react'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { useNow } from '@/hooks/useNow'
import { dayLabel, quickPicks } from '@/lib/dates'
import { cn } from '@/lib/utils'
import { Calendar } from './Calendar'

export interface DateFieldProps {
  /** title heads the popover: "Start date", "End date", "Due date". */
  title: string
  value: string | null
  onChange: (value: string | null) => void
  /** format renders the value in the field. */
  format?: (v: string) => string
  /** other is the paired date and pairText its footer label, like "Starts Mon, Oct 26". */
  other?: string | null
  pairText?: string
  lo?: string | null
  hi?: string | null
  /** tone colors the field for an overdue or due-today date. */
  tone?: 'over' | 'today'
  showIcon?: boolean
  align?: 'start' | 'end'
  className?: string
  'aria-label'?: string
}

/** DateField opens a date popover; picking a day saves it and closes, with no Save button. */
export function DateField({
  title,
  value,
  onChange,
  format = dayLabel,
  other,
  pairText,
  lo,
  hi,
  tone,
  showIcon = true,
  align = 'start',
  className,
  ...rest
}: DateFieldProps) {
  const { today } = useNow()
  const [open, setOpen] = useState(false)
  const pick = (v: string | null) => {
    setOpen(false)
    if (v !== value) onChange(v)
  }
  const ok = (d: string) => (!lo || d >= lo) && (!hi || d <= hi)

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={rest['aria-label'] ?? title}
          className={cn(
            'flex h-8 w-full min-w-0 items-center gap-2 rounded-md border bg-white px-2.5 text-left text-[13px] shadow-xs',
            tone === 'over'
              ? 'border-danger-line bg-danger-tint text-danger-ink'
              : tone === 'today'
                ? 'border-[#fed7aa] bg-today-tint text-[#9a3412]'
                : 'border-border',
            open && 'border-accent shadow-ring',
            !value && 'text-ink-faint',
            className,
          )}
        >
          {showIcon && <CalendarDays size={14} className="shrink-0 opacity-70" />}
          <span className="min-w-0 flex-1 truncate tabular-nums">{value ? format(value) : 'Not set'}</span>
          <ChevronDown size={14} className="shrink-0 text-ink-faint" />
        </button>
      </PopoverTrigger>
      <PopoverContent align={align} aria-label={title} className="flex flex-col gap-3">
        <span className="text-[12px] font-medium text-ink-muted">{title}</span>
        <div className="flex flex-wrap gap-1.5">
          {quickPicks(today).map((q) => {
            const on = q.value === value
            const disabled = !ok(q.value)
            return (
              <button
                key={q.label}
                type="button"
                disabled={disabled}
                onClick={() => pick(q.value)}
                className={cn(
                  'h-[26px] rounded-full border px-[9px] text-[12px]',
                  on ? 'border-accent bg-accent-tint font-semibold text-accent-ink' : 'border-border text-ink-secondary',
                  disabled && 'text-border-strong',
                )}
              >
                {q.label}
              </button>
            )
          })}
        </div>
        <Calendar today={today} month={value ?? other ?? today} selected={value} other={other} lo={lo} hi={hi} onPick={pick} />
        <div className="flex items-center gap-2 border-t border-border-subtle pt-2.5">
          <button
            type="button"
            disabled={!value}
            onClick={() => pick(null)}
            className={cn('h-7 rounded-md px-2.5 text-[12px] font-medium', value ? 'text-ink-muted hover:bg-bg-muted' : 'text-border-strong')}
          >
            Clear
          </button>
          {pairText && <span className="ml-auto text-[12px] text-ink-muted tabular-nums">{pairText}</span>}
        </div>
      </PopoverContent>
    </Popover>
  )
}
