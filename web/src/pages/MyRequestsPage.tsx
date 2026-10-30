import { PlusOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Segmented, Table, type TableProps } from 'antd'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useReleases } from '../api/releases'
import { useRequests } from '../api/requests'
import type { RequestDTO } from '../api/types'
import { useMe } from '../auth/AuthContext'
import { CancelRequestButton } from '../components/CancelRequestButton'
import { NewRequestModal } from '../components/NewRequestModal'
import { PageHeader } from '../components/PageHeader'
import { ReleasesTable } from '../components/ReleasesTable'
import { ServiceTile } from '../components/ServiceTile'
import { RequestStatusBadge } from '../components/StatusBadge'
import { errorText } from '../lib/errors'
import { formatDay, formatMoney } from '../lib/format'
import { isPending } from '../lib/labels'
import { useUrlState } from '../lib/urlState'

const PAGE_SIZE = 20

// MyRequestsPage: the caller's own requests, of two kinds — for a
// subscription and to give a licence back (?kind=release, filed from «Мои
// подписки»). The kind is in the address, so back from a request returns to
// the same list.
export function MyRequestsPage() {
  const url = useUrlState()
  const kind = url.get('kind') === 'release' ? 'release' : 'subscription'
  const [asking, setAsking] = useState(false)

  return (
    <>
      <PageHeader
        title="Мои заявки"
        subtitle={
          kind === 'release'
            ? 'Отказы от лицензий: от «Моих подписок» до решения'
            : 'Запросы на подписки и их статус'
        }
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setAsking(true)}>
            Новая заявка
          </Button>
        }
      />
      <Segmented
        value={kind}
        options={[
          { value: 'subscription', label: 'На подписку' },
          { value: 'release', label: 'На отказ' },
        ]}
        onChange={(v) => url.set({ kind: v === 'release' ? 'release' : null })}
        style={{ marginBottom: 20 }}
      />
      {kind === 'release' ? <MyReleases /> : <MySubscriptionRequests />}
      <NewRequestModal open={asking} onClose={() => setAsking(false)} />
    </>
  )
}

function MySubscriptionRequests() {
  const me = useMe()
  const navigate = useNavigate()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(PAGE_SIZE)

  const requests = useRequests({
    requester_id: me.id,
    limit: pageSize,
    offset: (page - 1) * pageSize,
  })

  // No sortable columns: the API has no sort parameter, and sorting one page
  // in the browser would mislead.
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
    {
      key: 'actions',
      align: 'right',
      render: (_, r) =>
        isPending(r.status) ? (
          // The row opens the request on click; the button must not.
          <span onClick={(e) => e.stopPropagation()}>
            <CancelRequestButton request={r} size="small" />
          </span>
        ) : null,
    },
  ]

  return (
    <>
      {requests.error && (
        <Alert type="error" showIcon title={errorText(requests.error)} style={{ marginBottom: 24 }} />
      )}
      <Card variant="borderless" styles={{ body: { padding: 0 } }}>
        <Table<RequestDTO>
          rowKey="id"
          columns={columns}
          dataSource={requests.data?.items}
          loading={requests.isFetching}
          className="table-clickable"
          locale={{ emptyText: 'Заявок пока нет' }}
          onRow={(r) => ({ onClick: () => navigate(`/requests/${r.id}`) })}
          pagination={{
            current: page,
            pageSize,
            total: requests.data?.total ?? 0,
            hideOnSinglePage: true,
            showSizeChanger: true,
            onChange: (p, size) => {
              setPage(size === pageSize ? p : 1)
              setPageSize(size)
            },
          }}
        />
      </Card>
    </>
  )
}

function MyReleases() {
  const me = useMe()
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(PAGE_SIZE)
  const releases = useReleases({
    requester_id: me.id,
    limit: pageSize,
    offset: (page - 1) * pageSize,
  })

  return (
    <>
      {releases.error && (
        <Alert type="error" showIcon title={errorText(releases.error)} style={{ marginBottom: 24 }} />
      )}
      <Card variant="borderless" styles={{ body: { padding: 0 } }}>
        <ReleasesTable
          mine
          items={releases.data?.items}
          loading={releases.isFetching}
          emptyText="Заявок на отказ нет. Отказаться от лицензии можно в «Моих подписках»"
          pagination={{
            current: page,
            pageSize,
            total: releases.data?.total ?? 0,
            hideOnSinglePage: true,
            showSizeChanger: true,
            onChange: (p, size) => {
              setPage(size === pageSize ? p : 1)
              setPageSize(size)
            },
          }}
        />
      </Card>
    </>
  )
}
