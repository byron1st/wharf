import { createContext, useCallback, useContext, useState, type ReactNode } from 'react'

/** KeptTasks are the tasks whose status changed in this visit, which Today keeps until the next one. */
interface KeptTasks {
  kept: Set<string>
  keep: (id: string) => void
}

const KeptTasksContext = createContext<KeptTasks | null>(null)

function useKeptState(): KeptTasks {
  const [kept, setKept] = useState<Set<string>>(() => new Set())
  const keep = useCallback((id: string) => setKept((s) => (s.has(id) ? s : new Set(s).add(id))), [])
  return { kept, keep }
}

/** KeptTasksProvider shares the kept tasks between Today and the modals over it. */
export function KeptTasksProvider({ children }: { children: ReactNode }) {
  return <KeptTasksContext.Provider value={useKeptState()}>{children}</KeptTasksContext.Provider>
}

/** useKeptTasks is the shared kept tasks, or ones of its own outside a KeptTasksProvider. */
export function useKeptTasks(): KeptTasks {
  const shared = useContext(KeptTasksContext)
  const own = useKeptState()
  return shared ?? own
}
