import { ApiError, NetworkError, api } from '@/lib/api'
import { notify } from '@/lib/toast'
import { queryClient } from '@/lib/queryClient'

const SERVER_DOWN = 'server-down'
let polling: ReturnType<typeof setInterval> | undefined

/** serverDown shows the sticky "Can't reach the Wharf server" toast and closes it once /api/healthz answers. */
export function serverDown() {
  const retry = async () => {
    try {
      await api.healthz()
      serverBack()
    } catch {
      serverDown()
    }
  }
  notify.error({
    id: SERVER_DOWN,
    icon: 'server',
    title: "Can't reach the Wharf server",
    description: (
      <>
        <span className="font-mono">{location.host}</span> isn't responding. Changes won't be saved until it's back.
      </>
    ),
    actions: [{ label: 'Retry', primary: true, onClick: retry }],
  })
  polling ??= setInterval(() => {
    api.healthz().then(serverBack, () => {})
  }, 3000)
}

/**
 * waitForServer polls /api/healthz after the server answered a request that
 * restarts it, and resolves true once it answers again or false after 30 s.
 */
export async function waitForServer(): Promise<boolean> {
  const started = Date.now()
  while (Date.now() - started < 30_000) {
    await new Promise((r) => setTimeout(r, 800))
    try {
      await api.healthz()
      return true
    } catch {
      // Still restarting.
    }
  }
  return false
}

function serverBack() {
  clearInterval(polling)
  polling = undefined
  notify.dismiss(SERVER_DOWN)
  void queryClient.invalidateQueries()
}

/**
 * reportSaveError tells the user a write failed after its optimistic change
 * was undone: the server is down, the item is gone, or the save failed.
 */
export function reportSaveError(err: unknown, what: string, retry: () => void) {
  if (err instanceof NetworkError) {
    serverDown()
    return
  }
  if (err instanceof ApiError && err.status === 404) {
    notify.info({
      icon: 'gone',
      title: `That ${what} no longer exists`,
      description: 'It was removed elsewhere. The list has been refreshed.',
    })
    void queryClient.invalidateQueries()
    return
  }
  const detail = err instanceof ApiError ? `${err.status} ${err.message}` : String(err)
  notify.error({
    icon: 'undo',
    title: `Couldn't save the ${what}`,
    description: `Your change was undone. ${detail}`,
    actions: [{ label: 'Retry', primary: true, onClick: retry }],
  })
}
