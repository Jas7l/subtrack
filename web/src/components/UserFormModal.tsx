import { Alert, App, Form, Input, Modal, Select } from 'antd'
import type { CreateUserBody, Role, UpdateUserBody, UserDTO } from '../api/types'
import { useCreateUser, useUpdateUser } from '../api/users'
import { useMe } from '../auth/AuthContext'
import { errorText } from '../lib/errors'
import { roleLabels } from '../lib/labels'
import { DepartmentSelect } from './DepartmentSelect'

interface Values {
  full_name: string
  email: string
  password?: string
  role: Role
  department_id?: number
}

const roleOptions = (Object.keys(roleLabels) as Role[]).map((r) => ({
  value: r,
  label: roleLabels[r],
}))

// UserFormModal creates a user, or edits `user` sending only what changed.
// The admin cannot take their own admin role away (the server answers 409),
// so the role is locked for them.
export function UserFormModal({
  open,
  onClose,
  user,
  onCreated,
}: {
  open: boolean
  onClose: () => void
  user?: UserDTO
  onCreated?: (u: UserDTO) => void
}) {
  const me = useMe()
  const { message } = App.useApp()
  const [form] = Form.useForm<Values>()
  const create = useCreateUser()
  const update = useUpdateUser()
  const mutation = user ? update : create

  const close = () => {
    create.reset()
    update.reset()
    onClose()
  }

  const submit = (v: Values) => {
    const fullName = v.full_name.trim()
    const email = v.email.trim()
    if (!user) {
      const body: CreateUserBody = {
        full_name: fullName,
        email,
        password: v.password ?? '',
        role: v.role,
      }
      if (v.department_id !== undefined) body.department_id = v.department_id
      create.mutate(body, {
        onSuccess: (u) => {
          message.success(`Пользователь ${u.full_name} создан`)
          close()
          onCreated?.(u)
        },
      })
      return
    }
    const body: UpdateUserBody = {}
    if (fullName !== user.full_name) body.full_name = fullName
    if (email !== user.email) body.email = email
    if (v.role !== user.role) body.role = v.role
    if ((v.department_id ?? null) !== user.department_id) body.department_id = v.department_id ?? null
    if (Object.keys(body).length === 0) {
      close()
      return
    }
    update.mutate(
      { id: user.id, body },
      {
        onSuccess: () => {
          message.success('Изменения сохранены')
          close()
        },
      },
    )
  }

  const self = user?.id === me.id

  return (
    <Modal
      title={user ? `Редактировать: ${user.full_name}` : 'Новый пользователь'}
      open={open}
      onCancel={close}
      onOk={() => form.submit()}
      okText={user ? 'Сохранить' : 'Создать'}
      confirmLoading={mutation.isPending}
      width={560}
      destroyOnHidden
    >
      {mutation.error && (
        <Alert type="error" showIcon title={errorText(mutation.error)} style={{ marginBottom: 16 }} />
      )}
      <Form
        form={form}
        layout="vertical"
        requiredMark={false}
        initialValues={
          user
            ? {
                full_name: user.full_name,
                email: user.email,
                role: user.role,
                department_id: user.department_id ?? undefined,
              }
            : { role: 'employee' }
        }
        onFinish={submit}
      >
        <Form.Item
          name="full_name"
          label="ФИО"
          rules={[{ required: true, whitespace: true, message: 'Укажите ФИО' }]}
        >
          <Input placeholder="Петров Пётр Петрович" maxLength={200} autoFocus />
        </Form.Item>
        <Form.Item
          name="email"
          label="Email"
          rules={[
            { required: true, whitespace: true, message: 'Укажите email' },
            { type: 'email', message: 'Неверный email' },
          ]}
        >
          <Input placeholder="petrov@company.ru" maxLength={200} />
        </Form.Item>
        {!user && (
          <Form.Item
            name="password"
            label="Пароль"
            extra="Передайте его пользователю: сменить пароль он сможет в профиле"
            rules={[
              { required: true, message: 'Задайте пароль' },
              { min: 8, message: 'Не короче 8 символов' },
              { max: 72, message: 'Не длиннее 72 символов' },
            ]}
          >
            <Input.Password autoComplete="new-password" />
          </Form.Item>
        )}
        <div className="form-row">
          <Form.Item
            name="role"
            label="Роль"
            extra={self ? 'Свою роль администратора снять нельзя' : undefined}
          >
            <Select options={roleOptions} disabled={self} />
          </Form.Item>
          <Form.Item
            name="department_id"
            label="Отдел"
            extra={
              user?.role === 'manager'
                ? 'Если человек руководит отделом, при переводе отдел останется без начальника'
                : undefined
            }
          >
            <DepartmentSelect />
          </Form.Item>
        </div>
      </Form>
    </Modal>
  )
}
