import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { request, type Query } from './client'
import type { AssignmentDTO, Page } from './types'

export function useAssignments(query: Query = {}) {
  return useQuery({
    queryKey: ['assignments', 'list', query],
    queryFn: () => request<Page<AssignmentDTO>>('/assignments', { query }),
    placeholderData: keepPreviousData,
  })
}

// Revoking frees a seat, so subscriptions are refetched too.
export function useRevokeAssignment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) =>
      request<AssignmentDTO>(`/assignments/${id}/revoke`, { method: 'POST' }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['assignments'] })
      void queryClient.invalidateQueries({ queryKey: ['subscriptions'] })
    },
  })
}
