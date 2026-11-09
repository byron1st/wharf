import { Check } from 'lucide-react'
import { cn } from '@/lib/utils'

/** Checkbox is the round-cornered todo checkbox: teal when checked. */
export function Checkbox({
  checked,
  onChange,
  label,
  size = 18,
  className,
  disabled,
}: {
  checked: boolean
  onChange: (checked: boolean) => void
  label: string
  size?: number
  className?: string
  disabled?: boolean
}) {
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={(e) => {
        e.stopPropagation()
        onChange(!checked)
      }}
      style={{ width: size, height: size }}
      className={cn(
        'flex shrink-0 items-center justify-center rounded-[5px] border-[1.5px] transition-colors',
        checked ? 'border-primary bg-primary text-white' : 'border-[#a1a1aa] bg-white hover:border-[#71717a]',
        className,
      )}
    >
      {checked && <Check size={size - 6} strokeWidth={3.2} />}
    </button>
  )
}
