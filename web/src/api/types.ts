// DTOs written by hand after docs/swagger.json: the spec marks no field as
// required and none as nullable, so generated types would be all-optional.
// Nullable fields are `| null`. Money is a string ("15000.00"), dates are
// "YYYY-MM-DD", moments are RFC 3339.

export type Role = 'admin' | 'manager' | 'employee'

export interface ErrorBody {
  description: string
}

export interface PageMeta {
  total: number
  limit: number
  offset: number
}

export interface Page<T> extends PageMeta {
  items: T[]
}

// GET /auth/me and the `user` of a login response.
export interface MeDTO {
  id: number
  email: string
  full_name: string
  role: Role
  department_id: number | null
  created_at: string
}

export interface LoginResponse {
  token: string
  token_type: string
  expires_at: string
  user: MeDTO
}

export interface UserDTO {
  id: number
  email: string
  full_name: string
  role: Role
  department_id: number | null
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface DepartmentDTO {
  id: number
  name: string
  head_id: number | null
  head_name: string | null
  employee_count: number
  created_at: string
  updated_at: string
}

export type NotificationType =
  | 'subscription_expiring'
  | 'subscription_expired'
  | 'payment_due'
  | 'request_created'
  | 'request_approved'
  | 'request_rejected'

export type NotificationEntity = 'request' | 'subscription' | 'transaction'

export interface NotificationDTO {
  id: number
  type: NotificationType
  title: string
  body: string
  is_read: boolean
  entity_type: NotificationEntity | null
  entity_id: number | null
  created_at: string
}

export interface NotificationPage extends Page<NotificationDTO> {
  unread: number
}
