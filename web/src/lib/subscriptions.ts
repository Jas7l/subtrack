import type { SubscriptionDTO } from '../api/types'
import { formatMoney } from './format'
import { billingPeriodLabels } from './labels'

// Seats as «used / total»; enterprise has no limit.
export function seatsText(s: SubscriptionDTO): string {
  return s.seats_total === null ? `${s.seats_used} · безлимит` : `${s.seats_used} / ${s.seats_total}`
}

// «15 000 ₽ / месяц»
export function costText(s: SubscriptionDTO): string {
  return `${formatMoney(s.cost, s.currency)} / ${billingPeriodLabels[s.billing_period].toLowerCase()}`
}

// Licences can be handed out only while the subscription is open.
export const isOpen = (s: SubscriptionDTO) => s.status === 'active' || s.status === 'expiring_soon'
