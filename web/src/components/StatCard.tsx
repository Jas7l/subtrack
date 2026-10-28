import { Card } from 'antd'
import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { tones, type Tone } from '../theme'

// StatCard is one number of a dashboard: an icon on a soft tone, the value
// and a caption. `to` makes the whole card a link; `highlight` paints it in
// the soft tone (the «Можно сэкономить» card).
export function StatCard({
  icon,
  tone = 'accent',
  label,
  value,
  hint,
  to,
  highlight = false,
}: {
  icon: ReactNode
  tone?: Tone
  label: string
  value: ReactNode
  hint?: ReactNode
  to?: string
  highlight?: boolean
}) {
  const { color, bg } = tones[tone]
  const card = (
    <Card
      variant="borderless"
      className={highlight ? 'stat-card stat-card-highlight' : 'stat-card'}
      style={highlight ? { background: bg } : undefined}
    >
      <span className="stat-icon" style={{ color, background: highlight ? 'transparent' : bg }}>
        {icon}
      </span>
      <div className="stat-label">{label}</div>
      <div className="stat-value" style={highlight ? { color } : undefined}>
        {value}
      </div>
      {hint && <div className="stat-hint">{hint}</div>}
    </Card>
  )
  return to ? (
    <Link to={to} className="stat-card-link">
      {card}
    </Link>
  ) : (
    card
  )
}
