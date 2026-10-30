import { Alert, App, Form, Input, Modal } from 'antd'
import { useCreateRelease } from '../api/releases'
import type { AssignmentDTO, ReleaseRequestDTO } from '../api/types'
import { errorText } from '../lib/errors'
import { ModalForm } from './ModalForm'

interface Values {
  reason?: string
}

// ReleaseRequestModal asks to give a licence back: POST /release-requests.
// Whoever manages the subscription decides — a manager of its department or
// the admin; a company-wide one is the admin's. The reason is optional. On
// an error the modal stays open and shows it.
export function ReleaseRequestModal({
  assignment,
  departmentName,
  open,
  onClose,
  onCreated,
}: {
  assignment: AssignmentDTO
  // The subscription's department, when known.
  departmentName?: string | null
  open: boolean
  onClose: () => void
  onCreated?: (r: ReleaseRequestDTO) => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<Values>()
  const create = useCreateRelease()

  const route =
    assignment.subscription_department_id === null
      ? 'Подписка общая на компанию: решение примет администратор.'
      : departmentName
        ? `Решение примет начальник отдела «${departmentName}» или администратор.`
        : 'Решение примет начальник отдела, которому принадлежит подписка, или администратор.'

  const submit = (v: Values) => {
    const reason = v.reason?.trim()
    create.mutate(
      { assignment_id: assignment.id, ...(reason ? { reason } : {}) },
      {
        onSuccess: (r) => {
          message.success('Заявка на отказ отправлена')
          onClose()
          onCreated?.(r)
        },
      },
    )
  }

  const close = () => {
    create.reset()
    onClose()
  }

  return (
    <Modal
      title={`Отказаться от «${assignment.subscription_name}»`}
      open={open}
      onCancel={close}
      onOk={() => form.submit()}
      okText="Отправить заявку"
      confirmLoading={create.isPending}
      destroyOnHidden
    >
      <p style={{ marginTop: 0, color: '#949BA4' }}>
        {route} Пока заявку не решат, лицензия остаётся за вами.
      </p>
      {create.error && (
        <Alert type="error" showIcon title={errorText(create.error)} style={{ marginBottom: 16 }} />
      )}
      <ModalForm form={form} layout="vertical" requiredMark={false} onFinish={submit}>
        <Form.Item name="reason" label="Причина (необязательно)">
          <Input.TextArea
            rows={3}
            placeholder="Например, перешли на другой сервис"
            maxLength={2000}
            autoFocus
          />
        </Form.Item>
      </ModalForm>
    </Modal>
  )
}
