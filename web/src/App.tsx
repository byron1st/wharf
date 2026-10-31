import { useEffect } from 'react'
import { useLocation, useNavigate } from 'react-router'
import { Toaster } from 'sonner'
import { SettingsDialog } from '@/components/settings/SettingsDialog'
import { TaskDialog } from '@/components/task/TaskDialog'
import { TodayPage } from '@/components/today/TodayPage'
import { ClockContext, useTicker } from '@/hooks/useNow'
import { SyncProvider } from '@/hooks/useSync'

/**
 * App is Today with modals over it: /task/:id opens a task, /settings/:tab
 * opens Settings. Closing a modal returns to the Today URL it was opened from.
 */
export function App() {
  const clock = useTicker()
  const location = useLocation()
  const navigate = useNavigate()
  const back = (location.state as { back?: string } | null)?.back ?? ''
  const close = () => navigate({ pathname: '/', search: back })

  const task = /^\/task\/(.+)$/.exec(location.pathname)
  const settings = /^\/settings(?:\/(general|atlassian))?\/?$/.exec(location.pathname)

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
            onClose={close}
            onOpen={(id) => navigate(`/task/${encodeURIComponent(id)}`, { state: { back } })}
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
