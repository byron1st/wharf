import { useState } from 'react'

/** useStoredFlags is a set of IDs kept in localStorage under key, such as collapsed tasks or unfolded cards. */
export function useStoredFlags(key: string) {
  const [flags, setFlags] = useState<Record<string, boolean>>(() => {
    try {
      return JSON.parse(localStorage.getItem(key) ?? '{}') as Record<string, boolean>
    } catch {
      return {}
    }
  })
  const set = (ids: string[], on: boolean) => {
    setFlags((prev) => {
      const next = { ...prev }
      for (const id of ids) {
        if (on) next[id] = true
        else delete next[id]
      }
      localStorage.setItem(key, JSON.stringify(next))
      return next
    })
  }
  return [flags, set] as const
}
