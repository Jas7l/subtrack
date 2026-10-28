import { Alert, App, Form, Input, Modal } from 'antd'
import { useCreateDepartment, useUpdateDepartment } from '../api/departments'
import type { CreateDepartmentBody, DepartmentDTO, UpdateDepartmentBody } from '../api/types'
import { errorText } from '../lib/errors'
import { ModalForm } from './ModalForm'
import { UserSelect } from './UserSelect'

interface Values {
  name: string
  head_id?: number
}

// The head is an active manager; one person heads at most one department.
const HEADS = { role: 'manager' }

// DepartmentFormModal creates a department or renames it and changes its
// head (cleared — no head). The new head moves into the department.
export function DepartmentFormModal({
  open,
  onClose,
  department,
}: {
  open: boolean
  onClose: () => void
  department?: DepartmentDTO
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<Values>()
  const create = useCreateDepartment()
  const update = useUpdateDepartment()
  const mutation = department ? update : create

  const close = () => {
    create.reset()
    update.reset()
    onClose()
  }

  const submit = (v: Values) => {
    const name = v.name.trim()
    if (!department) {
      const body: CreateDepartmentBody = { name }
      if (v.head_id !== undefined) body.head_id = v.head_id
      create.mutate(body, {
        onSuccess: (d) => {
          message.success(`Отдел «${d.name}» создан`)
          close()
        },
      })
      return
    }
    const body: UpdateDepartmentBody = {}
    if (name !== department.name) body.name = name
    if ((v.head_id ?? null) !== department.head_id) body.head_id = v.head_id ?? null
    if (Object.keys(body).length === 0) {
      close()
      return
    }
    update.mutate(
      { id: department.id, body },
      {
        onSuccess: () => {
          message.success('Отдел сохранён')
          close()
        },
      },
    )
  }

  return (
    <Modal
      title={department ? `Отдел «${department.name}»` : 'Новый отдел'}
      open={open}
      onCancel={close}
      onOk={() => form.submit()}
      okText={department ? 'Сохранить' : 'Создать'}
      confirmLoading={mutation.isPending}
      destroyOnHidden
    >
      {mutation.error && (
        <Alert type="error" showIcon title={errorText(mutation.error)} style={{ marginBottom: 16 }} />
      )}
      <ModalForm
        form={form}
        layout="vertical"
        requiredMark={false}
        initialValues={{ name: department?.name, head_id: department?.head_id ?? undefined }}
        onFinish={submit}
      >
        <Form.Item
          name="name"
          label="Название"
          rules={[{ required: true, whitespace: true, message: 'Укажите название' }]}
        >
          <Input placeholder="Например, Разработка" maxLength={200} autoFocus />
        </Form.Item>
        <Form.Item
          name="head_id"
          label="Начальник"
          extra="Только активный пользователь с ролью «Начальник отдела». Он будет переведён в этот отдел."
        >
          <UserSelect query={HEADS} allowClear placeholder="Без начальника" />
        </Form.Item>
      </ModalForm>
    </Modal>
  )
}
