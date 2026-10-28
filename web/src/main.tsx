import '@fontsource-variable/manrope'
import './index.css'

import { MutationCache, QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { App as AntApp, ConfigProvider } from 'antd'
import ruRU from 'antd/locale/ru_RU'
import dayjs from 'dayjs'
import 'dayjs/locale/ru'
import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { RouterProvider } from 'react-router/dom'
import { ApiError } from './api/client'
import { router } from './router'
import { themeConfig } from './theme'

dayjs.locale('ru')

const queryClient: QueryClient = new QueryClient({
  // Every change goes to the audit trail, so its pages (and a subscription's
  // «История изменений») are refreshed after any of them.
  mutationCache: new MutationCache({
    // Not awaited: the mutation would wait for the audit to reload.
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['audit'] })
    },
  }),
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      // Coming back to the tab reloads what is on screen, however fresh:
      // someone else may have acted meanwhile (another role in another tab).
      refetchOnWindowFocus: 'always',
      // Client errors (4xx) will not change on retry.
      retry: (count, err) =>
        !(err instanceof ApiError && err.status >= 400 && err.status < 500) && count < 2,
    },
  },
})

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <ConfigProvider locale={ruRU} theme={themeConfig}>
      <AntApp>
        <QueryClientProvider client={queryClient}>
          <RouterProvider router={router} />
        </QueryClientProvider>
      </AntApp>
    </ConfigProvider>
  </StrictMode>,
)
