import { useQueryClient } from '@tanstack/react-query'
import { createContext, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router'
import { keys, useAtlassianSettings, useLastSync } from '@/hooks/data'
import { ApiError, NetworkError, api, taskKey, type AtlassianSettings, type TaskSync } from '@/lib/api'
import { serverDown } from '@/lib/errors'
import { hhmm } from '@/lib/dates'
import { notify } from '@/lib/toast'
import { STATUS_LABEL } from '@/components/common/badges'

type SyncStatus = 'idle' | 'busy' | 'failed'

interface SyncState {
  status: SyncStatus
  run: () => void
}

const SyncContext = createContext<SyncState>({ status: 'idle', run: () => {} })

export const useSync = () => useContext(SyncContext)

const SYNC_TOAST = 'atlassian-sync'

/** failedKeys pulls the issue keys out of sync error messages, which carry "key=SEC-171". */
function failedKeys(errors: string[] = []): string[] {
  return [...new Set(errors.flatMap((e) => [...e.matchAll(/key=([A-Za-z][A-Za-z0-9]*-\d+)/g)].map((m) => m[1])))]
}

function siteHost(settings: AtlassianSettings | undefined): string {
  try {
    return new URL(settings?.site_url ?? '').host || 'Atlassian Cloud'
  } catch {
    return 'Atlassian Cloud'
  }
}

/** SyncProvider runs the Atlassian Cloud sync for the header button and Settings › Sync now. */
export function SyncProvider({ children }: { children: ReactNode }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const [status, setStatus] = useState<SyncStatus>('idle')
  const busy = useRef(false)
  const { data: atlassian } = useAtlassianSettings()
  const { data: last } = useLastSync()
  // seen is the synced_at of the last sync this page has shown data from;
  // undefined until the first read.
  const seen = useRef<string | null | undefined>(undefined)

  // A sync this page did not run, such as the server's automatic one or
  // another device's, changes the last sync: refetch everything else it may
  // have changed.
  useEffect(() => {
    if (last === undefined) return
    const at = last?.synced_at ?? null
    if (seen.current !== undefined && at !== seen.current && !busy.current) {
      void qc.invalidateQueries({ predicate: (q) => q.queryKey[0] !== keys.lastSync[0] })
    }
    seen.current = at
  }, [last, qc])

  function run() {
    if (busy.current) return
    busy.current = true
    setStatus('busy')
    notify.loading({
      id: SYNC_TOAST,
      title: 'Syncing Atlassian Cloud…',
      description: 'Fetching Jira issues and Service Desk requests.',
    })

    api.syncAtlassian().then(
      (r) => {
        busy.current = false
        setStatus('idle')
        seen.current = r.synced_at
        void qc.invalidateQueries()
        const failures = r.issue_failures + r.request_failures
        if (failures > 0) {
          const ks = failedKeys(r.errors)
          const which = ks.length ? `${ks.join(', ')} couldn't be read.` : `${failures} items couldn't be read.`
          notify.warn({
            id: SYNC_TOAST,
            title: 'Partly synced',
            description: `${r.issues - r.issue_failures} of ${r.issues} issues updated. ${which}`,
            actions: [{ label: 'Sync again', onClick: () => run() }],
          })
          return
        }
        const parts = [`${r.issues} issues updated`, `${r.open_requests} open requests`]
        if (r.closed_requests) parts.push(`${r.closed_requests} requests closed`)
        notify.success({
          id: SYNC_TOAST,
          title: 'Atlassian Cloud synced',
          meta: hhmm(new Date(r.synced_at)),
          description: parts.join(' · '),
        })
      },
      (err: unknown) => {
        busy.current = false
        setStatus('failed')
        if (err instanceof NetworkError) {
          notify.dismiss(SYNC_TOAST)
          serverDown()
          return
        }
        const openSettings = { label: 'Open settings', onClick: () => navigate('/settings/atlassian') }
        const retry = { label: 'Retry', primary: true, onClick: () => run() }
        const code = err instanceof ApiError ? err.code : undefined
        const msg = err instanceof Error ? err.message : String(err)
        if (code === 'atlassian_unauthorized') {
          const status = /\b403\b/.test(msg) ? '403 Forbidden' : '401 Unauthorized'
          notify.error({
            id: SYNC_TOAST,
            title: 'Atlassian Cloud sync failed',
            description: `${status}. Check the email and API key. Nothing was changed.`,
            actions: [openSettings, retry],
          })
        } else if (code === 'atlassian_unreachable') {
          notify.error({
            id: SYNC_TOAST,
            title: "Can't reach Atlassian Cloud",
            description: `${siteHost(atlassian)} didn't respond. Nothing was changed.`,
            actions: [retry],
          })
        } else if (code === 'atlassian_not_configured') {
          notify.error({
            id: SYNC_TOAST,
            title: 'Atlassian Cloud is not connected',
            description: 'Add your site URL, email and API key in Settings first.',
            actions: [openSettings],
          })
        } else {
          notify.error({
            id: SYNC_TOAST,
            title: 'Atlassian Cloud sync failed',
            description: `${msg} Nothing was changed.`,
            actions: [retry],
          })
        }
      },
    )
  }

  return <SyncContext.Provider value={{ status, run }}>{children}</SyncContext.Provider>
}

/** changeText turns the fields a single-issue sync changed into the toast line. */
function changeText(r: TaskSync): string {
  const out: string[] = []
  if (r.changes.includes('status')) out.push(`Status changed to ${STATUS_LABEL[r.task.status]}.`)
  if (r.changes.includes('title')) out.push('Title updated.')
  if (r.changes.includes('description')) out.push('Description updated.')
  if (r.changes.includes('start_date') || r.changes.includes('due_date')) out.push('Dates updated.')
  return out.join(' ')
}

/** useSyncTask pulls one Jira issue again, with the single-issue toasts. */
export function useSyncTask() {
  const qc = useQueryClient()
  return (id: string, jiraURL: string | null) => {
    const key = taskKey(id)
    const toastID = `sync-${id}`
    notify.loading({
      id: toastID,
      title: `Syncing ${key}…`,
      description: 'Pulling the latest title, status, dates and description.',
    })
    api.syncTask(id).then(
      (r) => {
        void qc.invalidateQueries({ queryKey: keys.task(id) })
        void qc.invalidateQueries({ queryKey: keys.tasks })
        if (r.changes.length === 0) {
          notify.success({
            id: toastID,
            icon: 'check',
            title: `${key} is up to date`,
            description: 'Nothing changed in Jira since the last sync.',
          })
        } else {
          notify.success({ id: toastID, title: `${key} synced`, meta: hhmm(new Date()), description: changeText(r) })
        }
      },
      (err: unknown) => {
        if (err instanceof NetworkError) {
          notify.dismiss(toastID)
          serverDown()
          return
        }
        if (err instanceof ApiError && err.code === 'jira_issue_not_found') {
          notify.warn({
            id: toastID,
            title: `${key} wasn't found in Jira`,
            description: 'It may have been deleted or moved. Wharf keeps the last synced copy.',
            actions: jiraURL ? [{ label: 'Open in Jira', onClick: () => window.open(jiraURL, '_blank', 'noopener') }] : [],
          })
          return
        }
        notify.error({
          id: toastID,
          title: `Couldn't sync ${key}`,
          description: err instanceof Error ? err.message : String(err),
        })
      },
    )
  }
}
