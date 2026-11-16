import { useEffect, useRef } from 'react'
import { useNavigate } from 'react-router'
import { useTodos } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { taskKey } from '@/lib/api'
import { dueReminders, reminderLabel } from '@/lib/dates'

/** canNotify reports whether the browser has the Notification API and the user allowed it. */
const canNotify = () => typeof Notification !== 'undefined' && Notification.permission === 'granted'

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
 * notification focuses the app on the todo's task.
 */
export function useReminderNotifications() {
  const { now } = useNow()
  const navigate = useNavigate()
  const { data: todos } = useTodos({ is_done: false })
  const since = useRef(now)

  useEffect(() => {
    if (!todos) return
    const from = since.current
    since.current = now
    if (!canNotify()) return
    for (const todo of todos) {
      for (const at of dueReminders(todo.reminders, from, now)) {
        // The tag makes Chrome show one notification even when several tabs fire it.
        const n = new Notification(todo.description, {
          body: `${taskKey(todo.task_id)} · ${reminderLabel(at)}`,
          tag: `wharf-reminder-${todo.id}-${at}`,
        })
        n.onclick = () => {
          window.focus()
          navigate(`/task/${encodeURIComponent(todo.task_id)}`)
          n.close()
        }
      }
    }
  }, [now, todos, navigate])
}
