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
            'flex h-8 w-full min-w-0 items-center gap-2 rounded-md border bg-white px-2.5 text-left text-[13px] shadow-card',
            tone === 'over'
              ? 'border-[#fecaca] bg-[#fef2f2] text-[#b91c1c]'
              : tone === 'today'
                ? 'border-[#fed7aa] bg-[#fff7ed] text-[#9a3412]'
                : 'border-[#e4e4e7]',
            open && 'border-primary shadow-ring',
            !value && 'text-[#a1a1aa]',
            className,
          )}
        >
          {showIcon && <CalendarDays size={14} className="shrink-0 opacity-70" />}
          <span className="min-w-0 flex-1 truncate tabular-nums">{value ? format(value) : 'Not set'}</span>
          <ChevronDown size={14} className="shrink-0 text-[#a1a1aa]" />
        </button>
      </PopoverTrigger>
      <PopoverContent align={align} aria-label={title} className="flex flex-col gap-3">
        <span className="text-[12px] font-medium text-[#71717a]">{title}</span>
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
                  on ? 'border-primary bg-primary-subtle font-semibold text-[#115e59]' : 'border-[#e4e4e7] text-[#3f3f46]',
                  disabled && 'text-[#d4d4d8]',
                )}
              >
                {q.label}
              </button>
            )
          })}
        </div>
        <Calendar today={today} month={value ?? other ?? today} selected={value} other={other} lo={lo} hi={hi} onPick={pick} />
        <div className="flex items-center gap-2 border-t border-[#f4f4f5] pt-2.5">
          <button
            type="button"
            disabled={!value}
            onClick={() => pick(null)}
            className={cn('h-7 rounded-md px-2.5 text-[12px] font-medium', value ? 'text-[#71717a] hover:bg-[#f4f4f5]' : 'text-[#d4d4d8]')}
          >
            Clear
          </button>
          {pairText && <span className="ml-auto text-[12px] text-[#71717a] tabular-nums">{pairText}</span>}
        </div>
      </PopoverContent>
    </Popover>
  )
}
