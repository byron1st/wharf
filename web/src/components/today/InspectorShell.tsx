import { X } from 'lucide-react'
import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function InspectorHeader({ children, onClose }: { children: ReactNode; onClose: () => void }) {
  return (
    <div className="flex items-center gap-2 border-b border-[#f4f4f5] px-4 py-3">
      {children}
      <button
        type="button"
        aria-label="Close"
        onClick={onClose}
        className="flex size-7 items-center justify-center rounded-md text-[#71717a] hover:bg-[#f4f4f5]"
      >
        <X size={14} />
      </button>
    </div>
  )
}

export function Label({ children, className }: { children: ReactNode; className?: string }) {
  return <span className={cn('text-[12px] font-medium text-[#3f3f46]', className)}>{children}</span>
}
