import {
  CheckCircleOutlined,
  ClockCircleOutlined,
  CloseCircleOutlined,
  CreditCardOutlined,
  FileAddOutlined,
  RollbackOutlined,
  StopOutlined,
} from '@ant-design/icons'
import type { ReactNode } from 'react'
import type { NotificationDTO, NotificationType } from '../api/types'
import { fromNow } from '../lib/format'
import { tones, type Tone } from '../theme'

const look: Record<NotificationType, { icon: ReactNode; tone: Tone }> = {
  subscription_expiring: { icon: <ClockCircleOutlined />, tone: 'warning' },
  subscription_expired: { icon: <StopOutlined />, tone: 'error' },
  payment_due: { icon: <CreditCardOutlined />, tone: 'info' },
  request_created: { icon: <FileAddOutlined />, tone: 'accent' },
  request_approved: { icon: <CheckCircleOutlined />, tone: 'success' },
  request_rejected: { icon: <CloseCircleOutlined />, tone: 'error' },
  release_created: { icon: <RollbackOutlined />, tone: 'warning' },
  release_approved: { icon: <CheckCircleOutlined />, tone: 'success' },
  release_rejected: { icon: <CloseCircleOutlined />, tone: 'error' },
}

// NotificationItem is one row of the bell popup and of the notifications page.
export function NotificationItem({
  item,
  compact = false,
  onClick,
}: {
  item: NotificationDTO
  compact?: boolean
  onClick: (item: NotificationDTO) => void
}) {
  const { icon, tone } = look[item.type] ?? { icon: <ClockCircleOutlined />, tone: 'muted' }
  const { color, bg } = tones[tone]
  return (
    <button
      type="button"
      className={`notice${item.is_read ? '' : ' notice-unread'}${compact ? ' notice-compact' : ''}`}
      onClick={() => onClick(item)}
    >
      <span className="notice-icon" style={{ color, background: bg }}>
        {icon}
      </span>
      <span className="notice-text">
        <span className="notice-title">{item.title}</span>
        {item.body && <span className="notice-body">{item.body}</span>}
        <span className="notice-time">{fromNow(item.created_at)}</span>
      </span>
      {!item.is_read && <span className="notice-dot" aria-label="Не прочитано" />}
    </button>
  )
}
