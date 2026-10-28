import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'
import { request, type Query } from './client'
import type { CreateDepartmentBody, DepartmentDTO, Page, UpdateDepartmentBody } from './types'

// A company has far fewer than 200 departments: lists and selects load them
// all at once.
export const ALL_DEPARTMENTS = { limit: 200 }

// Only the admin gets every department; anyone else gets their own.
export function useDepartments(query: Query = ALL_DEPARTMENTS, enabled = true) {
  return useQuery({
    queryKey: ['departments', 'list', query],
    queryFn: () => request<Page<DepartmentDTO>>('/departments', { query }),
    placeholderData: keepPreviousData,
    staleTime: 60_000,
    enabled,
  })
}

// departmentNames maps id → name for the tables whose rows carry only a
// department_id (users).
export function useDepartmentNames(enabled = true): Map<number, string> {
  const { data } = useDepartments(ALL_DEPARTMENTS, enabled)
  return new Map((data?.items ?? []).map((d) => [d.id, d.name]))
}

// A new head moves into the department, so people and the caller's own
// department (AuthContext) change too.
function afterChange(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ['departments'] })
  void queryClient.invalidateQueries({ queryKey: ['users'] })
  void queryClient.invalidateQueries({ queryKey: ['auth'] })
}

export function useCreateDepartment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateDepartmentBody) =>
      request<DepartmentDTO>('/departments', { method: 'POST', body }),
    onSuccess: () => afterChange(queryClient),
  })
}

export function useUpdateDepartment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, body }: { id: number; body: UpdateDepartmentBody }) =>
      request<DepartmentDTO>(`/departments/${id}`, { method: 'PATCH', body }),
    onSuccess: () => afterChange(queryClient),
  })
}

// Only an empty department can go (409 otherwise).
export function useDeleteDepartment() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: number) => request<void>(`/departments/${id}`, { method: 'DELETE' }),
    onSuccess: () => afterChange(queryClient),
  })
}
