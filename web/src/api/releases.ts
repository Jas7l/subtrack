import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { request, type Query } from './client'
import type { CreateReleaseBody, Page, ReleaseRequestDTO } from './types'

// Requests to give a licence back (/release-requests). `inbox: true` is what
// waits for the caller's decision: a manager's department's, and for the
// admin only those nobody else can decide (the server picks them).

export function useReleases(query: Query = {}, enabled = true) {
  return useQuery({
    queryKey: ['releases', 'list', query],
    queryFn: () => request<Page<ReleaseRequestDTO>>('/release-requests', { query }),
    placeholderData: keepPreviousData,
    enabled,
  })
}

// useInboxReleasesCount backs the badges: how many release requests wait for
// the caller. Only `total` is needed, so one item is fetched.
export function useInboxReleasesCount(enabled: boolean) {
  return useQuery({
    queryKey: ['releases', 'inbox-count'],
    queryFn: () =>
      request<Page<unknown>>('/release-requests', { query: { inbox: true, limit: 1 } }),
    select: (page) => page.total,
    enabled,
    refetchInterval: 30_000,
  })
}

export function useRelease(id: number) {
  return useQuery({
    queryKey: ['releases', 'one', id],
    queryFn: () => request<ReleaseRequestDTO>(`/release-requests/${id}`),
    // A non-numeric id in the address is a 400 on the server: not asked.
    enabled: Number.isInteger(id),
  })
}

// A release request changes the lists and badges, the licence it is about
// (its release_request_id, and on approval its status and the seats) and the
// notifications.
function afterChange(queryClient: QueryClient) {
  for (const key of ['releases', 'assignments', 'subscriptions', 'analytics', 'notifications']) {
    void queryClient.invalidateQueries({ queryKey: [key] })
  }
}

// A filing refreshes even when it fails: a 409 «already waits» or «already
// revoked» means the card on screen is stale.
export function useCreateRelease() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateReleaseBody) =>
      request<ReleaseRequestDTO>('/release-requests', { method: 'POST', body }),
    onSettled: () => afterChange(queryClient),
  })
}

// Decisions and cancels refresh even when they fail: the request may have
// been decided, cancelled or closed with its licence meanwhile.
export function useApproveRelease() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, comment }: { id: number; comment?: string }) =>
      request<ReleaseRequestDTO>(`/release-requests/${id}/approve`, {
        method: 'POST',
        // The server rejects an empty body: no comment is {}.
        body: comment ? { comment } : {},
      }),
    onSettled: () => afterChange(queryClient),
  })
}

export function useRejectRelease() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, comment }: { id: number; comment: string }) =>
      request<ReleaseRequestDTO>(`/release-requests/${id}/reject`, {
        method: 'POST',
        body: { comment },
      }),
    onSettled: () => afterChange(queryClient),
  })
}

export function useCancelRelease() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      request<ReleaseRequestDTO>(`/release-requests/${id}/cancel`, { method: 'POST' }),
    onSettled: () => afterChange(queryClient),
  })
}
