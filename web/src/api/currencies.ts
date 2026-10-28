import { useMutation, useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query'
import { request } from './client'
import type { CreateCurrencyBody, CurrencyDTO, UpdateCurrencyBody } from './types'

// The currency list changes rarely (only the admin edits it).
export function useCurrencies() {
  return useQuery({
    queryKey: ['currencies'],
    queryFn: () => request<{ items: CurrencyDTO[] }>('/currencies'),
    select: (res) => res.items,
    staleTime: 10 * 60_000,
  })
}

// Reports convert to roubles at the current rate: a new rate moves them.
function afterChange(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ['currencies'] })
  void queryClient.invalidateQueries({ queryKey: ['analytics'] })
}

export function useCreateCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateCurrencyBody) =>
      request<CurrencyDTO>('/currencies', { method: 'POST', body }),
    onSuccess: () => afterChange(queryClient),
  })
}

export function useUpdateCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ code, body }: { code: string; body: UpdateCurrencyBody }) =>
      request<CurrencyDTO>(`/currencies/${code}`, { method: 'PATCH', body }),
    onSuccess: () => afterChange(queryClient),
  })
}

// Only a currency nothing uses; RUB never.
export function useDeleteCurrency() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (code: string) => request<void>(`/currencies/${code}`, { method: 'DELETE' }),
    onSuccess: () => afterChange(queryClient),
  })
}
