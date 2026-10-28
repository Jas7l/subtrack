import { Link } from 'react-router'
import type { DetailRow } from '../lib/audit'

// AuditDetails: what a record changed, a line per field.
export function AuditDetails({ rows }: { rows: DetailRow[] }) {
  if (rows.length === 0) return <span className="cell-sub">Без подробностей</span>
  return (
    <dl className="audit-details">
      {rows.map((r) => (
        <div key={r.key} className="audit-detail">
          <dt>{r.label}</dt>
          <dd>{r.to ? <Link to={r.to}>{r.value}</Link> : r.value}</dd>
        </div>
      ))}
    </dl>
  )
}
