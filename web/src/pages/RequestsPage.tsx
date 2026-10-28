import { Alert, Badge, Card, Spin, Table, Tabs, type TableProps } from 'antd'
import { useNavigate } from 'react-router'
import { useRequests } from '../api/requests'
import type { RequestDTO } from '../api/types'
import { useAuth, useMe } from '../auth/AuthContext'
import { PageHeader } from '../components/PageHeader'
import { ServiceTile } from '../components/ServiceTile'
import { RequestStatusBadge } from '../components/StatusBadge'
import { UserAvatar } from '../components/UserAvatar'
import { errorText } from '../lib/errors'
import { formatDay, formatMoney } from '../lib/format'
import { useUrlState } from '../lib/urlState'

const columns: TableProps<RequestDTO>['columns'] = [
  {
    title: 'Сервис',
    key: 'name',
    render: (_, r) => (
      <div className="cell-service">
        <ServiceTile name={r.name} size={36} />
        <div>
          <div className="cell-title">{r.name}</div>
          {r.vendor && <div className="cell-sub">{r.vendor}</div>}
        </div>
      </div>
    ),
  },
  {
    title: 'Автор',
    key: 'requester',
    render: (_, r) => (
      <div className="cell-service">
        <UserAvatar id={r.requester_id} name={r.requester_name} size={32} />
        {r.requester_name}
      </div>
    ),
  },
  {
    title: 'Стоимость',
    key: 'cost',
    render: (_, r) => formatMoney(r.estimated_cost, r.currency),
  },
  {
    title: 'Статус',
    key: 'status',
    render: (_, r) => <RequestStatusBadge status={r.status} />,
  },
  {
    title: 'Подана',
    key: 'created',
    render: (_, r) => formatDay(r.created_at),
  },
]

// RequestsPage (manager): «Входящие» — waiting for the head's decision, and
// every request of the department. A manager who is not the head sees only
// the second. The admin's version is К4.
export function RequestsPage() {
  const me = useMe()
  const { isHead, departmentLoading } = useAuth()
  const navigate = useNavigate()
  const url = useUrlState()

  const tab = isHead && url.get('tab') !== 'all' ? 'inbox' : 'all'
  const requests = useRequests({
    status: tab === 'inbox' ? 'pending_manager' : undefined,
    limit: url.pageSize,
    offset: url.offset,
  })
  // The inbox count stays on its tab while «Все» is open.
  const inbox = useRequests({ status: 'pending_manager', limit: 1 })

  if (departmentLoading) {
    return (
      <div className="page-spin">
        <Spin size="large" />
      </div>
    )
  }

  const table = (
    <Card variant="borderless" styles={{ body: { padding: 0 } }}>
      <Table<RequestDTO>
        rowKey="id"
        columns={columns}
        dataSource={requests.data?.items}
        loading={requests.isFetching}
        className="table-clickable"
        locale={{ emptyText: tab === 'inbox' ? 'Новых заявок нет' : 'Заявок пока нет' }}
        onRow={(r) => ({ onClick: () => navigate(`/requests/${r.id}`) })}
        pagination={url.pagination(requests.data?.total)}
      />
    </Card>
  )

  return (
    <>
      <PageHeader
        title="Заявки отдела"
        subtitle={
          isHead
            ? 'Заявки сотрудников ждут вашего решения, затем уходят администратору'
            : me.department_id === null
              ? 'Вы не привязаны к отделу: здесь только ваши заявки'
              : 'Решения по заявкам принимает начальник отдела'
        }
      />
      {requests.error && (
        <Alert type="error" showIcon title={errorText(requests.error)} style={{ marginBottom: 24 }} />
      )}
      {isHead ? (
        <Tabs
          activeKey={tab}
          onChange={(key) => url.set({ tab: key === 'all' ? 'all' : null })}
          items={[
            {
              key: 'inbox',
              label: (
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                  Входящие <Badge count={inbox.data?.total ?? 0} size="small" />
                </span>
              ),
              children: table,
            },
            { key: 'all', label: 'Все заявки отдела', children: table },
          ]}
        />
      ) : (
        table
      )}
    </>
  )
}
