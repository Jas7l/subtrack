import { Alert, Badge, Button, Card, Select, Spin, Table, Tabs, type TableProps } from 'antd'
import { useNavigate } from 'react-router'
import { useHeadlessRequests, useRequests } from '../api/requests'
import type { RequestDTO, RequestStatus } from '../api/types'
import { useAuth, useMe } from '../auth/AuthContext'
import { DepartmentSelect } from '../components/DepartmentSelect'
import { PageHeader } from '../components/PageHeader'
import { ServiceTile } from '../components/ServiceTile'
import { RequestStatusBadge } from '../components/StatusBadge'
import { UserAvatar } from '../components/UserAvatar'
import { UserSelect } from '../components/UserSelect'
import { errorText } from '../lib/errors'
import { formatDay, formatMoney, plural } from '../lib/format'
import { requestStatusLabels } from '../lib/labels'
import { inboxStatus } from '../lib/permissions'
import { useLastPage, useUrlState } from '../lib/urlState'

const columns: NonNullable<TableProps<RequestDTO>['columns']> = [
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
    title: 'Отдел',
    key: 'department',
    render: (_, r) => r.department_name ?? <span className="cell-sub">—</span>,
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

const statusOptions = (Object.keys(requestStatusLabels) as RequestStatus[]).map((s) => ({
  value: s,
  label: requestStatusLabels[s],
}))

// RequestsPage is one page for both roles. «Входящие» / «Ждут меня» are the
// requests waiting for the caller's decision (pending_manager for a
// department head, pending_admin for the admin); the second tab is every
// request the server shows — the department's for a manager, all for the
// admin, with filters by status, department and requester. A manager who is
// not a head sees only the second, without tabs.
export function RequestsPage() {
  const me = useMe()
  const admin = me.role === 'admin'
  const { isHead, departmentLoading } = useAuth()
  const navigate = useNavigate()
  const url = useUrlState()

  const waiting = inboxStatus(me, isHead)
  const tab = waiting !== null && url.get('tab') !== 'all' ? 'inbox' : 'all'
  const filters: Record<string, string> = {}
  if (admin && tab === 'all') {
    filters.status = url.get('status')
    filters.department_id = url.get('department_id')
    filters.requester_id = url.get('requester_id')
  }
  const filtered = Object.values(filters).some(Boolean)
  const requests = useRequests({
    ...filters,
    status: tab === 'inbox' ? (waiting ?? undefined) : filters.status,
    limit: url.pageSize,
    offset: url.offset,
  })
  useLastPage(url, requests.data?.total)
  // The inbox count stays on its tab while «Все» is open.
  const inbox = useRequests({ status: waiting ?? undefined, limit: 1 })
  const headless = useHeadlessRequests(admin)

  if (departmentLoading) {
    return (
      <div className="page-spin">
        <Spin size="large" />
      </div>
    )
  }

  const shown = admin ? columns : columns.filter((c) => c.key !== 'department')
  // pending_manager nobody else can decide (no head, or the head made the
  // request) wait for the admin too; «Все» shows them (by department when
  // they are all in one).
  const n = headless.count
  const headlessNote = n > 0 && (
    <Alert
      type="warning"
      showIcon
      title={`${n} ${plural(n, 'заявка ждёт', 'заявки ждут', 'заявок ждут')} решения начальника отдела, но решить некому: начальника нет или это его собственная заявка. Решение за вами`}
      action={
        <Button
          size="small"
          onClick={() =>
            url.set({
              tab: 'all',
              status: 'pending_manager',
              department_id: headless.departments.length === 1 ? headless.departments[0] : null,
            })
          }
        >
          Показать
        </Button>
      }
      style={{ marginBottom: 16 }}
    />
  )
  const table = (
    <>
      {admin && tab === 'all' && (
        <div className="filters">
          <Select
            allowClear
            placeholder="Любой статус"
            value={filters.status || undefined}
            options={statusOptions}
            onChange={(v?: string) => url.set({ status: v ?? null })}
            style={{ width: 240 }}
          />
          <DepartmentSelect
            value={Number(filters.department_id) || undefined}
            onChange={(id) => url.set({ department_id: id ?? null })}
            placeholder="Любой отдел"
            style={{ width: 200 }}
          />
          <UserSelect
            allowClear
            includeBlocked
            value={Number(filters.requester_id) || undefined}
            onChange={(id) => url.set({ requester_id: id ?? null })}
            placeholder="Любой автор"
            style={{ width: 280 }}
          />
        </div>
      )}
      <Card variant="borderless" styles={{ body: { padding: 0 } }}>
        <Table<RequestDTO>
          rowKey="id"
          columns={shown}
          dataSource={requests.data?.items}
          loading={requests.isFetching}
          className="table-clickable"
          locale={{
            emptyText:
              tab === 'inbox' ? 'Новых заявок нет' : filtered ? 'Ничего не нашли' : 'Заявок пока нет',
          }}
          onRow={(r) => ({ onClick: () => navigate(`/requests/${r.id}`) })}
          pagination={url.pagination(requests.data?.total)}
        />
      </Card>
    </>
  )

  const subtitle = admin
    ? 'Заявки, согласованные начальниками, ждут вашего финального решения'
    : isHead
      ? 'Заявки сотрудников ждут вашего решения, затем уходят администратору'
      : me.department_id === null
        ? 'Вы не привязаны к отделу: здесь только ваши заявки'
        : 'Решения по заявкам принимает начальник отдела'

  return (
    <>
      <PageHeader title={admin ? 'Все заявки' : 'Заявки отдела'} subtitle={subtitle} />
      {requests.error && (
        <Alert type="error" showIcon title={errorText(requests.error)} style={{ marginBottom: 24 }} />
      )}
      {waiting !== null ? (
        <Tabs
          activeKey={tab}
          // Switching tabs drops the filters of «Все».
          onChange={(key) =>
            url.set({
              tab: key === 'all' ? 'all' : null,
              status: null,
              department_id: null,
              requester_id: null,
            })
          }
          items={[
            {
              key: 'inbox',
              label: (
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                  {admin ? 'Ждут меня' : 'Входящие'}{' '}
                  <Badge count={(inbox.data?.total ?? 0) + headless.count} size="small" />
                </span>
              ),
              children: (
                <>
                  {headlessNote}
                  {table}
                </>
              ),
            },
            { key: 'all', label: admin ? 'Все' : 'Все заявки отдела', children: table },
          ]}
        />
      ) : (
        table
      )}
    </>
  )
}
