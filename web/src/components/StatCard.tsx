import { Card } from 'antd'
import type { ReactNode } from 'react'
import { tones, type Tone } from '../theme'

// StatCard is one number of a dashboard: an icon on a soft tone, the value
// and a caption.
export function StatCard({
  icon,
  tone = 'accent',
  label,
  value,
  hint,
}: {
  icon: ReactNode
  tone?: Tone
  label: string
  value: ReactNode
  hint?: ReactNode
}) {
  const { color, bg } = tones[tone]
  return (
    <Card variant="borderless" className="stat-card">
      <span className="stat-icon" style={{ color, background: bg }}>
        {icon}
      </span>
      <div className="stat-label">{label}</div>
      <div className="stat-value">{value}</div>
      {hint && <div className="stat-hint">{hint}</div>}
    </Card>
  )
}
