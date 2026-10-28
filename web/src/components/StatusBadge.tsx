import type { ReactNode } from 'react'
import type { RequestStatus, SubscriptionStatus, TransactionStatus } from '../api/types'
import {
  requestStatusLabels,
  subscriptionStatusLabels,
  transactionStatusLabels,
} from '../lib/labels'
import { tones, type Tone } from '../theme'

// StatusBadge is the «pill»: a soft background of the status colour and the
// text with a dot of the same colour (CLIENT_PLAN.md, section 11).
export function StatusBadge({ tone, children }: { tone: Tone; children: ReactNode }) {
  const { color, bg } = tones[tone]
  return (
    <span className="status-badge" style={{ color, background: bg }}>
      <span className="status-badge-dot" style={{ background: color }} />
      {children}
    </span>
  )
}

const subscriptionTones: Record<SubscriptionStatus, Tone> = {
  active: 'success',
  expiring_soon: 'warning',
  expired: 'error',
  cancelled: 'muted',
}

export function SubscriptionStatusBadge({ status }: { status: SubscriptionStatus }) {
  return <StatusBadge tone={subscriptionTones[status]}>{subscriptionStatusLabels[status]}</StatusBadge>
}

const requestTones: Record<RequestStatus, Tone> = {
  pending_manager: 'warning',
  pending_admin: 'info',
  approved: 'success',
  rejected_by_manager: 'error',
  rejected_by_admin: 'error',
  cancelled: 'muted',
}

export function RequestStatusBadge({ status }: { status: RequestStatus }) {
  return <StatusBadge tone={requestTones[status]}>{requestStatusLabels[status]}</StatusBadge>
}

const transactionTones: Record<TransactionStatus, Tone> = {
  pending: 'warning',
  paid: 'success',
  failed: 'error',
}

export function TransactionStatusBadge({ status }: { status: TransactionStatus }) {
  return (
    <StatusBadge tone={transactionTones[status]}>{transactionStatusLabels[status]}</StatusBadge>
  )
}

export function UserStatusBadge({ active }: { active: boolean }) {
  return active ? (
    <StatusBadge tone="success">Активен</StatusBadge>
  ) : (
    <StatusBadge tone="muted">Заблокирован</StatusBadge>
  )
}
