import '@fontsource-variable/jetbrains-mono'
import 'pretendard/dist/web/variable/pretendardvariable.css'
import './index.css'
import { QueryClientProvider } from '@tanstack/react-query'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter } from 'react-router'
import { App } from './App'
import { serverDown } from './lib/errors'
import { queryClient, setNetworkErrorHandler } from './lib/queryClient'

setNetworkErrorHandler(serverDown)

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
)
