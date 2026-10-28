import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { request, type Query } from './client'
import type { Page, UserDTO } from './types'

// The server cuts the list by role: a manager gets their department only.
export function useUsers(query: Query = {}, enabled = true) {
  return useQuery({
    queryKey: ['users', 'list', query],
    queryFn: () => request<Page<UserDTO>>('/users', { query }),
    placeholderData: keepPreviousData,
    enabled,
  })
}

export function useUser(id: number) {
  return useQuery({
    queryKey: ['users', 'one', id],
    queryFn: () => request<UserDTO>(`/users/${id}`),
    enabled: Number.isInteger(id),
  })
}
