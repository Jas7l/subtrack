import { FileExcelOutlined, PlusOutlined } from '@ant-design/icons'
import { Alert, App, Button, Card, DatePicker, Select, Table, type TableProps } from 'antd'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useByCategory } from '../api/analytics'
import { exportSubscriptions, useSubscriptions } from '../api/subscriptions'
import type { SubscriptionDTO, SubscriptionStatus, SubscriptionType } from '../api/types'
import { useMe } from '../auth/AuthContext'
import { DepartmentSelect } from '../components/DepartmentSelect'
import { PageHeader } from '../components/PageHeader'
import { SearchInput } from '../components/SearchInput'
import { ServiceTile } from '../components/ServiceTile'
import { SubscriptionFormModal } from '../components/SubscriptionFormModal'
import { SubscriptionStatusBadge } from '../components/StatusBadge'
import { errorText } from '../lib/errors'
import { formatDate, parseDate } from '../lib/format'
import { subscriptionStatusLabels, subscriptionTypeLabels } from '../lib/labels'
import { costText, seatsText } from '../lib/subscriptions'
import { useLastPage, useUrlState } from '../lib/urlState'

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

const typeOptions = (Object.keys(subscriptionTypeLabels) as SubscriptionType[]).map((t) => ({
  value: t,
  label: subscriptionTypeLabels[t],
}))

// free_seats=false is «every seat taken»; enterprise matches neither.
const seatOptions = [
  { value: 'true', label: 'Есть свободные места' },
  { value: 'false', label: 'Все места заняты' },
]

// The admin's filters on top of the manager's search and status.
const ADMIN_FILTERS = ['department_id', 'category', 'type', 'end_from', 'end_to', 'free_seats']

// SubscriptionsPage is one page for both roles (CLIENT_PLAN.md, section 9,
// decision 7). A manager gets the department's subscriptions and the
// company-wide ones they hold a licence of — the server decides — with a
// search and a status. The admin gets every subscription, more filters,
// «Создать» and the Excel export with the same filters.
export function SubscriptionsPage() {
  const me = useMe()
  const admin = me.role === 'admin'
  const { message } = App.useApp()
  const navigate = useNavigate()
  const url = useUrlState()
  const [creating, setCreating] = useState(false)
  const [exporting, setExporting] = useState(false)
  // Category filter options: the categories in use.
  const categories = useByCategory({}, admin)

  const filters: Record<string, string> = { q: url.get('q'), status: url.get('status') }
  if (admin) for (const key of ADMIN_FILTERS) filters[key] = url.get(key)
  const filtered = Object.values(filters).some(Boolean)

  const subscriptions = useSubscriptions({ ...filters, limit: url.pageSize, offset: url.offset })
  useLastPage(url, subscriptions.data?.total)

  const runExport = () => {
    setExporting(true)
    exportSubscriptions(filters)
      .catch((err: unknown) => message.error(errorText(err)))
      .finally(() => setExporting(false))
  }

  const endFrom = url.get('end_from')
  const endTo = url.get('end_to')

  return (
    <>
      <PageHeader
        title={admin ? 'Подписки' : 'Подписки отдела'}
        subtitle={
          admin
            ? 'Все подписки компании'
            : 'Подписки вашего отдела и общие, которыми пользуетесь вы'
        }
        extra={
          admin && (
            <>
              <Button icon={<FileExcelOutlined />} loading={exporting} onClick={runExport}>
                Экспорт в Excel
              </Button>
              <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>
                Создать
              </Button>
            </>
          )
        }
      />
      <div className="filters">
        <SearchInput
          value={filters.q}
          onSearch={(q) => url.set({ q })}
          placeholder="Название или вендор"
        />
        <Select
          allowClear
          placeholder="Любой статус"
          value={filters.status || undefined}
          options={statusOptions}
          onChange={(v?: string) => url.set({ status: v ?? null })}
          style={{ width: 180 }}
        />
        {admin && (
          <>
            <DepartmentSelect
              value={Number(filters.department_id) || undefined}
              onChange={(id) => url.set({ department_id: id ?? null })}
              placeholder="Любой отдел"
              style={{ width: 200 }}
            />
            <Select
              allowClear
              showSearch
              placeholder="Любая категория"
              value={filters.category || undefined}
              loading={categories.isPending}
              options={(categories.data ?? [])
                .filter((c) => c.key !== '')
                .map((c) => ({ value: c.key, label: c.key }))}
              onChange={(v?: string) => url.set({ category: v ?? null })}
              style={{ width: 180 }}
            />
            <Select
              allowClear
              placeholder="Любой тип"
              value={filters.type || undefined}
              options={typeOptions}
              onChange={(v?: string) => url.set({ type: v ?? null })}
              style={{ width: 220 }}
            />
            <Select
              allowClear
              placeholder="Места"
              value={filters.free_seats || undefined}
              options={seatOptions}
              onChange={(v?: string) => url.set({ free_seats: v ?? null })}
              style={{ width: 200 }}
            />
            <DatePicker.RangePicker
              allowEmpty={[true, true]}
              format="DD.MM.YYYY"
              placeholder={['Окончание с', 'по']}
              value={[endFrom ? parseDate(endFrom) : null, endTo ? parseDate(endTo) : null]}
              onChange={(range) =>
                url.set({
                  end_from: range?.[0]?.format('YYYY-MM-DD') ?? null,
                  end_to: range?.[1]?.format('YYYY-MM-DD') ?? null,
                })
              }
            />
          </>
        )}
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
          locale={{ emptyText: filtered ? 'Ничего не нашли' : 'Подписок пока нет' }}
          onRow={(s) => ({ onClick: () => navigate(`/subscriptions/${s.id}`) })}
          pagination={url.pagination(subscriptions.data?.total)}
        />
      </Card>
      {admin && (
        <SubscriptionFormModal
          open={creating}
          onClose={() => setCreating(false)}
          onCreated={(s) => navigate(`/subscriptions/${s.id}`)}
        />
      )}
    </>
  )
}
