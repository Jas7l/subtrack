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

export type SubscriptionType = 'individual' | 'group' | 'enterprise'
export type SubscriptionStatus = 'active' | 'expiring_soon' | 'expired' | 'cancelled'
export type BillingPeriod = 'month' | 'quarter' | 'year'

export interface SubscriptionDTO {
  id: number
  name: string
  vendor: string
  category: string
  type: SubscriptionType
  cost: string
  currency: string
  billing_period: BillingPeriod
  start_date: string
  end_date: string | null
  next_payment_date: string | null
  status: SubscriptionStatus
  // null: a company-wide subscription.
  department_id: number | null
  department_name: string | null
  // null for enterprise (no seat limit).
  seats_total: number | null
  seats_used: number
  seats_free: number | null
  created_by: number | null
  created_at: string
  updated_at: string
}

export type AssignmentStatus = 'active' | 'revoked'

export interface AssignmentDTO {
  id: number
  subscription_id: number
  subscription_name: string
  user_id: number
  user_name: string
  status: AssignmentStatus
  assigned_by: number | null
  assigned_at: string
  revoked_by: number | null
  revoked_at: string | null
}

export type RequestStatus =
  | 'pending_manager'
  | 'rejected_by_manager'
  | 'pending_admin'
  | 'rejected_by_admin'
  | 'approved'
  | 'cancelled'

export interface RequestDTO {
  id: number
  requester_id: number
  requester_name: string
  department_id: number | null
  department_name: string | null
  name: string
  vendor: string
  justification: string
  estimated_cost: string | null
  currency: string
  // The subscription the licence was given in, once approved.
  subscription_id: number | null
  status: RequestStatus
  manager_id: number | null
  manager_name: string | null
  manager_comment: string
  manager_decided_at: string | null
  admin_id: number | null
  admin_name: string | null
  admin_comment: string
  admin_decided_at: string | null
  created_at: string
  updated_at: string
}

export interface CreateRequestBody {
  name: string
  vendor?: string
  justification: string
  estimated_cost?: string
  currency?: string
}

export interface CurrencyDTO {
  code: string
  name: string
  rate_to_rub: string
  updated_at: string
  updated_by: number | null
}
