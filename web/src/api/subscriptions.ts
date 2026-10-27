import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { request, type Query } from './client'
import type { Page, SubscriptionDTO } from './types'

// The server cuts the list by role: an employee gets exactly the
// subscriptions they hold a licence of.
export function useSubscriptions(query: Query = {}) {
  return useQuery({
    queryKey: ['subscriptions', 'list', query],
    queryFn: () => request<Page<SubscriptionDTO>>('/subscriptions', { query }),
    placeholderData: keepPreviousData,
  })
}
