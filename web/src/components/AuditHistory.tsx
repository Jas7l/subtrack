import { Alert, Card, Empty, Spin } from 'antd'
import { Link } from 'react-router'
import { useAudit } from '../api/audit'
import type { AuditEntityType } from '../api/types'
import { useDepartmentNames } from '../api/departments'
import { actionText, actionTone, detailRows, detailSummary } from '../lib/audit'
import { errorText } from '../lib/errors'
import { formatMoment } from '../lib/format'
import { tones } from '../theme'

const LIMIT = 10

// AuditHistory: the latest changes of one object (who, when, what), with a
// link to its whole history in the audit log. The admin only.
export function AuditHistory({ entityType, entityId }: { entityType: AuditEntityType; entityId: number }) {
  const query = { entity_type: entityType, entity_id: entityId }
  const audit = useAudit({ ...query, limit: LIMIT })
  const departments = useDepartmentNames()
  const items = audit.data?.items ?? []
  const total = audit.data?.total ?? 0

  return (
    <Card
      variant="borderless"
      title={`История изменений · ${total}`}
      extra={
        total > 0 && (
          <Link to={`/audit?entity_type=${entityType}&entity_id=${entityId}`}>Весь журнал →</Link>
        )
      }
      style={{ marginTop: 24 }}
    >
      {audit.error ? (
        <Alert type="error" showIcon title={errorText(audit.error)} />
      ) : audit.isPending ? (
        <Spin />
      ) : items.length === 0 ? (
        <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="Изменений не было" />
      ) : (
        <ol className="history">
          {items.map((e) => {
            const summary = detailSummary(detailRows(e, departments))
            return (
              <li key={e.id} className="history-item">
                <span className="event-dot" style={{ background: tones[actionTone(e.action)].color }} />
                <div className="history-body">
                  <div>
                    <span className="cell-title">{actionText(e.action)}</span>
                    {summary && <span className="history-summary"> · {summary}</span>}
                  </div>
                  <div className="cell-sub">
                    {e.actor_name ?? 'Система'} · {formatMoment(e.created_at)}
                  </div>
                </div>
              </li>
            )
          })}
        </ol>
      )}
    </Card>
  )
}
