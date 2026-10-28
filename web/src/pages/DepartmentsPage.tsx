import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons'
import { Alert, App, Button, Card, Popconfirm, Space, Table, type TableProps } from 'antd'
import { useState } from 'react'
import { Link } from 'react-router'
import { useDeleteDepartment, useDepartments } from '../api/departments'
import type { DepartmentDTO } from '../api/types'
import { DepartmentFormModal } from '../components/DepartmentFormModal'
import { PageHeader } from '../components/PageHeader'
import { SearchInput } from '../components/SearchInput'
import { UserAvatar } from '../components/UserAvatar'
import { errorText } from '../lib/errors'
import { useLastPage, useUrlState } from '../lib/urlState'

// DepartmentsPage (admin): departments with their head and head count.
// There is no department card: a row links to its people and subscriptions.
// Only an empty department (no active people, no live subscriptions) can be
// deleted; otherwise the server's 409 is shown.
export function DepartmentsPage() {
  const { message } = App.useApp()
  const url = useUrlState()
  const q = url.get('q')
  const departments = useDepartments({ q, limit: url.pageSize, offset: url.offset })
  useLastPage(url, departments.data?.total)
  const remove = useDeleteDepartment()
  // undefined — closed, null — a new department.
  const [editing, setEditing] = useState<DepartmentDTO | null>()

  const columns: TableProps<DepartmentDTO>['columns'] = [
    {
      title: 'Отдел',
      key: 'name',
      render: (_, d) => <span className="cell-title">{d.name}</span>,
    },
    {
      title: 'Начальник',
      key: 'head',
      render: (_, d) =>
        d.head_id !== null && d.head_name ? (
          <Link to={`/users/${d.head_id}`} className="cell-service">
            <UserAvatar id={d.head_id} name={d.head_name} size={32} />
            {d.head_name}
          </Link>
        ) : (
          <span className="cell-sub">Нет начальника</span>
        ),
    },
    { title: 'Сотрудников', key: 'employees', render: (_, d) => d.employee_count },
    {
      key: 'links',
      render: (_, d) => (
        <Space size="middle">
          <Link to={`/users?department_id=${d.id}`}>Сотрудники</Link>
          <Link to={`/subscriptions?department_id=${d.id}`}>Подписки</Link>
        </Space>
      ),
    },
    {
      key: 'actions',
      align: 'right',
      render: (_, d) => (
        <Space>
          <Button size="small" icon={<EditOutlined />} onClick={() => setEditing(d)}>
            Изменить
          </Button>
          <Popconfirm
            title={`Удалить отдел «${d.name}»?`}
            description="Удалить можно только отдел без сотрудников и подписок."
            okText="Удалить"
            cancelText="Оставить"
            okButtonProps={{ danger: true }}
            onConfirm={() =>
              remove.mutateAsync(d.id).then(
                () => message.success(`Отдел «${d.name}» удалён`),
                (err: unknown) => message.error(errorText(err)),
              )
            }
          >
            <Button size="small" danger icon={<DeleteOutlined />} aria-label="Удалить" />
          </Popconfirm>
        </Space>
      ),
    },
  ]

  return (
    <>
      <PageHeader
        title="Отделы"
        subtitle="Структура компании и начальники отделов"
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setEditing(null)}>
            Создать
          </Button>
        }
      />
      <div className="filters">
        <SearchInput value={q} onSearch={(v) => url.set({ q: v })} placeholder="Название отдела" />
      </div>
      {departments.error && (
        <Alert
          type="error"
          showIcon
          title={errorText(departments.error)}
          style={{ marginBottom: 24 }}
        />
      )}
      <Card variant="borderless" styles={{ body: { padding: 0 } }}>
        <Table<DepartmentDTO>
          rowKey="id"
          columns={columns}
          dataSource={departments.data?.items}
          loading={departments.isFetching}
          locale={{ emptyText: q ? 'Ничего не нашли' : 'Отделов пока нет' }}
          pagination={url.pagination(departments.data?.total)}
        />
      </Card>
      <DepartmentFormModal
        open={editing !== undefined}
        onClose={() => setEditing(undefined)}
        department={editing ?? undefined}
      />
    </>
  )
}
