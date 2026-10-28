import { Alert, App, Form, Modal } from 'antd'
import { useCreateAssignment } from '../api/assignments'
import type { SubscriptionDTO } from '../api/types'
import { errorText } from '../lib/errors'
import { UserSelect } from './UserSelect'

// GrantLicenceModal: POST /assignments. `holders` are the users who already
// have the licence; they are not offered (the server would answer 409). On an
// error (no free seats and the like) the modal stays open.
export function GrantLicenceModal({
  open,
  onClose,
  subscription,
  holders,
}: {
  open: boolean
  onClose: () => void
  subscription: SubscriptionDTO
  holders: ReadonlySet<number>
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<{ user_id: number }>()
  const create = useCreateAssignment()

  const close = () => {
    create.reset()
    onClose()
  }

  const submit = ({ user_id }: { user_id: number }) =>
    create.mutate(
      { subscription_id: subscription.id, user_id },
      {
        onSuccess: (a) => {
          message.success(`Лицензия выдана: ${a.user_name}`)
          form.resetFields()
          close()
        },
      },
    )

  return (
    <Modal
      title={`Выдать лицензию «${subscription.name}»`}
      open={open}
      onCancel={close}
      onOk={() => form.submit()}
      okText="Выдать"
      confirmLoading={create.isPending}
      destroyOnHidden
    >
      {create.error && (
        <Alert type="error" showIcon title={errorText(create.error)} style={{ marginBottom: 16 }} />
      )}
      <Form form={form} layout="vertical" requiredMark={false} onFinish={submit}>
        <Form.Item
          name="user_id"
          label="Сотрудник"
          rules={[{ required: true, message: 'Выберите сотрудника' }]}
        >
          <UserSelect exclude={holders} />
        </Form.Item>
      </Form>
    </Modal>
  )
}
