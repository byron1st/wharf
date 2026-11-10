import ReactMarkdown, { defaultUrlTransform } from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Check } from 'lucide-react'
import { cn } from '@/lib/utils'

/** Markdown renders a task description with GFM (task lists, tables); raw HTML is not rendered. */
export function Markdown({ source, className }: { source: string; className?: string }) {
  return (
    <div className={cn('md', className)}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        urlTransform={defaultUrlTransform}
        components={{
          a: ({ node: _node, ...props }) => <a {...props} target="_blank" rel="noreferrer noopener" />,
          input: ({ checked }) => (
            <span
              role="checkbox"
              aria-checked={!!checked}
              aria-disabled
              className={cn(
                'mr-2 inline-flex size-[14px] translate-y-[2px] items-center justify-center rounded-[3px] border',
                checked ? 'border-accent bg-accent text-white' : 'border-border-control bg-white',
              )}
            >
              {checked && <Check size={10} strokeWidth={3.5} />}
            </span>
          ),
          li: ({ node: _node, className: cls, children, ...props }) => {
            const task = cls?.includes('task-list-item')
            const done = task && Array.isArray(children) && children.some((c) => (c as { props?: { node?: { properties?: { checked?: boolean } } } })?.props?.node?.properties?.checked)
            return (
              <li {...props} className={cn(cls, done && 'text-ink-muted line-through')}>
                {children}
              </li>
            )
          },
        }}
      >
        {source}
      </ReactMarkdown>
    </div>
  )
}
