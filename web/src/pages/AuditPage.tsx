import { RobotOutlined } from '@ant-design/icons'
import { Alert, Button, Card, DatePicker, Select, Table, type TableProps } from 'antd'
import { Link } from 'react-router'
import { useAudit } from '../api/audit'
import { useDepartmentNames } from '../api/departments'
import type { AuditEntityType, AuditEntryDTO } from '../api/types'
import { AuditDetails } from '../components/AuditDetails'
import { PageHeader } from '../components/PageHeader'
import { StatusBadge } from '../components/StatusBadge'
import { UserAvatar } from '../components/UserAvatar'
import { UserSelect } from '../components/UserSelect'
import {
  actionLabels,
  actionText,
  actionTone,
  detailRows,
  detailSummary,
  entityLabels,
  entityLink,
  entityText,
} from '../lib/audit'
import { errorText } from '../lib/errors'
import { formatMoment, parseDate } from '../lib/format'
import { useUrlState } from '../lib/urlState'

const entityOptions = (Object.keys(entityLabels) as AuditEntityType[]).map((t) => ({
  value: t,
  label: entityLabels[t],
}))

// AuditPage (admin): who changed what, newest first. A row opens its
// details; the object links to its page. Filters live in the address: the
// user card opens the person's actions (?actor_id=), the subscription card its
// history (?entity_type=subscription&entity_id=).
export function AuditPage() {
  const url = useUrlState()
  const departments = useDepartmentNames()
  const filters = {
    actor_id: url.get('actor_id'),
    entity_type: url.get('entity_type'),
    entity_id: url.get('entity_id'),
    action: url.get('action'),
    from: url.get('from'),
    to: url.get('to'),
  }
  const filtered = Object.values(filters).some(Boolean)
  const audit = useAudit({ ...filters, limit: url.pageSize, offset: url.offset })

  // The actions of the chosen entity type, or all of them.
  const actionOptions = Object.keys(actionLabels)
    .filter((a) => !filters.entity_type || a.startsWith(`${filters.entity_type}.`))
    .map((a) => ({ value: a, label: actionLabels[a] }))

  const columns: TableProps<AuditEntryDTO>['columns'] = [
    {
      title: 'Когда',
      key: 'when',
      width: 150,
      render: (_, e) => <span className="cell-money">{formatMoment(e.created_at)}</span>,
    },
    {
      title: 'Кто',
      key: 'actor',
      render: (_, e) =>
        e.actor_id === null ? (
          <span className="cell-service cell-sub">
            <span className="system-avatar">
              <RobotOutlined />
            </span>
            Система
          </span>
        ) : (
          <Link to={`/users/${e.actor_id}`} className="cell-service">
            <UserAvatar id={e.actor_id} name={e.actor_name ?? '?'} size={28} />
            {e.actor_name ?? `#${e.actor_id}`}
          </Link>
        ),
    },
    {
      title: 'Действие',
      key: 'action',
      render: (_, e) => <StatusBadge tone={actionTone(e.action)}>{actionText(e.action)}</StatusBadge>,
    },
    {
      title: 'Объект',
      key: 'entity',
      render: (_, e) => {
        const to = entityLink(e)
        const text = entityText(e)
        return to ? <Link to={to}>{text}</Link> : text
      },
    },
    {
      title: 'Что изменилось',
      key: 'details',
      ellipsis: true,
      render: (_, e) => (
        <span className="cell-sub">{detailSummary(detailRows(e, departments)) || '—'}</span>
      ),
    },
  ]

  return (
    <>
      <PageHeader title="Аудит" subtitle="Журнал действий пользователей и системы" />
      <div className="filters">
        <UserSelect
          allowClear
          includeBlocked
          value={Number(filters.actor_id) || undefined}
          onChange={(id) => url.set({ actor_id: id ?? null })}
          placeholder="Любой пользователь"
          style={{ width: 260 }}
        />
        <Select
          allowClear
          placeholder="Любой объект"
          value={filters.entity_type || undefined}
          options={entityOptions}
          // Another type drops the action and the object of the old one.
          onChange={(v?: string) => url.set({ entity_type: v ?? null, action: null, entity_id: null })}
          style={{ width: 180 }}
        />
        <Select
          allowClear
          placeholder="Любое действие"
          value={filters.action || undefined}
          options={actionOptions}
          onChange={(v?: string) => url.set({ action: v ?? null })}
          style={{ width: 220 }}
        />
        <DatePicker.RangePicker
          allowEmpty={[true, true]}
          format="DD.MM.YYYY"
          placeholder={['Дата с', 'по']}
          value={[
            filters.from ? parseDate(filters.from) : null,
            filters.to ? parseDate(filters.to) : null,
          ]}
          onChange={(range) =>
            url.set({
              from: range?.[0]?.format('YYYY-MM-DD') ?? null,
              to: range?.[1]?.format('YYYY-MM-DD') ?? null,
            })
          }
        />
      </div>
      {filters.entity_id && (
        <Alert
          type="info"
          showIcon
          title={`Показана история одного объекта: ${entityLabels[filters.entity_type as AuditEntityType] ?? 'объект'} #${filters.entity_id}`}
          action={
            <Button size="small" type="link" onClick={() => url.set({ entity_id: null })}>
              Показать все
            </Button>
          }
          style={{ marginBottom: 16 }}
        />
      )}
      {audit.error && (
        <Alert type="error" showIcon title={errorText(audit.error)} style={{ marginBottom: 24 }} />
      )}
      <Card variant="borderless" styles={{ body: { padding: 0 } }}>
        <Table<AuditEntryDTO>
          rowKey="id"
          columns={columns}
          dataSource={audit.data?.items}
          loading={audit.isFetching}
          expandable={{
            expandedRowRender: (e) => <AuditDetails rows={detailRows(e, departments)} />,
            rowExpandable: (e) => Object.keys(e.details).length > 0,
            expandRowByClick: true,
          }}
          locale={{ emptyText: filtered ? 'Ничего не нашли' : 'Записей пока нет' }}
          pagination={url.pagination(audit.data?.total)}
        />
      </Card>
    </>
  )
}
