import type { TransactionDTO } from '../api/types'
import { daysLeft } from './format'

// isOverdue: a pending payment whose date has passed.
export const isOverdue = (t: TransactionDTO) =>
  t.status === 'pending' && daysLeft(t.payment_date) < 0

// Suggestions for the payment method; any text is accepted.
export const paymentMethods = ['Корпоративная карта', 'Счёт', 'Банковский перевод'].map(
  (value) => ({
    value,
  }),
)
