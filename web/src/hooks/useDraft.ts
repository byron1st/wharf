import { useEffect, useRef, useState } from 'react'

/**
 * useDraft keeps a text field's typing local and saves it 800ms after typing
 * stops or on blur. It follows the stored value while nothing is pending.
 */
export function useDraft(value: string, save: (v: string) => void) {
  const [draft, setDraft] = useState(value)
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined)
  const saved = useRef(value)
  const saveRef = useRef(save)
  saveRef.current = save

  useEffect(() => {
    if (!timer.current) setDraft(value)
    saved.current = value
  }, [value])

  const flush = (v: string) => {
    clearTimeout(timer.current)
    timer.current = undefined
    if (v !== saved.current) {
      saved.current = v
      saveRef.current(v)
    }
  }

  // Save what is pending when the field goes away, such as switching selection.
  const latest = useRef(draft)
  latest.current = draft
  useEffect(
    () => () => {
      if (timer.current) flush(latest.current)
    },
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [],
  )

  return {
    value: draft,
    dirty: draft !== saved.current,
    onChange: (v: string) => {
      setDraft(v)
      clearTimeout(timer.current)
      timer.current = setTimeout(() => flush(v), 800)
    },
    onBlur: () => flush(draft),
  }
}
