import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { download, request } from './client'
import type {
  DepartmentRowDTO,
  ForecastResponse,
  RecommendationsResponse,
  SliceDTO,
  SpendingResponse,
  SummaryDTO,
  TopRowDTO,
} from './types'

// A manager always gets their own department (the server ignores
// department_id for them); the admin picks one or none (the company).
interface Scope {
  department_id?: number
}

export function useSummary(scope: Scope = {}) {
  return useQuery({
    queryKey: ['analytics', 'summary', scope],
    queryFn: () => request<SummaryDTO>('/analytics/summary', { query: { ...scope } }),
  })
}

export function useByCategory(scope: Scope = {}, enabled = true) {
  return useQuery({
    queryKey: ['analytics', 'by-category', scope],
    queryFn: () =>
      request<{ items: SliceDTO[] }>('/analytics/by-category', { query: { ...scope } }),
    select: (res) => res.items,
    enabled,
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

// The admin only, below.

export function useByDepartment(enabled = true) {
  return useQuery({
    queryKey: ['analytics', 'by-department'],
    queryFn: () => request<{ items: DepartmentRowDTO[] }>('/analytics/by-department'),
    select: (res) => res.items,
    enabled,
  })
}

export function useTopSubscriptions(limit: number, scope: Scope = {}) {
  return useQuery({
    queryKey: ['analytics', 'top', limit, scope],
    queryFn: () =>
      request<{ items: TopRowDTO[] }>('/analytics/top-subscriptions', {
        query: { limit, ...scope },
      }),
    select: (res) => res.items,
  })
}

// Paid by month; the server's default is the last 12 months.
export function useSpending(scope: Scope = {}) {
  return useQuery({
    queryKey: ['analytics', 'spending', scope],
    queryFn: () => request<SpendingResponse>('/analytics/spending', { query: { ...scope } }),
  })
}

export function useRecommendations(scope: Scope = {}) {
  return useQuery({
    queryKey: ['analytics', 'recommendations', scope],
    queryFn: () =>
      request<RecommendationsResponse>('/analytics/recommendations', { query: { ...scope } }),
    placeholderData: keepPreviousData,
  })
}

// One workbook with every report (and the rates used).
export function exportAnalytics(scope: Scope = {}) {
  return download('/analytics/export', { ...scope })
}
