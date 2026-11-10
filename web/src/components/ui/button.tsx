import { cva, type VariantProps } from 'class-variance-authority'
import { Slot } from 'radix-ui'
import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-md font-medium transition-colors disabled:pointer-events-none disabled:opacity-50 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default: 'bg-accent text-white hover:bg-accent-hover',
        outline: 'border border-border bg-white text-ink shadow-xs hover:bg-bg-subtle',
        ghost: 'text-ink-secondary hover:bg-bg-muted hover:text-ink',
        destructive: 'border border-border bg-white text-danger hover:bg-danger-tint',
      },
      size: {
        default: 'h-8 px-3 text-[13px]',
        sm: 'h-8 px-2.5 text-[13px]',
        xs: 'h-7 px-2 text-[12px]',
        icon: 'size-8',
        'icon-sm': 'size-7',
      },
    },
    defaultVariants: { variant: 'default', size: 'default' },
  },
)

export function Button({
  className,
  variant,
  size,
  asChild,
  type = 'button',
  ...props
}: ComponentProps<'button'> & VariantProps<typeof buttonVariants> & { asChild?: boolean }) {
  const Comp = asChild ? Slot.Root : 'button'
  return <Comp type={type} className={cn(buttonVariants({ variant, size }), className)} {...props} />
}
