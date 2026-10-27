import { useQuery } from '@tanstack/react-query'
import { request } from './client'
import type { CurrencyDTO } from './types'

// The currency list changes rarely (only the admin edits it).
export function useCurrencies() {
  return useQuery({
    queryKey: ['currencies'],
    queryFn: () => request<{ items: CurrencyDTO[] }>('/currencies'),
    select: (res) => res.items,
    staleTime: 10 * 60_000,
  })
}
