import { describe, expect, it } from 'vitest'
import {
  atLocal,
  closedGroup,
  closedLabel,
  compareTodayTasks,
  doneLabel,
  reorderedIDs,
  dueTag,
  isTime,
  isTodayTask,
  isTodayTodo,
  quickPicks,
  reminderSummary,
  shortDay,
  sortReminders,
  weekStart,
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

describe('isTodayTask', () => {
  it('lists open tasks with or without todos for today', () => {
    expect(isTodayTask({ status: 'todo' }, false)).toBe(true)
    expect(isTodayTask({ status: 'in-progress' }, false)).toBe(true)
    expect(isTodayTask({ status: 'in-progress' }, true)).toBe(true)
  })

  it('lists done and canceled tasks only while they have todos for today', () => {
    expect(isTodayTask({ status: 'done' }, false)).toBe(false)
    expect(isTodayTask({ status: 'canceled' }, false)).toBe(false)
    expect(isTodayTask({ status: 'done' }, true)).toBe(true)
    expect(isTodayTask({ status: 'canceled' }, true)).toBe(true)
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

  it('orders reminders by instant, not by text, across UTC offsets', () => {
    // 10:00 KST is 01:00Z, earlier than 05:00Z although "...T10" sorts after "...T05" as text.
    const kst = '2026-10-27T10:00:00+09:00'
    const utc = '2026-10-27T05:00:00Z'
    expect(sortReminders([utc, kst])).toEqual([kst, utc])
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

describe('doneLabel', () => {
  it('says today for a todo completed today in local time, else the day', () => {
    expect(doneLabel(atLocal(today, '00:05').toISOString(), today)).toEqual({ label: 'Done today', today: true })
    expect(doneLabel(atLocal('2026-10-22', '23:55').toISOString(), today)).toEqual({ label: 'Done Oct 22', today: false })
  })

  it('treats a todo just checked, without a completion time yet, as done today', () => {
    expect(doneLabel(null, today).label).toBe('Done today')
  })
})

describe('compareTodayTasks', () => {
  const t = (id: string, position: number | null, due: string | null = null) => ({
    id,
    position,
    due_date: due && `${due}T00:00:00Z`,
  })

  it('puts positioned tasks first in position order, then the rest by due date and ID', () => {
    const tasks = [
      t('jira-C', null, '2026-10-28'),
      t('jira-A', null),
      t('local-B', 1),
      t('jira-D', null, '2026-10-27'),
      t('jira-E', 0, '2026-11-30'),
      t('jira-B', null),
    ]
    expect(tasks.sort(compareTodayTasks).map((x) => x.id)).toEqual(['jira-E', 'local-B', 'jira-D', 'jira-C', 'jira-A', 'jira-B'])
  })
})

describe('reorderedIDs', () => {
  // B is hidden from Today (say, done), but it still holds its place.
  const tasks = [
    { id: 'A', position: 0, due_date: null },
    { id: 'B', position: 1, due_date: null },
    { id: 'C', position: 2, due_date: null },
    { id: 'D', position: null, due_date: null },
  ]

  it('moves a task up before another', () => {
    expect(reorderedIDs(tasks, 'C', 'A')).toEqual(['C', 'A', 'B', 'D'])
  })

  it('moves a task down past a hidden one', () => {
    expect(reorderedIDs(tasks, 'A', 'D')).toEqual(['B', 'C', 'A', 'D'])
  })

  it('moves a task to the end, also when its target is gone', () => {
    expect(reorderedIDs(tasks, 'A', null)).toEqual(['B', 'C', 'D', 'A'])
    expect(reorderedIDs(tasks, 'A', 'GONE')).toEqual(['B', 'C', 'D', 'A'])
  })

  it('is null when nothing would move', () => {
    expect(reorderedIDs(tasks, 'B', 'C')).toBeNull()
    expect(reorderedIDs(tasks, 'D', null)).toBeNull()
  })
})

describe('closed tasks', () => {
  // Local noon, so the calendar day does not depend on the test's time zone.
  const at = (day: string) => atLocal(day, '12:00').toISOString()

  it('starts weeks on Monday', () => {
    expect(weekStart('2026-10-26')).toBe('2026-10-26')
    expect(weekStart('2026-10-29')).toBe('2026-10-26')
    expect(weekStart('2026-11-01')).toBe('2026-10-26')
  })

  it('groups by the week of the closing day', () => {
    expect(closedGroup(at('2026-10-26'), '2026-10-28')).toBe('this-week')
    expect(closedGroup(at('2026-10-25'), '2026-10-28')).toBe('last-week')
    expect(closedGroup(at('2026-10-19'), '2026-10-28')).toBe('last-week')
    expect(closedGroup(at('2026-10-18'), '2026-10-28')).toBe('earlier')
    expect(closedGroup(null, '2026-10-28')).toBe('earlier')
    // On a Sunday, the Monday six days before is still this week.
    expect(closedGroup(at('2026-10-26'), '2026-11-01')).toBe('this-week')
  })

  it('says when a task closed, and where for Jira', () => {
    expect(closedLabel(at(today), today, false)).toBe('Closed today')
    expect(closedLabel(at('2026-10-25'), today, false)).toBe('Closed yesterday')
    expect(closedLabel(at('2026-10-23'), today, false)).toBe('Closed Oct 23')
    expect(closedLabel(at('2026-10-19'), today, true)).toBe('Closed Oct 19 in Jira')
    expect(closedLabel(null, today, false)).toBe('Closed')
    expect(closedLabel(null, today, true)).toBe('Closed in Jira')
  })
})
