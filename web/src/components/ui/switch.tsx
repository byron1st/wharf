import { Switch as S } from 'radix-ui'
import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

export function Switch({ className, ...props }: ComponentProps<typeof S.Root>) {
  return (
    <S.Root
      className={cn(
        'inline-flex h-[18px] w-8 shrink-0 items-center rounded-full border border-transparent bg-bg-pressed transition-colors data-[state=checked]:bg-accent',
        className,
      )}
      {...props}
    >
      <S.Thumb className="block size-[14px] translate-x-[1px] rounded-full bg-white shadow transition-transform data-[state=checked]:translate-x-[15px]" />
    </S.Root>
  )
}
