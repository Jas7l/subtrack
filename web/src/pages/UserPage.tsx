import { Alert, Card, Descriptions, Empty, Table, type TableProps } from 'antd'
import { Link, useParams } from 'react-router'
import { useAssignments } from '../api/assignments'
import type { AssignmentDTO } from '../api/types'
import { useUser } from '../api/users'
import { useAuth } from '../auth/AuthContext'
import { PageHeader } from '../components/PageHeader'
import { QueryState } from '../components/QueryState'
import { ServiceTile } from '../components/ServiceTile'
import { UserStatusBadge } from '../components/StatusBadge'
import { UserAvatar } from '../components/UserAvatar'
import { errorText } from '../lib/errors'
import { formatDay } from '../lib/format'
import { roleLabels } from '../lib/labels'

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
  { title: 'Выдана', key: 'assigned', render: (_, a) => formatDay(a.assigned_at) },
]

// UserPage (manager): a person of the department and their licences,
// read-only. The admin's card with actions is К4.
export function UserPage() {
  const id = Number(useParams().id)
  const { department } = useAuth()
  const query = useUser(id)
  const assignments = useAssignments(
    { user_id: id, status: 'active', limit: 200 },
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
      />
      <div className="two-columns">
        <Card variant="borderless" title="Сотрудник">
          <Descriptions
            column={1}
            colon={false}
            items={[
              { key: 'email', label: 'Email', children: u.email },
              { key: 'role', label: 'Роль', children: roleLabels[u.role] },
              {
                key: 'department',
                label: 'Отдел',
                // A manager sees only their own department's people.
                children: u.department_id === department?.id ? department.name : '—',
              },
              { key: 'created', label: 'В системе с', children: formatDay(u.created_at) },
            ]}
          />
        </Card>
        <Card
          variant="borderless"
          title={`Лицензии · ${items.length}`}
          styles={{ body: { padding: items.length ? 0 : undefined } }}
        >
          {assignments.error && <Alert type="error" showIcon title={errorText(assignments.error)} />}
          {items.length === 0 && !assignments.isPending ? (
            <Empty description="Лицензий нет" />
          ) : (
            <Table<AssignmentDTO>
              rowKey="id"
              columns={columns}
              dataSource={items}
              loading={assignments.isFetching}
              pagination={false}
              footer={() => (
                <span className="cell-sub">
                  Видны только лицензии подписок вашего отдела.
                </span>
              )}
            />
          )}
        </Card>
      </div>
    </>
  )
}
