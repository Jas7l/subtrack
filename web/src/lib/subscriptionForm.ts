import dayjs, { type Dayjs } from 'dayjs'
import type {
  BillingPeriod,
  CreateSubscriptionBody,
  RequestDTO,
  SubscriptionDTO,
  SubscriptionType,
  UpdateSubscriptionBody,
} from '../api/types'
import { parseDate } from './format'

// The values of the subscription form (components/SubscriptionFields.tsx).
// department_id undefined — company-wide; status is shown only on editing,
// and expired only as the status an expired subscription has (it cannot be
// picked).
export interface SubscriptionValues {
  name: string
  vendor?: string
  category?: string
  type: SubscriptionType
  cost: string
  currency: string
  billing_period: BillingPeriod
  start_date: Dayjs
  end_date?: Dayjs | null
  next_payment_date?: Dayjs | null
  department_id?: number
  seats_total?: number | null
  status?: 'active' | 'cancelled' | 'expired'
}

const day = (d: Dayjs) => d.format('YYYY-MM-DD')
const optDay = (d: Dayjs | null | undefined) => (d ? day(d) : null)
// The server takes at most two decimals as a string ("1500.00").
const money = (v: string) => Number(v).toFixed(2)

export function newSubscriptionValues(): Partial<SubscriptionValues> {
  return { type: 'group', currency: 'RUB', billing_period: 'month', start_date: dayjs() }
}

// The approval of a request suggests a subscription for one person: the
// request's name, vendor, cost and department.
export function valuesFromRequest(r: RequestDTO): Partial<SubscriptionValues> {
  return {
    ...newSubscriptionValues(),
    type: 'individual',
    name: r.name,
    vendor: r.vendor || undefined,
    cost: r.estimated_cost ?? undefined,
    currency: r.currency,
    department_id: r.department_id ?? undefined,
  }
}

export function valuesFromSubscription(s: SubscriptionDTO): SubscriptionValues {
  return {
    name: s.name,
    vendor: s.vendor || undefined,
    category: s.category || undefined,
    type: s.type,
    cost: s.cost,
    currency: s.currency,
    billing_period: s.billing_period,
    start_date: parseDate(s.start_date),
    end_date: s.end_date ? parseDate(s.end_date) : null,
    next_payment_date: s.next_payment_date ? parseDate(s.next_payment_date) : null,
    department_id: s.department_id ?? undefined,
    seats_total: s.type === 'group' ? s.seats_total : null,
    // expiring_soon is the scheduler's and still «active»; expired stays
    // as it is until it is renewed (back to active) or cancelled.
    status: s.status === 'expiring_soon' ? 'active' : s.status,
  }
}

// toCreateBody: individual gets its one seat and enterprise no limit from the
// server, so seats_total goes only with group.
export function toCreateBody(v: SubscriptionValues): CreateSubscriptionBody {
  const body: CreateSubscriptionBody = {
    name: v.name.trim(),
    type: v.type,
    cost: money(v.cost),
    currency: v.currency,
    billing_period: v.billing_period,
    start_date: day(v.start_date),
  }
  if (v.vendor?.trim()) body.vendor = v.vendor.trim()
  if (v.category?.trim()) body.category = v.category.trim()
  if (v.end_date) body.end_date = day(v.end_date)
  if (v.next_payment_date) body.next_payment_date = day(v.next_payment_date)
  if (v.department_id !== undefined) body.department_id = v.department_id
  if (v.type === 'group' && v.seats_total) body.seats_total = v.seats_total
  return body
}

// toPatch keeps only what changed (an empty patch is a 400). A changed type
// resets the seats on the server, so a group always sends its seats then.
export function toPatch(v: SubscriptionValues, s: SubscriptionDTO): UpdateSubscriptionBody {
  const was = valuesFromSubscription(s)
  const p: UpdateSubscriptionBody = {}
  const text = (x: string | undefined) => x?.trim() ?? ''

  if (text(v.name) !== s.name) p.name = text(v.name)
  if (text(v.vendor) !== s.vendor) p.vendor = text(v.vendor)
  if (text(v.category) !== s.category) p.category = text(v.category)
  if (v.type !== s.type) p.type = v.type
  if (money(v.cost) !== s.cost) p.cost = money(v.cost)
  if (v.currency !== s.currency) p.currency = v.currency
  if (v.billing_period !== s.billing_period) p.billing_period = v.billing_period
  if (day(v.start_date) !== s.start_date) p.start_date = day(v.start_date)
  if (optDay(v.end_date) !== s.end_date) p.end_date = optDay(v.end_date)
  if (optDay(v.next_payment_date) !== s.next_payment_date) {
    p.next_payment_date = optDay(v.next_payment_date)
  }
  if ((v.department_id ?? null) !== s.department_id) p.department_id = v.department_id ?? null
  if (v.type === 'group' && v.seats_total && (v.type !== s.type || v.seats_total !== s.seats_total)) {
    p.seats_total = v.seats_total
  }
  // Renewing an expired subscription (or bringing a cancelled one back) is
  // active; a past next payment date is moved forward by the server.
  if (v.status && v.status !== was.status && v.status !== 'expired') p.status = v.status
  return p
}
