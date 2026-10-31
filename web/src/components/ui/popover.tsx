import { Popover as P } from 'radix-ui'
import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

export const Popover = P.Root
export const PopoverTrigger = P.Trigger
export const PopoverAnchor = P.Anchor

/** PopoverContent is the shared picker popover: 300px wide, 6px below its trigger. */
export function PopoverContent({ className, align = 'start', sideOffset = 6, ...props }: ComponentProps<typeof P.Content>) {
  return (
    <P.Portal>
      <P.Content
        align={align}
        sideOffset={sideOffset}
        collisionPadding={12}
        className={cn(
          'z-50 w-[300px] rounded-[10px] border border-[#e4e4e7] bg-white p-3 text-[#09090b] shadow-popover outline-none',
          className,
        )}
        {...props}
      />
    </P.Portal>
  )
}
