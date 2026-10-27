import { useQuery } from '@tanstack/react-query'
import { request } from './client'
import type { Page } from './types'

// usePendingRequestsCount backs the badge on «Заявки»: how many requests wait
// for the caller's decision (`pending_manager` for a head, `pending_admin` for
// an admin). Only `total` is needed, so one item is fetched.
export function usePendingRequestsCount(status: 'pending_manager' | 'pending_admin' | null) {
  return useQuery({
    queryKey: ['requests', 'pending-count', status],
    queryFn: () => request<Page<unknown>>('/requests', { query: { status, limit: 1 } }),
    select: (page) => page.total,
    enabled: status !== null,
    refetchInterval: 30_000,
  })
}
