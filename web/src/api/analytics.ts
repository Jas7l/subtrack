import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { request } from './client'
import type { ForecastResponse, SliceDTO, SummaryDTO } from './types'

// A manager always gets their own department (the server ignores
// department_id for them); the admin's dashboard (К6) passes it.
interface Scope {
  department_id?: number
}

export function useSummary(scope: Scope = {}) {
  return useQuery({
    queryKey: ['analytics', 'summary', scope],
    queryFn: () => request<SummaryDTO>('/analytics/summary', { query: { ...scope } }),
  })
}

export function useByCategory(scope: Scope = {}) {
  return useQuery({
    queryKey: ['analytics', 'by-category', scope],
    queryFn: () =>
      request<{ items: SliceDTO[] }>('/analytics/by-category', { query: { ...scope } }),
    select: (res) => res.items,
  })
}

export function useForecast(months: number, scope: Scope = {}) {
  return useQuery({
    queryKey: ['analytics', 'forecast', months, scope],
    queryFn: () =>
      request<ForecastResponse>('/analytics/forecast', { query: { months, ...scope } }),
    // Switching 6/12/24 keeps the old chart until the new one arrives.
    placeholderData: keepPreviousData,
  })
}
