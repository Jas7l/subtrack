import { Alert, App, Form, Modal } from 'antd'
import { useCreateSubscription, useUpdateSubscription } from '../api/subscriptions'
import type { SubscriptionDTO } from '../api/types'
import { errorText } from '../lib/errors'
import {
  newSubscriptionValues,
  toCreateBody,
  toPatch,
  valuesFromSubscription,
  type SubscriptionValues,
} from '../lib/subscriptionForm'
import { ModalForm } from './ModalForm'
import { SubscriptionFields } from './SubscriptionFields'

// SubscriptionFormModal creates a subscription, or edits `subscription`
// sending only what changed. On an error the modal stays open.
export function SubscriptionFormModal({
  open,
  onClose,
  subscription,
  onCreated,
}: {
  open: boolean
  onClose: () => void
  subscription?: SubscriptionDTO
  onCreated?: (s: SubscriptionDTO) => void
}) {
  const { message } = App.useApp()
  const [form] = Form.useForm<SubscriptionValues>()
  const create = useCreateSubscription()
  const update = useUpdateSubscription()
  const mutation = subscription ? update : create

  const close = () => {
    create.reset()
    update.reset()
    onClose()
  }

  const submit = (v: SubscriptionValues) => {
    if (!subscription) {
      create.mutate(toCreateBody(v), {
        onSuccess: (s) => {
          message.success(`Подписка «${s.name}» создана`)
          close()
          onCreated?.(s)
        },
      })
      return
    }
    const body = toPatch(v, subscription)
    if (Object.keys(body).length === 0) {
      close()
      return
    }
    update.mutate(
      { id: subscription.id, body },
      {
        onSuccess: () => {
          message.success('Подписка сохранена')
          close()
        },
      },
    )
  }

  return (
    <Modal
      title={subscription ? `Редактировать «${subscription.name}»` : 'Новая подписка'}
      open={open}
      onCancel={close}
      onOk={() => form.submit()}
      okText={subscription ? 'Сохранить' : 'Создать'}
      confirmLoading={mutation.isPending}
      width={640}
      destroyOnHidden
    >
      {mutation.error && (
        <Alert type="error" showIcon title={errorText(mutation.error)} style={{ marginBottom: 16 }} />
      )}
      <ModalForm
        form={form}
        layout="vertical"
        requiredMark={false}
        initialValues={subscription ? valuesFromSubscription(subscription) : newSubscriptionValues()}
        onFinish={submit}
      >
        <SubscriptionFields
          editing={!!subscription}
          seatsUsed={subscription?.seats_used ?? 0}
          expired={subscription?.status === 'expired'}
        />
      </ModalForm>
    </Modal>
  )
}
