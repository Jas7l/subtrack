import { PlusOutlined } from '@ant-design/icons'
import { Alert, Button, Card, Select, Table, type TableProps } from 'antd'
import { useState } from 'react'
import { useNavigate } from 'react-router'
import { useDepartmentNames } from '../api/departments'
import type { Role, UserDTO } from '../api/types'
import { useUsers } from '../api/users'
import { useMe } from '../auth/AuthContext'
import { DepartmentSelect } from '../components/DepartmentSelect'
import { PageHeader } from '../components/PageHeader'
import { NoDepartment } from '../components/QueryState'
import { SearchInput } from '../components/SearchInput'
import { UserStatusBadge } from '../components/StatusBadge'
import { UserActions } from '../components/UserActions'
import { UserAvatar } from '../components/UserAvatar'
import { UserFormModal } from '../components/UserFormModal'
import { errorText } from '../lib/errors'
import { roleLabels } from '../lib/labels'
import { useUrlState } from '../lib/urlState'

const baseColumns: NonNullable<TableProps<UserDTO>['columns']> = [
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

const roleOptions = (Object.keys(roleLabels) as Role[]).map((r) => ({
  value: r,
  label: roleLabels[r],
}))

const activeOptions = [
  { value: 'true', label: 'Активные' },
  { value: 'false', label: 'Заблокированные' },
]

// UsersPage is one page for both roles. A manager sees the people of their
// department, read-only. The admin sees everybody with filters (department,
// role, active, search), creates users and acts on a row through its «⋯»
// menu — the same actions as in the user's card.
export function UsersPage() {
  const me = useMe()
  const admin = me.role === 'admin'
  const navigate = useNavigate()
  const url = useUrlState()
  const [creating, setCreating] = useState(false)
  const departmentNames = useDepartmentNames(admin)

  const filters: Record<string, string> = { q: url.get('q') }
  if (admin) {
    filters.department_id = url.get('department_id')
    filters.role = url.get('role')
    filters.active = url.get('active')
  }
  const filtered = Object.values(filters).some(Boolean)
  const users = useUsers(
    { ...filters, limit: url.pageSize, offset: url.offset },
    admin || me.department_id !== null,
  )

  if (!admin && me.department_id === null) return <NoDepartment title="Сотрудники отдела" />

  const columns: TableProps<UserDTO>['columns'] = admin
    ? [
        ...baseColumns.slice(0, 3),
        {
          title: 'Отдел',
          key: 'department',
          render: (_, u) =>
            u.department_id === null ? (
              <span className="cell-sub">—</span>
            ) : (
              (departmentNames.get(u.department_id) ?? '…')
            ),
        },
        ...baseColumns.slice(3),
        {
          key: 'actions',
          align: 'right',
          width: 56,
          render: (_, u) => <UserActions user={u} variant="menu" />,
        },
      ]
    : baseColumns

  return (
    <>
      <PageHeader
        title={admin ? 'Пользователи' : 'Сотрудники отдела'}
        subtitle={admin ? 'Все учётные записи компании' : 'Кто работает в вашем отделе'}
        extra={
          admin && (
            <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreating(true)}>
              Создать
            </Button>
          )
        }
      />
      <div className="filters">
        <SearchInput value={filters.q} onSearch={(q) => url.set({ q })} placeholder="ФИО или email" />
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
              placeholder="Любая роль"
              value={filters.role || undefined}
              options={roleOptions}
              onChange={(v?: string) => url.set({ role: v ?? null })}
              style={{ width: 200 }}
            />
            <Select
              allowClear
              placeholder="Активные и заблокированные"
              value={filters.active || undefined}
              options={activeOptions}
              onChange={(v?: string) => url.set({ active: v ?? null })}
              style={{ width: 240 }}
            />
          </>
        )}
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
          locale={{
            emptyText: filtered
              ? 'Никого не нашли'
              : admin
                ? 'Пользователей пока нет'
                : 'В отделе пока никого нет',
          }}
          onRow={(u) => ({ onClick: () => navigate(`/users/${u.id}`) })}
          pagination={url.pagination(users.data?.total)}
        />
      </Card>
      {admin && (
        <UserFormModal
          open={creating}
          onClose={() => setCreating(false)}
          onCreated={(u) => navigate(`/users/${u.id}`)}
        />
      )}
    </>
  )
}
