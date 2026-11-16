import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
import { keys } from '@/hooks/data'
import { api, taskKey } from '@/lib/api'
import { dueReminders, reminderLabel } from '@/lib/dates'

/** canNotify reports whether the browser has the Notification API and the user allowed it. */
const canNotify = () => typeof Notification !== 'undefined' && Notification.permission === 'granted'

/** POLL_MS refetches open todos, so reminders changed on another device apply even in a background tab. */
const POLL_MS = 60_000

/** CATCH_UP_MS bounds how late a reminder may still notify, so waking from sleep doesn't replay every missed one. */
const CATCH_UP_MS = 5 * 60_000

/** requestNotificationPermission asks once for permission to notify; call it from a user gesture. */
export function requestNotificationPermission() {
  if (typeof Notification !== 'undefined' && Notification.permission === 'default') {
    void Notification.requestPermission()
  }
}

/**
 * useReminderNotifications shows a browser notification for each open todo's
 * reminder when it goes off while the app is open. It checks on every clock
 * tick, so a reminder fires within a minute boundary of its time; clicking the
 * notification focuses the app and calls onOpen with the todo's task ID; the
 * latest onOpen is used, so it may close over the current route. Reminders more than
 * CATCH_UP_MS late, such as those missed while the machine slept, are skipped.
 */
export function useReminderNotifications(now: Date, onOpen: (taskID: string) => void) {
  const open = useRef(onOpen)
  useEffect(() => {
    open.current = onOpen
  })
  const filter = { is_done: false }
  const { data: todos } = useQuery({
    queryKey: keys.todos(filter),
    queryFn: () => api.listTodos(filter),
    refetchInterval: POLL_MS,
    refetchIntervalInBackground: true,
  })
  const since = useRef(now)

  useEffect(() => {
    if (!todos) return
    const from = new Date(Math.max(since.current.getTime(), now.getTime() - CATCH_UP_MS))
    since.current = now
    if (!canNotify()) return
    for (const todo of todos) {
      for (const at of dueReminders(todo.reminders, from, now)) {
        // The tag makes Chrome show one notification even when several tabs fire it.
        let n: Notification
        try {
          n = new Notification(todo.description, {
            body: `${taskKey(todo.task_id)} · ${reminderLabel(at)}`,
            tag: `wharf-reminder-${todo.id}-${at}`,
          })
        } catch {
          // Some browsers (Chrome on Android) only notify from a service worker; skip rather than crash the app.
          return
        }
        n.onclick = () => {
          window.focus()
          open.current(todo.task_id)
          n.close()
        }
      }
    }
  }, [now, todos])
}
