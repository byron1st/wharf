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
        'flex h-8 w-full items-center gap-2 rounded-md border border-[#e4e4e7] bg-white px-2.5 text-[13px] shadow-card outline-none data-[state=open]:border-primary data-[state=open]:shadow-ring',
        className,
      )}
      {...props}
    >
      {children}
      {!hideIcon && (
        <S.Icon className="ml-auto text-[#a1a1aa]">
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
          'z-50 min-w-[var(--radix-select-trigger-width)] rounded-[10px] border border-[#e4e4e7] bg-white p-1 shadow-popover',
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
      className="relative flex h-8 cursor-pointer items-center gap-2 rounded-md pr-8 pl-2 text-[13px] outline-none select-none data-[highlighted]:bg-[#f4f4f5]"
      {...props}
    >
      <S.ItemText>{children}</S.ItemText>
      <S.ItemIndicator className="absolute right-2">
        <Check size={14} className="text-primary" />
      </S.ItemIndicator>
    </S.Item>
  )
}
