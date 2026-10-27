import { request } from './client'
import type { DepartmentDTO, LoginResponse, MeDTO } from './types'

export const authApi = {
  login: (email: string, password: string) =>
    request<LoginResponse>('/auth/login', { method: 'POST', body: { email, password } }),
  me: () => request<MeDTO>('/auth/me'),
  department: (id: number) => request<DepartmentDTO>(`/departments/${id}`),
}
