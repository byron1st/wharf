import { ChevronLeft, ChevronRight } from 'lucide-react'
import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { addDays, dayLabel, MONTHS, parseYmd, ymd } from '@/lib/dates'

const TONES = {
  teal: { sel: '#0f766e', band: '#e6f4f1', mid: '#115e59' },
  violet: { sel: '#7c3aed', band: '#f5f3ff', mid: '#5b21b6' },
}

export interface CalendarProps {
  today: string
  /** month is any date in the month shown first. */
  month: string
  selected: string | null
  /** other is the paired date, outlined, with the range between it and selected tinted. */
  other?: string | null
  /** lo and hi bound the days that can be picked, inclusive. */
  lo?: string | null
  hi?: string | null
  tone?: keyof typeof TONES
  onPick: (day: string) => void
}

/** Calendar is a Monday-first month grid; arrows move by day, PageUp/PageDown by month, Enter picks. */
export function Calendar({ today, month, selected, other, lo, hi, tone = 'teal', onPick }: CalendarProps) {
  const C = TONES[tone]
  const [shown, setShown] = useState(() => month.slice(0, 7) + '-01')
  const [focus, setFocus] = useState<string | null>(null)
  const grid = useRef<HTMLDivElement>(null)

  const first = parseYmd(shown)
  const lead = (first.getDay() + 6) % 7
  const dim = new Date(first.getFullYear(), first.getMonth() + 1, 0).getDate()
  const cells = Math.ceil((lead + dim) / 7) * 7
  const start = addDays(shown, -lead)
  const band =
    selected && other ? ([selected < other ? selected : other, selected < other ? other : selected] as const) : null
  const enabled = (d: string) => (!lo || d >= lo) && (!hi || d <= hi)

  useEffect(() => {
    if (focus) grid.current?.querySelector<HTMLButtonElement>(`[data-day="${focus}"]`)?.focus()
  }, [focus, shown])

  const move = (to: string) => {
    setFocus(to)
    if (to.slice(0, 7) !== shown.slice(0, 7)) setShown(to.slice(0, 7) + '-01')
  }
  const nav = (dl: number) => {
    const d = new Date(first.getFullYear(), first.getMonth() + dl, 1)
    setShown(ymd(d))
  }
  const onKey = (e: KeyboardEvent, day: string) => {
    const step = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7 }[e.key]
    if (step) {
      e.preventDefault()
      move(addDays(day, step))
    } else if (e.key === 'PageUp' || e.key === 'PageDown') {
      e.preventDefault()
      const d = parseYmd(day)
      d.setMonth(d.getMonth() + (e.key === 'PageUp' ? -1 : 1))
      move(ymd(d))
    }
  }

  const tabStop = focus ?? selected ?? today

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center gap-1.5">
        <span className="mr-auto pl-0.5 text-[14px] font-semibold">
          {MONTHS[first.getMonth()]} {first.getFullYear()}
        </span>
        {[
          { dl: -1, label: 'Previous month', Icon: ChevronLeft },
          { dl: 1, label: 'Next month', Icon: ChevronRight },
        ].map(({ dl, label, Icon }) => (
          <button
            key={dl}
            type="button"
            aria-label={label}
            onClick={() => nav(dl)}
            className="flex size-7 items-center justify-center rounded-md border border-[#e4e4e7] bg-white text-[#52525b] hover:bg-[#fafafa]"
          >
            <Icon size={14} />
          </button>
        ))}
      </div>
      <div ref={grid} role="grid" className="grid grid-cols-7 gap-y-[3px]">
        {['Mo', 'Tu', 'We', 'Th', 'Fr', 'Sa', 'Su'].map((d) => (
          <span key={d} className="h-[22px] text-center text-[11px] leading-[22px] text-[#a1a1aa]">
            {d}
          </span>
        ))}
        {Array.from({ length: cells }, (_, i) => {
          const day = addDays(start, i)
          const inMonth = day.slice(0, 7) === shown.slice(0, 7)
          const disabled = !inMonth || !enabled(day)
          const isSel = inMonth && day === selected
          const isOther = inMonth && !!other && day === other && !isSel
          const isToday = inMonth && day === today
          const inBand = inMonth && band && day >= band[0] && day <= band[1]
          const radius = inBand
            ? day === band[0] && day === band[1]
              ? '6px'
              : day === band[0]
                ? '6px 0 0 6px'
                : day === band[1]
                  ? '0 6px 6px 0'
                  : '0'
            : '0'
          return (
            <span
              key={day}
              className="flex justify-center"
              style={{ background: inBand ? C.band : 'transparent', borderRadius: radius }}
            >
              <button
                type="button"
                data-day={day}
                tabIndex={day === tabStop && inMonth ? 0 : -1}
                disabled={disabled}
                aria-label={`${dayLabel(day)}${isToday ? ', today' : ''}${isSel ? ', selected' : ''}`}
                aria-pressed={isSel}
                onClick={() => onPick(day)}
                onKeyDown={(e) => onKey(e, day)}
                className="h-8 w-[34px] rounded-md text-[13px] tabular-nums enabled:hover:bg-[#f4f4f5]"
                style={{
                  border: isOther
                    ? `1px solid ${C.sel}`
                    : isToday && !isSel
                      ? '1px solid #fdba74'
                      : '1px solid transparent',
                  background: isSel ? C.sel : undefined,
                  color: isSel ? '#fff' : disabled ? '#d4d4d8' : isToday ? '#c2410c' : isOther || inBand ? C.mid : '#09090b',
                  fontWeight: isSel || isOther || isToday ? 600 : 400,
                }}
              >
                {parseYmd(day).getDate()}
              </button>
            </span>
          )
        })}
      </div>
    </div>
  )
}
