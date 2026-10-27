import type { ReactNode } from 'react'

export function PageHeader({
  title,
  subtitle,
  extra,
}: {
  title: ReactNode
  subtitle?: ReactNode
  extra?: ReactNode
}) {
  return (
    <div className="page-header">
      <div>
        <h1 className="page-title">{title}</h1>
        {subtitle && <p className="page-subtitle">{subtitle}</p>}
      </div>
      {extra && <div style={{ display: 'flex', flexWrap: 'wrap', gap: 10 }}>{extra}</div>}
    </div>
  )
}
