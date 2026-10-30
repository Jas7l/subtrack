import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { request, type Query } from './client'
import type { CreateUserBody, Page, UpdateUserBody, UserDTO } from './types'

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

// A user change moves department heads and head counts; blocking also
// revokes every licence (seats, analytics) and cancels the user's pending
// requests and release requests. A role or department change moves who
// decides release requests. The admin may edit themselves.
function afterChange(queryClient: QueryClient) {
  const keys = [
    'users',
    'departments',
    'assignments',
    'subscriptions',
    'requests',
    'releases',
    'analytics',
    'auth',
  ]
  for (const key of keys) {
    void queryClient.invalidateQueries({ queryKey: [key] })
  }
}

export function useCreateUser() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateUserBody) => request<UserDTO>('/users', { method: 'POST', body }),
    onSuccess: () => afterChange(queryClient),
  })
}

export function useUpdateUser() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdateUserBody }) =>
      request<UserDTO>(`/users/${id}`, { method: 'PATCH', body }),
    onSuccess: () => afterChange(queryClient),
  })
}

// An admin's reset; the user changes their own password in the profile.
export function useSetPassword() {
  return useMutation({
    mutationFn: ({ id, password }: { id: number; password: string }) =>
      request<void>(`/users/${id}/password`, { method: 'PUT', body: { password } }),
  })
}
