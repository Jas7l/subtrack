import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { request, type Query } from './client'
import type { CreateRequestBody, Page, RequestDTO } from './types'

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

export function useRequests(query: Query = {}) {
  return useQuery({
    queryKey: ['requests', 'list', query],
    queryFn: () => request<Page<RequestDTO>>('/requests', { query }),
    placeholderData: keepPreviousData,
  })
}

export function useRequest(id: number) {
  return useQuery({
    queryKey: ['requests', 'one', id],
    queryFn: () => request<RequestDTO>(`/requests/${id}`),
  })
}

// A request changes lists, the badge counter and the notifications of whoever
// decides it (or of the requester).
function afterChange(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ['requests'] })
  void queryClient.invalidateQueries({ queryKey: ['notifications'] })
}

export function useCreateRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateRequestBody) =>
      request<RequestDTO>('/requests', { method: 'POST', body }),
    onSuccess: () => afterChange(queryClient),
  })
}

export function useCancelRequest() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => request<RequestDTO>(`/requests/${id}/cancel`, { method: 'POST' }),
    onSuccess: () => afterChange(queryClient),
  })
}
