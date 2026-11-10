import { Command, useCommandState } from 'cmdk'
import { Search } from 'lucide-react'
import type { ReactNode } from 'react'

export interface SearchItem {
  id: string
  /** value is what the search matches against. */
  value: string
  content: ReactNode
}

/** SearchPicker is an inline cmdk list with a search box, used to link requests, todos and tasks. */
export function SearchPicker({
  placeholder,
  items,
  empty,
  onPick,
  footer,
}: {
  placeholder: string
  items: SearchItem[]
  empty: string
  onPick: (id: string) => void
  footer?: (search: string) => ReactNode
}) {
  return (
    <Command
      className="overflow-hidden rounded-lg border border-[#c7d2fe] bg-white shadow-xs"
      loop
      filter={(value, search) => (value.toLowerCase().includes(search.toLowerCase()) ? 1 : 0)}
    >
      <div className="flex h-9 items-center gap-2 border-b border-border-subtle px-3">
        <Search size={14} className="text-ink-faint" />
        <Command.Input
          autoFocus
          placeholder={placeholder}
          className="h-full min-w-0 flex-1 bg-transparent text-[13px] outline-none placeholder:text-ink-faint"
        />
      </div>
      <Command.List className="max-h-[260px] overflow-y-auto p-1">
        <Command.Empty className="px-2 py-3 text-[12px] text-ink-muted">{empty}</Command.Empty>
        {items.map((it) => (
          <Command.Item
            key={it.id}
            value={`${it.value} ${it.id}`}
            onSelect={() => onPick(it.id)}
            className="flex min-h-9 cursor-pointer items-center gap-2 rounded-md px-2 text-[13px] data-[selected=true]:bg-bg-muted"
          >
            {it.content}
          </Command.Item>
        ))}
      </Command.List>
      {footer && <SearchFooter render={footer} />}
    </Command>
  )
}

function SearchFooter({ render }: { render: (search: string) => ReactNode }) {
  const search = useCommandState((s) => s.search)
  return <>{render(search)}</>
}
