import { ArrowUpCircle, ExternalLink } from 'lucide-react'
import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { useUpdateStatus } from '@/hooks/data'
import { ApiError, NetworkError, api } from '@/lib/api'
import { serverDown, waitForServer } from '@/lib/errors'
import { notify } from '@/lib/toast'

/**
 * UpdateBanner offers a newer Wharf release once the server has found one.
 * Upgrading replaces the server binary, which then restarts; the page reloads
 * so the new web app is loaded too.
 */
export function UpdateBanner() {
  const { data: update } = useUpdateStatus()
  const [upgrading, setUpgrading] = useState(false)

  if (!update?.available || !update.latest) return null
  const latest = update.latest

  const upgrade = async () => {
    setUpgrading(true)
    notify.loading({ id: 'update', title: `Upgrading Wharf to ${latest}…`, description: 'The server restarts when it is done.' })
    try {
      await api.applyUpdate()
    } catch (err) {
      setUpgrading(false)
      notify.dismiss('update')
      if (err instanceof NetworkError) return serverDown()
      notify.error({
        title: "Couldn't upgrade Wharf",
        description:
          err instanceof ApiError && err.code === 'invalid_release'
            ? 'The downloaded release did not pass verification. Nothing was changed.'
            : err instanceof Error
              ? err.message
              : String(err),
      })
      return
    }
    if (await waitForServer()) {
      window.location.reload()
    } else {
      setUpgrading(false)
      notify.dismiss('update')
      serverDown()
    }
  }

  return (
    <div
      role="status"
      className="flex items-center gap-3 rounded-lg border border-[#bfdbfe] bg-tag-blue-bg px-3.5 py-2.5 text-[13px] text-[#1e3a8a]"
    >
      <ArrowUpCircle size={15} className="shrink-0 text-tag-blue-solid" />
      <span className="flex-1">
        Wharf <span className="font-medium">{latest}</span> is available. You have {update.current}.
      </span>
      {update.release_url && (
        <a
          href={update.release_url}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 text-[13px] text-tag-blue-ink underline-offset-2 hover:underline"
        >
          Release notes
          <ExternalLink size={13} />
        </a>
      )}
      <Button size="sm" disabled={upgrading} onClick={() => void upgrade()}>
        {upgrading ? 'Upgrading…' : 'Upgrade'}
      </Button>
    </div>
  )
}
