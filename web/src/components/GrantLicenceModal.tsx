import { Alert, App, Form, Modal } from 'antd'
import { useCreateAssignment } from '../api/assignments'
import type { SubscriptionDTO, UserDTO } from '../api/types'
import { errorText } from '../lib/errors'
import { SubscriptionSelect } from './SubscriptionSelect'
import { UserSelect } from './UserSelect'

interface Values {
  subscription_id: number
  user_id: number
}

// GrantLicenceModal: POST /assignments. A fixed `subscription` (its card) or
// a fixed `user` (the user's card) is not asked for; with neither (the
// licences page) both are picked. `holders` are the users who already have
// the licence of the fixed subscription; they are not offered (the server
// would answer 409). On an error (no free seats and the like) the modal
// stays open.
export function GrantLicenceModal({
  open,
  onClose,
  subscription,
  user,
  holders,
}: {
  open: boolean
  onClose: () => void
  subscription?: SubscriptionDTO
  user?: UserDTO
  holders?: ReadonlySet<number>
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<Values>()
  const create = useCreateAssignment()

  const close = () => {
    create.reset()
    onClose()
  }

  const submit = (v: Values) =>
    create.mutate(
      {
        subscription_id: subscription?.id ?? v.subscription_id,
        user_id: user?.id ?? v.user_id,
      },
      {
        onSuccess: (a) => {
          message.success(`Лицензия «${a.subscription_name}» выдана: ${a.user_name}`)
          form.resetFields()
          close()
        },
      },
    )

  const title = subscription
    ? `Выдать лицензию «${subscription.name}»`
    : user
      ? `Выдать лицензию: ${user.full_name}`
      : 'Выдать лицензию'

  return (
    <Modal
      title={title}
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
        {!subscription && (
          <Form.Item
            name="subscription_id"
            label="Подписка"
            rules={[{ required: true, message: 'Выберите подписку' }]}
          >
            <SubscriptionSelect />
          </Form.Item>
        )}
        {!user && (
          <Form.Item
            name="user_id"
            label="Сотрудник"
            rules={[{ required: true, message: 'Выберите сотрудника' }]}
          >
            <UserSelect exclude={holders} />
          </Form.Item>
        )}
      </Form>
    </Modal>
  )
}
