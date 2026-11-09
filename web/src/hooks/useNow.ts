import { createContext, useContext, useEffect, useState } from 'react'
import { ymd } from '@/lib/dates'

export interface Clock {
  now: Date
  /** today is the local calendar date of now. */
  today: string
}

export const ClockContext = createContext<Clock>({ now: new Date(), today: ymd(new Date()) })

/** useTicker returns a clock that ticks on every minute boundary. */
export function useTicker(): Clock {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    let timer: ReturnType<typeof setTimeout>
    const tick = () => {
      const d = new Date()
      setNow(d)
      timer = setTimeout(tick, 60_000 - (d.getSeconds() * 1000 + d.getMilliseconds()) + 50)
    }
    tick()
    return () => clearTimeout(timer)
  }, [])
  return { now, today: ymd(now) }
}

/** useNow reads the shared clock, so the whole app agrees on what today is. */
export function useNow(): Clock {
  return useContext(ClockContext)
}
