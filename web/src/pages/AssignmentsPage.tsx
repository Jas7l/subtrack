import { PlusOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Select, Table, type TableProps } from 'antd'
import { useState } from 'react'
import { Link } from 'react-router'
import { useAssignments } from '../api/assignments'
import type { AssignmentDTO, AssignmentStatus } from '../api/types'
import { GrantLicenceModal } from '../components/GrantLicenceModal'
import { PageHeader } from '../components/PageHeader'
import { RevokeLicenceButton } from '../components/RevokeLicenceButton'
import { ServiceTile } from '../components/ServiceTile'
import { StatusBadge } from '../components/StatusBadge'
import { SubscriptionSelect } from '../components/SubscriptionSelect'
import { UserAvatar } from '../components/UserAvatar'
import { UserSelect } from '../components/UserSelect'
import { errorText } from '../lib/errors'
import { formatDay } from '../lib/format'
import { assignmentStatusLabels } from '../lib/labels'
import { useUrlState } from '../lib/urlState'

const columns: TableProps<AssignmentDTO>['columns'] = [
  {
    title: 'Подписка',
    key: 'subscription',
    render: (_, a) => (
      <Link to={`/subscriptions/${a.subscription_id}`} className="cell-service">
        <ServiceTile name={a.subscription_name} size={32} />
        <span className="cell-title">{a.subscription_name}</span>
      </Link>
    ),
  },
  {
    title: 'Сотрудник',
    key: 'user',
    render: (_, a) => (
      <Link to={`/users/${a.user_id}`} className="cell-service">
        <UserAvatar id={a.user_id} name={a.user_name} size={32} />
        {a.user_name}
      </Link>
    ),
  },
  {
    title: 'Выдал(а)',
    key: 'assigned',
    render: (_, a) => <Who name={a.assigned_by_name} when={a.assigned_at} />,
  },
  {
    title: 'Отозвал(а)',
    key: 'revoked',
    render: (_, a) =>
      a.revoked_at ? (
        <Who name={a.revoked_by_name} when={a.revoked_at} />
      ) : (
        <span className="cell-sub">—</span>
      ),
  },
  {
    title: 'Статус',
    key: 'status',
    render: (_, a) => (
      <StatusBadge tone={a.status === 'active' ? 'success' : 'muted'}>
        {assignmentStatusLabels[a.status]}
      </StatusBadge>
    ),
  },
  {
    key: 'actions',
    align: 'right',
    render: (_, a) => a.status === 'active' && <RevokeLicenceButton assignment={a} />,
  },
]

// Who did it and when. A licence revoked by blocking its holder or
// cancelling the subscription names the admin who did that; there is no
// name only if that user row is gone.
function Who({ name, when }: { name: string | null; when: string }) {
  return (
    <div>
      <div>{name ?? '—'}</div>
      <div className="cell-sub">{formatDay(when)}</div>
    </div>
  )
}

const statusOptions = (Object.keys(assignmentStatusLabels) as AssignmentStatus[]).map((s) => ({
  value: s,
  label: assignmentStatusLabels[s],
}))

// AssignmentsPage (admin): every licence, active and revoked, with filters
// by subscription, user and status; «Выдать» picks both.
export function AssignmentsPage() {
  const url = useUrlState()
  const [granting, setGranting] = useState(false)
  const filters = {
    subscription_id: url.get('subscription_id'),
    user_id: url.get('user_id'),
    status: url.get('status'),
  }
  const filtered = Object.values(filters).some(Boolean)
  const assignments = useAssignments({ ...filters, limit: url.pageSize, offset: url.offset })

  return (
    <>
      <PageHeader
        title="Лицензии"
        subtitle="Кому выданы места в подписках"
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setGranting(true)}>
            Выдать лицензию
          </Button>
        }
      />
      <div className="filters">
        <SubscriptionSelect
          allowClear
          value={Number(filters.subscription_id) || undefined}
          onChange={(id) => url.set({ subscription_id: id ?? null })}
          placeholder="Любая подписка"
          style={{ width: 280 }}
        />
        <UserSelect
          allowClear
          includeBlocked
          value={Number(filters.user_id) || undefined}
          onChange={(id) => url.set({ user_id: id ?? null })}
          placeholder="Любой сотрудник"
          style={{ width: 280 }}
        />
        <Select
          allowClear
          placeholder="Любой статус"
          value={filters.status || undefined}
          options={statusOptions}
          onChange={(v?: string) => url.set({ status: v ?? null })}
          style={{ width: 180 }}
        />
      </div>
      {assignments.error && (
        <Alert
          type="error"
          showIcon
          title={errorText(assignments.error)}
          style={{ marginBottom: 24 }}
        />
      )}
      <Card variant="borderless" styles={{ body: { padding: 0 } }}>
        <Table<AssignmentDTO>
          rowKey="id"
          columns={columns}
          dataSource={assignments.data?.items}
          loading={assignments.isFetching}
          locale={{ emptyText: filtered ? 'Ничего не нашли' : 'Лицензий пока нет' }}
          pagination={url.pagination(assignments.data?.total)}
        />
      </Card>
      <GrantLicenceModal open={granting} onClose={() => setGranting(false)} />
    </>
  )
}
