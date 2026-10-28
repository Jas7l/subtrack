import { HistoryOutlined, PlusOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Descriptions, Empty, Table, type TableProps } from 'antd'
import { useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { useAssignments } from '../api/assignments'
import { useDepartmentNames } from '../api/departments'
import { useRequests } from '../api/requests'
import type { AssignmentDTO, RequestDTO, UserDTO } from '../api/types'
import { useUser } from '../api/users'
import { useAuth, useMe } from '../auth/AuthContext'
import { GrantLicenceModal } from '../components/GrantLicenceModal'
import { PageHeader } from '../components/PageHeader'
import { QueryState } from '../components/QueryState'
import { RevokeLicenceButton } from '../components/RevokeLicenceButton'
import { ServiceTile } from '../components/ServiceTile'
import { RequestStatusBadge, UserStatusBadge } from '../components/StatusBadge'
import { UserActions } from '../components/UserActions'
import { UserAvatar } from '../components/UserAvatar'
import { errorText } from '../lib/errors'
import { formatDay, formatMoney } from '../lib/format'
import { roleLabels } from '../lib/labels'

const licenceColumns: NonNullable<TableProps<AssignmentDTO>['columns']> = [
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
  { title: 'Выдана', key: 'assigned', render: (_, a) => formatDay(a.assigned_at) },
]

const requestColumns: TableProps<RequestDTO>['columns'] = [
  {
    title: 'Сервис',
    key: 'name',
    render: (_, r) => (
      <div className="cell-service">
        <ServiceTile name={r.name} size={32} />
        <span className="cell-title">{r.name}</span>
      </div>
    ),
  },
  {
    title: 'Стоимость',
    key: 'cost',
    render: (_, r) => formatMoney(r.estimated_cost, r.currency),
  },
  { title: 'Статус', key: 'status', render: (_, r) => <RequestStatusBadge status={r.status} /> },
  { title: 'Подана', key: 'created', render: (_, r) => formatDay(r.created_at) },
]

// A person holds far fewer than 200 licences.
const ALL = { limit: 200 }

// UserPage: a person and their licences. A manager sees a person of their
// department, read-only, and only the licences of the department's
// subscriptions. The admin also edits, blocks and resets the password (the
// same UserActions as in the table), hands out and revokes licences, sees
// the person's requests and opens their actions in the audit.
export function UserPage() {
  const id = Number(useParams().id)
  const me = useMe()
  const admin = me.role === 'admin'
  const navigate = useNavigate()
  const { department } = useAuth()
  const departmentNames = useDepartmentNames(admin)
  const query = useUser(id)
  const assignments = useAssignments(
    { user_id: id, status: 'active', ...ALL },
    query.data !== undefined,
  )

  if (!query.data) {
    return (
      <QueryState
        error={query.error}
        missing={!Number.isInteger(id)}
        notFound="Пользователь не найден"
      />
    )
  }
  const u = query.data
  const items = assignments.data?.items ?? []
  const departmentName =
    u.department_id === null
      ? '—'
      : admin
        ? (departmentNames.get(u.department_id) ?? '…')
        : // A manager sees only their own department's people.
          u.department_id === department?.id
          ? department.name
          : '—'

  return (
    <>
      <PageHeader
        title={
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 16 }}>
            <UserAvatar id={u.id} name={u.full_name} size={52} />
            {u.full_name}
          </span>
        }
        subtitle={
          <span style={{ display: 'inline-flex', alignItems: 'center', gap: 12 }}>
            <UserStatusBadge active={u.is_active} />
            {roleLabels[u.role]}
          </span>
        }
        extra={
          admin && (
            <>
              <Button icon={<HistoryOutlined />} onClick={() => navigate(`/audit?actor_id=${u.id}`)}>
                Действия
              </Button>
              <UserActions user={u} variant="buttons" />
            </>
          )
        }
      />
      <div className="two-columns">
        <Card variant="borderless" title="Сотрудник">
          <Descriptions
            column={1}
            colon={false}
            items={[
              { key: 'email', label: 'Email', children: u.email },
              { key: 'role', label: 'Роль', children: roleLabels[u.role] },
              { key: 'department', label: 'Отдел', children: departmentName },
              { key: 'created', label: 'В системе с', children: formatDay(u.created_at) },
            ]}
          />
        </Card>
        <Licences user={u} items={items} query={assignments} admin={admin} />
      </div>
      {admin && <Requests user={u} />}
    </>
  )
}

function Licences({
  user: u,
  items,
  query,
  admin,
}: {
  user: UserDTO
  items: AssignmentDTO[]
  query: ReturnType<typeof useAssignments>
  admin: boolean
}) {
  const [granting, setGranting] = useState(false)
  const columns: TableProps<AssignmentDTO>['columns'] = admin
    ? [
        ...licenceColumns,
        {
          key: 'actions',
          align: 'right',
          render: (_, a) => <RevokeLicenceButton assignment={a} />,
        },
      ]
    : licenceColumns

  return (
    <Card
      variant="borderless"
      title={`Лицензии · ${items.length}`}
      extra={
        // Only an active user can get a licence.
        admin &&
        u.is_active && (
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setGranting(true)}>
            Выдать лицензию
          </Button>
        )
      }
      styles={{ body: { padding: items.length ? 0 : undefined } }}
    >
      {query.error && <Alert type="error" showIcon title={errorText(query.error)} />}
      {items.length === 0 && !query.isPending ? (
        <Empty description="Лицензий нет" />
      ) : (
        <Table<AssignmentDTO>
          rowKey="id"
          columns={columns}
          dataSource={items}
          loading={query.isFetching}
          pagination={false}
          footer={
            admin
              ? undefined
              : () => (
                  <span className="cell-sub">Видны только лицензии подписок вашего отдела.</span>
                )
          }
        />
      )}
      {admin && (
        <GrantLicenceModal open={granting} onClose={() => setGranting(false)} user={u} />
      )}
    </Card>
  )
}

// The person's own requests, newest first; the full list is on /requests.
function Requests({ user: u }: { user: UserDTO }) {
  const navigate = useNavigate()
  const requests = useRequests({ requester_id: u.id, limit: 10 })
  const items = requests.data?.items ?? []
  const total = requests.data?.total ?? 0

  return (
    <Card
      variant="borderless"
      title={`Заявки · ${total}`}
      extra={total > items.length && <Link to={`/requests?tab=all&requester_id=${u.id}`}>Все заявки</Link>}
      style={{ marginTop: 24 }}
      styles={{ body: { padding: items.length ? 0 : undefined } }}
    >
      {requests.error && <Alert type="error" showIcon title={errorText(requests.error)} />}
      {items.length === 0 && !requests.isPending ? (
        <Empty description="Заявок нет" />
      ) : (
        <Table<RequestDTO>
          rowKey="id"
          columns={requestColumns}
          dataSource={items}
          loading={requests.isFetching}
          className="table-clickable"
          onRow={(r) => ({ onClick: () => navigate(`/requests/${r.id}`) })}
          pagination={false}
        />
      )}
    </Card>
  )
}
