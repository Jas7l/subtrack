import { Alert, Badge, Button, Card, Segmented, Select, Spin, Table, Tabs, type TableProps } from 'antd'
import type { ReactNode } from 'react'
import { useNavigate } from 'react-router'
import { useInboxReleasesCount, useReleases } from '../api/releases'
import { useHeadlessRequests, useRequests } from '../api/requests'
import type { ReleaseStatus, RequestDTO, RequestStatus } from '../api/types'
import { useAuth, useMe } from '../auth/AuthContext'
import { DepartmentSelect } from '../components/DepartmentSelect'
import { PageHeader } from '../components/PageHeader'
import { ReleasesTable } from '../components/ReleasesTable'
import { ServiceTile } from '../components/ServiceTile'
import { RequestStatusBadge } from '../components/StatusBadge'
import { UserAvatar } from '../components/UserAvatar'
import { UserSelect } from '../components/UserSelect'
import { errorText } from '../lib/errors'
import { formatDay, formatMoney, plural } from '../lib/format'
import { releaseStatusLabels, requestStatusLabels } from '../lib/labels'
import { hasReleaseInbox, inboxStatus } from '../lib/permissions'
import { useLastPage, useUrlState, type UrlState } from '../lib/urlState'

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

const releaseStatusOptions = (Object.keys(releaseStatusLabels) as ReleaseStatus[]).map((s) => ({
  value: s,
  label: releaseStatusLabels[s],
}))

// Switching the kind or a tab drops the filters of «Все».
const noFilters = { status: null, department_id: null, requester_id: null }

// RequestsPage is one page for both roles, with two kinds of requests:
// for a subscription (?kind absent) and to give a licence back
// (?kind=release). Each has «Входящие» / «Ждут меня» — what waits for the
// caller's decision — and a tab with every request the server shows (the
// department's for a manager, all for the admin, with filters). The switch
// counts what waits in each kind; the menu badge is their sum.
export function RequestsPage() {
  const me = useMe()
  const admin = me.role === 'admin'
  const { isHead, departmentLoading } = useAuth()
  const url = useUrlState()
  const kind = url.get('kind') === 'release' ? 'release' : 'subscription'

  // The same queries as the tabs below (shared through the cache).
  const waiting = inboxStatus(me, isHead)
  const requestsInbox = useRequests({ status: waiting ?? undefined, limit: 1 })
  const headless = useHeadlessRequests(admin)
  const releasesInbox = useInboxReleasesCount(hasReleaseInbox(me))

  if (departmentLoading) {
    return (
      <div className="page-spin">
        <Spin size="large" />
      </div>
    )
  }

  const requestsCount = (waiting !== null ? (requestsInbox.data?.total ?? 0) : 0) + headless.count
  const releasesCount = releasesInbox.data ?? 0
  const option = (label: string, count: number): ReactNode => (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
      {label} <Badge count={count} size="small" />
    </span>
  )

  const subtitle =
    kind === 'release'
      ? admin
        ? 'Отказы от лицензий решают начальники отделов подписок; вам — те, что решить некому, но закрыть можно любую'
        : me.department_id === null
          ? 'Вы не привязаны к отделу: здесь только ваши заявки'
          : 'Сотрудники отказываются от лицензий подписок вашего отдела: одобрите — лицензия освободится'
      : admin
        ? 'Заявки, согласованные начальниками, ждут вашего финального решения'
        : isHead
          ? 'Заявки сотрудников ждут вашего решения, затем уходят администратору'
          : me.department_id === null
            ? 'Вы не привязаны к отделу: здесь только ваши заявки'
            : 'Решения по заявкам принимает начальник отдела'

  return (
    <>
      <PageHeader title={admin ? 'Все заявки' : 'Заявки отдела'} subtitle={subtitle} />
      <Segmented
        value={kind}
        options={[
          { value: 'subscription', label: option('На подписку', requestsCount) },
          { value: 'release', label: option('На отказ', releasesCount) },
        ]}
        onChange={(v) => url.set({ kind: v === 'release' ? 'release' : null, tab: null, ...noFilters })}
        style={{ marginBottom: 20 }}
      />
      {kind === 'release' ? (
        <ReleaseRequests url={url} />
      ) : (
        <SubscriptionRequests url={url} waiting={waiting} headless={headless} />
      )}
    </>
  )
}

function SubscriptionRequests({
  url,
  waiting,
  headless,
}: {
  url: UrlState
  waiting: ReturnType<typeof inboxStatus>
  headless: ReturnType<typeof useHeadlessRequests>
}) {
  const me = useMe()
  const admin = me.role === 'admin'
  const navigate = useNavigate()

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

  return (
    <>
      {requests.error && (
        <Alert type="error" showIcon title={errorText(requests.error)} style={{ marginBottom: 24 }} />
      )}
      {waiting !== null ? (
        <Tabs
          activeKey={tab}
          onChange={(key) => url.set({ tab: key === 'all' ? 'all' : null, ...noFilters })}
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

// ReleaseRequests: requests to give a licence back. Whoever manages the
// subscription decides: a manager of its department — every one of them
// has an inbox, not only the head — or the admin, whose inbox holds only
// those nobody else can decide (the server picks them); the admin may still
// close any from «Все».
function ReleaseRequests({ url }: { url: UrlState }) {
  const me = useMe()
  const admin = me.role === 'admin'
  const hasInbox = hasReleaseInbox(me)

  const tab = hasInbox && url.get('tab') !== 'all' ? 'inbox' : 'all'
  const filters: Record<string, string> = {}
  if (admin && tab === 'all') {
    filters.status = url.get('status')
    filters.department_id = url.get('department_id')
    filters.requester_id = url.get('requester_id')
  }
  const filtered = Object.values(filters).some(Boolean)
  const releases = useReleases({
    ...filters,
    inbox: tab === 'inbox' ? true : undefined,
    limit: url.pageSize,
    offset: url.offset,
  })
  useLastPage(url, releases.data?.total)
  const inbox = useInboxReleasesCount(hasInbox)

  const table = (
    <>
      {admin && tab === 'all' && (
        <div className="filters">
          <Select
            allowClear
            placeholder="Любой статус"
            value={filters.status || undefined}
            options={releaseStatusOptions}
            onChange={(v?: string) => url.set({ status: v ?? null })}
            style={{ width: 240 }}
          />
          <DepartmentSelect
            value={Number(filters.department_id) || undefined}
            onChange={(id) => url.set({ department_id: id ?? null })}
            placeholder="Любой отдел подписки"
            style={{ width: 240 }}
          />
          <UserSelect
            allowClear
            includeBlocked
            value={Number(filters.requester_id) || undefined}
            onChange={(id) => url.set({ requester_id: id ?? null })}
            placeholder="Любой сотрудник"
            style={{ width: 280 }}
          />
        </div>
      )}
      <Card variant="borderless" styles={{ body: { padding: 0 } }}>
        <ReleasesTable
          items={releases.data?.items}
          loading={releases.isFetching}
          withDepartment={admin}
          emptyText={
            tab === 'inbox'
              ? 'Новых заявок на отказ нет'
              : filtered
                ? 'Ничего не нашли'
                : 'Заявок на отказ пока нет'
          }
          pagination={url.pagination(releases.data?.total)}
        />
      </Card>
    </>
  )

  return (
    <>
      {releases.error && (
        <Alert type="error" showIcon title={errorText(releases.error)} style={{ marginBottom: 24 }} />
      )}
      {hasInbox ? (
        <Tabs
          activeKey={tab}
          onChange={(key) => url.set({ tab: key === 'all' ? 'all' : null, ...noFilters })}
          items={[
            {
              key: 'inbox',
              label: (
                <span style={{ display: 'inline-flex', alignItems: 'center', gap: 8 }}>
                  {admin ? 'Ждут меня' : 'Входящие'} <Badge count={inbox.data ?? 0} size="small" />
                </span>
              ),
              children: (
                <>
                  {admin && (
                    <Alert
                      type="info"
                      showIcon
                      title="Здесь — отказы, которые решить больше некому: подписка общая или в её отделе нет начальника. Остальные решают начальники отделов, но закрыть любую можно во вкладке «Все»"
                      style={{ marginBottom: 16 }}
                    />
                  )}
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
