import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request, type Query } from './client'
import type { NotificationPage } from './types'

// Notifications are polled: the server has no push channel.
const POLL_MS = 30_000

export function useLatestNotifications() {
  return useQuery({
    queryKey: ['notifications', 'latest'],
    queryFn: () => request<NotificationPage>('/notifications', { query: { limit: 10 } }),
    refetchInterval: POLL_MS,
  })
}

export function useNotifications(query: Query) {
  return useQuery({
    queryKey: ['notifications', 'list', query],
    queryFn: () => request<NotificationPage>('/notifications', { query }),
    placeholderData: keepPreviousData,
    refetchInterval: POLL_MS,
  })
}

export function useMarkRead() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => request<void>(`/notifications/${id}/read`, { method: 'POST' }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notifications'] }),
  })
}

export function useMarkAllRead() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => request<void>('/notifications/read-all', { method: 'POST' }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notifications'] }),
  })
}
