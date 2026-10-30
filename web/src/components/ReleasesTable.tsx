import { Table, type TableProps } from 'antd'
import { useNavigate } from 'react-router'
import type { ReleaseRequestDTO } from '../api/types'
import { formatDay } from '../lib/format'
import { CancelReleaseButton } from './CancelReleaseButton'
import { ServiceTile } from './ServiceTile'
import { ReleaseStatusBadge } from './StatusBadge'
import { UserAvatar } from './UserAvatar'

// ReleasesTable lists requests to give a licence back; a row opens the
// request. `mine` is the requester's own list: no «Сотрудник» column, and a
// «Отменить» while the request waits. `withDepartment` adds the
// subscription's department (the admin's lists).
export function ReleasesTable({
  items,
  loading,
  pagination,
  emptyText,
  mine = false,
  withDepartment = false,
}: {
  items: ReleaseRequestDTO[] | undefined
  loading: boolean
  pagination: TableProps<ReleaseRequestDTO>['pagination']
  emptyText: string
  mine?: boolean
  withDepartment?: boolean
}) {
  const navigate = useNavigate()
  const columns: NonNullable<TableProps<ReleaseRequestDTO>['columns']> = [
    {
      title: 'Подписка',
      key: 'subscription',
      render: (_, r) => (
        <div className="cell-service">
          <ServiceTile name={r.subscription_name} size={36} />
          <div className="cell-title">{r.subscription_name}</div>
        </div>
      ),
    },
    ...(mine
      ? []
      : [
          {
            title: 'Сотрудник',
            key: 'requester',
            render: (_: unknown, r: ReleaseRequestDTO) => (
              <div className="cell-service">
                <UserAvatar id={r.requester_id} name={r.requester_name} size={32} />
                {r.requester_name}
              </div>
            ),
          },
        ]),
    ...(withDepartment
      ? [
          {
            title: 'Отдел подписки',
            key: 'department',
            render: (_: unknown, r: ReleaseRequestDTO) =>
              r.department_name ?? <span className="cell-sub">общая</span>,
          },
        ]
      : []),
    {
      title: 'Причина',
      key: 'reason',
      ellipsis: true,
      render: (_, r) => r.reason || <span className="cell-sub">—</span>,
    },
    {
      title: 'Статус',
      key: 'status',
      render: (_, r) => <ReleaseStatusBadge status={r.status} />,
    },
    {
      title: 'Подана',
      key: 'created',
      render: (_, r) => formatDay(r.created_at),
    },
    ...(mine
      ? [
          {
            key: 'actions',
            align: 'right' as const,
            render: (_: unknown, r: ReleaseRequestDTO) =>
              r.status === 'pending' ? (
                // The row opens the request on click; the button must not.
                <span onClick={(e) => e.stopPropagation()}>
                  <CancelReleaseButton id={r.id} size="small" />
                </span>
              ) : null,
          },
        ]
      : []),
  ]

  return (
    <Table<ReleaseRequestDTO>
      rowKey="id"
      columns={columns}
      dataSource={items}
      loading={loading}
      className="table-clickable"
      locale={{ emptyText }}
      onRow={(r) => ({ onClick: () => navigate(`/release-requests/${r.id}`) })}
      pagination={pagination}
    />
  )
}
