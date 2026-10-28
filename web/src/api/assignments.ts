import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { request, type Query } from './client'
import type { AssignmentDTO, CreateAssignmentBody, Page } from './types'

export function useAssignments(query: Query = {}, enabled = true) {
  return useQuery({
    queryKey: ['assignments', 'list', query],
    queryFn: () => request<Page<AssignmentDTO>>('/assignments', { query }),
    placeholderData: keepPreviousData,
    enabled,
  })
}

// A licence changes the seats of its subscription and the department's
// numbers (seats used, utilisation).
function afterChange(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ['assignments'] })
  void queryClient.invalidateQueries({ queryKey: ['subscriptions'] })
  void queryClient.invalidateQueries({ queryKey: ['analytics'] })
}

export function useCreateAssignment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateAssignmentBody) =>
      request<AssignmentDTO>('/assignments', { method: 'POST', body }),
    onSuccess: () => afterChange(queryClient),
  })
}

export function useRevokeAssignment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      request<AssignmentDTO>(`/assignments/${id}/revoke`, { method: 'POST' }),
    onSuccess: () => afterChange(queryClient),
  })
}
