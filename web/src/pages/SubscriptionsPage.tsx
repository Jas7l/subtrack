import { SearchOutlined } from '@ant-design/icons'
import { Alert, Card, Input, Select, Table, type TableProps } from 'antd'
import { useNavigate } from 'react-router'
import { useSubscriptions } from '../api/subscriptions'
import type { SubscriptionDTO, SubscriptionStatus } from '../api/types'
import { PageHeader } from '../components/PageHeader'
import { ServiceTile } from '../components/ServiceTile'
import { SubscriptionStatusBadge } from '../components/StatusBadge'
import { errorText } from '../lib/errors'
import { formatDate } from '../lib/format'
import { subscriptionStatusLabels, subscriptionTypeLabels } from '../lib/labels'
import { costText, seatsText } from '../lib/subscriptions'
import { useUrlState } from '../lib/urlState'

// No sortable columns: the API has no sort parameter.
const columns: TableProps<SubscriptionDTO>['columns'] = [
  {
    title: 'Сервис',
    key: 'name',
    render: (_, s) => (
      <div className="cell-service">
        <ServiceTile name={s.name} size={36} />
        <div>
          <div className="cell-title">{s.name}</div>
          {s.vendor && <div className="cell-sub">{s.vendor}</div>}
        </div>
      </div>
    ),
  },
  {
    title: 'Отдел',
    key: 'department',
    render: (_, s) => s.department_name ?? <span className="cell-sub">Общая</span>,
  },
  { title: 'Тип', key: 'type', render: (_, s) => subscriptionTypeLabels[s.type] },
  { title: 'Места', key: 'seats', render: (_, s) => seatsText(s) },
  { title: 'Стоимость', key: 'cost', render: (_, s) => costText(s) },
  { title: 'Окончание', key: 'end', render: (_, s) => formatDate(s.end_date) },
  {
    title: 'Статус',
    key: 'status',
    render: (_, s) => <SubscriptionStatusBadge status={s.status} />,
  },
]

const statusOptions = (Object.keys(subscriptionStatusLabels) as SubscriptionStatus[]).map((s) => ({
  value: s,
  label: subscriptionStatusLabels[s],
}))

// SubscriptionsPage (manager): the department's subscriptions and the
// company-wide ones the manager holds a licence of — the server decides.
// The admin's registry with CRUD is К4.
export function SubscriptionsPage() {
  const navigate = useNavigate()
  const url = useUrlState()
  const q = url.get('q')
  const status = url.get('status')

  const subscriptions = useSubscriptions({
    q,
    status,
    limit: url.pageSize,
    offset: url.offset,
  })

  return (
    <>
      <PageHeader
        title="Подписки отдела"
        subtitle="Подписки вашего отдела и общие, которыми пользуетесь вы"
      />
      <div className="filters">
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="Название или вендор"
          defaultValue={q}
          onPressEnter={(e) => url.set({ q: e.currentTarget.value.trim() })}
          // The clear button sends an empty value.
          onChange={(e) => {
            if (e.target.value === '' && q) url.set({ q: null })
          }}
          onBlur={(e) => {
            if (e.currentTarget.value.trim() !== q) url.set({ q: e.currentTarget.value.trim() })
          }}
          style={{ width: 320 }}
        />
        <Select
          allowClear
          placeholder="Любой статус"
          value={status || undefined}
          options={statusOptions}
          onChange={(v?: string) => url.set({ status: v ?? null })}
          style={{ width: 200 }}
        />
      </div>
      {subscriptions.error && (
        <Alert
          type="error"
          showIcon
          title={errorText(subscriptions.error)}
          style={{ marginBottom: 24 }}
        />
      )}
      <Card variant="borderless" styles={{ body: { padding: 0 } }}>
        <Table<SubscriptionDTO>
          rowKey="id"
          columns={columns}
          dataSource={subscriptions.data?.items}
          loading={subscriptions.isFetching}
          className="table-clickable"
          locale={{ emptyText: q || status ? 'Ничего не нашли' : 'Подписок пока нет' }}
          onRow={(s) => ({ onClick: () => navigate(`/subscriptions/${s.id}`) })}
          pagination={url.pagination(subscriptions.data?.total)}
        />
      </Card>
    </>
  )
}
