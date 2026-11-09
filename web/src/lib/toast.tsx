import type { ReactNode } from 'react'
import { toast } from 'sonner'
import { Check, CircleAlert, CircleCheck, CircleX, ServerCrash, TriangleAlert, Undo2, SearchX, X } from 'lucide-react'
import { cn } from '@/lib/utils'

// Toasts follow docs/design §6: success and info close after 4 seconds,
// warnings and errors stay until dismissed or retried, and a loading toast is
// replaced in place by its result by reusing its id.

type Variant = 'loading' | 'success' | 'info' | 'warn' | 'error'

export interface ToastAction {
  label: string
  onClick: () => void
  primary?: boolean
}

export interface ToastOptions {
  id?: string
  title: string
  meta?: string
  description?: ReactNode
  actions?: ToastAction[]
  icon?: 'server' | 'undo' | 'gone' | 'check'
}

const ICONS = {
  server: ServerCrash,
  undo: Undo2,
  gone: SearchX,
  check: Check,
}

function Card({ id, variant, opts }: { id: string | number; variant: Variant; opts: ToastOptions }) {
  const tone =
    variant === 'warn'
      ? 'bg-[#fffbeb] border-[#fde68a] text-[#92400e]'
      : variant === 'error'
        ? 'bg-[#fef2f2] border-[#fecaca] text-[#b91c1c]'
        : 'bg-white border-[#e4e4e7] text-[#09090b]'
  const sub = variant === 'warn' ? 'text-[#92400e]' : variant === 'error' ? 'text-[#b91c1c]' : 'text-[#52525b]'
  const Icon = opts.icon
    ? ICONS[opts.icon]
    : variant === 'success'
      ? CircleCheck
      : variant === 'warn'
        ? TriangleAlert
        : variant === 'error'
          ? CircleX
          : variant === 'info'
            ? CircleAlert
            : null
  const iconColor =
    variant === 'success'
      ? 'text-[#16a34a]'
      : variant === 'warn'
        ? 'text-[#d97706]'
        : variant === 'error'
          ? 'text-[#dc2626]'
          : 'text-[#52525b]'
  const sticky = variant === 'warn' || variant === 'error'

  return (
    <div
      role={variant === 'error' ? 'alert' : 'status'}
      className={cn(
        'relative w-[356px] max-w-[380px] overflow-hidden rounded-[10px] border p-4 pr-9 shadow-toast font-sans',
        tone,
      )}
    >
      <div className="flex gap-3">
        <span className="mt-[2px] shrink-0">
          {variant === 'loading' ? (
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" className="animate-spin" aria-hidden>
              <circle cx="12" cy="12" r="9" stroke="#e4e4e7" strokeWidth="3" />
              <path d="M21 12a9 9 0 0 0-9-9" stroke="#52525b" strokeWidth="3" strokeLinecap="round" />
            </svg>
          ) : Icon ? (
            <Icon size={16} className={iconColor} aria-hidden />
          ) : null}
        </span>
        <div className="min-w-0 flex-1">
          <p className="m-0 text-[14px] font-semibold leading-5">
            {opts.title}
            {opts.meta && (
              <span className="ml-2 font-mono text-[11px] font-normal text-[#a1a1aa]">{opts.meta}</span>
            )}
          </p>
          {opts.description && <div className={cn('mt-0.5 text-[13px] leading-5', sub)}>{opts.description}</div>}
          {opts.actions && opts.actions.length > 0 && (
            <div className="mt-3 flex gap-2">
              {opts.actions.map((a) => (
                <button
                  key={a.label}
                  type="button"
                  onClick={() => {
                    toast.dismiss(id)
                    a.onClick()
                  }}
                  className={cn(
                    'h-7 rounded-md px-2.5 text-[13px] font-medium',
                    a.primary
                      ? 'bg-[#dc2626] text-white hover:bg-[#b91c1c]'
                      : variant === 'warn'
                        ? 'border border-[#fcd34d] bg-white text-[#78350f] hover:bg-[#fffbeb]'
                        : 'border border-[#e4e4e7] bg-white text-[#09090b] hover:bg-[#fafafa]',
                  )}
                >
                  {a.label}
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
      {sticky && (
        <button
          type="button"
          aria-label="Dismiss"
          onClick={() => toast.dismiss(id)}
          className="absolute top-3 right-3 flex size-5 items-center justify-center rounded text-current opacity-70 hover:opacity-100"
        >
          <X size={14} />
        </button>
      )}
      {variant === 'loading' && (
        <div className="absolute right-0 bottom-0 left-0 h-[3px] overflow-hidden bg-[#f4f4f5]">
          <div className="h-full w-1/3 animate-[toast-progress_1.4s_ease-in-out_infinite] bg-[#0f766e]" />
        </div>
      )}
    </div>
  )
}

function show(variant: Variant, opts: ToastOptions) {
  const duration = variant === 'loading' || variant === 'warn' || variant === 'error' ? Infinity : 4000
  return toast.custom((id) => <Card id={id} variant={variant} opts={opts} />, { id: opts.id, duration })
}

export const notify = {
  loading: (o: ToastOptions) => show('loading', o),
  success: (o: ToastOptions) => show('success', o),
  info: (o: ToastOptions) => show('info', o),
  warn: (o: ToastOptions) => show('warn', o),
  error: (o: ToastOptions) => show('error', o),
  dismiss: (id: string) => toast.dismiss(id),
}
