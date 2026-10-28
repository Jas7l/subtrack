import { SearchOutlined } from '@ant-design/icons'
import { Alert, Card, Input, Table, type TableProps } from 'antd'
import { useNavigate } from 'react-router'
import type { UserDTO } from '../api/types'
import { useUsers } from '../api/users'
import { useMe } from '../auth/AuthContext'
import { PageHeader } from '../components/PageHeader'
import { NoDepartment } from '../components/QueryState'
import { UserStatusBadge } from '../components/StatusBadge'
import { UserAvatar } from '../components/UserAvatar'
import { errorText } from '../lib/errors'
import { roleLabels } from '../lib/labels'
import { useUrlState } from '../lib/urlState'

const columns: TableProps<UserDTO>['columns'] = [
  {
    title: 'Сотрудник',
    key: 'name',
    render: (_, u) => (
      <div className="cell-service">
        <UserAvatar id={u.id} name={u.full_name} size={36} />
        <span className="cell-title">{u.full_name}</span>
      </div>
    ),
  },
  { title: 'Email', key: 'email', render: (_, u) => u.email },
  { title: 'Роль', key: 'role', render: (_, u) => roleLabels[u.role] },
  {
    title: 'Статус',
    key: 'active',
    render: (_, u) => <UserStatusBadge active={u.is_active} />,
  },
]

// UsersPage (manager): the people of the manager's department, read-only.
// The admin's users with CRUD are К4.
export function UsersPage() {
  const me = useMe()
  const navigate = useNavigate()
  const url = useUrlState()
  const q = url.get('q')
  const users = useUsers({ q, limit: url.pageSize, offset: url.offset }, me.department_id !== null)

  if (me.department_id === null) return <NoDepartment title="Сотрудники отдела" />

  return (
    <>
      <PageHeader title="Сотрудники отдела" subtitle="Кто работает в вашем отделе" />
      <div className="filters">
        <Input
          allowClear
          prefix={<SearchOutlined />}
          placeholder="ФИО или email"
          defaultValue={q}
          onPressEnter={(e) => url.set({ q: e.currentTarget.value.trim() })}
          onChange={(e) => {
            if (e.target.value === '' && q) url.set({ q: null })
          }}
          onBlur={(e) => {
            if (e.currentTarget.value.trim() !== q) url.set({ q: e.currentTarget.value.trim() })
          }}
          style={{ width: 320 }}
        />
      </div>
      {users.error && (
        <Alert type="error" showIcon title={errorText(users.error)} style={{ marginBottom: 24 }} />
      )}
      <Card variant="borderless" styles={{ body: { padding: 0 } }}>
        <Table<UserDTO>
          rowKey="id"
          columns={columns}
          dataSource={users.data?.items}
          loading={users.isFetching}
          className="table-clickable"
          locale={{ emptyText: q ? 'Никого не нашли' : 'В отделе пока никого нет' }}
          onRow={(u) => ({ onClick: () => navigate(`/users/${u.id}`) })}
          pagination={url.pagination(users.data?.total)}
        />
      </Card>
    </>
  )
}
