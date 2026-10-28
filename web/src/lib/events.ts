import type { SubscriptionDTO, TransactionDTO } from '../api/types'
import { daysLeft, formatMoney, plural } from './format'

// A line of «Ближайшие события» on the admin's dashboard.
export interface DashEvent {
  key: string
  tone: 'error' | 'warning' | 'muted'
  text: string
  days: number
  to: string
}

// How far the dashboard looks ahead for payments and back for expiries.
const PAYMENTS_AHEAD_DAYS = 14
const EXPIRED_BACK_DAYS = 30

// whenText: «сегодня», «через 3 дня», «5 дней назад».
export function whenText(days: number): string {
  if (days === 0) return 'сегодня'
  const n = Math.abs(days)
  const word = `${n} ${plural(n, 'день', 'дня', 'дней')}`
  return days > 0 ? `через ${word}` : `${word} назад`
}

// dashEvents puts together what needs the admin's attention soon: overdue
// payments first, then upcoming payments (by next_payment_date; pending
// payments appear only once that date has come) and expiring subscriptions
// by date, then subscriptions expired lately, the latest first. Identical
// lines (individual copies of one product paid the same day) become one
// with «×N».
export function dashEvents(subs: SubscriptionDTO[], pending: TransactionDTO[]): DashEvent[] {
  const overdue: DashEvent[] = []
  const soon: DashEvent[] = []
  const past: DashEvent[] = []

  for (const t of pending) {
    const days = daysLeft(t.payment_date)
    ;(days < 0 ? overdue : soon).push({
      key: `t${t.id}`,
      tone: days < 0 ? 'error' : 'warning',
      text: `${t.subscription_name} — ${days < 0 ? 'платёж просрочен' : `платёж ${formatMoney(t.amount, t.currency)}`}`,
      days,
      to: '/transactions',
    })
  }
  for (const s of subs) {
    const open = s.status === 'active' || s.status === 'expiring_soon'
    if (open && s.next_payment_date) {
      const days = daysLeft(s.next_payment_date)
      if (days >= 0 && days <= PAYMENTS_AHEAD_DAYS) {
        soon.push({
          key: `p${s.id}`,
          tone: 'warning',
          text: `${s.name} — платёж ${formatMoney(s.cost, s.currency)}`,
          days,
          to: `/subscriptions/${s.id}`,
        })
      }
    }
    if (s.status === 'expiring_soon' && s.end_date) {
      soon.push({
        key: `e${s.id}`,
        tone: 'warning',
        text: `${s.name} — подписка заканчивается`,
        days: daysLeft(s.end_date),
        to: `/subscriptions/${s.id}`,
      })
    }
    if (s.status === 'expired' && s.end_date) {
      const days = daysLeft(s.end_date)
      if (days >= -EXPIRED_BACK_DAYS) {
        past.push({
          key: `x${s.id}`,
          tone: 'muted',
          text: `${s.name} — подписка истекла`,
          days,
          to: `/subscriptions/${s.id}`,
        })
      }
    }
  }
  overdue.sort((a, b) => a.days - b.days)
  soon.sort((a, b) => a.days - b.days)
  past.sort((a, b) => b.days - a.days)
  return merge([...overdue, ...soon, ...past])
}

function merge(events: DashEvent[]): DashEvent[] {
  const seen = new Map<string, { event: DashEvent; count: number }>()
  for (const e of events) {
    const key = `${e.text}|${e.days}`
    const same = seen.get(key)
    if (same) same.count++
    else seen.set(key, { event: e, count: 1 })
  }
  return [...seen.values()].map(({ event, count }) =>
    count > 1 ? { ...event, text: `${event.text} ×${count}` } : event,
  )
}
