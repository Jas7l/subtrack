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
  | 'release_created'
  | 'release_approved'
  | 'release_rejected'

export type NotificationEntity = 'request' | 'subscription' | 'transaction' | 'release_request'

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
  // null: a company-wide subscription.
  subscription_department_id: number | null
  user_id: number
  user_name: string
  status: AssignmentStatus
  assigned_by: number | null
  assigned_by_name: string | null
  assigned_at: string
  // Blocking the holder or cancelling the subscription records the admin who
  // did it; the ids are null only if that user row is gone.
  revoked_by: number | null
  revoked_by_name: string | null
  revoked_at: string | null
  // The holder's request to give the licence back that still waits.
  release_request_id: number | null
}

export type ReleaseStatus = 'pending' | 'approved' | 'rejected' | 'cancelled'

// A holder asking to give a licence back. department_id is the
// subscription's department now — whoever manages it decides (null: a
// company-wide subscription, the admin's).
export interface ReleaseRequestDTO {
  id: number
  assignment_id: number
  subscription_id: number
  subscription_name: string
  department_id: number | null
  department_name: string | null
  requester_id: number
  requester_name: string
  reason: string
  status: ReleaseStatus
  decided_by: number | null
  decided_by_name: string | null
  comment: string
  decided_at: string | null
  created_at: string
  updated_at: string
}

export interface CreateReleaseBody {
  assignment_id: number
  reason?: string
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

// POST /users. No department_id: the user is in no department.
export interface CreateUserBody {
  full_name: string
  email: string
  password: string
  role: Role
  department_id?: number
}

// PATCH /users/{id}: only the changed fields; department_id null — remove
// from the department.
export interface UpdateUserBody {
  full_name?: string
  email?: string
  role?: Role
  department_id?: number | null
  is_active?: boolean
}

export interface CreateDepartmentBody {
  name: string
  head_id?: number
}

// PATCH /departments/{id}: head_id null leaves the department without a head.
export interface UpdateDepartmentBody {
  name?: string
  head_id?: number | null
}

// POST /subscriptions. seats_total: required for group, 1 or absent for
// individual, absent for enterprise. No department_id — company-wide.
export interface CreateSubscriptionBody {
  name: string
  vendor?: string
  category?: string
  type: SubscriptionType
  cost: string
  currency: string
  billing_period: BillingPeriod
  start_date: string
  end_date?: string
  next_payment_date?: string
  department_id?: number
  seats_total?: number
}

// PATCH /subscriptions/{id}: only the changed fields; null clears end_date,
// next_payment_date or department_id. People set the status to active or
// cancelled only.
export interface UpdateSubscriptionBody {
  name?: string
  vendor?: string
  category?: string
  type?: SubscriptionType
  cost?: string
  currency?: string
  billing_period?: BillingPeriod
  start_date?: string
  end_date?: string | null
  next_payment_date?: string | null
  status?: 'active' | 'cancelled'
  department_id?: number | null
  seats_total?: number
}

// The subscription the admin creates while approving a request: name, vendor
// and department_id default to the request's (department_id null —
// company-wide).
export interface NewSubscriptionBody extends Omit<CreateSubscriptionBody, 'name' | 'department_id'> {
  name?: string
  department_id?: number | null
}

export interface ApproveBody {
  comment?: string
  // The admin picks exactly one: an existing subscription or a new one.
  subscription_id?: number
  subscription?: NewSubscriptionBody
}

export interface CreateAssignmentBody {
  subscription_id: number
  user_id: number
}

// Analytics: sums are in roubles at the current rates, as strings.
export interface PaymentStatsDTO {
  paid_this_month_rub: string
  paid_this_year_rub: string
  pending_count: number
  pending_rub: string
  overdue_count: number
  failed_count: number
}

export interface SummaryDTO {
  department_id: number | null
  active_subscriptions: number
  expiring_soon: number
  expired: number
  monthly_cost_rub: string
  annual_cost_rub: string
  seats_total: number
  seats_used: number
  seats_free: number
  // null when there are no counted seats.
  utilization_pct: number | null
  enterprise_subscriptions: number
  licences: number
  active_users: number
  cost_per_user_rub: string | null
  // The admin only.
  payments?: PaymentStatsDTO
  recommendations?: number
  potential_savings_rub?: string
}

// One row of a breakdown by category or vendor; key "" — without one.
export interface SliceDTO {
  key: string
  subscriptions: number
  monthly_cost_rub: string
  share_pct: number
}

// One department of GET /analytics/by-department; department_id null — the
// company-wide subscriptions (then active_users and cost_per_user_rub are
// null too).
export interface DepartmentRowDTO {
  department_id: number | null
  name: string
  subscriptions: number
  monthly_cost_rub: string
  seats_total: number
  seats_used: number
  utilization_pct: number | null
  active_users: number | null
  cost_per_user_rub: string | null
}

// One row of GET /analytics/top-subscriptions.
export interface TopRowDTO {
  subscription_id: number
  name: string
  vendor: string
  department_name: string | null
  status: SubscriptionStatus
  monthly_cost_rub: string
  annual_cost_rub: string
  // null for enterprise.
  seats_total: number | null
  seats_used: number
  utilization_pct: number | null
  // null when no licence is handed out.
  cost_per_licence_rub: string | null
}

export interface SpendingMonthDTO {
  // "YYYY-MM"
  month: string
  amount_rub: string
  payments: number
}

export interface SpendingResponse {
  from: string
  to: string
  months: SpendingMonthDTO[]
  total_rub: string
}

export interface ForecastMonthDTO {
  // "YYYY-MM"
  month: string
  scheduled_rub: string
  payments: number
  // The monthly cost of subscriptions without a next payment date.
  estimated_rub: string
  total_rub: string
}

export interface ForecastResponse {
  months: ForecastMonthDTO[]
  total_rub: string
  run_rate_rub: string
  // Active subscriptions without a next payment date.
  unscheduled: number
}

// POST /currencies. rate_to_rub — how many roubles one unit costs.
export interface CreateCurrencyBody {
  code: string
  name: string
  rate_to_rub: string
}

// PATCH /currencies/{code}: only the changed fields.
export interface UpdateCurrencyBody {
  name?: string
  rate_to_rub?: string
}

export type TransactionStatus = 'pending' | 'paid' | 'failed'

export interface TransactionDTO {
  id: number
  subscription_id: number
  subscription_name: string
  amount: string
  currency: string
  payment_date: string
  status: TransactionStatus
  // "" — not given.
  payment_method: string
  invoice_number: string
  paid_at: string | null
  created_at: string
  updated_at: string
}

// POST /transactions: a payment entered by hand is stored as paid. Without
// currency — the subscription's.
export interface CreateTransactionBody {
  subscription_id: number
  amount: string
  payment_date: string
  currency?: string
  payment_method?: string
  invoice_number?: string
}

// POST /transactions/{id}/pay; empty fields are left out.
export interface PayTransactionBody {
  payment_method?: string
  invoice_number?: string
}

export interface NotificationSettingsDTO {
  expiring_days: number
  payment_days: number
  updated_at: string
  updated_by: number | null
}

// PUT /settings/notifications.
export interface NotificationSettingsBody {
  expiring_days: number
  payment_days: number
}

export type RecommendationType =
  | 'unused_seats'
  | 'no_holders'
  | 'duplicate_vendor'
  | 'individual_to_group'
  | 'enterprise_low_usage'
  | 'annual_billing'
  | 'expired_with_holders'

export interface RecommendationDTO {
  type: RecommendationType
  subscription_ids: number[]
  title: string
  text: string
  // null: the saving cannot be computed.
  savings_rub_year: string | null
}

export interface RecommendationsResponse {
  items: RecommendationDTO[]
  total_savings_rub_year: string
}

export type AuditEntityType =
  | 'user'
  | 'department'
  | 'subscription'
  | 'assignment'
  | 'request'
  | 'release_request'
  | 'transaction'
  | 'settings'
  | 'currency'

// One record of GET /audit. actor_id null — the system (the scheduler).
// action is "<entity>.<verb>", e.g. subscription.update; details hold what
// changed (only the changed fields for an update).
export interface AuditEntryDTO {
  id: number
  actor_id: number | null
  actor_name: string | null
  action: string
  entity_type: AuditEntityType
  // null for settings and currencies (the code is in details).
  entity_id: number | null
  details: Record<string, unknown>
  created_at: string
}
