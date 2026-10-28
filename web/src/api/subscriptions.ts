import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { download, request, type Query } from './client'
import type {
  CreateSubscriptionBody,
  Page,
  SubscriptionDTO,
  UpdateSubscriptionBody,
} from './types'

// The server cuts the list by role: an employee gets exactly the
// subscriptions they hold a licence of.
export function useSubscriptions(query: Query = {}, enabled = true) {
  return useQuery({
    queryKey: ['subscriptions', 'list', query],
    queryFn: () => request<Page<SubscriptionDTO>>('/subscriptions', { query }),
    placeholderData: keepPreviousData,
    enabled,
  })
}

export function useSubscription(id: number) {
  return useQuery({
    queryKey: ['subscriptions', 'one', id],
    queryFn: () => request<SubscriptionDTO>(`/subscriptions/${id}`),
    enabled: Number.isInteger(id),
  })
}

// Cancelling or deleting revokes the licences; any change moves the numbers.
function afterChange(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ['subscriptions'] })
  void queryClient.invalidateQueries({ queryKey: ['assignments'] })
  void queryClient.invalidateQueries({ queryKey: ['analytics'] })
}

export function useCreateSubscription() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateSubscriptionBody) =>
      request<SubscriptionDTO>('/subscriptions', { method: 'POST', body }),
    onSuccess: () => afterChange(queryClient),
  })
}

export function useUpdateSubscription() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdateSubscriptionBody }) =>
      request<SubscriptionDTO>(`/subscriptions/${id}`, { method: 'PATCH', body }),
    onSuccess: () => afterChange(queryClient),
  })
}

// A soft delete: the subscription disappears from every list.
export function useDeleteSubscription() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => request<void>(`/subscriptions/${id}`, { method: 'DELETE' }),
    onSuccess: () => afterChange(queryClient),
  })
}

// The Excel file takes the same filters as the list (without paging).
export function exportSubscriptions(query: Query) {
  return download('/subscriptions/export', query)
}
