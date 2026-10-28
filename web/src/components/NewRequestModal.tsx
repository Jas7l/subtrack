import { Alert, App, Form, Input, InputNumber, Modal, Select, Space } from 'antd'
import { useCurrencies } from '../api/currencies'
import { useCreateRequest } from '../api/requests'
import type { CreateRequestBody, RequestDTO } from '../api/types'
import { useAuth, useMe } from '../auth/AuthContext'
import { errorText } from '../lib/errors'
import { ModalForm } from './ModalForm'

interface Values {
  name: string
  vendor?: string
  justification: string
  estimated_cost?: string | null
  currency: string
}

// NewRequestModal asks for a subscription: POST /requests. On an error the
// modal stays open and shows the reason.
export function NewRequestModal({
  open,
  onClose,
  onCreated,
}: {
  open: boolean
  onClose: () => void
  onCreated?: (req: RequestDTO) => void
}) {
  const me = useMe()
  const { department } = useAuth()
  const { message } = App.useApp()
  const [form] = Form.useForm<Values>()
  const currencies = useCurrencies()
  const create = useCreateRequest()

  // A head's request and a request from a department without a head go
  // straight to the admin.
  const toAdmin = me.role === 'manager' || !department?.head_id
  const route = toAdmin
    ? 'Заявку сразу рассмотрит администратор.'
    : 'Заявку рассмотрит начальник отдела, затем администратор.'

  const submit = (v: Values) => {
    const body: CreateRequestBody = {
      name: v.name.trim(),
      justification: v.justification.trim(),
      currency: v.currency,
    }
    if (v.vendor?.trim()) body.vendor = v.vendor.trim()
    if (v.estimated_cost) body.estimated_cost = Number(v.estimated_cost).toFixed(2)
    create.mutate(body, {
      onSuccess: (req) => {
        message.success('Заявка отправлена')
        onClose()
        onCreated?.(req)
      },
    })
  }

  const close = () => {
    create.reset()
    onClose()
  }

  return (
    <Modal
      title="Запросить подписку"
      open={open}
      onCancel={close}
      onOk={() => form.submit()}
      okText="Отправить"
      confirmLoading={create.isPending}
      width={560}
      destroyOnHidden
    >
      <p style={{ marginTop: 0, color: '#949BA4' }}>{route}</p>
      {create.error && (
        <Alert type="error" showIcon title={errorText(create.error)} style={{ marginBottom: 16 }} />
      )}
      <ModalForm
        form={form}
        layout="vertical"
        requiredMark={false}
        initialValues={{ currency: 'RUB' }}
        onFinish={submit}
      >
        <Form.Item
          name="name"
          label="Сервис"
          rules={[{ required: true, whitespace: true, message: 'Укажите, какой сервис нужен' }]}
        >
          <Input placeholder="Например, Figma" maxLength={200} autoFocus />
        </Form.Item>
        <Form.Item name="vendor" label="Вендор">
          <Input placeholder="Например, Figma Inc." maxLength={200} />
        </Form.Item>
        <Form.Item
          name="justification"
          label="Зачем нужна"
          rules={[{ required: true, whitespace: true, message: 'Опишите, для чего нужна подписка' }]}
        >
          <Input.TextArea rows={3} placeholder="Для каких задач и как часто" maxLength={2000} />
        </Form.Item>
        <Form.Item label="Примерная стоимость">
          <Space.Compact style={{ width: '100%' }}>
            <Form.Item name="estimated_cost" noStyle>
              <InputNumber<string>
                stringMode
                min="0"
                max="999999999999.99"
                precision={2}
                placeholder="Если знаете"
                style={{ width: '100%' }}
              />
            </Form.Item>
            <Form.Item name="currency" noStyle>
              <Select
                style={{ width: 110 }}
                loading={currencies.isPending}
                options={(currencies.data ?? [{ code: 'RUB', name: 'Рубль' }]).map((c) => ({
                  value: c.code,
                  label: c.code,
                  title: c.name,
                }))}
              />
            </Form.Item>
          </Space.Compact>
        </Form.Item>
      </ModalForm>
    </Modal>
  )
}
