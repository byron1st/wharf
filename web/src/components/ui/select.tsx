import { Select as S } from 'radix-ui'
import { Check, ChevronDown } from 'lucide-react'
import type { ComponentProps, ReactNode } from 'react'
import { cn } from '@/lib/utils'

export const Select = S.Root
export const SelectValue = S.Value

export function SelectTrigger({
  className,
  children,
  hideIcon,
  ...props
}: ComponentProps<typeof S.Trigger> & { hideIcon?: boolean }) {
  return (
    <S.Trigger
      className={cn(
        'flex h-8 w-full items-center gap-2 rounded-md border border-border bg-white px-2.5 text-[13px] shadow-xs outline-none data-[state=open]:border-accent data-[state=open]:shadow-ring',
        className,
      )}
      {...props}
    >
      {children}
      {!hideIcon && (
        <S.Icon className="ml-auto text-ink-faint">
          <ChevronDown size={14} />
        </S.Icon>
      )}
    </S.Trigger>
  )
}

export function SelectContent({ className, children, ...props }: ComponentProps<typeof S.Content>) {
  return (
    <S.Portal>
      <S.Content
        position="popper"
        sideOffset={6}
        className={cn(
          'z-50 min-w-[var(--radix-select-trigger-width)] rounded-xl border border-border bg-white p-1 shadow-lg',
          className,
        )}
        {...props}
      >
        <S.Viewport>{children}</S.Viewport>
      </S.Content>
    </S.Portal>
  )
}

export function SelectItem({ children, ...props }: ComponentProps<typeof S.Item> & { children: ReactNode }) {
  return (
    <S.Item
      className="relative flex h-8 cursor-pointer items-center gap-2 rounded-md pr-8 pl-2 text-[13px] outline-none select-none data-[highlighted]:bg-bg-muted"
      {...props}
    >
      <S.ItemText>{children}</S.ItemText>
      <S.ItemIndicator className="absolute right-2">
        <Check size={14} className="text-accent" />
      </S.ItemIndicator>
    </S.Item>
  )
}
