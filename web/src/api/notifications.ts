import { useQuery } from '@tanstack/react-query'
import { request } from './client'
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
