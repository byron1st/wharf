import { cva, type VariantProps } from 'class-variance-authority'
import { Slot } from 'radix-ui'
import type { ComponentProps } from 'react'
import { cn } from '@/lib/utils'

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-md font-medium transition-colors disabled:pointer-events-none disabled:opacity-50 [&_svg]:shrink-0',
  {
    variants: {
      variant: {
        default: 'bg-primary text-white hover:bg-primary-hover',
        outline: 'border border-[#e4e4e7] bg-white text-[#09090b] shadow-card hover:bg-[#fafafa]',
        ghost: 'text-[#52525b] hover:bg-[#f4f4f5] hover:text-[#09090b]',
        destructive: 'border border-[#e4e4e7] bg-white text-[#dc2626] hover:bg-[#fef2f2]',
      },
      size: {
        default: 'h-9 px-3 text-[14px]',
        sm: 'h-8 px-2.5 text-[13px]',
        xs: 'h-7 px-2 text-[12px]',
        icon: 'size-9',
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
