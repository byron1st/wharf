// Date helpers. Calendar dates are "YYYY-MM-DD" strings in local time; todo
// dates are stored that way, and task dates are stored as UTC midnight.

import type { TaskStatus } from './api'

const DOW = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']
const DOW_LONG = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']
const MON = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec']
export const MONTHS = [
  'January',
  'February',
  'March',
  'April',
  'May',
  'June',
  'July',
  'August',
  'September',
  'October',
  'November',
  'December',
]

const pad = (n: number) => String(n).padStart(2, '0')

/** ymd formats d as a local calendar date. */
export function ymd(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** parseYmd returns local midnight of a calendar date. */
export function parseYmd(s: string): Date {
  const [y, m, d] = s.split('-').map(Number)
  return new Date(y, m - 1, d)
}

export function addDays(s: string, n: number): string {
  const d = parseYmd(s)
  d.setDate(d.getDate() + n)
  return ymd(d)
}

/** diffDays is b − a in whole days. */
export function diffDays(a: string, b: string): number {
  return Math.round((parseYmd(b).getTime() - parseYmd(a).getTime()) / 864e5)
}

/** fromTaskDate reads a task date (RFC3339 at UTC midnight) as a calendar date. */
export function fromTaskDate(v: string | null): string | null {
  return v ? v.slice(0, 10) : null
}

/** toTaskDate writes a calendar date as a task date. */
export function toTaskDate(v: string | null): string | null {
  return v ? `${v}T00:00:00Z` : null
}

export const hhmm = (d: Date) => `${pad(d.getHours())}:${pad(d.getMinutes())}`
export const dow = (s: string) => DOW[parseYmd(s).getDay()]
/** "Oct 28" */
export const monthDay = (s: string) => {
  const d = parseYmd(s)
  return `${MON[d.getMonth()]} ${d.getDate()}`
}
/** "Mon, Oct 26" */
export const dayLabel = (s: string) => `${dow(s)}, ${monthDay(s)}`
/** "Oct 30, 2026" */
export const longDate = (s: string) => `${monthDay(s)}, ${parseYmd(s).getFullYear()}`
/** "Monday, October 26" */
export const headerDate = (d: Date) => `${DOW_LONG[d.getDay()]}, ${MONTHS[d.getMonth()]} ${d.getDate()}`

/** shortDay is "Today", a weekday within the next six days, or "Oct 30". */
export function shortDay(s: string, today: string): string {
  const n = diffDays(today, s)
  if (n === 0) return 'Today'
  if (n > 0 && n < 7) return dow(s)
  return monthDay(s)
}

/** relDays is "today", "in 4 days" or "2 days ago". */
export function relDays(s: string, today: string): string {
  const n = diffDays(today, s)
  if (n === 0) return 'today'
  if (n === 1) return 'tomorrow'
  if (n === -1) return 'yesterday'
  return n > 0 ? `in ${n} days` : `${-n} days ago`
}

/** ageText is "today", "1 day" or "5 days" since s. */
export function ageText(s: string, today: string): string {
  const n = diffDays(s, today)
  if (n <= 0) return 'today'
  return n === 1 ? '1 day' : `${n} days`
}

export type Tone = 'over' | 'today' | 'plain'

export const toneClass: Record<Tone, string> = {
  over: 'text-danger-ink bg-danger-tint',
  today: 'text-[#9a3412] bg-today-tint',
  plain: 'text-ink-secondary bg-bg-muted',
}

/** whenPill is the When column of a todo row, from its end date or else its start date. */
export function whenPill(t: { start_date: string | null; end_date: string | null }, today: string) {
  if (t.end_date) {
    const n = diffDays(t.end_date, today)
    if (n > 0) return { label: n === 1 ? '1 day late' : `${n} days late`, tone: 'over' as Tone }
    if (n === 0) return { label: 'Due today', tone: 'today' as Tone }
    return { label: `By ${monthDay(t.end_date)}`, tone: 'plain' as Tone }
  }
  if (t.start_date === today) return { label: 'Starts today', tone: 'plain' as Tone }
  if (t.start_date && t.start_date < today) return { label: `Since ${monthDay(t.start_date)}`, tone: 'plain' as Tone }
  return { label: 'No date', tone: 'plain' as Tone }
}

/** dueTag is the due tag of a task group. */
export function dueTag(due: string | null, today: string, done: boolean) {
  if (!due) return { label: 'No due date', tone: 'plain' as Tone }
  if (due < today && !done) return { label: 'Overdue', tone: 'over' as Tone }
  if (due === today) return { label: 'Due today', tone: 'today' as Tone }
  return { label: `Due ${monthDay(due)}`, tone: 'plain' as Tone }
}

/** doneLabel is the When column of a done todo: "Done today" or "Done Oct 22", from when it was completed. */
export function doneLabel(completedAt: string | null, today: string) {
  const day = completedAt ? ymd(new Date(completedAt)) : today
  return day === today ? { label: 'Done today', today: true } : { label: `Done ${monthDay(day)}`, today: false }
}

/** compareTodayTasks orders Today's task groups: by stored position, then unpositioned ones by due date, undated last, then by ID. */
export function compareTodayTasks(
  a: { id: string; position: number | null; due_date: string | null },
  b: { id: string; position: number | null; due_date: string | null },
): number {
  if (a.position !== b.position) {
    if (a.position === null) return 1
    if (b.position === null) return -1
    return a.position - b.position
  }
  const da = fromTaskDate(a.due_date) ?? '9999'
  const db = fromTaskDate(b.due_date) ?? '9999'
  return da === db ? a.id.localeCompare(b.id) : da < db ? -1 : 1
}

/**
 * reorderedIDs is every task's ID in Today's order after moving id right
 * before the task before, or last when before is null or no longer listed.
 * It covers hidden tasks too, so a task that comes back to Today returns to
 * its place. It is null when nothing would move.
 */
export function reorderedIDs(
  tasks: { id: string; position: number | null; due_date: string | null }[],
  id: string,
  before: string | null,
): string[] | null {
  const order = [...tasks].sort(compareTodayTasks).map((t) => t.id)
  const next = order.filter((x) => x !== id)
  const at = before === null ? -1 : next.indexOf(before)
  next.splice(at < 0 ? next.length : at, 0, id)
  return next.some((x, i) => x !== order[i]) ? next : null
}

/** isTodayTodo picks the todos Today lists: open ones that can start by today. */
export function isTodayTodo(t: { is_done: boolean; start_date: string | null }, today: string): boolean {
  return !t.is_done && (!t.start_date || t.start_date <= today)
}

/** isTodayTask is whether Today lists a task: any open one, and a done or canceled one only while it has todos for today. */
export function isTodayTask(task: { status: TaskStatus }, hasTodayTodos: boolean): boolean {
  return hasTodayTodos || !isClosedStatus(task.status)
}

/** isClosedStatus is whether a task status is done or canceled. */
export const isClosedStatus = (s: TaskStatus) => s === 'done' || s === 'canceled'

/** isSent reports whether a reminder already went off; wharf keeps no sent flag. */
export const isSent = (at: string, now: Date) => new Date(at).getTime() <= now.getTime()

/** sortReminders orders reminder instants by time, whatever their UTC offsets. */
export const sortReminders = (reminders: string[]) =>
  [...reminders].sort((a, b) => Date.parse(a) - Date.parse(b))

/** reminderSummary is the Reminder column: the next upcoming one, else the last sent one. */
export function reminderSummary(reminders: string[], now: Date) {
  const sorted = sortReminders(reminders)
  const upcoming = sorted.filter((r) => !isSent(r, now))
  const sent = sorted.filter((r) => isSent(r, now))
  const today = ymd(now)
  if (upcoming.length) {
    const d = new Date(upcoming[0])
    const more = upcoming.length > 1 ? ` +${upcoming.length - 1}` : ''
    return { label: `${shortDay(ymd(d), today)} ${hhmm(d)}${more}`, upcoming: true, sent: sent.length }
  }
  if (sent.length) return { label: `Sent ${hhmm(new Date(sent[sent.length - 1]))}`, upcoming: false, sent: sent.length }
  return null
}

/** reminderLabel is "Mon, Oct 26 · 11:00". */
export function reminderLabel(at: string): string {
  const d = new Date(at)
  return `${dayLabel(ymd(d))} · ${hhmm(d)}`
}

/** atLocal is the instant of a local calendar date and HH:MM time. */
export function atLocal(day: string, time: string): Date {
  const d = parseYmd(day)
  const [h, m] = time.split(':').map(Number)
  d.setHours(h, m, 0, 0)
  return d
}

export const isTime = (t: string) => /^([01]\d|2[0-3]):[0-5]\d$/.test(t)

/** quickPicks are the date picker chips: Today, Tomorrow, the coming Friday, and next Monday. */
export function quickPicks(today: string) {
  const day = parseYmd(today).getDay()
  const fri = addDays(today, (5 - day + 7) % 7 || 7)
  const mon = addDays(today, (1 - day + 7) % 7 || 7)
  return [
    { label: 'Today', value: today },
    { label: 'Tomorrow', value: addDays(today, 1) },
    { label: dow(fri), value: fri },
    { label: 'Next Mon', value: mon },
  ]
}

/** startOfToday is local midnight as RFC3339, for the completed_since filter. */
export function startOfToday(now: Date): string {
  const d = new Date(now)
  d.setHours(0, 0, 0, 0)
  return d.toISOString()
}

export type ClosedGroup = 'this-week' | 'last-week' | 'earlier'

/** weekStart is the Monday of the week of a calendar date; weeks start on Monday. */
export function weekStart(s: string): string {
  return addDays(s, -((parseYmd(s).getDay() + 6) % 7))
}

/** closedGroup puts a closing time in this week, last week, or earlier; an unknown time is earlier. */
export function closedGroup(closedAt: string | null, today: string): ClosedGroup {
  if (!closedAt) return 'earlier'
  const day = ymd(new Date(closedAt))
  const monday = weekStart(today)
  if (day >= monday) return 'this-week'
  if (day >= addDays(monday, -7)) return 'last-week'
  return 'earlier'
}

/** closedLabel is "Closed today", "Closed yesterday" or "Closed Oct 23", plus " in Jira"; just "Closed" when unknown. */
export function closedLabel(closedAt: string | null, today: string, jira: boolean): string {
  const where = jira ? ' in Jira' : ''
  if (!closedAt) return `Closed${where}`
  const day = ymd(new Date(closedAt))
  const n = diffDays(day, today)
  const when = n === 0 ? 'today' : n === 1 ? 'yesterday' : monthDay(day)
  return `Closed ${when}${where}`
}
