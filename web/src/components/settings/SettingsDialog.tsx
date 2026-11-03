import { useQueryClient } from '@tanstack/react-query'
import { Dialog as D } from 'radix-ui'
import {
  Check,
  CircleAlert,
  CircleCheck,
  Diamond,
  ExternalLink,
  Eye,
  EyeOff,
  File,
  FileText,
  Folder,
  FolderOpen,
  GitBranch,
  Link2,
  Lock,
  RefreshCw,
  SlidersHorizontal,
  Ticket,
  X,
} from 'lucide-react'
import { useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { keys, useAppSettings, useAtlassianSettings, useLastSync } from '@/hooks/data'
import { useNow } from '@/hooks/useNow'
import { useSync } from '@/hooks/useSync'
import { ApiError, NetworkError, api, type AtlassianSettings } from '@/lib/api'
import { serverDown } from '@/lib/errors'
import { hhmm, ymd } from '@/lib/dates'
import { notify } from '@/lib/toast'
import { cn } from '@/lib/utils'

type Tab = 'general' | 'atlassian'

/** showError reports a failed settings call: the server is down, or what it said. */
function showError(title: string, err: unknown) {
  if (err instanceof NetworkError) return serverDown()
  notify.error({ title, description: err instanceof Error ? err.message : String(err) })
}

/** tilde shortens a home directory path for display. */
const tilde = (p: string) => p.replace(/^\/(Users|home)\/[^/]+/, '~')

function General() {
  const qc = useQueryClient()
  const { data: app } = useAppSettings()
  const [restarting, setRestarting] = useState(false)

  const call = async (title: string, fn: () => Promise<unknown>) => {
    try {
      await fn()
    } catch (err) {
      if (err instanceof ApiError && err.code === 'unsupported_os') {
        notify.info({ title, description: 'This only works when the Wharf server runs on macOS.' })
      } else {
        showError(title, err)
      }
    } finally {
      void qc.invalidateQueries({ queryKey: keys.app })
    }
  }

  const restart = async () => {
    setRestarting(true)
    notify.loading({ id: 'restart', title: 'Restarting the Wharf server…', description: 'It loads the new config file.' })
    try {
      await api.restart()
    } catch (err) {
      setRestarting(false)
      notify.dismiss('restart')
      if (err instanceof ApiError && err.code === 'invalid_config_file') {
        notify.error({ title: "Can't use that config file", description: err.message })
      } else {
        showError("Couldn't restart the server", err)
      }
      return
    }
    // The server answers 202 and then restarts; wait until it answers again.
    const started = Date.now()
    const poll = async () => {
      await new Promise((r) => setTimeout(r, 800))
      try {
        await api.healthz()
        setRestarting(false)
        notify.success({ id: 'restart', title: 'Server restarted', description: 'It now reads the new config file.' })
        void qc.invalidateQueries()
      } catch {
        if (Date.now() - started < 30_000) return poll()
        setRestarting(false)
        notify.dismiss('restart')
        serverDown()
      }
    }
    void poll()
  }

  if (!app) return null
  const pending = app.pending_config_path
  const port = app.addr.split(':').pop()
  return (
    <section className="flex flex-col gap-3">
      <h3 className="m-0 text-[15px] font-semibold">App Server</h3>
      <div className="flex flex-col gap-1.5">
        <span className="text-[13px] font-medium">Config file</span>
        <div className="flex gap-2">
          <span
            className={cn(
              'flex h-9 min-w-0 flex-1 items-center gap-2 rounded-md border bg-[#fafafa] px-3 font-mono text-[13px]',
              pending ? 'border-[#fcd34d] bg-[#fffbeb]' : 'border-[#e4e4e7]',
            )}
            title={pending ?? app.config_path}
          >
            <File size={14} className="shrink-0 text-[#71717a]" />
            <span className="truncate">{tilde(pending ?? app.config_path)}</span>
          </span>
          <Button variant="outline" onClick={() => void call("Couldn't open the file chooser", api.chooseConfig)}>
            <Folder size={15} />
            Choose…
          </Button>
          <Button variant="outline" onClick={() => void call("Couldn't open the config file", api.openConfig)}>
            <ExternalLink size={15} />
            Open
          </Button>
        </div>
        <span className="text-[12px] text-[#71717a]">
          A <span className="font-mono">.toml</span> file. Port and database below are read from it — edit the file to change
          them.
        </span>
      </div>
      <span className="mt-2 flex items-center gap-1.5 text-[12px] font-medium text-[#52525b]">
        <Lock size={12} />
        From config.toml · read-only
      </span>
      <div className="overflow-hidden rounded-lg border border-[#e4e4e7]">
        <div className="grid min-h-10 grid-cols-[108px_minmax(0,1fr)] items-center border-b border-[#f4f4f5] px-3.5 text-[13px]">
          <span className="text-[#71717a]">Port</span>
          <span>
            <span className="font-mono">{port}</span>
            <span className="text-[12px] text-[#71717a]"> · http://localhost:{port}</span>
          </span>
        </div>
        <div className="grid min-h-10 grid-cols-[108px_minmax(0,1fr)_auto] items-center gap-2 px-3.5 text-[13px]">
          <span className="text-[#71717a]">Database</span>
          <span className="truncate font-mono" title={app.db_path}>
            {tilde(app.db_path)}
          </span>
          <span className="rounded-md bg-[#f4f4f5] px-2 py-0.5 text-[12px]">SQLite</span>
        </div>
      </div>
      {pending && (
        <div role="status" className="mt-3 flex items-center gap-3 rounded-lg border border-[#fde68a] bg-[#fffbeb] px-3.5 py-2.5 text-[13px] text-[#92400e]">
          <CircleAlert size={14} className="shrink-0 text-[#d97706]" />
          <span className="flex-1">The server is still using the previous file. Restart it to load the new config.</span>
          <Button
            variant="outline"
            size="sm"
            className="border-[#fcd34d] text-[#78350f]"
            disabled={restarting}
            onClick={() => void call("Couldn't undo the change", api.clearPendingConfig)}
          >
            Undo
          </Button>
          <Button size="sm" disabled={restarting} onClick={() => void restart()}>
            Restart server
          </Button>
        </div>
      )}
    </section>
  )
}

type Test = { state: 'idle' } | { state: 'testing' } | { state: 'ok'; at: string } | { state: 'failed'; message: string }

function Field({ label, hint, children }: { label: string; hint: string; children: ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-[13px] font-medium">{label}</span>
      {children}
      <span className="text-[12px] text-[#71717a]">{hint}</span>
    </label>
  )
}

const input =
  'h-9 w-full rounded-md border border-[#e4e4e7] px-3 text-[14px] outline-none placeholder:text-[#a1a1aa] focus:border-primary focus:shadow-ring'

function Atlassian() {
  const { data: stored } = useAtlassianSettings()
  return stored ? <AtlassianForm stored={stored} /> : null
}

function AtlassianForm({ stored }: { stored: AtlassianSettings }) {
  const qc = useQueryClient()
  const { today } = useNow()
  const { data: last } = useLastSync()
  const { status: syncStatus, run } = useSync()
  const [form, setForm] = useState<AtlassianSettings>(stored)
  const [showKey, setShowKey] = useState(false)
  const [test, setTest] = useState<Test>({ state: 'idle' })
  const [saving, setSaving] = useState(false)

  const dirty = (form.site_url !== stored.site_url || form.email !== stored.email || form.api_token !== stored.api_token)
  const set = (patch: Partial<AtlassianSettings>) => {
    setForm((f) => ({ ...f, ...patch }))
    setTest({ state: 'idle' })
  }

  const runTest = async () => {
    setTest({ state: 'testing' })
    try {
      const r = await api.testAtlassianSettings(form)
      setTest({ state: 'ok', at: hhmm(new Date(r.checked_at)) })
    } catch (err) {
      if (err instanceof NetworkError) {
        setTest({ state: 'idle' })
        serverDown()
        return
      }
      const code = err instanceof ApiError ? err.code : undefined
      setTest({
        state: 'failed',
        message:
          code === 'atlassian_unauthorized'
            ? 'Atlassian rejected the email or API key.'
            : code === 'atlassian_unreachable'
              ? "Atlassian Cloud didn't respond. Check the URL."
              : err instanceof Error
                ? err.message
                : String(err),
      })
    }
  }

  const save = async () => {
    setSaving(true)
    try {
      await api.saveAtlassianSettings(form)
      await qc.invalidateQueries({ queryKey: keys.atlassian })
      notify.success({ title: 'Atlassian Cloud settings saved' })
    } catch (err) {
      showError("Couldn't save the Atlassian settings", err)
    } finally {
      setSaving(false)
    }
  }

  const lastAt = last ? new Date(last.synced_at) : null
  const failures = last ? last.issue_failures + last.request_failures : 0

  return (
    <div className="flex flex-col gap-6">
      <section className="flex flex-col gap-4">
        <h3 className="m-0 flex items-baseline gap-2 text-[15px] font-semibold">
          Connection
          <span className="text-[12px] font-normal text-[#71717a]">Jira issues and Service Desk requests</span>
        </h3>
        <Field label="URL" hint="Your Atlassian Cloud site. Also used for “Open in Jira” links.">
          <input
            className={cn(input, 'font-mono text-[13px]')}
            placeholder="https://your-site.atlassian.net"
            value={form.site_url}
            onChange={(e) => set({ site_url: e.target.value })}
          />
        </Field>
        <Field label="Email" hint="The Atlassian account the API key belongs to.">
          <input
            className={input}
            type="email"
            placeholder="you@company.com"
            value={form.email}
            onChange={(e) => set({ email: e.target.value })}
          />
        </Field>
        <div className="flex flex-col gap-1.5">
          <label htmlFor="api-key" className="text-[13px] font-medium">
            API Key
          </label>
          <div className="flex gap-2">
            <input
              id="api-key"
              className={cn(input, 'font-mono text-[13px]')}
              type={showKey ? 'text' : 'password'}
              autoComplete="off"
              value={form.api_token}
              onChange={(e) => set({ api_token: e.target.value })}
            />
            <Button
              variant="outline"
              size="icon"
              aria-label={showKey ? 'Hide API key' : 'Show API key'}
              onClick={() => setShowKey(!showKey)}
            >
              {showKey ? <EyeOff size={15} /> : <Eye size={15} />}
            </Button>
          </div>
          <span className="text-[12px] text-[#71717a]">Create one in your Atlassian account under Security → API tokens.</span>
        </div>
        <div
          className={cn(
            'flex min-h-[56px] items-center gap-2 rounded-lg border px-4 py-2.5 text-[13px]',
            test.state === 'ok'
              ? 'border-[#bbf7d0] bg-[#f0fdf4] text-[#15803d]'
              : test.state === 'failed'
                ? 'border-[#fecaca] bg-[#fef2f2] text-[#b91c1c]'
                : 'border-[#e4e4e7] bg-[#fafafa] text-[#52525b]',
          )}
        >
          {test.state === 'testing' ? (
            <RefreshCw size={15} className="shrink-0 animate-spin" />
          ) : test.state === 'ok' ? (
            <CircleCheck size={15} className="shrink-0" />
          ) : test.state === 'failed' ? (
            <CircleAlert size={15} className="shrink-0" />
          ) : null}
          <span className="flex-1" role="status">
            {test.state === 'testing'
              ? 'Testing connection…'
              : test.state === 'ok'
                ? `Connected — Jira and Service Desk are reachable. Checked ${test.at}.`
                : test.state === 'failed'
                  ? test.message
                  : 'Not tested yet.'}
          </span>
          <Button variant="outline" size="sm" disabled={test.state === 'testing'} onClick={() => void runTest()}>
            <Link2 size={14} />
            Test connection
          </Button>
        </div>
        <div className="flex justify-end gap-2">
          <Button variant="outline" disabled={!dirty || saving} onClick={() => setForm(stored)}>
            Discard
          </Button>
          <Button disabled={!dirty || saving} onClick={() => void save()}>
            Save
          </Button>
        </div>
      </section>

      <hr className="m-0 border-0 border-t border-[#f4f4f5]" />

      <section className="flex flex-col gap-4">
        <h3 className="m-0 flex items-baseline gap-2 text-[15px] font-semibold">
          Sync
          <span className="text-[12px] font-normal text-[#71717a]">Runs only when you start it</span>
        </h3>
        <div className="flex items-center gap-4 rounded-xl border border-[#e4e4e7] px-5 py-4">
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <span className="text-[12px] text-[#71717a]">Last sync</span>
            {last && lastAt ? (
              <>
                <span className="flex items-center gap-2 text-[16px] font-semibold tabular-nums">
                  {ymd(lastAt) === today ? 'Today' : lastAt.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })},{' '}
                  {hhmm(lastAt)}
                  {failures ? (
                    <span className="inline-flex h-[22px] items-center gap-1 rounded-full bg-[#fffbeb] px-2 text-[12px] font-medium text-[#92400e]">
                      <CircleAlert size={12} />
                      Partial
                    </span>
                  ) : (
                    <span className="inline-flex h-[22px] items-center gap-1 rounded-full bg-[#dcfce7] px-2 text-[12px] font-medium text-[#15803d]">
                      <Check size={12} />
                      Complete
                    </span>
                  )}
                </span>
                <span className="text-[13px] text-[#52525b]">
                  {last.issues} issues · {last.open_requests} open requests imported or updated
                </span>
              </>
            ) : (
              <span className="text-[14px] text-[#52525b]">Never synced</span>
            )}
          </div>
          <Button className="h-10 px-4 text-[15px]" disabled={syncStatus === 'busy'} onClick={run}>
            <RefreshCw size={16} className={cn(syncStatus === 'busy' && 'animate-spin')} />
            {syncStatus === 'busy' ? 'Syncing…' : 'Sync now'}
          </Button>
        </div>
        <div className="flex flex-col gap-2">
          <span className="text-[14px] font-semibold">What a sync does</span>
          <ul className="m-0 flex list-none flex-col gap-2 p-0 text-[13px] text-[#3f3f46]">
            {[
              [Diamond, 'bg-[#eff6ff] text-[#1d4ed8]', <>Imports Jira issues <b>assigned to you that aren’t done</b>, and updates the ones already here.</>],
              [Ticket, 'bg-[#eef2ff] text-[#4338ca]', 'Refreshes Service Desk requests. Done and Canceled requests drop out of Today.'],
              [Lock, 'bg-[#f4f4f5] text-[#52525b]', 'Only reads from Atlassian. Todos, notes and links you add in Wharf are never written back.'],
              [FileText, 'bg-[#f4f4f5] text-[#52525b]', 'Issues that leave your list, like resolved or reassigned ones, are fetched again one by one. Done and Canceled ones drop out of Today once their todos for today are done.'],
            ].map(([Icon, cls, text], i) => {
              const I = Icon as typeof Diamond
              return (
                <li key={i} className="flex items-center gap-2.5">
                  <span className={cn('flex size-[18px] shrink-0 items-center justify-center rounded', cls as string)}>
                    <I size={11} />
                  </span>
                  <span>{text as ReactNode}</span>
                </li>
              )
            })}
          </ul>
        </div>
      </section>
    </div>
  )
}

const NAV: { id: Tab; label: string; Icon: typeof Diamond }[] = [
  { id: 'general', label: 'General', Icon: SlidersHorizontal },
  { id: 'atlassian', label: 'Atlassian Cloud', Icon: Diamond },
]
const SOON = [
  { label: 'GitLab', Icon: GitBranch },
  { label: 'Notion', Icon: File },
  { label: 'Local Directories', Icon: FolderOpen },
]

/** SettingsDialog is the Settings modal at /settings/general and /settings/atlassian. */
export function SettingsDialog({ tab, onTab, onClose }: { tab: Tab; onTab: (t: Tab) => void; onClose: () => void }) {
  const { data: app } = useAppSettings()
  return (
    <D.Root open onOpenChange={(o) => !o && onClose()}>
      <D.Portal>
        <D.Overlay className="fixed inset-0 z-40 flex items-start justify-center bg-[rgba(9,9,11,0.48)] px-6 pt-14">
          <D.Content
            aria-describedby={undefined}
            onOpenAutoFocus={(e) => e.preventDefault()}
            className="flex h-[min(800px,calc(100vh-112px))] w-full max-w-[920px] overflow-hidden rounded-[14px] bg-white shadow-dialog outline-none"
          >
            <nav aria-label="Settings sections" className="flex w-[210px] shrink-0 flex-col border-r border-[#f4f4f5] bg-[#fafafa] px-2.5 pt-5 pb-4">
              <D.Title className="m-0 mb-3 px-2.5 text-[16px] font-semibold">Settings</D.Title>
              {NAV.map(({ id, label, Icon }) => (
                <button
                  key={id}
                  type="button"
                  aria-current={tab === id ? 'page' : undefined}
                  onClick={() => onTab(id)}
                  className={cn(
                    'flex h-9 items-center gap-2.5 rounded-lg px-2.5 text-left text-[14px]',
                    tab === id ? 'bg-primary-tint font-medium text-[#0f766e]' : 'text-[#27272a] hover:bg-[#f4f4f5]',
                  )}
                >
                  <Icon size={15} className={id === 'atlassian' && tab !== id ? 'text-[#1d4ed8]' : undefined} />
                  {label}
                </button>
              ))}
              <hr className="mx-2.5 my-2 border-0 border-t border-[#e4e4e7]" />
              {SOON.map(({ label, Icon }) => (
                <span key={label} aria-disabled className="flex h-9 items-center gap-2.5 px-2.5 text-[14px] whitespace-nowrap text-[#a1a1aa]">
                  <Icon size={15} />
                  {label}
                  <span className="ml-auto rounded-full border border-[#e4e4e7] bg-white px-1.5 text-[10px] text-[#71717a]">Soon</span>
                </span>
              ))}
              <span className="mt-auto px-2.5 text-[11px] text-[#a1a1aa]">Wharf · {app?.version ?? ''}</span>
            </nav>
            <div className="flex min-w-0 flex-1 flex-col">
              <header className="flex items-start border-b border-[#f4f4f5] px-6 pt-5 pb-4">
                <div className="flex flex-col gap-0.5">
                  <h2 className="m-0 text-[18px] font-semibold">{tab === 'general' ? 'General' : 'Atlassian Cloud'}</h2>
                  <p className="m-0 text-[13px] text-[#71717a]">
                    {tab === 'general' ? 'The local app server behind Wharf.' : 'Jira issues and Service Desk requests.'}
                  </p>
                </div>
                <D.Close
                  aria-label="Close settings"
                  className="ml-auto flex size-8 items-center justify-center rounded-md text-[#52525b] hover:bg-[#f4f4f5]"
                >
                  <X size={18} />
                </D.Close>
              </header>
              <div className="min-h-0 flex-1 overflow-y-auto px-6 py-6">{tab === 'general' ? <General /> : <Atlassian />}</div>
            </div>
          </D.Content>
        </D.Overlay>
      </D.Portal>
    </D.Root>
  )
}
