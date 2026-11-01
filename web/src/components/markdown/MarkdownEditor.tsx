import { Bold, Code, Heading, Italic, Link, List, ListChecks, SquareCode, Check } from 'lucide-react'
import { useRef } from 'react'
import { useDraft } from '@/hooks/useDraft'
import { cn } from '@/lib/utils'
import { Markdown } from './Markdown'

/** MdIcon is the small "M↓" mark next to Description labels. */
export function MdIcon() {
  return (
    <svg width="16" height="10" viewBox="0 0 16 10" aria-hidden className="inline shrink-0 text-[#a1a1aa]">
      <rect x="0.5" y="0.5" width="15" height="9" rx="1.5" fill="none" stroke="currentColor" />
      <path d="M3 7.5v-5l2 2.5 2-2.5v5M11 2.5v5m-1.6-1.7L11 7.5l1.6-1.7" fill="none" stroke="currentColor" strokeWidth="1" />
    </svg>
  )
}

export function ModeTabs({ mode, onMode }: { mode: 'preview' | 'write'; onMode: (m: 'preview' | 'write') => void }) {
  return (
    <div role="tablist" className="flex rounded-md bg-[#f4f4f5] p-[3px]">
      {(['preview', 'write'] as const).map((m) => (
        <button
          key={m}
          type="button"
          role="tab"
          aria-selected={mode === m}
          onClick={() => onMode(m)}
          className={cn(
            'h-6 rounded px-2.5 text-[12px] font-medium capitalize',
            mode === m ? 'bg-white text-[#09090b] shadow-[0_1px_2px_rgba(0,0,0,0.08)]' : 'text-[#71717a]',
          )}
        >
          {m}
        </button>
      ))}
    </div>
  )
}

const TOOLS = [
  { label: 'Heading', Icon: Heading, before: '## ', after: '', line: true },
  { label: 'Bold', Icon: Bold, before: '**', after: '**' },
  { label: 'Italic', Icon: Italic, before: '_', after: '_' },
  { label: 'Code', Icon: Code, before: '`', after: '`' },
  { label: 'Link', Icon: Link, before: '[', after: '](url)' },
  { label: 'List', Icon: List, before: '- ', after: '', line: true },
  { label: 'Task list', Icon: ListChecks, before: '- [ ] ', after: '', line: true },
  { label: 'Code block', Icon: SquareCode, before: '```\n', after: '\n```', line: true },
]

/**
 * MarkdownEditor edits a description with Preview / Write tabs. Writes are
 * saved 800ms after typing stops and on blur, through onSave.
 */
export function MarkdownEditor({
  value,
  onSave,
  saving,
  mode,
  onMode,
  rows = 10,
  emptyText = 'No description',
}: {
  value: string
  onSave: (v: string) => void
  saving?: boolean
  mode: 'preview' | 'write'
  onMode: (m: 'preview' | 'write') => void
  rows?: number
  emptyText?: string
}) {
  const { value: draft, dirty, onChange: change, onBlur } = useDraft(value, onSave)
  const area = useRef<HTMLTextAreaElement>(null)

  const apply = (t: (typeof TOOLS)[number]) => {
    const el = area.current
    if (!el) return
    const { selectionStart: s, selectionEnd: e } = el
    let start = s
    if (t.line) start = draft.lastIndexOf('\n', s - 1) + 1
    const sel = draft.slice(start, e)
    const next = draft.slice(0, start) + t.before + sel + t.after + draft.slice(e)
    change(next)
    requestAnimationFrame(() => {
      el.focus()
      el.setSelectionRange(start + t.before.length, start + t.before.length + sel.length)
    })
  }

  if (mode === 'preview') {
    return draft.trim() ? (
      <div className="rounded-lg border border-[#f4f4f5] bg-[#fafafa] p-3">
        <Markdown source={draft} />
      </div>
    ) : (
      <button
        type="button"
        onClick={() => onMode('write')}
        className="min-h-[40px] rounded-lg border border-[#f4f4f5] bg-[#fafafa] p-3 text-left text-[13px] text-[#a1a1aa]"
      >
        {emptyText}
      </button>
    )
  }

  return (
    <div className="overflow-hidden rounded-lg border border-[#e4e4e7] bg-white shadow-card focus-within:border-primary">
      <div role="toolbar" aria-label="Markdown formatting" className="flex flex-wrap items-center gap-px border-b border-[#f4f4f5] bg-[#fafafa] px-1.5 py-1">
        {TOOLS.map((t, i) => (
          <span key={t.label} className="flex items-center">
            {(i === 5 || i === 7) && <span className="mx-1 h-4 w-px bg-[#e4e4e7]" />}
            <button
              type="button"
              aria-label={t.label}
              title={t.label}
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => apply(t)}
              className="flex size-7 items-center justify-center rounded text-[#52525b] hover:bg-[#f4f4f5] hover:text-[#09090b]"
            >
              <t.Icon size={14} />
            </button>
          </span>
        ))}
      </div>
      <textarea
        ref={area}
        aria-label="Description"
        value={draft}
        rows={rows}
        onChange={(e) => change(e.target.value)}
        onBlur={onBlur}
        className="block w-full resize-y border-none px-3 py-2.5 font-mono text-[12.5px] leading-[1.7] outline-none"
      />
      <div className="flex items-center gap-2 border-t border-[#f4f4f5] px-3 py-1.5 text-[11px] text-[#71717a]">
        <MdIcon />
        <span className="font-mono">Markdown **bold** · `code` · - [ ] task · ``` block</span>
        <span className={cn('ml-auto flex items-center gap-1', saving || dirty ? 'text-[#a1a1aa]' : 'text-[#16a34a]')}>
          {saving || dirty ? (
            'Saving…'
          ) : (
            <>
              <Check size={12} />
              Saved
            </>
          )}
        </span>
      </div>
    </div>
  )
}
