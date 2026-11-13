import { useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router'
import { Toaster } from 'sonner'
import { ClosedTasksDialog, NO_CLOSED_FILTERS } from '@/components/closed/ClosedTasksDialog'
import { SettingsDialog } from '@/components/settings/SettingsDialog'
import { TaskDialog } from '@/components/task/TaskDialog'
import { TodayPage } from '@/components/today/TodayPage'
import { ClockContext, useTicker } from '@/hooks/useNow'
import { SyncProvider } from '@/hooks/useSync'

/** NavState is what a modal URL carries: the Today search to return to, and whether a task was opened from Closed tasks. */
interface NavState {
  back?: string
  fromClosed?: boolean
}

/**
 * App is Today with modals over it: /task/:id opens a task, /settings/:tab
 * opens Settings, /closed opens Closed tasks. Closing a modal returns to the
 * Today URL it was opened from, or to Closed tasks for a task opened there.
 */
export function App() {
  const clock = useTicker()
  const location = useLocation()
  const navigate = useNavigate()
  const { back = '', fromClosed = false } = (location.state as NavState | null) ?? {}
  const close = () => navigate({ pathname: '/', search: back })
  // Kept here so the search and filters survive a visit to a task opened from the list.
  const [closedFilters, setClosedFilters] = useState(NO_CLOSED_FILTERS)

  const task = /^\/task\/(.+)$/.exec(location.pathname)
  const settings = /^\/settings(?:\/(general|atlassian))?\/?$/.exec(location.pathname)
  const closed = /^\/closed\/?$/.test(location.pathname)

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === ',') {
        e.preventDefault()
        navigate('/settings/general', { state: { back: location.search } })
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [navigate, location.search])

  return (
    <ClockContext.Provider value={clock}>
      <SyncProvider>
        <TodayPage />
        {task && (
          <TaskDialog
            id={decodeURIComponent(task[1])}
            onClose={fromClosed ? () => navigate('/closed', { state: { back } }) : close}
            onOpen={(id) => navigate(`/task/${encodeURIComponent(id)}`, { state: { back, fromClosed } })}
          />
        )}
        {closed && (
          <ClosedTasksDialog
            filters={closedFilters}
            onFilters={setClosedFilters}
            onClose={close}
            onOpen={(id) => navigate(`/task/${encodeURIComponent(id)}`, { state: { back, fromClosed: true } })}
          />
        )}
        {settings && (
          <SettingsDialog
            tab={settings[1] === 'atlassian' ? 'atlassian' : 'general'}
            onTab={(tab) => navigate(`/settings/${tab}`, { replace: true, state: { back } })}
            onClose={close}
          />
        )}
        <Toaster position="bottom-right" visibleToasts={3} gap={10} toastOptions={{ unstyled: true }} />
      </SyncProvider>
    </ClockContext.Provider>
  )
}
