import { QueryCache, QueryClient } from '@tanstack/react-query'
import { NetworkError } from '@/lib/api'

let onNetworkError: () => void = () => {}

/** setNetworkErrorHandler lets errors.tsx react to failed reads without an import cycle. */
export function setNetworkErrorHandler(fn: () => void) {
  onNetworkError = fn
}

export const queryClient = new QueryClient({
  queryCache: new QueryCache({
    onError: (err) => {
      if (err instanceof NetworkError) onNetworkError()
    },
  }),
  defaultOptions: {
    queries: { retry: false, refetchOnWindowFocus: true, staleTime: 10_000 },
  },
})
