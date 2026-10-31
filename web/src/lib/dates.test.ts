import { describe, expect, it } from 'vitest'
import {
  atLocal,
  dueTag,
  isTime,
  isTodayTodo,
  quickPicks,
  reminderSummary,
  shortDay,
  whenPill,
} from './dates'

// The design's clock: Monday, October 26, 2026, 14:07 local time.
const today = '2026-10-26'
const now = atLocal(today, '14:07')

describe('whenPill', () => {
  it('counts late days from the end date', () => {
    expect(whenPill({ start_date: '2026-10-21', end_date: '2026-10-23' }, today)).toEqual({
      label: '3 days late',
      tone: 'over',
    })
    expect(whenPill({ start_date: null, end_date: '2026-10-25' }, today).label).toBe('1 day late')
  })

  it('marks the end date today and later ones', () => {
    expect(whenPill({ start_date: null, end_date: today }, today)).toEqual({ label: 'Due today', tone: 'today' })
    expect(whenPill({ start_date: '2026-10-26', end_date: '2026-10-28' }, today).label).toBe('By Oct 28')
  })

  it('falls back to the start date', () => {
    expect(whenPill({ start_date: today, end_date: null }, today).label).toBe('Starts today')
    expect(whenPill({ start_date: '2026-10-20', end_date: null }, today).label).toBe('Since Oct 20')
    expect(whenPill({ start_date: null, end_date: null }, today).label).toBe('No date')
  })
})

describe('dueTag', () => {
  it('tells overdue, due today and later apart', () => {
    expect(dueTag('2026-10-24', today, false)).toEqual({ label: 'Overdue', tone: 'over' })
    expect(dueTag(today, today, false)).toEqual({ label: 'Due today', tone: 'today' })
    expect(dueTag('2026-11-03', today, false).label).toBe('Due Nov 3')
    expect(dueTag(null, today, false).label).toBe('No due date')
  })

  it('does not call a finished task overdue', () => {
    expect(dueTag('2026-10-24', today, true).label).toBe('Due Oct 24')
  })
})

describe('isTodayTodo', () => {
  it('keeps open todos that can start by today', () => {
    expect(isTodayTodo({ is_done: false, start_date: null }, today)).toBe(true)
    expect(isTodayTodo({ is_done: false, start_date: today }, today)).toBe(true)
    expect(isTodayTodo({ is_done: false, start_date: '2026-10-28' }, today)).toBe(false)
    expect(isTodayTodo({ is_done: true, start_date: null }, today)).toBe(false)
  })
})

describe('reminderSummary', () => {
  const at = (day: string, time: string) => atLocal(day, time).toISOString()

  it('shows the next upcoming reminder and how many more follow', () => {
    expect(reminderSummary([at('2026-10-29', '09:00'), at('2026-10-27', '10:00')], now)).toMatchObject({
      label: 'Tue 10:00 +1',
      upcoming: true,
    })
    expect(reminderSummary([at(today, '16:30')], now)?.label).toBe('Today 16:30')
  })

  it('shows the last sent one when none are left', () => {
    expect(reminderSummary([at(today, '09:00'), at(today, '11:00')], now)).toMatchObject({
      label: 'Sent 11:00',
      upcoming: false,
      sent: 2,
    })
  })

  it('is empty without reminders', () => {
    expect(reminderSummary([], now)).toBeNull()
  })
})

describe('shortDay', () => {
  it('names days within the coming week and dates beyond', () => {
    expect(shortDay(today, today)).toBe('Today')
    expect(shortDay('2026-10-27', today)).toBe('Tue')
    expect(shortDay('2026-11-02', today)).toBe('Nov 2')
    expect(shortDay('2026-10-20', today)).toBe('Oct 20')
  })
})

describe('quickPicks', () => {
  it('offers today, tomorrow, the coming Friday and next Monday', () => {
    expect(quickPicks(today).map((q) => `${q.label}=${q.value}`)).toEqual([
      'Today=2026-10-26',
      'Tomorrow=2026-10-27',
      'Fri=2026-10-30',
      'Next Mon=2026-11-02',
    ])
  })

  it('skips to the next week on a Friday', () => {
    expect(quickPicks('2026-10-30')[2]).toEqual({ label: 'Fri', value: '2026-11-06' })
  })
})

describe('isTime', () => {
  it('takes 24-hour HH:MM only', () => {
    expect(isTime('09:30')).toBe(true)
    expect(isTime('23:59')).toBe(true)
    expect(isTime('9:30')).toBe(false)
    expect(isTime('24:00')).toBe(false)
  })
})
