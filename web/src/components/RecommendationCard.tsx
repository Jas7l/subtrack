import {
  AppstoreAddOutlined,
  CalendarOutlined,
  CopyOutlined,
  ExclamationCircleOutlined,
  TeamOutlined,
  UsergroupAddOutlined,
  UserDeleteOutlined,
} from '@ant-design/icons'
import type { ReactNode } from 'react'
import type { RecommendationDTO, RecommendationType } from '../api/types'
import { formatMoney } from '../lib/format'
import { tones, type Tone } from '../theme'
import { SubscriptionLink } from './SubscriptionLink'

// The icon, tone and short label of each rule of the server.
const kinds: Record<RecommendationType, { icon: ReactNode; tone: Tone; label: string }> = {
  unused_seats: { icon: <TeamOutlined />, tone: 'accent', label: 'Лишние места' },
  no_holders: { icon: <UserDeleteOutlined />, tone: 'warning', label: 'Никто не пользуется' },
  duplicate_vendor: { icon: <CopyOutlined />, tone: 'info', label: 'Один вендор' },
  individual_to_group: {
    icon: <UsergroupAddOutlined />,
    tone: 'info',
    label: 'Объединить в групповую',
  },
  enterprise_low_usage: {
    icon: <AppstoreAddOutlined />,
    tone: 'warning',
    label: 'Безлимит используют мало',
  },
  annual_billing: { icon: <CalendarOutlined />, tone: 'success', label: 'Годовая оплата' },
  expired_with_holders: {
    icon: <ExclamationCircleOutlined />,
    tone: 'error',
    label: 'Истекла, а лицензии выданы',
  },
}

// RecommendationCard: one piece of advice. The compact one (the dashboard)
// has no subscription links: the whole list is a click away.
export function RecommendationCard({
  item,
  compact = false,
}: {
  item: RecommendationDTO
  compact?: boolean
}) {
  const kind = kinds[item.type] ?? { icon: <ExclamationCircleOutlined />, tone: 'muted', label: '' }
  const { color, bg } = tones[kind.tone]
  return (
    <div className={compact ? 'rec rec-compact' : 'rec'}>
      <span className="rec-icon" style={{ color, background: bg }}>
        {kind.icon}
      </span>
      <div className="rec-body">
        {!compact && kind.label && (
          <div className="rec-kind" style={{ color }}>
            {kind.label}
          </div>
        )}
        <div className="cell-title">{item.title}</div>
        <div className="rec-text">{item.text}</div>
        {!compact && item.subscription_ids.length > 0 && (
          <div className="rec-links">
            {item.subscription_ids.map((id) => (
              <SubscriptionLink key={id} id={id} />
            ))}
          </div>
        )}
      </div>
      <div className="rec-savings">
        {item.savings_rub_year ? (
          <>
            <div className="rec-savings-value">{formatMoney(item.savings_rub_year)}</div>
            <div className="cell-sub">в год</div>
          </>
        ) : (
          <div className="cell-sub">оценить нельзя</div>
        )}
      </div>
    </div>
  )
}
